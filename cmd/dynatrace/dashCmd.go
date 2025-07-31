package dynatrace

import (
	"fmt"

	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	dashboardName string
	clusterId     string
)

func newCmdDashboard() *cobra.Command {
	urlCmd := &cobra.Command{
		Use:               "dashboard --cluster-id CLUSTER_ID",
		Aliases:           []string{"dash"},
		Short:             "Get the Dyntrace Cluster Overview Dashboard for a given MC or HCP cluster",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			// We need the Dynatrace URL
			hcpCluster, err := FetchClusterDetails(clusterId)
			if err != nil {
				cmdutil.CheckErr(err)
			}

			// Get credentials
			accessToken, err := getDocumentAccessToken()
			if err != nil {
				fmt.Printf("Could not get access token %s\n", err)
				return
			}

			// Search for the dashboard
			id, err := getDocumentIDByNameAndType(hcpCluster.DynatraceURL, accessToken, dashboardName, DTDashboardType)
			if err != nil {
				fmt.Printf("Could not find dashboard named '%s': %s\n", dashboardName, err)
				return
			}

			// Tell the user
			dashUrl := hcpCluster.DynatraceURL + "ui/apps/dynatrace.dashboards/dashboard/" + id + "#vfilter__id=" + hcpCluster.externalID
			fmt.Printf("\n\nDashboard URL:\n  %s\n", dashUrl)
		},
	}

	urlCmd.Flags().StringVar(&dashboardName, "dash", "Central ROSA HCP Dashboard", "Name of the dashboard you wish to find")
	urlCmd.Flags().StringVarP(&clusterId, "cluster-id", "C", "", "Provide the id of the cluster")
	_ = urlCmd.MarkFlagRequired("cluster-id")

	return urlCmd
}
