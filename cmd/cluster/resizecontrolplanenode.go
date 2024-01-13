package cluster

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/printer"
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
	cluster        *cmv1.Cluster

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

	connection, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	cluster, err := utils.GetCluster(connection, o.clusterID)
	if err != nil {
		return err
	}

	o.cluster = cluster

	// Ensure we store the internal OCM cluster id
	o.clusterID = cluster.ID()

	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return errors.New("this command is only available for AWS clusters")
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

type optionsDialogResponse int64

const (
	Undefined optionsDialogResponse = 0
	Retry                           = 1
	Skip                            = 2
	Force                           = 3
	Cancel                          = 4
)

func retryCancelDialog(procedure string) (optionsDialogResponse, error) {
	fmt.Printf("Do you want to retry %s or cancel this command? (retry/cancel):\n", procedure)

	reader := bufio.NewReader(os.Stdin)

	responseBytes, _, err := reader.ReadLine()
	if err != nil {
		return Undefined, fmt.Errorf("reader.ReadLine() resulted in an error: %s", err)
	}

	response := strings.ToUpper(string(responseBytes))

	switch response {
	case "RETRY":
		return Retry, nil
	case "CANCEL":
		return Cancel, nil
	default:
		fmt.Println("Invalid response, expected 'retry' or 'cancel' (case-insensitive).")
		return retryCancelDialog(procedure)
	}

}

func withRetryCancelOption(fn func() error, procedure string) (err error) {
	err = fn()
	if err == nil {
		return nil
	}
	dialogResponse, err := retryCancelDialog(procedure)
	if err != nil {
		return err
	}

	switch dialogResponse {
	case Retry:
		return withRetryCancelOption(fn, procedure)
	case Cancel:
		return errors.New("exiting")
	default:
		// This would be a programming error
		return errors.New("unhandled enumerator in withRetryCancelOption")
	}
}

func retrySkipCancelDialog(procedure string) (optionsDialogResponse, error) {
	fmt.Printf("Do you want to retry %[1]s, skip %[1]s or cancel this command? (retry/skip/cancel):\n", procedure)

	reader := bufio.NewReader(os.Stdin)

	responseBytes, _, err := reader.ReadLine()
	if err != nil {
		return Undefined, fmt.Errorf("reader.ReadLine() resulted in an error: %s", err)
	}

	response := strings.ToUpper(string(responseBytes))

	switch response {
	case "RETRY":
		return Retry, nil
	case "SKIP":
		return Skip, nil
	case "CANCEL":
		return Cancel, nil
	default:
		fmt.Println("Invalid response, expected 'retry', 'skip' or 'cancel' (case-insensitive).")
		return retrySkipCancelDialog(procedure)
	}

}

func withRetrySkipCancelOption(fn func() error, procedure string) (err error) {
	err = fn()
	if err == nil {
		return nil
	}
	dialogResponse, err := retrySkipCancelDialog(procedure)
	if err != nil {
		return err
	}

	switch dialogResponse {
	case Retry:
		return withRetrySkipCancelOption(fn, procedure)
	case Skip:
		fmt.Printf("Skipping %s...\n", procedure)
	case Cancel:
		return errors.New("exiting")
	default:
		// This would be a programming error
		return errors.New("unhandled enumerator in withRetrySkipCancelOption")
	}
	return nil
}

func retrySkipForceCancelDialog(procedure string) (optionsDialogResponse, error) {
	fmt.Printf("Do you want to retry %s, skip %s, force %s or cancel this command? (retry/skip/force/cancel):\n", procedure, procedure, procedure)

	reader := bufio.NewReader(os.Stdin)

	responseBytes, _, err := reader.ReadLine()
	if err != nil {
		return Undefined, fmt.Errorf("reader.ReadLine() resulted in an error: %s", err)
	}

	response := strings.ToUpper(string(responseBytes))

	switch response {
	case "RETRY":
		return Retry, nil
	case "SKIP":
		return Skip, nil
	case "FORCE":
		return Force, nil
	case "CANCEL":
		return Cancel, nil
	default:
		fmt.Println("Invalid response, expected 'retry', 'skip', 'force' or 'cancel' (case-insensitive).")
		return retrySkipForceCancelDialog(procedure)
	}
}

func forceDrainNode(nodeID string) error {
	printer.PrintlnGreen("Force draining node... This might take a minute or two...")
	cmd := fmt.Sprintf("oc adm drain %s --ignore-daemonsets --delete-emptydir-data --force --as backplane-cluster-admin", nodeID)
	output, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to force drain:\n%s", strings.TrimSpace(string(output)))
	}
	return nil
}

func drainNode(nodeID string) error {
	printer.PrintlnGreen("Draining node", nodeID)

	// TODO: replace subprocess call with API call
	cmd := fmt.Sprintf("oc adm drain %s --ignore-daemonsets --delete-emptydir-data --as backplane-cluster-admin", nodeID)
	output, err := exec.Command("bash", "-c", cmd).CombinedOutput()

	if err != nil {
		fmt.Println("Failed to drain node:")
		fmt.Println(strings.TrimSpace(string(output)))

		dialogResponse, err := retrySkipForceCancelDialog("draining node")
		if err != nil {
			return err
		}

		switch dialogResponse {
		case Retry:
			return drainNode(nodeID)
		case Skip:
			fmt.Println("Skipping node drain")
		case Force:
			err = withRetrySkipCancelOption(func() error { return forceDrainNode(nodeID) }, "force draining")
			if err != nil {
				return err
			}
		case Cancel:
			return errors.New("exiting")
		}
	}
	return nil
}

func stopNode(ctx context.Context, awsClient resizeControlPlaneNodeAWSClient, nodeID string) error {
	printer.PrintfGreen("Stopping ec2 instance %s. This might take a minute or two...\n", nodeID)

	_, err := awsClient.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{nodeID},
	})
	if err != nil {
		return fmt.Errorf("unable to request stop of ec2 instance: %v", err)
	}

	waiter := ec2.NewInstanceStoppedWaiter(awsClient)
	describeInstancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: []string{nodeID},
	}

	err = waiter.Wait(ctx, describeInstancesInput, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("unable to stop or timed out while stopping ec2 instance: %s", err)
	}
	return nil
}

func modifyInstanceAttribute(ctx context.Context, awsClient resizeControlPlaneNodeAWSClient, nodeID string, newMachineType string) error {
	printer.PrintlnGreen("Modifying machine type of instance:", nodeID, "to", newMachineType)

	modifyInstanceAttributeInput := &ec2.ModifyInstanceAttributeInput{InstanceId: &nodeID, InstanceType: &types.AttributeValue{Value: &newMachineType}}

	_, err := awsClient.ModifyInstanceAttribute(ctx, modifyInstanceAttributeInput)
	if err != nil {
		return fmt.Errorf("unable to modify ec2 instance: %v", err)
	}
	return nil
}

func startNode(ctx context.Context, awsClient resizeControlPlaneNodeAWSClient, nodeID string) error {
	printer.PrintfGreen("Starting instance %s. This might take a minute or two...\n", nodeID)

	_, err := awsClient.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{nodeID},
	})
	if err != nil {
		return fmt.Errorf("unable to request start of ec2 instance: %v", err)
	}

	waiter := ec2.NewInstanceRunningWaiter(awsClient)
	describeInstancesInput := &ec2.DescribeInstancesInput{
		InstanceIds: []string{nodeID},
	}

	err = waiter.Wait(ctx, describeInstancesInput, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("unable to run or timed out while running ec2 instance: %s", err)
	}
	return nil
}

func uncordonNode(nodeID string) error {
	printer.PrintlnGreen("Uncordoning node", nodeID)
	// TODO: replace subprocess call with API call
	cmd := fmt.Sprintf("oc adm uncordon %s", nodeID)
	output, err := exec.Command("bash", "-c", cmd).CombinedOutput()

	if err != nil {
		fmt.Printf("Failed to uncordon node: %s", strings.TrimSpace(string(output)))
		return err
	}
	return nil
}

// Start and stop calls require the internal AWS instance ID
// Machinetype patch requires the tag "Name"
func getNodeAwsInstanceData(ctx context.Context, node string, awsClient resizeControlPlaneNodeAWSClient) (string, string, error) {
	params := &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("private-dns-name"),
				Values: []string{node},
			},
		},
	}
	ret, err := awsClient.DescribeInstances(ctx, params)
	if err != nil {
		return "", "", err
	}

	awsInstanceID := *(ret.Reservations[0].Instances[0].InstanceId)

	var machineName string
	tags := ret.Reservations[0].Instances[0].Tags
	for _, t := range tags {
		if *t.Key == "Name" {
			machineName = *t.Value
		}
	}

	if machineName == "" {
		return "", "", errors.New("could not retrieve node machine name")
	}

	fmt.Println("Node", node, "found as AWS internal InstanceId", awsInstanceID, "with machine name", machineName)

	return machineName, awsInstanceID, nil
}

func patchMachineType(machine string, machineType string) error {
	printer.PrintlnGreen("Patching machine type of machine", machine, "to", machineType)
	cmd := `oc -n openshift-machine-api patch machine ` + machine + ` --patch "{\"spec\":{\"providerSpec\":{\"value\":{\"instanceType\":\"` + machineType + `\"}}}}" --type merge --as backplane-cluster-admin`
	output, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Could not patch machine type:\n%s", strings.TrimSpace(string(output)))
	}
	return nil
}

type resizeControlPlaneNodeAWSClient interface {
	ec2.DescribeInstancesAPIClient
	StartInstances(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	StopInstances(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
	ModifyInstanceAttribute(ctx context.Context, params *ec2.ModifyInstanceAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyInstanceAttributeOutput, error)
}

func (o *resizeControlPlaneNodeOptions) run() error {
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	cfg, err := osdCloud.CreateAWSV2Config(ocmClient, o.cluster)
	if err != nil {
		return err
	}
	awsClient := ec2.NewFromConfig(cfg)

	machineName, nodeAwsID, err := getNodeAwsInstanceData(context.TODO(), o.node, awsClient)
	if err != nil {
		return err
	}
	fmt.Println() // Add an empty line for better output formatting

	// drain node with oc adm drain <node> --ignore-daemonsets --delete-emptydir-data
	// drainNode has its own retry dialog.
	err = drainNode(o.node)
	if err != nil {
		return err
	}
	fmt.Println() // Add an empty line for better output formatting

	// Stop the node instance
	err = withRetryCancelOption(func() error { return stopNode(context.TODO(), awsClient, nodeAwsID) }, "stopping node")
	if err != nil {
		return err
	}
	fmt.Println() // Add an empty line for better output formatting

	// Once stopped, change the instance type
	err = withRetryCancelOption(func() error { return modifyInstanceAttribute(context.TODO(), awsClient, nodeAwsID, o.newMachineType) }, "modify instance attribute")
	if err != nil {
		return err
	}
	fmt.Println() // Add an empty line for better output formatting

	// Start the node instance
	err = withRetryCancelOption(func() error { return startNode(context.TODO(), awsClient, nodeAwsID) }, "starting node")
	if err != nil {
		return err
	}
	fmt.Println() // Add an empty line for better output formatting

	// uncordon node with oc adm uncordon <node>
	err = withRetrySkipCancelOption(func() error { return uncordonNode(o.node) }, "uncordoning node")
	if err != nil {
		return err
	}
	fmt.Println() // Add an empty line for better output formatting

	fmt.Println("To continue, please confirm that the node is up and running and that the cluster is in the desired state to proceed.")
	if !utils.ConfirmPrompt() {
		return nil
	}
	fmt.Println() // Add an empty line for better output formatting

	fmt.Println("To finish the node resize, it is suggested to update the machine spec. This requires ***elevated privileges***. Do you want to proceed?")
	if !utils.ConfirmPrompt() {
		fmt.Println("Node resized, machine type not patched. Exiting...")
		return nil
	}
	fmt.Println() // Add an empty line for better output formatting

	// Patch node machine to update .spec
	err = withRetryCancelOption(func() error { return patchMachineType(machineName, o.newMachineType) }, "patch machine type")
	if err != nil {
		fmt.Println("Control plane node resized but could not patch machine .spec.")
		return err
	}

	fmt.Println("Control plane node successfully resized.")

	return nil
}
