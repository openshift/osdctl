package cost

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/deckarep/golang-set"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"log"
)

// reconcileCmd represents the reconcile command
func newCmdReconcile(streams genericclioptions.IOStreams) *cobra.Command {
	reconcileCmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Checks if there's a cost category for every OU. If an OU is missing a cost category, creates the cost category",
		Run: func(cmd *cobra.Command, args []string) {

			awsClient, err := opsCost.initAWSClients()
			cmdutil.CheckErr(err)

			//Get flags
			OUid, err := cmd.Flags().GetString("ou")
			if err != nil {
				log.Fatalln("OU flag:", err)
			}

			//Get information regarding Organizational Unit
			OU := getOU(awsClient, OUid)

			if err := reconcileCostCategories(OU, awsClient); err != nil {
				log.Fatalln("Error reconciling cost categories:", err)
			}
		},
	}
	reconcileCmd.Flags().String("ou", "", "get OU ID")
	if err := reconcileCmd.MarkFlagRequired("ou"); err != nil {
		log.Fatalln("OU flag:", err)
	}

	return reconcileCmd
}

//Checks if there's a cost category for every OU. If not, creates the missing cost category. This should be ran every 24 hours.
func reconcileCostCategories(OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) error {
	costCategoryCreated := false
	costCategoriesSet := mapset.NewSet()

	var nextToken *string

	//Populate costCategoriesSet with cost categories by looping until existingCostCategories.NextToken is null
	for {
		existingCostCategories, err := awsClient.ListCostCategoryDefinitions(&costexplorer.ListCostCategoryDefinitionsInput{
			NextToken: nextToken,
		})

		if err != nil {
			return err
		}

		//Loop through and add to costCategoriesSet. Set makes lookup easier
		for _, costCategory := range existingCostCategories.CostCategoryReferences {
			costCategoriesSet.Add(*costCategory.Name)
		}

		if existingCostCategories.NextToken == nil {
			break
		}
		nextToken = existingCostCategories.NextToken //If NextToken != nil, keep looping
	}

	OUs, err := getOUsRecursive(OU, awsClient)
	if err != nil {
		return err
	}
	//Loop through every OU under OpenShift and create cost category if missing
	for _, OU := range OUs {
		if !costCategoriesSet.Contains(*OU.Id) {
			if err := createCostCategory(OU.Id, OU, awsClient); err != nil {
				return err
			}
			costCategoryCreated = true
		}
	}

	if !costCategoryCreated {
		fmt.Println("Cost categories are up-to-date. No cost category created.")
	}

	return nil
}
