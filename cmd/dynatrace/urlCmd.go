package dynatrace

import (
	"fmt"

	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func newCmdURL() *cobra.Command {
	var clusterID string

	urlCmd := &cobra.Command{
		Use:               "url --cluster-id <cluster-identifier>",
		Short:             "Get the Dynatrace Tenant URL for a given MC or HCP cluster",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {

			hcpCluster, err := FetchClusterDetails(clusterID)
			if err != nil {
				cmdutil.CheckErr(err)
			}
			fmt.Println("Dynatrace Environment URL - ", hcpCluster.DynatraceURL)
		},
	}

	urlCmd.Flags().StringVarP(&clusterID, "cluster-id", "C", "", "ID of the cluster")
	_ = urlCmd.MarkFlagRequired("cluster-id")

	return urlCmd
}
