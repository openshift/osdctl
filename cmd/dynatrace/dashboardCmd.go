package dynatrace

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	dashboardName string
	clusterId     string
)

// openBrowser attempts to open the specified URL in the default system browser.
// Supports Linux (xdg-open), Windows (rundll32), and macOS (open).
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	case "darwin":
		cmd = "open"
	default:
		return fmt.Errorf("unsupported platform")
	}

	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

func newCmdDashboard() *cobra.Command {
	urlCmd := &cobra.Command{
		Use:               "dashboard --cluster-id CLUSTER_ID",
		Aliases:           []string{"dash"},
		Short:             "Get the Dynatrace Cluster Overview Dashboard for a given MC or HCP cluster",
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

			// Only try to open browser if not in a container environment
			if !utils.IsContainerEnvironment() {
				// Open the dashboard in the default browser
				fmt.Println("\nOpening dashboard in your browser...")
				if err := openBrowser(dashUrl); err != nil {
					fmt.Printf("Could not open browser automatically: %s\n", err)
				}
			} else {
				fmt.Println("\nRunning in container mode - open the URL above in your host browser.")
			}
		},
	}

	urlCmd.Flags().StringVar(&dashboardName, "dash", "Central ROSA HCP Dashboard", "Name of the dashboard you wish to find")
	urlCmd.Flags().StringVarP(&clusterId, "cluster-id", "C", "", "Provide the id of the cluster")
	_ = urlCmd.MarkFlagRequired("cluster-id")

	return urlCmd
}
