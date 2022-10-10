package cluster

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/osdCloud"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// resizeControlPlaneNodeOptions defines the struct for running resizeControlPlaneNode command
type resizeControlPlaneNodeOptions struct {
	clusterID      string
	node           string
	newMachineType string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

// This command requires to previously be logged in via `ocm login`
func newCmdResizeControlPlaneNode(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newResizeControlPlaneNodeOptions(streams, flags, globalOpts)
	resizeControlPlaneNodeCmd := &cobra.Command{
		Use:               "resize-control-plane-node",
		Short:             "Resize a control plane node. Requires previous login to the api server via `ocm login` and being tunneled to the backplane.",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	resizeControlPlaneNodeCmd.Flags().StringVar(&ops.node, "node", "", "The control plane node to resize (e.g. ip-127.0.0.1.eu-west-2.compute.internal)")
	resizeControlPlaneNodeCmd.Flags().StringVar(&ops.newMachineType, "machine-type", "", "The target AWS machine type to resize to (e.g. m5.2xlarge)")
	resizeControlPlaneNodeCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "c", "", "The internal ID of the cluster to perform actions on")
	resizeControlPlaneNodeCmd.MarkFlagRequired("cluster-id")
	resizeControlPlaneNodeCmd.MarkFlagRequired("node")
	resizeControlPlaneNodeCmd.MarkFlagRequired("machine-type")

	return resizeControlPlaneNodeCmd
}

func newResizeControlPlaneNodeOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *resizeControlPlaneNodeOptions {
	return &resizeControlPlaneNodeOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *resizeControlPlaneNodeOptions) complete(cmd *cobra.Command, _ []string) error {
	err := utils.IsValidClusterKey(o.clusterID)
	if err != nil {
		return err
	}

	connection := utils.CreateConnection()
	defer connection.Close()

	cluster, err := utils.GetCluster(connection, o.clusterID)
	if err != nil {
		return err
	}

	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("This command is only available for AWS clusters")
	}
	/*
		Ideally we would want additional validation here for:
		- the machine type exists
		- the node exists on the cluster

		As this command is idempotent, it will just fail on a later stage if e.g. the
		machine type doesn't exist and can be re-run.
	*/

	return nil
}

type drainDialogResponse int64

const (
	Undefined drainDialogResponse = 0
	Skip                          = 1
	Force                         = 2
	Cancel                        = 3
)

func drainRecoveryDialog() (drainDialogResponse, error) {
	fmt.Println("Do you want to skip drain, force drain or cancel this command? (skip/force/cancel):")

	reader := bufio.NewReader(os.Stdin)

	responseBytes, _, err := reader.ReadLine()
	if err != nil {
		return Undefined, fmt.Errorf("reader.ReadLine() resulted in an error: %s", err)
	}

	response := strings.ToUpper(string(responseBytes))

	switch response {
	case "SKIP":
		return Skip, nil
	case "FORCE":
		return Force, nil
	case "CANCEL":
		return Cancel, nil
	default:
		fmt.Println("Invalid response, expected 'skip', 'force' or 'cancel' (case-insensitive).")
		return drainRecoveryDialog()
	}
}

func drainNode(nodeID string) error {
	fmt.Println("Draining node", nodeID)

	// TODO: replace subprocess call with API call
	cmd := fmt.Sprintf("oc adm drain %s --ignore-daemonsets --delete-emptydir-data", nodeID)
	output, err := exec.Command("bash", "-c", cmd).Output()

	if err != nil {
		fmt.Println("Failed to drain node:", strings.TrimSpace(string(output)))

		dialogResponse, err := drainRecoveryDialog()
		if err != nil {
			return err
		}

		switch dialogResponse {
		case Skip:
			fmt.Println("Skipping node drain")
		case Force:
			// TODO: replace subprocess call with API call
			fmt.Println("Force draining node... This might take a minute or two...")
			cmd := fmt.Sprintf("oc adm drain %s --ignore-daemonsets --delete-emptydir-data --force", nodeID)
			err = exec.Command("bash", "-c", cmd).Run()
			if err != nil {
				return err
			}
		case Cancel:
			return fmt.Errorf("Exiting...")
		}
	}
	return nil
}

func stopNode(awsClient *awsprovider.Client, nodeID string) error {
	fmt.Printf("Stopping ec2 instance %s. This might take a minute or two...\n", nodeID)

	stopInstancesInput := &ec2.StopInstancesInput{InstanceIds: []*string{aws.String(nodeID)}}

	stopInstanceOutput, err := (*awsClient).StopInstances(stopInstancesInput)
	if err != nil {
		return fmt.Errorf("Unable to request stop of ec2 instance, output: %s. Error %s", stopInstanceOutput, err)
	}

	describeInstancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(nodeID)},
	}

	err = (*awsClient).WaitUntilInstanceStopped(describeInstancesInput)
	if err != nil {
		return fmt.Errorf("Unable to stop of ec2 instance: %s", err)
	}
	return nil
}

func modifyInstanceAttribute(awsClient *awsprovider.Client, nodeID string, newMachineType string) error {
	fmt.Println("Modifying machine type of instance:", nodeID, "to", newMachineType)

	modifyInstanceAttributeInput := &ec2.ModifyInstanceAttributeInput{InstanceId: &nodeID, InstanceType: &ec2.AttributeValue{Value: &newMachineType}}

	modifyInstanceOutput, err := (*awsClient).ModifyInstanceAttribute(modifyInstanceAttributeInput)
	if err != nil {
		return fmt.Errorf("Unable to modify ec2 instance, output: %s. Error: %s", modifyInstanceOutput, err)
	}
	return nil
}

func startNode(awsClient *awsprovider.Client, nodeID string) error {
	fmt.Printf("Starting instance %s. This might take a minute or two...\n", nodeID)

	startInstancesInput := &ec2.StartInstancesInput{InstanceIds: []*string{aws.String(nodeID)}}
	startInstanceOutput, err := (*awsClient).StartInstances(startInstancesInput)
	if err != nil {
		return fmt.Errorf("Unable to request start of ec2 instance, output: %s. Error %s", startInstanceOutput, err)
	}

	describeInstancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(nodeID)},
	}

	err = (*awsClient).WaitUntilInstanceRunning(describeInstancesInput)
	if err != nil {
		return fmt.Errorf("Unable to get ec2 instance up and running: %s", err)
	}
	return nil
}

func uncordonNode(nodeID string) error {
	fmt.Println("Uncordoning node", nodeID)

	// TODO: replace subprocess call with API call
	cmd := fmt.Sprintf("oc adm uncordon %s", nodeID)
	_, err := exec.Command("bash", "-c", cmd).Output()

	return err
}

// Start and stop calls require the internal AWS instance ID
// Machinetype patch requires the tag "Name"
func getNodeAwsInstanceData(node string, awsClient *awsprovider.Client) (string, string, error) {
	params := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("private-dns-name"),
				Values: []*string{aws.String(node)},
			},
		},
	}
	ret, err := (*awsClient).DescribeInstances(params)
	if err != nil {
		return "", "", err
	}

	awsInstanceID := *(ret.Reservations[0].Instances[0].InstanceId)

	var machineName string = ""
	tags := ret.Reservations[0].Instances[0].Tags
	for _, t := range tags {
		if *t.Key == "Name" {
			machineName = *t.Value
		}
	}

	if machineName == "" {
		return "", "", fmt.Errorf("Could not retrieve node machine name.")
	}

	fmt.Println("Node", node, "found as AWS internal InstanceId", awsInstanceID, "with machine name", machineName)

	return machineName, awsInstanceID, nil
}

func patchMachineType(machine string, machineType string) error {
	fmt.Println("Patching machine type of machine", machine, "to", machineType)
	cmd := `oc -n openshift-machine-api patch machine ` + machine + ` --patch "{\"spec\":{\"providerSpec\":{\"value\":{\"instanceType\":\"` + machineType + `\"}}}}" --type merge --as backplane-cluster-admin`
	err := exec.Command("bash", "-c", cmd).Run()
	if err != nil {
		return fmt.Errorf("Could not patch machine type: %s", err)
	}
	return nil
}

func (o *resizeControlPlaneNodeOptions) run() error {
	awsClient, err := osdCloud.CreateAWSClient(o.clusterID)
	if err != nil {
		return err
	}

	machineName, nodeAwsID, err := getNodeAwsInstanceData(o.node, &awsClient)
	if err != nil {
		return err
	}

	// drain node with oc adm drain <node> --ignore-daemonsets --delete-emptydir-data
	err = drainNode(o.node)
	if err != nil {
		return err
	}

	// Stop the node instance
	err = stopNode(&awsClient, nodeAwsID)
	if err != nil {
		return err
	}

	// Once stopped, change the instance type
	err = modifyInstanceAttribute(&awsClient, nodeAwsID, o.newMachineType)
	if err != nil {
		return err
	}

	// Start the node instance
	err = startNode(&awsClient, nodeAwsID)
	if err != nil {
		return err
	}

	// uncordon node with oc adm uncordon <node>
	err = uncordonNode(o.node)
	if err != nil {
		return err
	}

	fmt.Println("To continue, please confirm that the node is up and running and that the cluster is in the desired state to proceed.")
	err = utils.ConfirmSend()
	if err != nil {
		return err
	}

	fmt.Println("To finish the node resize, it is suggested to update the machine spec. This requires ***elevated privileges***. Do you want to proceed?")
	err = utils.ConfirmSend()
	if err != nil {
		fmt.Println("Node resized, machine type not patched. Exiting...")
		return err
	}

	// Patch node machine to update .spec
	err = patchMachineType(machineName, o.newMachineType)
	if err != nil {
		return err
	}

	fmt.Println("Control plane node successfully resized.")

	return nil
}
