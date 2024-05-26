package cloudtrail

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	ctUtil "github.com/openshift/osdctl/cmd/cloudtrail/pkg"
	ctAws "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	search = ".*Client.UnauthorizedOperation.*"
)

type permissonOptions struct {
	clusterID string
	startTime string
}

func newCmdPermissionDenied() *cobra.Command {
	ops := &permissonOptions{}
	permissionDeniedCmd := &cobra.Command{
		Use:   "permission-denied",
		Short: "Prints cloudtrail permission-denied events to console.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run()
		},
	}
	permissionDeniedCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	permissionDeniedCmd.Flags().StringVarP(&ops.startTime, "since", "", "1h", "Specifies that only events that occur within the specified time are returned.Defaults to 1h.Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")
	permissionDeniedCmd.MarkFlagRequired("cluster-id")
	return permissionDeniedCmd
}

func (p *permissonOptions) run() error {
	err := utils.IsValidClusterKey(p.clusterID)
	if err != nil {
		return err
	}
	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to ocm: %w", err)
	}
	defer connection.Close()

	cluster, err := utils.GetClusterAnyStatus(connection, p.clusterID)
	if err != nil {
		return err
	}
	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("[ERROR] this command is only available for AWS clusters")
	}

	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	cloudtrailClient := cloudtrail.NewFromConfig(cfg)
	lookupEvents, err := ctAws.GetEvents(time.Now(), cloudtrailClient)
	if err != nil {
		return err
	}
	permissionDeniedEvent := ctUtil.Filters[2](lookupEvents, search)
	ctUtil.PrintEvents(permissionDeniedEvent, false, false)

	return err

}
