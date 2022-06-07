package cost

import (
	"log"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// costCmd represents the cost command
func NewCmdCost(streams genericclioptions.IOStreams, kubeCli k8sclient.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	opsCost = newCostOptions(streams, kubeCli)
	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "Cost Management related utilities",
		Long: `The cost command allows for cost management on the AWS platform (other 
platforms may be added in the future)`,
	}

	//Set flags
	costCmd.PersistentFlags().StringVarP(&opsCost.accessKeyID, "aws-access-key-id", "a", "", "AWS Access Key ID")
	costCmd.PersistentFlags().StringVarP(&opsCost.secretAccessKey, "aws-secret-access-key", "x", "", "AWS Secret Access Key")
	costCmd.PersistentFlags().StringVarP(&opsCost.profile, "aws-profile", "p", "", "specify AWS profile")
	costCmd.PersistentFlags().StringVarP(&opsCost.configFile, "aws-config", "c", "", "specify AWS config file path")
	costCmd.PersistentFlags().StringVarP(&opsCost.region, "aws-region", "g", common.DefaultRegion, "specify AWS region")

	//Add commands
	costCmd.AddCommand(newCmdGet(streams, globalOpts))
	costCmd.AddCommand(newCmdReconcile(streams))
	costCmd.AddCommand(newCmdCreate(streams))
	costCmd.AddCommand(newCmdList(streams, kubeCli, globalOpts))

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

	kubeCli k8sclient.Client
}

func newCostOptions(streams genericclioptions.IOStreams, kubeCli k8sclient.Client) *costOptions {
	return &costOptions{
		IOStreams: streams,
		kubeCli:   kubeCli,
	}
}

//Initiate AWS clients for Organizations and Cost Explorer services using, if given, credentials in flags, else, credentials in the environment
func (opsCost *costOptions) initAWSClients() (awsprovider.Client, error) {
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
		return nil, err
	}

	return awsClient, err
}

//Gets information regarding Organizational Unit
func getOU(org awsprovider.Client, OUid string) *organizations.OrganizationalUnit {
	// if we're passed the root OU, skip querying and pass the ou id back
	rootRegex, _ := regexp.Compile("r-[a-z]{4}")
	if rootRegex.MatchString(OUid) {
		ou := &organizations.OrganizationalUnit{
			Id:   aws.String(OUid),
			Name: aws.String("root"),
		}
		return ou
	}

	// Otherwise get the OU from AWS
	result, err := org.DescribeOrganizationalUnit(&organizations.DescribeOrganizationalUnitInput{
		OrganizationalUnitId: aws.String(OUid),
	})
	if err != nil {
		log.Fatalln("Cannot get Organizational Unit:", err)
	}

	return result.OrganizationalUnit
}
