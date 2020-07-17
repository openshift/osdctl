package cost

import (
	"fmt"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
	"log"

	"github.com/spf13/cobra"
)

// createCmd represents the create command
func newCmdCreate(streams genericclioptions.IOStreams) *cobra.Command {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a cost category for the given OU",
		Run: func(cmd *cobra.Command, args []string) {

			cmdutil.CheckErr(opsCost.complete(cmd, args))
			org, ce, err := opsCost.initAWSClients()
			cmdutil.CheckErr(err)

			//OU Flag
			OUid, err := cmd.Flags().GetString("ou")
			if err != nil {
				log.Fatalln("OU flag:", err)
			}

			//Get Organizational Unit
			OU := organizations.OrganizationalUnit{Id: aws.String(OUid)}

			createCostCategory(&OUid, &OU, org, ce)
		},
	}
	createCmd.Flags().String("ou", "", "get OU ID")
	err := createCmd.MarkFlagRequired("ou")
	if err != nil {
		log.Fatalln("OU flag:", err)
	}

	return createCmd
}

//Store flag options for get command
type createOptions struct {
	ou string

	genericclioptions.IOStreams
}

//Create Cost Category for OU given as argument for -ccc flag
func createCostCategory(OUid *string, OU *organizations.OrganizationalUnit, org awsprovider.OrganizationsClient, ce awsprovider.CostExplorerClient) {
	accounts := getAccountsRecursive(OU, org)

	_, err := ce.CreateCostCategoryDefinition(&costexplorer.CreateCostCategoryDefinitionInput{
		Name:        OUid,
		RuleVersion: aws.String("CostCategoryExpression.v1"),
		Rules: []*costexplorer.CostCategoryRule{
			{
				Rule: &costexplorer.Expression{
					Dimensions: &costexplorer.DimensionValues{
						Key:    aws.String("LINKED_ACCOUNT"),
						Values: accounts,
					},
				},
				Value: OUid,
			},
		},
	})
	if err != nil {
		log.Fatalln("Error creating cost category:", err)
	}

	fmt.Println("Created Cost Category for", *OUid)
}
