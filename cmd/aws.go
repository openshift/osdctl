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
package cmd

import (
	"github.com/spf13/cobra"

	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
)

// awsCmd represents the aws command
var awsCmd = &cobra.Command{
	Use:   "aws",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("aws called")
	},
}

func init() {
	awsCmd.AddCommand(getCmd)
}

func initAWSClients() (*organizations.Organizations, *costexplorer.CostExplorer) {

	//Initialize a session with the osd-staging-1 profile or any user that has access to the desired info
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: "osd-staging-1",
	})
	if err != nil {
		log.Fatalln("Unable to generate session:",err)
	}

	//Create Cost Explorer client
	ce := costexplorer.New(sess)
	//Create Organizations client
	org := organizations.New(sess)

	return org, ce
}