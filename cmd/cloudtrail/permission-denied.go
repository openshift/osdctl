package cloudtrail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ctUtil "github.com/openshift/osdctl/cmd/cloudtrail/pkg"
	ctAws "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	errorCode = ".*Client.UnauthorizedOperation.*"
)

type permissonOptions struct {
	clusterID      string
	startTime      string
	printEventUrl  bool
	printRawEvents bool
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
	permissionDeniedCmd.Flags().BoolVarP(&ops.printEventUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	permissionDeniedCmd.Flags().BoolVarP(&ops.printRawEvents, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
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

	startTime, err := ctUtil.ParseDurationToUTC(p.startTime)
	if err != nil {
		return err
	}

	fetchFilterPrint := func(client cloudtrail.Client, startTime time.Time, p *permissonOptions) error {
		lookupOutput, err := ctAws.GetEvents(startTime, &client)
		if err != nil {
			return err
		}

		permissionDeniedEvent := ctUtil.Filters[1](lookupOutput, errorCode)
		ctUtil.PrintEvents(*permissionDeniedEvent, p.printEventUrl, p.printRawEvents)
		fmt.Println("")
		return err
	}

	arn, accountId, err := ctAws.Whoami(*sts.NewFromConfig(cfg))
	fmt.Printf("[INFO] Checking Permission Denied History since %v for AWS Account %v as %v \n", startTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)
	if err := fetchFilterPrint(*cloudTrailclient, startTime, p); err != nil {
		return err
	}

	if DefaultRegion != cfg.Region {
		defaultConfig, err := config.LoadDefaultConfig(
			context.Background(),
			config.WithRegion(DefaultRegion))
		if err != nil {
			return err
		}

		defaultCloudtrailClient := cloudtrail.New(cloudtrail.Options{
			Region:      DefaultRegion,
			Credentials: cfg.Credentials,
			HTTPClient:  cfg.HTTPClient,
		})
		fmt.Printf("[INFO] Fetching Cloudtrail Global Permission Denied Event History from %v Region...", defaultConfig.Region)
		if err := fetchFilterPrint(*defaultCloudtrailClient, startTime, p); err != nil {
			return err
		}
	}

	return err

}
