package jira

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Cmd = &cobra.Command{
	Use:   "jira",
	Short: "Provides a set of commands for interacting with Jira",
	Args:  cobra.NoArgs,
}

func init() {
	Cmd.AddCommand(quickTaskCmd)
	Cmd.AddCommand(createHandoverAnnouncmentCmd)

	createHandoverAnnouncmentCmd.Flags().String("summary", "", "Enter Summary/Title for the Announcment")
	createHandoverAnnouncmentCmd.Flags().String("description", "", "Enter Description for the Announcment")
	createHandoverAnnouncmentCmd.Flags().String("products", "", "Comma-separated list of products (e.g. 'Product A,Product B')")
	createHandoverAnnouncmentCmd.Flags().String("customer", "", "Customer name")
	createHandoverAnnouncmentCmd.Flags().String("cluster", "", "Cluster ID")
	createHandoverAnnouncmentCmd.Flags().String("version", "", "Affected Openshift Version (e.g 4.16 or 4.15.32)")

	flags := []string{"summary", "description", "products", "customer", "cluster", "version"}
	for _, flag := range flags {
		if err := viper.BindPFlag(flag, createHandoverAnnouncmentCmd.Flags().Lookup(flag)); err != nil {
			log.Printf("Failed to bind flag '%s': %v", flag, err)
		}
	}
}
