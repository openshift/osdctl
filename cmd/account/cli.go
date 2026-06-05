package account

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// newCmdCli implements the Cli command which generates temporary STS cli credentials for the specified account cr
func newCmdCli() *cobra.Command {
	ops := &cliOptions{}
	cliCmd := &cobra.Command{
		Use:               "cli",
		Short:             "Generate temporary AWS CLI credentials on demand",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	cliCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	cliCmd.Flags().StringVarP(&ops.awsAccountID, "accountId", "i", "", "AWS Account ID")
	cliCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	cliCmd.Flags().StringVarP(&ops.output, "output", "o", "env", "Output type (env, json)")
	cliCmd.Flags().StringVarP(&ops.region, "region", "r", "", "Region")

	return cliCmd
}

// cliOptions defines the struct for running the cli command
type cliOptions struct {
	output  string
	verbose bool

	awsAccountID string
	awsProfile   string
	region       string
}

func (o *cliOptions) complete(cmd *cobra.Command) error {
	if o.awsAccountID == "" {
		return fmt.Errorf("please specify account number with '-i'")
	}

	if o.region == "" {
		o.region = "us-east-1"
	}

	return nil
}

func (o *cliOptions) run() error {

	var err error

	// Build the base AWS client using the provide credentials (profile or env vars)
	awsClient, err := aws.NewAwsClient(o.awsProfile, o.region, "")
	if err != nil {
		fmt.Printf("Could not build AWS Client: %s\n", err)
		return err
	}

	// Get the right partition for the final ARN
	partition, err := aws.GetAwsPartition(awsClient)
	if err != nil {
		return err
	}

	// Generate a session name using the SRE's kerberos ID
	sessionName, err := osdCloud.GenerateRoleSessionName(awsClient)
	if err != nil {
		fmt.Printf("Could not generate Session Name: %s\n", err)
		return err
	}

	var assumedRoleCreds *types.Credentials
	// If the cluster is non-CCS, or an AWS Account ID was provided with -i, try and use OrganizationAccountAccessRole
	assumedRoleCreds, err = osdCloud.GenerateOrganizationAccountAccessCredentials(awsClient, o.awsAccountID, sessionName, partition)
	if err != nil {
		fmt.Printf("Could not build AWS Client for OrganizationAccountAccessRole: %s\n", err)
		return err
	}

	switch o.output {
	case "json":
		out := struct {
			AccessKeyId     string `json:"AccessKeyId"`
			Expiration      string `json:"Expiration"`
			SecretAccessKey string `json:"SecretAccessKey"`
			SessionToken    string `json:"SessionToken"`
			Region          string `json:"Region"`
		}{
			AccessKeyId:     *assumedRoleCreds.AccessKeyId,
			Expiration:      assumedRoleCreds.Expiration.String(),
			SecretAccessKey: *assumedRoleCreds.SecretAccessKey,
			SessionToken:    *assumedRoleCreds.SessionToken,
			Region:          o.region,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil { //nolint:gosec // G117 false positive — intentionally outputting AWS credentials
			return err
		}
	default:
		fmt.Printf("export AWS_ACCESS_KEY_ID=%s\n", *assumedRoleCreds.AccessKeyId)
		fmt.Printf("export AWS_SECRET_ACCESS_KEY=%s\n", *assumedRoleCreds.SecretAccessKey)
		fmt.Printf("export AWS_SESSION_TOKEN=%s\n", *assumedRoleCreds.SessionToken)
		fmt.Printf("export AWS_DEFAULT_REGION=%s\n", o.region)
		fmt.Printf("export AWS_REGION=%s\n", o.region)
	}

	return nil
}
