package cost

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/deckarep/golang-set"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"log"
)

// reconcileCmd represents the reconcile command
func newCmdReconcile(streams genericclioptions.IOStreams) *cobra.Command {
	var reconcileCmd = &cobra.Command{
		Use:   "reconcile",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Run: func(cmd *cobra.Command, args []string) {
			//Set OU as Openshift: reconciliateCostCategories will then create cost categories for v4 and its child OUs
			OU := organizations.OrganizationalUnit{Id: aws.String("ou-0wd6-3q0027q7")}

			//Initialize AWS clients
			//org, ce := initAWSClients()

			reconciliateCostCategories(&OU, org, ce)
		},
	}

	return reconcileCmd
}

//Checks if there's a cost category for every OU. If not, creates the missing cost category. This should be ran every 24 hours.
func reconciliateCostCategories(OU *organizations.OrganizationalUnit, org *organizations.Organizations, ce *costexplorer.CostExplorer) {
	costCategoryCreated := false

	OUs := getOUsRecursive(OU, org)
	costCategoriesSet := mapset.NewSet()

	existingCostCategories, err := ce.ListCostCategoryDefinitions(&costexplorer.ListCostCategoryDefinitionsInput{})

	//Populate costCategoriesSet with cost categories by looping until existingCostCategories.NextToken is null
	for {
		if err != nil {
			log.Fatalln("Error listing cost categories:", err)
		}

		//Loop through and add to costCategoriesSet. Set makes lookup easier
		for _, costCategory := range existingCostCategories.CostCategoryReferences {
			costCategoriesSet.Add(*costCategory.Name)
		}

		fmt.Println("hello from reconcile loop")
		fmt.Println("length cc:", len(existingCostCategories.CostCategoryReferences))

		if existingCostCategories.NextToken == nil {
			break
		}

		//Get accounts
		existingCostCategories, err = ce.ListCostCategoryDefinitions(&costexplorer.ListCostCategoryDefinitionsInput{})
	}

	//Loop through every OU under OpenShift and create cost category if missing
	for _, OU := range OUs {
		if !costCategoriesSet.Contains(*OU.Id) {
			createCostCategory(OU.Id, OU, org, ce)
			costCategoryCreated = true
		}
	}

	if !costCategoryCreated {
		fmt.Println("Cost categories are up-to-date. No cost category created.")
	}
}
