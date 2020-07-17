package cost

import (
	"fmt"
	"github.com/openshift/osd-utils-cli/cmd/common"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"log"
)

// costCmd represents the cost command
func NewCmdCost(streams genericclioptions.IOStreams) *cobra.Command {
	opsCost = newCostOptions(streams)
	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "Cost Management related utilities",
		Long: `The cost command allows for cost management on the AWS platform (other 
platforms may be added in the future)`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
		},
	}

	//Set flags
	costCmd.PersistentFlags().StringVarP(&opsCost.accessKeyID, "aws-access-key-id", "a", "", "AWS Access Key ID")
	costCmd.PersistentFlags().StringVarP(&opsCost.secretAccessKey, "aws-secret-access-key", "x", "", "AWS Secret Access Key")
	costCmd.PersistentFlags().StringVarP(&opsCost.profile, "aws-profile", "p", "", "specify AWS profile")
	costCmd.PersistentFlags().StringVarP(&opsCost.configFile, "aws-config", "c", "", "specify AWS config file path")
	costCmd.PersistentFlags().StringVarP(&opsCost.region, "aws-region", "g", common.DefaultRegion, "specify AWS region")

	//Add commands
	costCmd.AddCommand(newCmdGet(streams))
	costCmd.AddCommand(newCmdReconcile(streams))
	costCmd.AddCommand(newCmdCreate(streams))
	costCmd.AddCommand(newCmdList(streams))

	return costCmd
}

var opsCost *costOptions

// costOptions defines the struct for running Cost command
type costOptions struct {
	// AWS config
	accessKeyID     string
	secretAccessKey string
	configFile      string
	profile         string
	region          string

	genericclioptions.IOStreams
}

func newCostOptions(streams genericclioptions.IOStreams) *costOptions {
	return &costOptions{
		IOStreams: streams,
	}
}

func (opsCost *costOptions) initAWSClients() (awsprovider.OrganizationsClient, awsprovider.CostExplorerClient, error) {
	//Initialize AWS clients
	var (
		awsClient awsprovider.Client
		err       error
	)

	if opsCost.accessKeyID == "" && opsCost.secretAccessKey == "" {
		awsClient, err = awsprovider.NewAwsClient(opsCost.profile, opsCost.region, opsCost.configFile)
	} else {
		awsClient, err = awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
			AccessKeyID:     opsCost.accessKeyID,
			SecretAccessKey: opsCost.secretAccessKey,
			Region:          opsCost.region,
		})
	}

	if err != nil {
		log.Fatalln("Error getting AWS clients:", err)
	}

	return awsClient.GetOrg(), awsClient.GetCE(), err
}

func (opsCost *costOptions) complete(cmd *cobra.Command, _ []string) error {
	if opsCost.accessKeyID == "" && opsCost.secretAccessKey == "" {
		fmt.Fprintln(opsCost.Out, "AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are not provided, reading credentials from config file or env vars.")
	} else if opsCost.accessKeyID == "" || opsCost.secretAccessKey == "" {
		return cmdutil.UsageErrorf(cmd, "The flag aws-access-key-id and aws-secret-access-key should be set or not set at the same time")
	}

	return nil
}
