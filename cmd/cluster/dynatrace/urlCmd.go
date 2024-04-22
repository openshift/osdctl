package dynatrace

import (
	"fmt"

	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func newCmdURL() *cobra.Command {
	urlCmd := &cobra.Command{
		Use:               "url CLUSTER_ID",
		Short:             "Get the Dyntrace Tenant URL for a given MC or HCP cluster",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			_, _, dtURL, err := fetchClusterDetails(args[0])
			if err != nil {
				cmdutil.CheckErr(err)
			}
			fmt.Println("Dynatrace Environment URL - ", dtURL)
		},
	}
	return urlCmd
}
