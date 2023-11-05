/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cloudtrail

import (
	"fmt"

	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/spf13/cobra"
)

// HelloCmd represents the Hello command
var helloCmd = &cobra.Command{
	Use:   "Hello",
	Short: "Prints out Hello Cloundtrail to the console",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Hello Cloudtrail")
	},
}

type required struct {
	clusterID string
}

func test(i *required) {
	cfg, err := osdCloud.CreateAWSV2Config(i.clusterID)

	if err != nil {
		fmt.Println(err)
		return
	}

}

func init() {

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// HelloCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// HelloCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
