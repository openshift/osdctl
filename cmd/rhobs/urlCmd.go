package rhobs

import (
	"fmt"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
)

var cmdCellOptions = struct {
	clusterId string
}{}

func newCmdCell() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cell --cluster-id <cluster-identifier>",
		Short: "Get the RHOBS cell for a given MC or HCP cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceErrors = true

			if cmdCellOptions.clusterId == "" {
				var err error

				cmdCellOptions.clusterId, err = k8s.GetCurrentCluster()
				if err != nil {
					return fmt.Errorf("failed to retrieve ID for current cluster: %v", err)
				}
			}

			cmd.SilenceUsage = true

			return retrieveCell()
		},
	}

	cmd.Flags().StringVarP(&cmdCellOptions.clusterId, "cluster-id", "C", "", "Name or Internal ID of the cluster (defaults to current cluster context)")

	return cmd
}

func retrieveCell() error {
	rhobsCell, err := GetRhobsCell(cmdCellOptions.clusterId)
	if err != nil {
		return fmt.Errorf("failed to retrieve RHOBS cell: %v", err)
	}

	fmt.Println("RHOBS cell -", rhobsCell)

	return nil
}
