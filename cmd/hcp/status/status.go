package status

import (
	"fmt"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type statusOptions struct {
	clusterID string
}

// NewCmdStatus creates and returns the status command.
func NewCmdStatus() *cobra.Command {
	opts := &statusOptions{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show HCP cluster health status from OCM live resources",
		Long: `Display a comprehensive health overview of a ROSA HCP cluster using
data from the OCM live resources endpoint. Shows ManifestWork sync status,
HostedCluster conditions, certificate status, and NodePool health.`,
		Example: `  # Show HCP cluster status
  osdctl hcp status --cluster-id ${CLUSTER_ID}`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run()
		},
	}

	cmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Cluster name, ID, or external ID")
	_ = cmd.MarkFlagRequired("cluster-id")

	return cmd
}

func (o *statusOptions) run() error {
	conn, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("failed to create OCM connection: %w", err)
	}
	defer conn.Close()

	cluster, err := utils.GetCluster(conn, o.clusterID)
	if err != nil {
		return fmt.Errorf("failed to find cluster: %w", err)
	}

	if !cluster.Hypershift().Enabled() {
		return fmt.Errorf("cluster %q is not an HCP cluster", o.clusterID)
	}

	liveResponse, err := conn.ClustersMgmt().V1().Clusters().Cluster(cluster.ID()).Resources().Live().Get().Send()
	if err != nil {
		return fmt.Errorf("failed to get live resources: %w", err)
	}

	resources := liveResponse.Body().Resources()
	if len(resources) == 0 {
		return fmt.Errorf("no live resources found for cluster %s", cluster.ID())
	}

	status, err := parseLiveResources(resources, cluster.ID())
	if err != nil {
		return fmt.Errorf("failed to parse live resources: %w", err)
	}

	status.ClusterID = cluster.ExternalID()
	status.ClusterName = cluster.Name()
	status.ClusterState = string(cluster.State())

	printStatus(status)

	return nil
}
