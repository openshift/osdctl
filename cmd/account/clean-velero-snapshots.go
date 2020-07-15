package account

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/openshift/osd-utils-cli/cmd/common"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

const (
	cleanVeleroSnapshotsUsage = "The flag aws-access-key-id and aws-secret-access-key should be set or not set at the same time"
)

// newCmdCleanVeleroSnapshots implements the command which cleans
// up S3 buckets whose name start with managed-velero
func newCmdCleanVeleroSnapshots(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newCleanVeleroSnapshotsOptions(streams)
	cleanCmd := &cobra.Command{
		Use:               "clean-velero-snapshots",
		Short:             "Cleans up S3 buckets whose name start with managed-velero",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	cleanCmd.Flags().StringVarP(&ops.accessKeyID, "aws-access-key-id", "a", "", "AWS Access Key ID")
	cleanCmd.Flags().StringVarP(&ops.secretAccessKey, "aws-secret-access-key", "x", "", "AWS Secret Access Key")
	cleanCmd.Flags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile")
	cleanCmd.Flags().StringVarP(&ops.configFile, "aws-config", "c", "", "specify AWS config file path")
	cleanCmd.Flags().StringVarP(&ops.region, "aws-region", "r", common.DefaultRegion, "specify AWS region")

	return cleanCmd
}

// cleanVeleroSnapshotsOptions defines the struct for running Console command
type cleanVeleroSnapshotsOptions struct {
	// AWS config
	accessKeyID     string
	secretAccessKey string
	configFile      string
	profile         string
	region          string

	genericclioptions.IOStreams
}

func newCleanVeleroSnapshotsOptions(streams genericclioptions.IOStreams) *cleanVeleroSnapshotsOptions {
	return &cleanVeleroSnapshotsOptions{
		IOStreams: streams,
	}
}

func (o *cleanVeleroSnapshotsOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.accessKeyID == "" && o.secretAccessKey == "" {
		fmt.Fprintln(o.Out, "AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are not provided, reading credentials from config file or env vars.")
	} else if o.accessKeyID == "" || o.secretAccessKey == "" {
		return cmdutil.UsageErrorf(cmd, cleanVeleroSnapshotsUsage)
	}

	return nil
}

func (o *cleanVeleroSnapshotsOptions) run() error {
	var (
		awsClient awsprovider.Client
		err       error
	)
	if o.accessKeyID == "" && o.secretAccessKey == "" {
		awsClient, err = awsprovider.NewAwsClient(o.profile, o.region, o.configFile)
	} else {
		awsClient, err = awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
			AccessKeyID:     o.accessKeyID,
			SecretAccessKey: o.secretAccessKey,
			Region:          o.region,
		})
	}

	if err != nil {
		return err
	}

	return awsprovider.DeleteS3BucketsWithPrefix(awsClient, "managed-velero")
}
