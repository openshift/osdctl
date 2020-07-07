/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cost

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/deckarep/golang-set"
	"github.com/spf13/cobra"
	"log"
)

// reconcileCmd represents the reconcile command
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
		org, ce := initAWSClients()

		reconciliateCostCategories(&OU, org, ce)
	},
}

//Check if there's a cost category for every OU. If not, create the missing cost category. This should be ran every 24 hours. reconciliateCostCategories should be called with
func reconciliateCostCategories(OU *organizations.OrganizationalUnit, org *organizations.Organizations, ce *costexplorer.CostExplorer) {
	OUs := getOUsRecursive(OU, org)
	costCategoriesSet := mapset.NewSet()

	existingCostCategories, err := ce.ListCostCategoryDefinitions(&costexplorer.ListCostCategoryDefinitionsInput{})
	if err != nil {
		log.Fatalln("Error listing cost categories:",err)
	}
	//Loop through and add cost categories to set. Makes lookup easier
	for _, costCategory := range existingCostCategories.CostCategoryReferences {
		costCategoriesSet.Add(*costCategory.Name)
	}

	//Loop through every OU under root
	for _, OU := range OUs {
		if !costCategoriesSet.Contains(*OU.Id) {
			createCostCategory(OU.Id, OU, org, ce)
		}
	}
}

