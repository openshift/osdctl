package resize

// cspell:ignore embiggen
import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	infraPkg "github.com/openshift/osdctl/pkg/infra"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	resizedInfraNodeServiceLogTemplate    = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/infranode_resized.json"
	resizedInfraNodeServiceLogTemplateGCP = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/gcp/GCP_infranode_resized_auto.json"
)

type Infra struct {
	client    client.Client
	hive      client.Client
	hiveAdmin client.Client

	cluster   *cmv1.Cluster
	clusterId string

	// instanceType is the type of instance being resized to
	instanceType string

	// reason to provide for elevation (eg: OHSS/PG ticket)
	reason string

	// reason to provide for resize
	justification string

	// OHSS ticket to reference in SL
	ohss string

	// hiveOcmUrl is the OCM environment URL for Hive operations
	hiveOcmUrl string
}

func newCmdResizeInfra() *cobra.Command {
	r := &Infra{}

	infraResizeCmd := &cobra.Command{
		Use:   "infra",
		Short: "Resize an OSD/ROSA cluster's infra nodes",
		Long: `Resize an OSD/ROSA cluster's infra nodes

  This command automates most of the "machinepool dance" to safely resize infra nodes for production classic OSD/ROSA 
  clusters. This DOES NOT work in non-production due to environmental differences.

  Remember to follow the SOP for preparation and follow up steps:

    https://github.com/openshift/ops-sop/blob/master/v4/howto/resize-infras-workers.md
`,
		Example: `
  # Automatically vertically scale infra nodes to the next size
  osdctl cluster resize infra --cluster-id ${CLUSTER_ID}

  # Resize infra nodes to a specific instance type
  osdctl cluster resize infra --cluster-id ${CLUSTER_ID} --instance-type "r5.xlarge"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.RunInfra(context.Background())
		},
	}

	infraResizeCmd.Flags().StringVarP(&r.clusterId, "cluster-id", "C", "", "OCM internal/external cluster id or cluster name to resize infra nodes for.")
	infraResizeCmd.Flags().StringVar(&r.instanceType, "instance-type", "", "(optional) Override for an AWS or GCP instance type to resize the infra nodes to, by default supported instance types are automatically selected.")
	infraResizeCmd.Flags().StringVar(&r.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)")
	infraResizeCmd.Flags().StringVar(&r.justification, "justification", "", "The justification behind resize")
	infraResizeCmd.Flags().StringVar(&r.ohss, "ohss", "", "OHSS ticket tracking this infra node resize")
	infraResizeCmd.Flags().StringVar(&r.hiveOcmUrl, "hive-ocm-url", "", "(optional) OCM environment URL for hive operations. Aliases: 'production', 'staging', 'integration'. If not specified, uses the same OCM environment as the target cluster.")

	_ = infraResizeCmd.MarkFlagRequired("cluster-id")
	_ = infraResizeCmd.MarkFlagRequired("justification")
	_ = infraResizeCmd.MarkFlagRequired("reason")
	_ = infraResizeCmd.MarkFlagRequired("ohss")

	return infraResizeCmd
}

func (r *Infra) New() error {
	// Only validate the instanceType value if one is provided, otherwise we rely on embiggenMachinePool to provide the size
	if r.instanceType != "" {
		if err := validateInstanceSize(r.instanceType, "infra"); err != nil {
			return err
		}
	}

	// Validate --hive-ocm-url if provided
	if r.hiveOcmUrl != "" {
		_, err := utils.ValidateAndResolveOcmUrl(r.hiveOcmUrl)
		if err != nil {
			return fmt.Errorf("invalid --hive-ocm-url: %w", err)
		}
	}

	scheme := runtime.NewScheme()

	if err := machinev1beta1.Install(scheme); err != nil {
		return err
	}
	if err := hivev1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}

	var hive *cmv1.Cluster
	var c, hc, hac client.Client

	if r.hiveOcmUrl != "" {
		// Multi-environment path - enables staging/integration testing
		targetOCM, err := utils.CreateConnection()
		if err != nil {
			return fmt.Errorf("failed to create target OCM connection: %w", err)
		}
		defer targetOCM.Close()

		hiveOCM, err := utils.CreateConnectionWithUrl(r.hiveOcmUrl)
		if err != nil {
			return fmt.Errorf("failed to create hive OCM connection with URL '%s': %w", r.hiveOcmUrl, err)
		}
		defer hiveOCM.Close()

		cluster, err := utils.GetClusterAnyStatus(targetOCM, r.clusterId)
		if err != nil {
			return fmt.Errorf("failed to get OCM cluster info for %s: %s", r.clusterId, err)
		}
		r.cluster = cluster
		r.clusterId = cluster.ID()

		hive, err = utils.GetHiveClusterWithConn(cluster.ID(), targetOCM, hiveOCM)
		if err != nil {
			return fmt.Errorf("failed to get hive cluster (OCM URL:'%s'): %w", r.hiveOcmUrl, err)
		}

		c, err = k8s.New(cluster.ID(), client.Options{Scheme: scheme})
		if err != nil {
			return err
		}

		hc, err = k8s.NewWithConn(hive.ID(), client.Options{Scheme: scheme}, hiveOCM)
		if err != nil {
			return fmt.Errorf("failed to create hive k8s client (OCM URL:'%s'): %w", r.hiveOcmUrl, err)
		}

		hac, err = k8s.NewAsBackplaneClusterAdminWithConn(hive.ID(), client.Options{Scheme: scheme}, hiveOCM, []string{
			r.reason,
			fmt.Sprintf("Need elevation for %s cluster in order to resize it to instance type %s", r.clusterId, r.instanceType),
		}...)
		if err != nil {
			return fmt.Errorf("failed to create hive admin k8s client (OCM URL:'%s'): %w", r.hiveOcmUrl, err)
		}
	} else {
		// Original path - backward compatible
		ocmClient, err := utils.CreateConnection()
		if err != nil {
			return err
		}
		defer ocmClient.Close()

		cluster, err := utils.GetClusterAnyStatus(ocmClient, r.clusterId)
		if err != nil {
			return fmt.Errorf("failed to get OCM cluster info for %s: %s", r.clusterId, err)
		}
		r.cluster = cluster
		r.clusterId = cluster.ID()

		hive, err = utils.GetHiveCluster(cluster.ID())
		if err != nil {
			return err
		}

		c, err = k8s.New(cluster.ID(), client.Options{Scheme: scheme})
		if err != nil {
			return err
		}

		hc, err = k8s.New(hive.ID(), client.Options{Scheme: scheme})
		if err != nil {
			return err
		}

		hac, err = k8s.NewAsBackplaneClusterAdmin(hive.ID(), client.Options{Scheme: scheme}, []string{
			r.reason,
			fmt.Sprintf("Need elevation for %s cluster in order to resize it to instance type %s", r.clusterId, r.instanceType),
		}...)
		if err != nil {
			return err
		}
	}

	r.clusterId = r.cluster.ID()
	r.client = c
	r.hive = hc
	r.hiveAdmin = hac

	return nil
}

func (r *Infra) RunInfra(ctx context.Context) error {
	if err := r.New(); err != nil {
		return fmt.Errorf("failed to initialize command: %v", err)
	}

	log.Printf("resizing infra nodes for %s - %s", r.cluster.Name(), r.clusterId)
	originalMp, err := infraPkg.GetInfraMachinePool(ctx, r.hive, r.clusterId)
	if err != nil {
		return err
	}
	originalInstanceType, err := getInstanceType(originalMp)
	if err != nil {
		return fmt.Errorf("failed to parse instance type from machinepool: %v", err)
	}

	newMp, err := r.embiggenMachinePool(originalMp)
	if err != nil {
		return err
	}
	instanceType, err := getInstanceType(newMp)
	if err != nil {
		return fmt.Errorf("failed to parse instance type from machinepool: %v", err)
	}

	log.Printf("planning to resize to instance type from %s to %s", originalInstanceType, instanceType)
	if !utils.ConfirmPrompt() {
		log.Printf("exiting")
		return nil
	}

	clients := infraPkg.DanceClients{
		ClusterClient: r.client,
		HiveClient:    r.hive,
		HiveAdmin:     r.hiveAdmin,
	}

	if err := infraPkg.RunMachinePoolDance(ctx, clients, originalMp, newMp, r.terminateCloudInstances); err != nil {
		return err
	}

	postCmd := generateServiceLog(newMp, r.instanceType, r.justification, r.clusterId, r.ohss)
	if err := postCmd.Run(); err != nil {
		fmt.Println("Failed to generate service log. Please manually send a service log to the customer for the blocked egresses with:")
		fmt.Printf("osdctl servicelog post %v -t %v -p %v\n",
			r.clusterId, resizedInfraNodeServiceLogTemplate, strings.Join(postCmd.TemplateParams, " -p "))
	}

	return nil
}

func (r *Infra) embiggenMachinePool(mp *hivev1.MachinePool) (*hivev1.MachinePool, error) {
	embiggen := map[string]string{
		"m5.xlarge":  "r5.xlarge",
		"m5.2xlarge": "r5.2xlarge",
		"r5.xlarge":  "r5.2xlarge",
		"r5.2xlarge": "r5.4xlarge",
		"r5.4xlarge": "r5.8xlarge",
		// GCP
		"custom-4-32768-ext": "custom-8-65536-ext",
		"custom-8-65536-ext": "custom-16-131072-ext",
		"n2-highmem-4":       "n2-highmem-8",
		"n2-highmem-8":       "n2-highmem-16",
	}

	// Update instance type sizing
	if r.instanceType != "" {
		log.Printf("using override instance type: %s", r.instanceType)
	} else {
		instanceType, err := getInstanceType(mp)
		if err != nil {
			return nil, err
		}
		if _, ok := embiggen[instanceType]; !ok {
			return nil, fmt.Errorf("resizing instance type %s not supported", instanceType)
		}

		r.instanceType = embiggen[instanceType]
	}

	cloudProvider := r.cluster.CloudProvider().ID()
	newInstanceType := r.instanceType

	return infraPkg.CloneMachinePool(mp, func(newMp *hivev1.MachinePool) error {
		switch cloudProvider {
		case "aws":
			newMp.Spec.Platform.AWS.InstanceType = newInstanceType
		case "gcp":
			newMp.Spec.Platform.GCP.InstanceType = newInstanceType
		default:
			return fmt.Errorf("cloud provider not supported: %s, only AWS and GCP are supported", cloudProvider)
		}
		return nil
	})
}

func getInstanceType(mp *hivev1.MachinePool) (string, error) {
	if mp.Spec.Platform.AWS != nil {
		return mp.Spec.Platform.AWS.InstanceType, nil
	} else if mp.Spec.Platform.GCP != nil {
		return mp.Spec.Platform.GCP.InstanceType, nil
	}

	return "", errors.New("unsupported platform, only AWS and GCP are supported")
}

// Adding change in serviceLog as per the cloud provider.
func generateServiceLog(mp *hivev1.MachinePool, instanceType, justification, clusterId, ohss string) servicelog.PostCmdOptions {
	if mp.Spec.Platform.AWS != nil {
		return servicelog.PostCmdOptions{
			Template:       resizedInfraNodeServiceLogTemplate,
			ClusterId:      clusterId,
			TemplateParams: []string{fmt.Sprintf("INSTANCE_TYPE=%s", instanceType), fmt.Sprintf("JUSTIFICATION=%s", justification), fmt.Sprintf("JIRA_ID=%s", ohss)},
		}
	} else if mp.Spec.Platform.GCP != nil {
		return servicelog.PostCmdOptions{
			Template:       resizedInfraNodeServiceLogTemplateGCP,
			ClusterId:      clusterId,
			TemplateParams: []string{fmt.Sprintf("INSTANCE_TYPE=%s", instanceType), fmt.Sprintf("JUSTIFICATION=%s", justification)},
		}
	}
	return servicelog.PostCmdOptions{}
}

func (r *Infra) terminateCloudInstances(ctx context.Context, nodeList *corev1.NodeList) error {
	if len(nodeList.Items) == 0 {
		return nil
	}

	var instanceIDs []string

	for _, node := range nodeList.Items {
		instanceIDs = append(instanceIDs, convertProviderIDtoInstanceID(node.Spec.ProviderID))
	}

	switch r.cluster.CloudProvider().ID() {
	case "aws":
		var ocmClient interface{}
		if mOCM, ok := ctx.Value("ocm").(interface{ ClustersMgmt() interface{} }); ok {
			ocmClient = mOCM
		} else {
			var err error
			ocmClient, err = utils.CreateConnection()
			if err != nil {
				return err
			}
			defer ocmClient.(*sdk.Connection).Close()
		}

		if mBuilder, ok := ctx.Value("aws_builder").(interface {
			CreateAWSV2Config(interface{}, *cmv1.Cluster) (awssdk.Config, error)
		}); ok {
			cfg, err := mBuilder.CreateAWSV2Config(ocmClient, r.cluster)
			if err != nil {
				return err
			}

			cfg.Region = r.cluster.Region().ID()
			awsClient := ec2.NewFromConfig(cfg)
			_, err = awsClient.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: instanceIDs,
			})
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					code := apiErr.ErrorCode()
					message := apiErr.ErrorMessage()
					log.Printf("AWS ERROR: %v - %v\n", code, message)
				} else {
					log.Printf("ERROR: %v\n", err.Error())
				}
				return err
			}
		} else {
			cfg, err := osdCloud.CreateAWSV2Config(ocmClient.(*sdk.Connection), r.cluster)
			if err != nil {
				return err
			}
			awsClient := ec2.NewFromConfig(cfg)
			_, err = awsClient.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: instanceIDs,
			})
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					code := apiErr.ErrorCode()
					message := apiErr.ErrorMessage()
					log.Printf("AWS ERROR: %v - %v\n", code, message)
				} else {
					log.Printf("ERROR: %v\n", err.Error())
				}
				return err
			}
		}

	case "gcp":
		// There isn't currently a way to programmatically retrieve backplane credentials for GCP
		log.Printf("GCP support for manually terminating instances not yet supported. "+
			"Please use backplane to login and terminate the instances manually: %v", strings.Join(instanceIDs, ", "))
		return nil

	default:
		return fmt.Errorf("cloud provider not supported: %s, only AWS is supported", r.cluster.CloudProvider().ID())
	}

	log.Printf("requested termination of instances: %v", strings.Join(instanceIDs, ", "))

	return nil
}

// convertProviderIDtoInstanceID converts a provider ID to an instance ID
// ProviderIDs come in the format: aws:///us-east-1a/i-0a1b2c3d4e5f6g7h8
// or: gce://some-string/europe-west4-a/my-cluster-name-n65hp-infra-a-4fbrd
// InstanceIDs come in the format: i-0a1b2c3d4e5f6g7h8
// or: my-cluster-name-n65hp-infra-a-4fbrd
func convertProviderIDtoInstanceID(providerID string) string {
	providerIDSplit := strings.Split(providerID, "/")
	return providerIDSplit[len(providerIDSplit)-1]
}

// validateInstanceSize accepts a string for the requested new instance type and returns an error
// if the instance type is invalid
func validateInstanceSize(newInstanceSize string, nodeType string) error {
	if !slices.Contains(supportedInstanceTypes[nodeType], newInstanceSize) {
		return fmt.Errorf("instance type %s not supported for %s nodes", newInstanceSize, nodeType)
	}
	return nil
}
