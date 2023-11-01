/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cloudtrail

import (
	"github.com/spf13/cobra"
)

// CloudtrailCmd represents the cloudtrail command
var CloudtrailCmd = &cobra.Command{
	Use:   "cloudtrail",
	Short: "cloudtrail is a palette that contains cloudtrail commands",
	Long:  `cloudtrail is a palette that contains cloudtrail commands`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	CloudtrailCmd.AddCommand(helloCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// cloudtrailCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// cloudtrailCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
