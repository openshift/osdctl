package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	infraPkg "github.com/openshift/osdctl/pkg/infra"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	changeVolumeTypeCPMSNamespace = "openshift-machine-api"
	changeVolumeTypeCPMSName      = "cluster"

	pollInterval       = 30 * time.Second
	rolloutPollTimeout = 45 * time.Minute
)

var validVolumeTypes = []string{"gp3"}

type changeVolumeTypeOptions struct {
	clusterID  string
	cluster    *cmv1.Cluster
	reason     string
	targetType string
	role       string // "control-plane", "infra", or "" (both)

	client      client.Client
	clientAdmin client.Client

	hiveClient      client.Client
	hiveAdminClient client.Client
}

func newCmdChangeVolumeType() *cobra.Command {
	ops := &changeVolumeTypeOptions{}
	cmd := &cobra.Command{
		Use:   "change-ebs-volume-type",
		Short: "Change EBS volume type for control plane and/or infra nodes by replacing machines",
		Long: `Change the EBS volume type for control plane and/or infra nodes on a ROSA/OSD cluster.

This command replaces machines to change volume types (not in-place modification).
For control plane nodes, it patches the ControlPlaneMachineSet (CPMS) which automatically
rolls nodes one at a time. For infra nodes, it uses the Hive MachinePool dance to safely
replace all infra nodes with new ones using the target volume type.

Pre-flight checks are performed automatically before making changes.`,
		Example: `  # Change both control plane and infra volumes to gp3
  osdctl cluster change-ebs-volume-type -C ${CLUSTER_ID} --type gp3 --reason "SREP-3811"

  # Change only control plane volumes to gp3
  osdctl cluster change-ebs-volume-type -C ${CLUSTER_ID} --type gp3 --role control-plane --reason "SREP-3811"

  # Change only infra volumes to gp3
  osdctl cluster change-ebs-volume-type -C ${CLUSTER_ID} --type gp3 --role infra --reason "SREP-3811"`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run(context.Background())
		},
	}

	cmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The internal/external ID of the cluster")
	cmd.Flags().StringVar(&ops.targetType, "type", "", "Target EBS volume type (gp3)")
	cmd.Flags().StringVar(&ops.role, "role", "", "Node role to change: control-plane, infra (default: both)")
	cmd.Flags().StringVar(&ops.reason, "reason", "", "Reason for elevation (OHSS/PD/JIRA ticket)")

	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

func (o *changeVolumeTypeOptions) validate() error {
	if err := utils.IsValidClusterKey(o.clusterID); err != nil {
		return err
	}

	valid := false
	for _, t := range validVolumeTypes {
		if o.targetType == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid volume type: %s (must be one of: %s)", o.targetType, strings.Join(validVolumeTypes, ", "))
	}

	if o.role != "" && o.role != "control-plane" && o.role != "infra" {
		return fmt.Errorf("invalid role: %s (must be 'control-plane' or 'infra')", o.role)
	}

	return nil
}

func (o *changeVolumeTypeOptions) init() error {
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
	o.clusterID = cluster.ID()

	if strings.ToLower(cluster.CloudProvider().ID()) != "aws" {
		return fmt.Errorf("this command only supports AWS clusters (cluster is %s)", cluster.CloudProvider().ID())
	}

	if cluster.Hypershift().Enabled() {
		return errors.New("this command does not support HCP clusters")
	}

	scheme := runtime.NewScheme()
	if err := machinev1.Install(scheme); err != nil {
		return err
	}
	if err := machinev1beta1.Install(scheme); err != nil {
		return err
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}

	c, err := k8s.New(o.clusterID, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	o.client = c

	cAdmin, err := k8s.NewAsBackplaneClusterAdmin(o.clusterID, client.Options{Scheme: scheme}, []string{
		o.reason,
		fmt.Sprintf("Changing EBS volume type to %s for cluster %s", o.targetType, o.clusterID),
	}...)
	if err != nil {
		return err
	}
	o.clientAdmin = cAdmin

	// Set up Hive clients for infra node replacement via machinepool dance
	if o.role == "" || o.role == "infra" {
		hiveScheme := runtime.NewScheme()
		if err := hivev1.AddToScheme(hiveScheme); err != nil {
			return err
		}
		if err := corev1.AddToScheme(hiveScheme); err != nil {
			return err
		}

		hive, err := utils.GetHiveCluster(o.clusterID)
		if err != nil {
			return fmt.Errorf("failed to get hive cluster: %v", err)
		}

		hc, err := k8s.New(hive.ID(), client.Options{Scheme: hiveScheme})
		if err != nil {
			return fmt.Errorf("failed to create hive client: %v", err)
		}
		o.hiveClient = hc

		hac, err := k8s.NewAsBackplaneClusterAdmin(hive.ID(), client.Options{Scheme: hiveScheme}, []string{
			o.reason,
			fmt.Sprintf("Changing EBS volume type to %s for cluster %s", o.targetType, o.clusterID),
		}...)
		if err != nil {
			return fmt.Errorf("failed to create hive admin client: %v", err)
		}
		o.hiveAdminClient = hac
	}

	return nil
}

func (o *changeVolumeTypeOptions) run(ctx context.Context) error {
	if err := o.validate(); err != nil {
		return err
	}

	if err := o.init(); err != nil {
		return err
	}

	fmt.Printf("Cluster: %s (%s)\n", o.cluster.Name(), o.clusterID)
	fmt.Printf("Target volume type: %s\n", o.targetType)
	fmt.Printf("Role: %s\n", roleDisplay(o.role))
	fmt.Printf("Reason: %s\n\n", o.reason)

	// Pre-flight checks
	if err := o.preFlightChecks(ctx); err != nil {
		return fmt.Errorf("pre-flight checks failed: %v", err)
	}

	doControlPlane := o.role == "" || o.role == "control-plane"
	doInfra := o.role == "" || o.role == "infra"

	// Control plane
	if doControlPlane {
		if err := o.changeControlPlaneVolumeType(ctx); err != nil {
			return fmt.Errorf("control plane volume type change failed: %v", err)
		}
	}

	// Infra
	if doInfra {
		if err := o.changeInfraVolumeType(ctx); err != nil {
			return fmt.Errorf("infra volume type change failed: %v", err)
		}
	}

	printer.PrintlnGreen("\nVolume type change completed successfully!")
	return nil
}

// preFlightChecks verifies cluster health before making changes.
func (o *changeVolumeTypeOptions) preFlightChecks(ctx context.Context) error {
	fmt.Println("Running pre-flight checks...")

	// Check 1: CPMS state (if changing control plane)
	if o.role == "" || o.role == "control-plane" {
		cpms := &machinev1.ControlPlaneMachineSet{}
		if err := o.client.Get(ctx, client.ObjectKey{Namespace: changeVolumeTypeCPMSNamespace, Name: changeVolumeTypeCPMSName}, cpms); err != nil {
			return fmt.Errorf("failed to get CPMS: %v", err)
		}

		if cpms.Spec.State != machinev1.ControlPlaneMachineSetStateActive {
			return fmt.Errorf("CPMS is not Active (state: %s). Cannot proceed with control plane changes", cpms.Spec.State)
		}

		if cpms.Status.ReadyReplicas != 3 {
			return fmt.Errorf("CPMS does not have 3 ready replicas (ready: %d)", cpms.Status.ReadyReplicas)
		}
		fmt.Printf("  CPMS: Active, %d/3 ready\n", cpms.Status.ReadyReplicas)
	}

	// Check 2: Master nodes ready
	masterNodes := &corev1.NodeList{}
	if err := o.client.List(ctx, masterNodes, client.MatchingLabels{"node-role.kubernetes.io/master": ""}); err != nil {
		return fmt.Errorf("failed to list master nodes: %v", err)
	}
	readyMasters := countReadyNodes(masterNodes)
	if readyMasters != 3 {
		return fmt.Errorf("expected 3 ready master nodes, found %d", readyMasters)
	}
	fmt.Printf("  Master nodes: %d/3 Ready\n", readyMasters)

	// Check 3: Infra nodes ready (if changing infra)
	if o.role == "" || o.role == "infra" {
		infraNodes := &corev1.NodeList{}
		if err := o.client.List(ctx, infraNodes, client.MatchingLabels{"node-role.kubernetes.io/infra": ""}); err != nil {
			return fmt.Errorf("failed to list infra nodes: %v", err)
		}
		readyInfra := countReadyNodes(infraNodes)
		totalInfra := len(infraNodes.Items)
		if totalInfra == 0 {
			return fmt.Errorf("no infra nodes found")
		}
		if readyInfra != totalInfra {
			return fmt.Errorf("not all infra nodes are ready (%d/%d)", readyInfra, totalInfra)
		}
		fmt.Printf("  Infra nodes: %d/%d Ready\n", readyInfra, totalInfra)
	}

	// Check 4: etcd pods running
	etcdPods := &corev1.PodList{}
	if err := o.client.List(ctx, etcdPods, client.InNamespace("openshift-etcd"), client.MatchingLabels{"app": "etcd"}); err != nil {
		return fmt.Errorf("failed to list etcd pods: %v", err)
	}
	runningEtcd := 0
	for _, pod := range etcdPods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			runningEtcd++
		}
	}
	if runningEtcd != 3 {
		return fmt.Errorf("expected 3 running etcd pods, found %d", runningEtcd)
	}
	fmt.Printf("  etcd: %d/3 Running\n", runningEtcd)

	printer.PrintlnGreen("  All pre-flight checks passed!")
	fmt.Println()
	return nil
}

// changeControlPlaneVolumeType patches the CPMS to trigger a rolling replacement.
func (o *changeVolumeTypeOptions) changeControlPlaneVolumeType(ctx context.Context) error {
	printer.PrintlnGreen("=== Changing control plane volume type ===")

	cpms := &machinev1.ControlPlaneMachineSet{}
	if err := o.client.Get(ctx, client.ObjectKey{Namespace: changeVolumeTypeCPMSNamespace, Name: changeVolumeTypeCPMSName}, cpms); err != nil {
		return fmt.Errorf("failed to get CPMS: %v", err)
	}

	// Unmarshal the provider spec to read current blockDevices
	awsSpec := &machinev1beta1.AWSMachineProviderConfig{}
	if err := json.Unmarshal(cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value.Raw, awsSpec); err != nil {
		return fmt.Errorf("failed to unmarshal CPMS provider spec: %v", err)
	}

	if len(awsSpec.BlockDevices) == 0 {
		return fmt.Errorf("CPMS has no blockDevices configured")
	}

	currentType := ""
	if awsSpec.BlockDevices[0].EBS != nil && awsSpec.BlockDevices[0].EBS.VolumeType != nil {
		currentType = *awsSpec.BlockDevices[0].EBS.VolumeType
	}

	if currentType == o.targetType {
		fmt.Printf("Control plane volumes are already %s - skipping\n", o.targetType)
		return nil
	}

	fmt.Printf("Current control plane volume type: %s\n", currentType)
	fmt.Printf("Target volume type: %s\n", o.targetType)

	// Update volume type, preserving all other EBS settings (volumeSize, encrypted, kmsKey)
	targetType := o.targetType
	awsSpec.BlockDevices[0].EBS.VolumeType = &targetType
	awsSpec.BlockDevices[0].EBS.Iops = nil

	// Confirm
	fmt.Printf("\nThis will replace all 3 control plane nodes one at a time (~35-45 min).\n")
	if !utils.ConfirmPrompt() {
		return errors.New("aborted by user")
	}

	// Marshal and patch
	rawBytes, err := json.Marshal(awsSpec)
	if err != nil {
		return fmt.Errorf("failed to marshal updated provider spec: %v", err)
	}

	patch := client.MergeFrom(cpms.DeepCopy())
	cpms.Spec.Template.OpenShiftMachineV1Beta1Machine.Spec.ProviderSpec.Value = &runtime.RawExtension{Raw: rawBytes}

	if err := o.clientAdmin.Patch(ctx, cpms, patch); err != nil {
		return fmt.Errorf("failed to patch CPMS: %v", err)
	}

	printer.PrintlnGreen("CPMS patched successfully. Rolling replacement in progress...")
	fmt.Println("Monitoring rollout (this will take ~35-45 minutes)...")

	// Monitor the rollout
	if err := o.monitorCPMSRollout(ctx); err != nil {
		return err
	}

	printer.PrintlnGreen("Control plane volume type change complete!")
	return nil
}

// monitorCPMSRollout polls the CPMS until all replicas are updated.
func (o *changeVolumeTypeOptions) monitorCPMSRollout(ctx context.Context) error {
	pollCtx, cancel := context.WithTimeout(ctx, rolloutPollTimeout)
	defer cancel()
	return wait.PollUntilContextTimeout(pollCtx, pollInterval, rolloutPollTimeout, true, func(ctx context.Context) (bool, error) {
		cpms := &machinev1.ControlPlaneMachineSet{}
		if err := o.client.Get(ctx, client.ObjectKey{Namespace: changeVolumeTypeCPMSNamespace, Name: changeVolumeTypeCPMSName}, cpms); err != nil {
			log.Printf("Error checking CPMS status: %v", err)
			return false, nil
		}

		updated := cpms.Status.UpdatedReplicas
		ready := cpms.Status.ReadyReplicas

		log.Printf("[%s] CPMS: %d/3 updated, %d ready", time.Now().Format("15:04:05"), updated, ready)

		if updated == 3 && ready >= 3 {
			return true, nil
		}
		return false, nil
	})
}

const (
	volumeTypeChangedServiceLogTemplate = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/infranode_volume_type_changed.json"
)

// changeInfraVolumeType uses the Hive MachinePool dance from pkg/infra
// to replace infra nodes with new ones using the target volume type.
func (o *changeVolumeTypeOptions) changeInfraVolumeType(ctx context.Context) error {
	printer.PrintlnGreen("\n=== Changing infra node volume type ===")

	targetType := o.targetType
	previousType := ""

	originalMp, err := infraPkg.GetInfraMachinePool(ctx, o.hiveClient, o.clusterID)
	if err != nil {
		return err
	}

	newMp, err := infraPkg.CloneMachinePool(originalMp, func(mp *hivev1.MachinePool) error {
		if mp.Spec.Platform.AWS == nil {
			return fmt.Errorf("infra MachinePool has no AWS platform configuration")
		}
		previousType = mp.Spec.Platform.AWS.Type
		if previousType == targetType {
			return fmt.Errorf("infra volumes are already %s", targetType)
		}
		fmt.Printf("Current infra volume type: %s\n", previousType)
		fmt.Printf("Target volume type: %s\n", targetType)
		mp.Spec.Platform.AWS.Type = targetType
		mp.Spec.Platform.AWS.IOPS = 0
		return nil
	})
	if err != nil {
		return err
	}

	clients := infraPkg.DanceClients{
		ClusterClient: o.client,
		HiveClient:    o.hiveClient,
		HiveAdmin:     o.hiveAdminClient,
	}

	if err := infraPkg.RunMachinePoolDance(ctx, clients, originalMp, newMp, nil); err != nil {
		return err
	}

	// Post service log
	postCmd := servicelog.PostCmdOptions{
		Template:  volumeTypeChangedServiceLogTemplate,
		ClusterId: o.clusterID,
		TemplateParams: []string{
			fmt.Sprintf("PREVIOUS_VOLUME_TYPE=%s", previousType),
			fmt.Sprintf("NEW_VOLUME_TYPE=%s", targetType),
			fmt.Sprintf("REASON=%s", o.reason),
		},
	}
	if err := postCmd.Run(); err != nil {
		fmt.Println("Failed to post service log. Please manually send a service log with:")
		fmt.Printf("osdctl servicelog post %s -t %s -p %s\n",
			o.clusterID, volumeTypeChangedServiceLogTemplate, strings.Join(postCmd.TemplateParams, " -p "))
	}

	printer.PrintlnGreen("Infra volume type change complete!")
	return nil
}

func countReadyNodes(nodes *corev1.NodeList) int {
	ready := 0
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready++
			}
		}
	}
	return ready
}

func roleDisplay(role string) string {
	if role == "" {
		return "control-plane + infra"
	}
	return role
}
