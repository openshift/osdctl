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
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
	"log"

	"github.com/spf13/cobra"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		//OU Flag
		OUid, err := cmd.Flags().GetString("ou")
		if err != nil {
			log.Fatalln("OU flag:", err)
		}

		//Get Organizational Unit
		OU := organizations.OrganizationalUnit{Id: aws.String(OUid)}
		//Initialize AWS clients
		org, ce := initAWSClients()

		createCostCategory(&OUid, &OU, org, ce)
	},
}

func init() {
	createCmd.Flags().String("ou", "", "get OU ID")
	err := createCmd.MarkFlagRequired("ou")
	if err != nil {
		log.Fatalln("OU flag:", err)
	}
}

//Create Cost Category for OU given as argument for -ccc flag
func createCostCategory(OUid *string, OU *organizations.OrganizationalUnit, org *organizations.Organizations, ce *costexplorer.CostExplorer) {
	accounts := getAccountsRecursive(OU, org)

	_, err := ce.CreateCostCategoryDefinition(&costexplorer.CreateCostCategoryDefinitionInput{
		Name: OUid,
		RuleVersion: aws.String("CostCategoryExpression.v1"),
		Rules: []*costexplorer.CostCategoryRule{
			{
				Rule: &costexplorer.Expression{
					Dimensions: &costexplorer.DimensionValues{
						Key: aws.String("LINKED_ACCOUNT"),
						Values: accounts,
					},
				},
				Value: OUid,
			},
		},
	})
	if err != nil {
		log.Fatalln("Error creating cost category:",err)
	}

	fmt.Println("Created Cost Category for", *OUid)
}