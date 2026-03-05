package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
)

// changeVolumeTypeOptions holds the configuration for the change-volume-type command,
// including cluster identification, volume target settings, and optional performance parameters.
type changeVolumeTypeOptions struct {
	clusterID  string
	cluster    *cmv1.Cluster
	reason     string
	volumeID   string
	targetType string
	iops       *int32
	throughput *int32
	dryRun     bool
}

// newCmdChangeVolumeType creates the cobra command for changing an EBS volume type
// on an AWS cluster. It registers required flags (cluster-id, volume-id, target-type, reason)
// and optional performance flags (iops, throughput, dry-run).
func newCmdChangeVolumeType() *cobra.Command {
	ops := &changeVolumeTypeOptions{}
	cmd := &cobra.Command{
		Use:   "change-volume-type --cluster-id <cluster-id> --volume-id <volume-id> --target-type <type>",
		Short: "Change EBS volume type (e.g., io1 to gp3) for cluster volumes",
		Long: `Change the type of an EBS volume attached to a cluster.

Common use cases:
  - Migrate master node volumes from io1 to gp3 for cost optimization
  - Change volume performance characteristics without data loss

IMPORTANT:
  - This operation is performed online (no downtime required)
  - AWS will migrate the volume in the background
  - The node and cluster remain operational during the change
  - Requires backplane elevation (--reason flag)
  - IOPS and throughput are only set if explicitly provided`,
		Example: `  # Migrate a master volume from io1 to gp3 with custom IOPS
  osdctl cluster change-volume-type \
    --cluster-id 2abc123def456 \
    --volume-id vol-1234567890abcdef0 \
    --target-type gp3 \
    --iops 3000 \
    --throughput 125 \
    --reason "SREP-3811 - Master volume cost optimization"

  # Change to gp3 with default performance (AWS defaults)
  osdctl cluster change-volume-type \
    --cluster-id 2abc123def456 \
    --volume-id vol-1234567890abcdef0 \
    --target-type gp3 \
    --reason "SREP-3811"

  # Dry run to preview changes
  osdctl cluster change-volume-type \
    --cluster-id 2abc123def456 \
    --volume-id vol-1234567890abcdef0 \
    --target-type gp3 \
    --reason "SREP-3811" \
    --dry-run`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			// Parse optional int32 flags (only set if provided)
			if cmd.Flags().Changed("iops") {
				val, _ := cmd.Flags().GetInt32("iops")
				ops.iops = &val
			}
			if cmd.Flags().Changed("throughput") {
				val, _ := cmd.Flags().GetInt32("throughput")
				ops.throughput = &val
			}
			cmdutil.CheckErr(ops.run())
		},
	}

	// Required flags
	cmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID (internal ID or external ID)")
	cmd.Flags().StringVarP(&ops.volumeID, "volume-id", "v", "", "EBS volume ID (e.g., vol-1234567890abcdef0)")
	cmd.Flags().StringVarP(&ops.targetType, "target-type", "t", "", "Target volume type (gp2, gp3, io1, io2, st1, sc1)")
	cmd.Flags().StringVar(&ops.reason, "reason", "", "Reason for elevation (OHSS/PD/JIRA ticket)")

	// Optional flags - no defaults, only set if explicitly provided
	cmd.Flags().Int32("iops", 0, "Provisioned IOPS (for io1, io2, gp3). If not specified, keeps current value or uses AWS defaults.")
	cmd.Flags().Int32("throughput", 0, "Throughput in MB/s (for gp3 only). If not specified, uses AWS defaults.")
	cmd.Flags().BoolVar(&ops.dryRun, "dry-run", false, "Dry run - show what would be changed without executing")

	// Mark required flags
	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("volume-id")
	_ = cmd.MarkFlagRequired("target-type")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

// run executes the volume type change workflow: validates inputs, retrieves the cluster
// and volume from AWS, verifies ownership, and initiates the EBS ModifyVolume API call.
// It monitors modification progress and reports the final volume configuration.
func (o *changeVolumeTypeOptions) run() error {
	// Validate cluster ID
	if err := utils.IsValidClusterKey(o.clusterID); err != nil {
		return err
	}

	// Validate target type
	validTypes := []string{"gp2", "gp3", "io1", "io2", "st1", "sc1"}
	valid := false
	for _, t := range validTypes {
		if o.targetType == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid target type: %s (must be one of: %v)", o.targetType, validTypes)
	}

	// Validate IOPS/throughput compatibility with target type
	iopsTypes := map[string]bool{"io1": true, "io2": true, "gp3": true}
	if o.iops != nil && !iopsTypes[o.targetType] {
		return fmt.Errorf("--iops is not supported for volume type %s (only io1, io2, gp3)", o.targetType)
	}
	if o.throughput != nil && o.targetType != "gp3" {
		return fmt.Errorf("--throughput is only supported for volume type gp3, not %s", o.targetType)
	}

	// Create OCM connection
	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("failed to create OCM connection: %w", err)
	}
	defer connection.Close()

	// Get cluster
	cluster, err := utils.GetCluster(connection, o.clusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}
	o.cluster = cluster
	o.clusterID = cluster.ID()

	// Verify AWS cluster (case-insensitive)
	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("this command only supports AWS clusters (cluster is %s)", cluster.CloudProvider().ID())
	}

	fmt.Printf("Cluster: %s (%s)\n", cluster.Name(), cluster.ID())
	fmt.Printf("Region: %s\n", cluster.Region().ID())
	fmt.Printf("Reason: %s\n", o.reason)
	fmt.Printf("Volume: %s\n", o.volumeID)
	fmt.Printf("Target Type: %s\n", o.targetType)
	if o.iops != nil {
		fmt.Printf("IOPS: %d\n", *o.iops)
	} else {
		fmt.Printf("IOPS: (use current value or AWS defaults)\n")
	}
	if o.targetType == "gp3" {
		if o.throughput != nil {
			fmt.Printf("Throughput: %d MB/s\n", *o.throughput)
		} else {
			fmt.Printf("Throughput: (use AWS defaults)\n")
		}
	}

	// Get AWS credentials via backplane.
	// Note: CreateAWSV2Config does not accept a reason parameter; the --reason flag
	// serves as an SRE audit trail in command output, consistent with other commands
	// (e.g., detach-stuck-volume). Wiring reason into AWS elevation requires updating
	// the shared osdCloud API signature and is tracked separately.
	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return fmt.Errorf("failed to create AWS config: %w", err)
	}
	awsClient := ec2.NewFromConfig(cfg)

	// Describe current volume
	describeOutput, err := awsClient.DescribeVolumes(context.TODO(), &ec2.DescribeVolumesInput{
		VolumeIds: []string{o.volumeID},
	})
	if err != nil {
		return fmt.Errorf("failed to describe volume: %w", err)
	}

	if len(describeOutput.Volumes) == 0 {
		return fmt.Errorf("volume %s not found", o.volumeID)
	}

	volume := describeOutput.Volumes[0]

	// Validate volume belongs to this cluster via strict ownership tag
	clusterOwned := false
	infraID := cluster.InfraID()
	expectedClusterTagKey := "kubernetes.io/cluster/" + infraID
	for _, tag := range volume.Tags {
		if tag.Key != nil && tag.Value != nil {
			if *tag.Key == expectedClusterTagKey && (*tag.Value == "owned" || *tag.Value == "shared") {
				clusterOwned = true
				break
			}
		}
	}
	if !clusterOwned {
		return fmt.Errorf("volume %s does not belong to cluster %s (infra ID: %s). Refusing to modify volume that may belong to another cluster", o.volumeID, cluster.ID(), infraID)
	}

	currentType := string(volume.VolumeType)
	currentSize := *volume.Size
	currentIops := int32(0)
	if volume.Iops != nil {
		currentIops = *volume.Iops
	}

	fmt.Printf("\nCurrent volume configuration:\n")
	fmt.Printf("  Type: %s\n", currentType)
	fmt.Printf("  Size: %d GB\n", currentSize)
	fmt.Printf("  IOPS: %d\n", currentIops)
	fmt.Printf("  State: %s\n", volume.State)

	// Check if already target type
	if currentType == o.targetType {
		return fmt.Errorf("volume is already type %s - no change needed", o.targetType)
	}

	// Check if modification is in progress
	modOutput, err := awsClient.DescribeVolumesModifications(context.TODO(), &ec2.DescribeVolumesModificationsInput{
		VolumeIds: []string{o.volumeID},
	})
	if err == nil && len(modOutput.VolumesModifications) > 0 {
		latestMod := modOutput.VolumesModifications[0]
		state := latestMod.ModificationState
		if state == types.VolumeModificationStateModifying || state == types.VolumeModificationStateOptimizing {
			return fmt.Errorf("volume modification already in progress (state: %s)", state)
		}
	}

	// Dry run mode
	if o.dryRun {
		fmt.Printf("\n✅ DRY RUN MODE - No changes will be made\n")
		fmt.Printf("\nWould modify volume:\n")
		fmt.Printf("  From: %s (%d IOPS)\n", currentType, currentIops)
		fmt.Printf("  To:   %s", o.targetType)
		if o.iops != nil {
			fmt.Printf(" (%d IOPS", *o.iops)
			if o.targetType == "gp3" && o.throughput != nil {
				fmt.Printf(", %d MB/s throughput", *o.throughput)
			}
			fmt.Printf(")")
		} else {
			fmt.Printf(" (keep current IOPS or use AWS defaults)")
		}
		fmt.Printf("\n")
		return nil
	}

	// Confirm modification
	fmt.Printf("\n⚠️  WARNING: This will modify the volume online.\n")
	fmt.Printf("The volume will remain attached and usable during the modification.\n")
	fmt.Printf("Modification typically takes 15-60 minutes, with optimization continuing in background.\n\n")

	// Build modify input
	modifyInput := &ec2.ModifyVolumeInput{
		VolumeId:   &o.volumeID,
		VolumeType: types.VolumeType(o.targetType),
	}

	// Add IOPS only if explicitly provided
	if o.iops != nil && (o.targetType == "io1" || o.targetType == "io2" || o.targetType == "gp3") {
		modifyInput.Iops = o.iops
	}

	// Add throughput only if explicitly provided for gp3
	if o.throughput != nil && o.targetType == "gp3" {
		modifyInput.Throughput = o.throughput
	}

	// Execute modification
	fmt.Printf("🔄 Initiating volume modification...\n")
	modifyOutput, err := awsClient.ModifyVolume(context.TODO(), modifyInput)
	if err != nil {
		return fmt.Errorf("failed to modify volume: %w", err)
	}

	modification := modifyOutput.VolumeModification
	fmt.Printf("\n✅ Volume modification initiated!\n")
	fmt.Printf("\nModification details:\n")
	fmt.Printf("  Modification State: %s\n", modification.ModificationState)
	fmt.Printf("  Original Type: %s\n", modification.OriginalVolumeType)
	fmt.Printf("  Target Type: %s\n", modification.TargetVolumeType)
	if modification.Progress != nil {
		fmt.Printf("  Progress: %d%%\n", *modification.Progress)
	}

	fmt.Printf("\nMonitoring modification progress...\n")
	fmt.Printf("(Press Ctrl+C to stop monitoring - modification will continue in background)\n\n")

	// Monitor progress
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	completed := false
	for i := 0; i < 120; i++ { // Monitor for up to 60 minutes
		<-ticker.C

		modOutput, err := awsClient.DescribeVolumesModifications(context.TODO(), &ec2.DescribeVolumesModificationsInput{
			VolumeIds: []string{o.volumeID},
		})
		if err != nil {
			fmt.Printf("Warning: failed to check modification status: %v\n", err)
			continue
		}

		if len(modOutput.VolumesModifications) == 0 {
			// Empty list may indicate eventual consistency; verify actual volume type
			volOut, derr := awsClient.DescribeVolumes(context.TODO(), &ec2.DescribeVolumesInput{
				VolumeIds: []string{o.volumeID},
			})
			if derr == nil && len(volOut.Volumes) > 0 && string(volOut.Volumes[0].VolumeType) == o.targetType {
				fmt.Printf("\n✅ Volume type is now %s. Modification completed.\n", o.targetType)
				completed = true
				break
			}
			continue
		}

		latestMod := modOutput.VolumesModifications[0]
		state := latestMod.ModificationState
		progress := int64(0)
		if latestMod.Progress != nil {
			progress = *latestMod.Progress
		}

		fmt.Printf("[%s] State: %s, Progress: %d%%\n", time.Now().Format("15:04:05"), state, progress)

		if state == types.VolumeModificationStateCompleted {
			fmt.Printf("\n✅ Volume modification COMPLETED!\n")
			completed = true
			break
		}

		if state == types.VolumeModificationStateFailed {
			fmt.Printf("\n❌ Volume modification FAILED!\n")
			if latestMod.StatusMessage != nil {
				fmt.Printf("Error: %s\n", *latestMod.StatusMessage)
			}
			return fmt.Errorf("volume modification failed")
		}
	}

	// Check if monitoring timed out without completion
	if !completed {
		fmt.Printf("\n⚠️  Monitoring timeout after 60 minutes.\n")
		fmt.Printf("Volume modification is still in progress and will continue in the background.\n")
		fmt.Printf("Check AWS Console (EC2 → Volumes → %s → Modifications tab) for current status.\n", o.volumeID)
		return fmt.Errorf("monitoring timeout: volume modification still in progress after 60 minutes")
	}

	// Final status (only shown if completed)
	describeOutput, err = awsClient.DescribeVolumes(context.TODO(), &ec2.DescribeVolumesInput{
		VolumeIds: []string{o.volumeID},
	})
	if err == nil && len(describeOutput.Volumes) > 0 {
		volume := describeOutput.Volumes[0]
		fmt.Printf("\nFinal volume configuration:\n")
		fmt.Printf("  Type: %s\n", volume.VolumeType)
		fmt.Printf("  Size: %d GB\n", *volume.Size)
		if volume.Iops != nil {
			fmt.Printf("  IOPS: %d\n", *volume.Iops)
		}
		if volume.Throughput != nil {
			fmt.Printf("  Throughput: %d MB/s\n", *volume.Throughput)
		}
		fmt.Printf("  State: %s\n", volume.State)
	}

	fmt.Printf("\n✅ Volume type change completed successfully!\n")
	return nil
}
