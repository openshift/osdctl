package cost

import (
	"fmt"

	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"

	"github.com/spf13/cobra"
)

// createCmd represents the create command
func newCmdCreate(streams genericclioptions.IOStreams) *cobra.Command {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a cost category for the given OU",
		Run: func(cmd *cobra.Command, args []string) {

			awsClient, err := opsCost.initAWSClients()
			cmdutil.CheckErr(err)

			//OU Flag
			OUid, err := cmd.Flags().GetString("ou")
			if err != nil {
				log.Fatalln("OU flag:", err)
			}

			//Get information regarding Organizational Unit
			OU := getOU(awsClient, OUid)

			if err := createCostCategory(&OUid, OU, awsClient); err != nil {
				log.Fatalf("Error creating cost category for %s: %v", OUid, err)
			}
		},
	}
	createCmd.Flags().String("ou", "", "get OU ID")
	if err := createCmd.MarkFlagRequired("ou"); err != nil {
		log.Fatalln("OU flag:", err)
	}

	return createCmd
}

//Store flag options for create command
type createOptions struct {
	ou string
	genericclioptions.IOStreams
}

//Create Cost Category for OU given as argument for -ou flag
func createCostCategory(OUid *string, OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) error {
	//Gets all (not only immediate) accounts under the given OU
	accounts, err := getAccountsRecursive(OU, awsClient)
	if err != nil {
		return err
	}

	accountIdList := []*string{}
	for i := range accounts {
		accountIdList = append(accountIdList, &accounts[i].accountId)

	}

	_, err = awsClient.CreateCostCategoryDefinition(&costexplorer.CreateCostCategoryDefinitionInput{
		Name:        OUid,
		RuleVersion: aws.String("CostCategoryExpression.v1"),
		Rules: []*costexplorer.CostCategoryRule{
			{
				Rule: &costexplorer.Expression{
					Dimensions: &costexplorer.DimensionValues{
						Key:    aws.String("LINKED_ACCOUNT"),
						Values: accountIdList,
					},
				},
				Value: OUid,
			},
		},
	})
	if err != nil {
		return err
	}

	fmt.Printf("Created Cost Category for %s (%s) OU\n", *OU.Name, *OU.Id)

	return nil
}
