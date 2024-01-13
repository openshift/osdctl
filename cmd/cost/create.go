package cost

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	costExplorerTypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	organizationTypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
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

// Create Cost Category for OU given as argument for -ou flag
func createCostCategory(OUid *string, OU *organizationTypes.OrganizationalUnit, awsClient awsprovider.Client) error {
	//Gets all (not only immediate) accounts under the given OU
	accountsRecursiveResults, err := getAccountsRecursive(OU, awsClient)
	if err != nil {
		return err
	}

	accounts := make([]string, len(accountsRecursiveResults))
	for _, account := range accountsRecursiveResults {
		accounts = append(accounts, *account)
	}

	_, err = awsClient.CreateCostCategoryDefinition(&costexplorer.CreateCostCategoryDefinitionInput{
		Name:        OUid,
		RuleVersion: "CostCategoryExpression.v1",
		Rules: []costExplorerTypes.CostCategoryRule{
			{
				Rule: &costExplorerTypes.Expression{
					Dimensions: &costExplorerTypes.DimensionValues{
						Key:    "LINKED_ACCOUNT",
						Values: accounts,
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
