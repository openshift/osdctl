package cluster

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	capav1beta2 "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type cleanup struct {
	awsClient   cleanupAWSClient
	client      client.Client
	cluster     *cmv1.Cluster
	mgmtCluster *cmv1.Cluster

	// ClusterId is the internal or external OCM cluster ID.
	// This is optional, but typically is used to automatically detect the correct settings.
	ClusterId string
	// Yes default confirmation prompts to yes
	Yes bool
}

type cleanupAWSClient interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

func newCmdCleanupLeakedEC2() *cobra.Command {
	c := &cleanup{}

	cleanupCmd := &cobra.Command{
		Use:   "cleanup-leaked-ec2",
		Short: "Remediate impact of https://issues.redhat.com/browse/OCPBUGS-23174",
		Example: `
  # Run against a given ROSA HCP cluster
  osdctl cluster cleanup-leaked-ec2 --cluster-id ${CLUSTER_ID}

  # Assess all "error" state ROSA HCP clusters for impact
  for cluster in $(ocm list cluster -p search="hypershift.enabled='true' and state='error'" --columns='id' --no-headers);
  do
    osdctl cluster cleanup-leaked-ec2 --cluster-id ${cluster}
  done
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.Run(context.Background())
		},
	}

	cleanupCmd.Flags().StringVarP(&c.ClusterId, "cluster-id", "C", "", "OCM internal/external cluster id to check for impact of OCPBUGS-23174.")
	cleanupCmd.Flags().BoolVarP(&c.Yes, "yes", "y", false, "(optional) Skip confirmation prompt when terminating instances")

	cleanupCmd.MarkFlagRequired("cluster-id")

	return cleanupCmd
}

func (c *cleanup) New(ctx context.Context) error {
	log.Printf("searching OCM for cluster: %s", c.ClusterId)
	conn, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer conn.Close()

	cluster, err := utils.GetClusterAnyStatus(conn, c.ClusterId)
	if err != nil {
		return fmt.Errorf("failed to get OCM cluster info for %s: %v", c.ClusterId, err)
	}
	c.cluster = cluster
	log.Printf("cluster %s found from OCM: %s", c.ClusterId, cluster.ID())

	if !cluster.Hypershift().Enabled() {
		return fmt.Errorf("this command is only meant for ROSA HCP clusters")
	}

	log.Printf("getting AWS credentials from backplane-api")
	cfg, err := osdCloud.CreateAWSV2Config(conn, c.cluster)
	if err != nil {
		return fmt.Errorf("failed to get credentials automatically from backplane-api: %v", err)
	}
	log.Println(ctx, "retrieved AWS credentials from backplane-api")
	c.awsClient = ec2.NewFromConfig(cfg)

	mgmtCluster, err := utils.GetManagementCluster(c.cluster.ID())
	if err != nil {
		return err
	}
	c.mgmtCluster = mgmtCluster

	scheme := runtime.NewScheme()
	if err := capav1beta2.AddToScheme(scheme); err != nil {
		return err
	}
	client, err := k8s.New(c.mgmtCluster.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	c.client = client

	return nil
}

func (c *cleanup) Run(ctx context.Context) error {
	if err := c.New(ctx); err != nil {
		return err
	}

	if err := c.RemediateOCPBUGS23174(ctx); err != nil {
		return err
	}

	return nil
}

func (c *cleanup) RemediateOCPBUGS23174(ctx context.Context) error {
	awsmachines := &capav1beta2.AWSMachineList{}
	if err := c.client.List(ctx, awsmachines, client.MatchingLabels{
		"cluster.x-k8s.io/cluster-name": c.cluster.ID(),
	}); err != nil {
		return err
	}

	expectedInstances := map[string]bool{}
	for _, awsmachine := range awsmachines.Items {
		if awsmachine.Spec.InstanceID != nil {
			expectedInstances[*awsmachine.Spec.InstanceID] = true
		}
	}
	log.Printf("expected instances: %v", expectedInstances)

	resp, err := c.awsClient.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				// We don't want to terminate pending because they would just be started and might not have been picked up by the MC yet.
				// Importantly we also don't want to count terminated instances as leaked, as it causes confusion.
				Name:   aws.String("instance-state-name"),
				Values: []string{"running", "stopped"},
			},
			{
				Name:   aws.String("tag:red-hat-managed"),
				Values: []string{"true"},
			},
			{
				Name:   aws.String(fmt.Sprintf("tag:sigs.k8s.io/cluster-api-provider-aws/cluster/%s", c.cluster.ID())),
				Values: []string{"owned"},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to find EC2 instances associated with %s: %v", c.cluster.ID(), err)
	}

	leakedInstances := []string{}
	for _, reservation := range resp.Reservations {
		for _, instance := range reservation.Instances {
			if _, ok := expectedInstances[*instance.InstanceId]; !ok {
				leakedInstances = append(leakedInstances, *instance.InstanceId)
			}
		}
	}

	if len(leakedInstances) > 0 {
		log.Printf("terminating %d leaked instances: %v", len(leakedInstances), leakedInstances)
		if c.Yes || utils.ConfirmPrompt() {
			if _, err := c.awsClient.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: leakedInstances,
			}); err != nil {
				return fmt.Errorf("failed to automatically cleanup EC2 instances: %v", err)
			}

			switch c.cluster.State() {
			case cmv1.ClusterStateError:
				fallthrough
			case cmv1.ClusterStateUninstalling:
				log.Printf("success - cluster was in state: %s and should be uninstalled soon", c.cluster.State())
			default:
				log.Printf("success - cluster is in state: %s", c.cluster.State())
			}
			return nil
		}
	}

	log.Println("found 0 leaked instances")
	return nil
}
