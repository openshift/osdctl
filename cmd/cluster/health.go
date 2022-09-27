package cluster

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	k8spkg "github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// healthOptions defines the struct for running health command
// This command requires the ocm API Token https://cloud.redhat.com/openshift/token be available in the OCM_TOKEN env variable.

type healthOptions struct {
	k8sclusterresourcefactory k8spkg.ClusterResourceFactoryOptions
	output                    string
	verbose                   bool

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

// newCmdHealth implements the health command to describe number of running instances in cluster and the expected number of nodes
func newCmdHealth(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newHealthOptions(streams, flags, globalOpts)
	healthCmd := &cobra.Command{
		Use:               "health",
		Short:             "Describes health of cluster nodes and provides other cluster vitals. Requires being logged into the cluster's corresponding hive cluster.",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	ops.k8sclusterresourcefactory.AttachCobraCliFlags(healthCmd)
	healthCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	healthCmd.MarkFlagRequired("cluster-id")
	return healthCmd
}

func newHealthOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *healthOptions {
	return &healthOptions{
		k8sclusterresourcefactory: k8spkg.ClusterResourceFactoryOptions{
			Flags: flags,
		},
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *healthOptions) complete(cmd *cobra.Command, _ []string) error {

	err := CompleteValidation(&o.k8sclusterresourcefactory, o.IOStreams)
	if err != nil {
		return err
	}

	o.output = o.GlobalOptions.Output

	return nil
}

type ClusterHealthCondensedObject struct {
	ID       string   `yaml:"ID"`
	Name     string   `yaml:"Name"`
	Provider string   `yaml:"Provider"`
	AZs      []string `yaml:"AZs"`
	Expected struct {
		Master int         `yaml:"Master"`
		Infra  int         `yaml:"Infra"`
		Worker interface{} `yaml:"Worker"`
	} `yaml:"Expected nodes"`
	Actual struct {
		Total          int `yaml:"Total"`
		Stopped        int `yaml:"Stopped"`
		RunningMasters int `yaml:"Running Masters"`
		RunningInfra   int `yaml:"Running Infra"`
		RunningWorker  int `yaml:"Running Worker"`
	} `yaml:"Actual nodes"`
}

var healthObject = ClusterHealthCondensedObject{}

func (o *healthOptions) run() error {

	// This call gets the availability zone of the cluster, as well as the expected number of nodes.
	//az, clusterName, compute, infra, master, ascMin, ascMax, err := ocmDescribe(o.k8sclusterresourcefactory.ClusterID)
	cluster, err := ocmDescribe(o.k8sclusterresourcefactory.ClusterID)

	if cluster.Nodes().AvailabilityZones() != nil {

		if cluster.Nodes().AutoscaleCompute().MinReplicas() != 0 {
			min := strconv.Itoa(cluster.Nodes().AutoscaleCompute().MinReplicas())
			max := strconv.Itoa(cluster.Nodes().AutoscaleCompute().MaxReplicas())
			healthObject.Expected.Worker = string(fmt.Sprintf("%v - %v", min, max))
		}
		if cluster.Nodes().Compute() != 0 {
			healthObject.Expected.Worker = int(cluster.Nodes().Compute())
		}

	}
	if err != nil {
		return err
	}

	// This aws client connects to an OpenShift AWS account and we use it here to get credentials to access a customer's account.
	awsClient, err := o.k8sclusterresourcefactory.GetCloudProvider(o.verbose)
	if err != nil {
		return err
	}

	creds := o.k8sclusterresourcefactory.Awscloudfactory.Credentials

	if o.k8sclusterresourcefactory.Awscloudfactory.RoleName != "OrganizationAccountAccessRole" {
		creds, err = awsprovider.GetAssumeRoleCredentials(awsClient,
			&o.k8sclusterresourcefactory.Awscloudfactory.ConsoleDuration, aws.String(o.k8sclusterresourcefactory.Awscloudfactory.SessionName),
			aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s",
				o.k8sclusterresourcefactory.AccountID,
				o.k8sclusterresourcefactory.Awscloudfactory.RoleName)))
		if err != nil {
			klog.Error("Failed to assume ManagedOpenShiftSupport role. Customer either deleted role or denied SREP access.")
			return err
		}
	}

	reg := cluster.Region().ID()

	//This call creates a client that is connected to the customer's account and we will use it to get the information on customer's running instances etc.
	awsJumpClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
		AccessKeyID:     *creds.AccessKeyId,
		SecretAccessKey: *creds.SecretAccessKey,
		SessionToken:    *creds.SessionToken,
		Region:          reg,
	})
	if err != nil {
		return err
	}

	instances, err := awsJumpClient.DescribeInstances(&ec2.DescribeInstancesInput{})
	runningMasters := 0
	runningInfra := 0
	runningWorkers := 0
	totalStopped := 0
	totalCluster := 0

	//Here we count the number of customer's running worker, infra and master instances in the cluster in the given region. To decide if the instance belongs to the cluster we are checking the Name Tag on the instance.
	for idx := range instances.Reservations {
		for _, inst := range instances.Reservations[idx].Instances {
			tags := GetTags(inst)
			for _, t := range tags {
				if *t.Key == "Name" {
					if strings.HasPrefix(*t.Value, cluster.Name()) && strings.Contains(*t.Value, "master") {
						totalCluster += 1
						if *inst.State.Name == "running" {
							runningMasters += 1
						}
						if *inst.State.Name == "stopped" {
							totalStopped += 1
						}

					} else if strings.HasPrefix(*t.Value, cluster.Name()) && strings.Contains(*t.Value, "infra") {
						totalCluster += 1
						if *inst.State.Name == "running" {
							runningInfra += 1
						}
						if *inst.State.Name == "stopped" {
							totalStopped += 1
						}
					} else if strings.HasPrefix(*t.Value, cluster.Name()) && strings.Contains(*t.Value, "worker") {
						totalCluster += 1
						if *inst.State.Name == "running" {
							runningWorkers += 1
						}
						if *inst.State.Name == "stopped" {
							totalStopped += 1
						}

					}
				}
			}

		}

	}

	healthObject.Actual.Stopped = totalStopped
	healthObject.Actual.RunningMasters = runningMasters
	healthObject.Actual.RunningInfra = runningInfra
	healthObject.Actual.RunningWorker = runningWorkers
	healthObject.Actual.Total = totalCluster

	if err != nil {
		log.Fatalf("Error getting instances %v", err)
		return err
	}

	healthOutput, err := yaml.Marshal(&healthObject)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Fprintf(o.IOStreams.Out, "\n \n")
	fmt.Printf(string(healthOutput))

	return nil
}

//This command implements the ocm describe clsuter call via osm-sdk.
//This call requires the ocm API Token https://cloud.redhat.com/openshift/token be available in the OCM_TOKEN env variable.
//Example: export OCM_TOKEN=$(jq -r .refresh_token ~/.ocm.json)
func ocmDescribe(clusterID string) (*v1.Cluster, error) {
	connection := utils.CreateConnection()
	defer connection.Close()

	// Get the client for the resource that manages the collection of clusters:
	collection := connection.ClustersMgmt().V1().Clusters()
	resource := collection.Cluster(clusterID)

	// Send the request to retrieve the cluster:
	response, err := resource.Get().Send()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster: %v\n", err)
		os.Exit(1)
	}

	// Print the result:
	cluster := response.Body()
	cloudProvider := cluster.CloudProvider().ID()
	cloudProviderMessage := strings.ToUpper(cloudProvider)

	healthObject.ID = cluster.ID()
	healthObject.Name = cluster.Name()
	healthObject.Provider = cloudProviderMessage
	healthObject.AZs = cluster.Nodes().AvailabilityZones()
	healthObject.Expected.Infra = cluster.Nodes().Infra()
	healthObject.Expected.Master = cluster.Nodes().Master()

	if cloudProvider != "aws" {
		return cluster, fmt.Errorf("This command is only supported for AWS clusters. The command is not supported for %s clusters.", cloudProviderMessage)
	}
	return cluster, err
}

func GetTags(instance *ec2.Instance) []*ec2.Tag {

	tags := instance.Tags
	//fmt.Printf("\n%v ", tags)
	return tags
}
