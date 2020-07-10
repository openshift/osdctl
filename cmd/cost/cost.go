package cost

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/openshift/osd-utils-cli/cmd/common"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"log"
)

// costCmd represents the cost command
func NewCmdCost(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newCostOptions(streams)
	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "Cost Management related utilities",
		Long: `The cost command allows for cost management on the AWS platform (other 
platforms may be added in the future)`,
		Run: func(cmd *cobra.Command, args []string) {

			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.initAWSClients())

			//ops.initAWSClients()
		},
	}

	//Set flags
	costCmd.PersistentFlags().StringVarP(&ops.accessKeyID, "aws-access-key-id", "a", "", "AWS Access Key ID")
	costCmd.PersistentFlags().StringVarP(&ops.secretAccessKey, "aws-secret-access-key", "x", "", "AWS Secret Access Key")
	costCmd.PersistentFlags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile")
	costCmd.PersistentFlags().StringVarP(&ops.configFile, "aws-config", "c", "", "specify AWS config file path")
	costCmd.PersistentFlags().StringVarP(&ops.region, "aws-region", "z", common.DefaultRegion, "specify AWS region")

	//costCmd.AddCommand(awsCmd)
	costCmd.AddCommand(newCmdGet(streams))
	costCmd.AddCommand(newCmdReconcile(streams))

	return costCmd
}

var org *organizations.Organizations
var ce *costexplorer.CostExplorer

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

func (ops *costOptions) initAWSClients() error {
	//ops := newCostOptions(streams)

	//Initialize AWS clients
	var (
		awsClient awsprovider.Client
		err       error
	)

	if ops.accessKeyID == "" && ops.secretAccessKey == "" {
		awsClient, err = awsprovider.NewAwsClient(ops.profile, ops.region, ops.configFile)
	} else {
		awsClient, err = awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
			AwsIDKey:     ops.accessKeyID,
			AwsAccessKey: ops.secretAccessKey,
			AwsRegion:    ops.region,
		})
	}

	if err != nil {
		log.Fatalln("Error getting AWS clients:", err)
	}

	org = awsClient.GetOrg()
	ce = awsClient.GetCE()

	return err
}

func (ops *costOptions) complete(cmd *cobra.Command, _ []string) error {
	if ops.accessKeyID == "" && ops.secretAccessKey == "" {
		fmt.Fprintln(ops.Out, "AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are not provided, reading credentials from config file or env vars.")
	} else if ops.accessKeyID == "" || ops.secretAccessKey == "" {
		return cmdutil.UsageErrorf(cmd, "The flag aws-access-key-id and aws-secret-access-key should be set or not set at the same time")
	}

	return nil
}

//func (ops *costOptions) run() error {
//	var (
//		awsClient awsprovider.Client
//		err       error
//	)
//	if ops.accessKeyID == "" && ops.secretAccessKey == "" {
//		awsClient, err = awsprovider.NewAwsClient(ops.profile, ops.region, ops.configFile)
//	} else {
//		awsClient, err = awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
//			AwsIDKey:     ops.accessKeyID,
//			AwsAccessKey: ops.secretAccessKey,
//			AwsRegion:    ops.region,
//		})
//	}
//
//	if err != nil {
//		return err
//	}
//}


