package resize

// cspell:ignore embiggen
import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	twentyMinuteTimeout                   = 20 * time.Minute
	twentySecondIncrement                 = 20 * time.Second
	resizedInfraNodeServiceLogTemplate    = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/infranode_resized.json"
	GCPresizedInfraNodeServiceLogTemplate = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/gcp/GCP_infranode_resized_auto.json"
	infraNodeLabel                        = "node-role.kubernetes.io/infra"
	temporaryInfraNodeLabel               = "osdctl.openshift.io/infra-resize-temporary-machinepool"
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

	infraResizeCmd.MarkFlagRequired("cluster-id")
	infraResizeCmd.MarkFlagRequired("justification")
	infraResizeCmd.MarkFlagRequired("reason")
	infraResizeCmd.MarkFlagRequired("ohss")

	return infraResizeCmd
}

func (r *Infra) New() error {
	if err := validateInstanceSize(r.instanceType, "infra"); err != nil {
		return err
	}

	scheme := runtime.NewScheme()

	// Register machinev1beta1 for Machines
	if err := machinev1beta1.Install(scheme); err != nil {
		return err
	}

	// Register hivev1 for MachinePools
	if err := hivev1.AddToScheme(scheme); err != nil {
		return err
	}

	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}

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

	hive, err := utils.GetHiveCluster(cluster.ID())
	if err != nil {
		return err
	}

	c, err := k8s.New(cluster.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	hc, err := k8s.New(hive.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	hac, err := k8s.NewAsBackplaneClusterAdmin(hive.ID(), client.Options{Scheme: scheme}, []string{
		r.reason,
		fmt.Sprintf("Need elevation for %s cluster in order to resize it to instance type %s", r.clusterId, r.instanceType),
	}...)
	if err != nil {
		return err
	}

	r.clusterId = cluster.ID()
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
	originalMp, err := r.getInfraMachinePool(ctx)
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
	tempMp := newMp.DeepCopy()
	tempMp.Name = fmt.Sprintf("%s2", tempMp.Name)
	tempMp.Spec.Name = fmt.Sprintf("%s2", tempMp.Spec.Name)
	tempMp.Spec.Labels[temporaryInfraNodeLabel] = ""

	instanceType, err := getInstanceType(tempMp)
	if err != nil {
		return fmt.Errorf("failed to parse instance type from machinepool: %v", err)
	}

	// Create the temporary machinepool
	log.Printf("planning to resize to instance type from %s to %s", originalInstanceType, instanceType)
	if !utils.ConfirmPrompt() {
		log.Printf("exiting")
		return nil
	}

	log.Printf("creating temporary machinepool %s, with instance type %s", tempMp.Name, instanceType)
	if err := r.hiveAdmin.Create(ctx, tempMp); err != nil {
		return err
	}

	// This selector will match all infra nodes
	selector, err := labels.Parse(infraNodeLabel)
	if err != nil {
		return err
	}

	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		nodes := &corev1.NodeList{}

		if err := r.client.List(ctx, nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
			log.Printf("error retrieving nodes list, continuing to wait: %s", err)
			return false, nil
		}

		readyNodes := 0
		log.Printf("waiting for %d infra nodes to be reporting Ready", int(*originalMp.Spec.Replicas)*2)
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady {
					if cond.Status == corev1.ConditionTrue {
						readyNodes++
						log.Printf("found node %s reporting Ready", node.Name)
					}
				}
			}
		}

		switch {
		case readyNodes >= int(*originalMp.Spec.Replicas)*2:
			return true, nil
		default:
			log.Printf("found %d infra nodes reporting Ready, continuing to wait", readyNodes)
			return false, nil
		}
	}); err != nil {
		return err
	}

	// Identify the original nodes and temp nodes
	// requireInfra matches all infra nodes using the selector from above
	requireInfra, err := labels.NewRequirement(infraNodeLabel, selection.Exists, nil)
	if err != nil {
		return err
	}

	// requireNotTempNode matches all nodes that do not have the temporaryInfraNodeLabel, created with the new (temporary) machine pool
	requireNotTempNode, err := labels.NewRequirement(temporaryInfraNodeLabel, selection.DoesNotExist, nil)
	if err != nil {
		return err
	}

	// requireTempNode matches the opposite of above, all nodes that *do* have the temporaryInfraNodeLabel
	requireTempNode, err := labels.NewRequirement(temporaryInfraNodeLabel, selection.Exists, nil)
	if err != nil {
		return err
	}

	// infraNode + notTempNode = original nodes
	originalNodeSelector := selector.Add(*requireInfra, *requireNotTempNode)

	// infraNode + tempNode = temp nodes
	tempNodeSelector := selector.Add(*requireInfra, *requireTempNode)

	originalNodes := &corev1.NodeList{}
	if err := r.client.List(ctx, originalNodes, &client.ListOptions{LabelSelector: originalNodeSelector}); err != nil {
		return err
	}

	// Delete original machinepool
	log.Printf("deleting original machinepool %s, with instance type %s", originalMp.Name, originalInstanceType)
	if err := r.hiveAdmin.Delete(ctx, originalMp); err != nil {
		return err
	}

	// Wait for original machinepool to delete
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		mp := &hivev1.MachinePool{}
		err := r.hive.Get(ctx, client.ObjectKey{Namespace: originalMp.Namespace, Name: originalMp.Name}, mp)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			log.Printf("error retrieving machines list, continuing to wait: %s", err)
			return false, nil
		}

		log.Printf("original machinepool %s/%s still exists, continuing to wait", originalMp.Namespace, originalMp.Name)
		return false, nil
	}); err != nil {
		return err
	}

	// Wait for original nodes to delete
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		// Re-check for originalNodes to see if they have been deleted
		return skipError(wrapResult(r.nodesMatchExpectedCount(ctx, originalNodeSelector, 0)), "error matching expected count")
	}); err != nil {
		switch {
		case errors.Is(err, wait.ErrWaitTimeout):
			log.Printf("Warning: timed out waiting for nodes to drain: %v. Terminating backing cloud instances.", err.Error())

			// Terminate the backing cloud instances if they are not removed by the 20 minute timeout
			err := r.terminateCloudInstances(ctx, originalNodes)
			if err != nil {
				return err
			}

			if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
				log.Printf("waiting for nodes to terminate")
				return skipError(wrapResult(r.nodesMatchExpectedCount(ctx, originalNodeSelector, 0)), "error matching expected count")
			}); err != nil {
				if errors.Is(err, wait.ErrWaitTimeout) {
					log.Printf("timed out waiting for nodes to terminate: %v.", err.Error())
				}
				return err
			}
		default:
			return err
		}
	}

	// Create new permanent machinepool
	log.Printf("creating new machinepool %s, with instance type %s", newMp.Name, instanceType)
	if err := r.hiveAdmin.Create(ctx, newMp); err != nil {
		return err
	}

	// Wait for new permanent machines to become nodes
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		nodes := &corev1.NodeList{}
		selector, err := labels.Parse("node-role.kubernetes.io/infra=")
		if err != nil {
			// This should never happen, so we do not have to skip this error
			return false, err
		}

		if err := r.client.List(ctx, nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
			log.Printf("error retrieving nodes list, continuing to wait: %s", err)
			return false, nil
		}

		readyNodes := 0
		log.Printf("waiting for %d infra nodes to be reporting Ready", int(*originalMp.Spec.Replicas)*2)
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady {
					if cond.Status == corev1.ConditionTrue {
						readyNodes++
						log.Printf("found node %s reporting Ready", node.Name)
					}
				}
			}
		}

		switch {
		case readyNodes >= int(*originalMp.Spec.Replicas)*2:
			return true, nil
		default:
			log.Printf("found %d infra nodes reporting Ready, continuing to wait", readyNodes)
			return false, nil
		}
	}); err != nil {
		return err
	}

	tempNodes := &corev1.NodeList{}
	if err := r.client.List(ctx, tempNodes, &client.ListOptions{LabelSelector: tempNodeSelector}); err != nil {
		return err
	}

	// Delete temp machinepool
	log.Printf("deleting temporary machinepool %s, with instance type %s", tempMp.Name, instanceType)
	if err := r.hiveAdmin.Delete(ctx, tempMp); err != nil {
		return err
	}

	// Wait for temporary machinepool to delete
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		mp := &hivev1.MachinePool{}
		err := r.hive.Get(ctx, client.ObjectKey{Namespace: tempMp.Namespace, Name: tempMp.Name}, mp)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			log.Printf("error retrieving old machine details, continuing to wait: %s", err)
			return false, nil
		}

		log.Printf("temporary machinepool %s/%s still exists, continuing to wait", tempMp.Namespace, tempMp.Name)
		return false, nil
	}); err != nil {
		return err
	}

	// Wait for infra node count to return to normal
	log.Printf("waiting for infra node count to return to: %d", int(*originalMp.Spec.Replicas))
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		nodes := &corev1.NodeList{}
		selector, err := labels.Parse("node-role.kubernetes.io/infra=")
		if err != nil {
			// This should never happen, so we do not have to skip this errorreturn false, err
			return false, err
		}

		if err := r.client.List(ctx, nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
			log.Printf("error retrieving nodes list, continuing to wait: %s", err)
			return false, nil
		}

		switch len(nodes.Items) {
		case int(*originalMp.Spec.Replicas):
			log.Printf("found %d infra nodes, infra resize complete", len(nodes.Items))
			return true, nil
		default:
			log.Printf("found %d infra nodes, continuing to wait", len(nodes.Items))
			return false, nil
		}
	}); err != nil {
		switch {
		case errors.Is(err, wait.ErrWaitTimeout):
			log.Printf("Warning: timed out waiting for nodes to drain: %v. Terminating backing cloud instances.", err.Error())

			err := r.terminateCloudInstances(ctx, tempNodes)
			if err != nil {
				return err
			}

			if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
				log.Printf("waiting for nodes to terminate")
				return skipError(wrapResult(r.nodesMatchExpectedCount(ctx, tempNodeSelector, 0)), "error matching expected count")
			}); err != nil {
				if errors.Is(err, wait.ErrWaitTimeout) {
					log.Printf("timed out waiting for nodes to terminate: %v.", err.Error())
				}
				return err
			}
		default:
			return err
		}
	}

	postCmd := generateServiceLog(tempMp, r.instanceType, r.justification, r.clusterId, r.ohss)
	if err := postCmd.Run(); err != nil {
		fmt.Println("Failed to generate service log. Please manually send a service log to the customer for the blocked egresses with:")
		fmt.Printf("osdctl servicelog post %v -t %v -p %v\n",
			r.clusterId, resizedInfraNodeServiceLogTemplate, strings.Join(postCmd.TemplateParams, " -p "))
	}

	return nil
}

func (r *Infra) getInfraMachinePool(ctx context.Context) (*hivev1.MachinePool, error) {
	ns := &corev1.NamespaceList{}
	selector, err := labels.Parse(fmt.Sprintf("api.openshift.com/id=%s", r.clusterId))
	if err != nil {
		return nil, err
	}

	if err := r.hive.List(ctx, ns, &client.ListOptions{LabelSelector: selector, Limit: 1}); err != nil {
		return nil, err
	}
	if len(ns.Items) != 1 {
		return nil, fmt.Errorf("expected 1 namespace, found %d namespaces with tag: api.openshift.com/id=%s", len(ns.Items), r.clusterId)
	}

	log.Printf("found namespace: %s", ns.Items[0].Name)

	mpList := &hivev1.MachinePoolList{}
	if err := r.hive.List(ctx, mpList, &client.ListOptions{Namespace: ns.Items[0].Name}); err != nil {
		return nil, err
	}

	for _, mp := range mpList.Items {
		mp := mp
		if mp.Spec.Name == "infra" {
			log.Printf("found machinepool %s", mp.Name)
			return &mp, nil
		}
	}

	return nil, fmt.Errorf("did not find the infra machinepool in namespace: %s", ns.Items[0].Name)
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
	}

	newMp := &hivev1.MachinePool{}
	mp.DeepCopyInto(newMp)

	// Unset fields we want to be regenerated
	newMp.CreationTimestamp = metav1.Time{}
	newMp.Finalizers = []string{}
	newMp.ResourceVersion = ""
	newMp.Generation = 0
	newMp.SelfLink = ""
	newMp.UID = ""
	newMp.Status = hivev1.MachinePoolStatus{}

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

	switch r.cluster.CloudProvider().ID() {
	case "aws":
		newMp.Spec.Platform.AWS.InstanceType = r.instanceType
	case "gcp":
		newMp.Spec.Platform.GCP.InstanceType = r.instanceType
	default:
		return nil, fmt.Errorf("cloud provider not supported: %s, only AWS and GCP are supported", r.cluster.CloudProvider().ID())
	}

	return newMp, nil
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
			Template:       GCPresizedInfraNodeServiceLogTemplate,
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
		ocmClient, err := utils.CreateConnection()
		if err != nil {
			return err
		}
		defer ocmClient.Close()
		cfg, err := osdCloud.CreateAWSV2Config(ocmClient, r.cluster)
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

// nodesMatchExpectedCount accepts a context, labelselector and count of expected nodes, and ]
// returns true if the nodelist matching the labelselector is equal to the expected count
func (r *Infra) nodesMatchExpectedCount(ctx context.Context, labelSelector labels.Selector, count int) (bool, error) {
	nodeList := &corev1.NodeList{}

	if err := r.client.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return false, err
	}

	if len(nodeList.Items) == count {
		return true, nil
	}

	return false, nil
}

// validateInstanceSize accepts a string for the requested new instance type and returns an error
// if the instance type is invalid
func validateInstanceSize(newInstanceSize string, nodeType string) error {
	if !slices.Contains(supportedInstanceTypes[nodeType], newInstanceSize) {
		return fmt.Errorf("instance type %s not supported for %s nodes", newInstanceSize, nodeType)
	}
	return nil
}

// having an error when being in a retry loop, should not be handled as an error, and we should just display it and continue
// in case we have a function that return a bool status and an error, we can use following helper
// f being a function returning (bool, error), replace
//
//	return f(...)
//
// by
//
//	return skipError(wrapResult(f(...)), "message to context the error")
//
// and then the return will always have error set to nil, but a continuing message will be displayed in case of error
type result struct {
	condition bool
	err       error
}

func wrapResult(condition bool, err error) result {
	return result{condition, err}
}

func skipError(res result, msg string) (bool, error) {
	if res.err != nil {
		log.Printf("%s, continuing to wait: %s", msg, res.err)
	}
	return res.condition, nil
}
