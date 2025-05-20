package cloudtrail

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type permissionDeniedEventsOptions struct {
	ClusterID string
	StartTime string
	PrintUrl  bool
	PrintRaw  bool
}

func newCmdPermissionDenied() *cobra.Command {
	opts := &permissionDeniedEventsOptions{} //Stores address of struct into opts

	permissionDeniedCmd := &cobra.Command{
		Use:   "permission-denied-events",
		Short: "Prints cloudtrail permission-denied events to console.",
		RunE: func(cmd *cobra.Command, args []string) error {
			//runs run() for errorchecking
			return opts.run()
		},
	}

	permissionDeniedCmd.Flags().StringVarP(&opts.ClusterID, "cluster-id", "C", "", "Cluster ID")
	permissionDeniedCmd.Flags().StringVarP(&opts.StartTime, "since", "", "5m", "Specifies that only events that occur within the specified time are returned.Defaults to 5m. Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")
	permissionDeniedCmd.Flags().BoolVarP(&opts.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	permissionDeniedCmd.Flags().BoolVarP(&opts.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	permissionDeniedCmd.MarkFlagRequired("cluster-id")
	return permissionDeniedCmd
}

func isforbiddenEvent(event types.Event) (bool, error) {
	// Checks if there exist a Client.UnauthorizedOperation and return error if true
	permissionDeniedErrorRegexp := ".*Client.UnauthorizedOperation.*"

	check, err := regexp.Compile(permissionDeniedErrorRegexp)
	if err != nil {
		return false, fmt.Errorf("failed to compile regex: %w", err)
	}
	raw, err := ExtractUserDetails(event.CloudTrailEvent)
	if err != nil {
		return false, fmt.Errorf("[ERROR] failed to extract raw CloudTrail event details: %w", err)
	}
	errorCode := raw.ErrorCode
	if errorCode != "" && check.MatchString(errorCode) {
		return true, nil
	}

	return false, nil
}
func (p *permissionDeniedEventsOptions) run() error {

	// check for valid cluster key
	err := utils.IsValidClusterKey(p.ClusterID)
	if err != nil {
		return err
	}

	// check connection
	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to ocm: %w", err)
	}
	defer connection.Close()

	// See status of cluster
	cluster, err := utils.GetClusterAnyStatus(connection, p.ClusterID)
	if err != nil {
		return err
	}

	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("[ERROR] this command is only available for AWS clusters")
	}

	//cfg?
	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	startTime, err := ParseDurationBefore(p.StartTime, time.Now().UTC())
	if err != nil {
		return err
	}

	arn, accountId, err := Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}
	fmt.Printf("[INFO] Checking Permission Denied History since %v for AWS Account %v as %v \n", startTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)
	lookupOutput, err := GetEvents(cloudTrailclient, startTime, time.Now().UTC(), false)
	if err != nil {
		return err
	}

	filteredEvents, err := ApplyFilters(lookupOutput,
		func(event types.Event) (bool, error) {
			return isforbiddenEvent(event)
		},
	)
	if err != nil {
		return err
	}

	PrintEvents(filteredEvents, p.PrintUrl, p.PrintRaw)

	if DefaultRegion != cfg.Region {
		defaultConfig, err := config.LoadDefaultConfig(
			context.Background(),
			config.WithRegion(DefaultRegion))
		if err != nil {
			return err
		}
		// ???
		defaultCloudtrailClient := cloudtrail.New(cloudtrail.Options{
			Region:      DefaultRegion,
			Credentials: cfg.Credentials,
			HTTPClient:  cfg.HTTPClient,
		})
		fmt.Printf("[INFO] Fetching Cloudtrail Global Permission Denied Event History from %v Region...", defaultConfig.Region)
		lookupOutput, err := GetEvents(defaultCloudtrailClient, startTime, time.Now().UTC(), false)
		if err != nil {
			return err
		}
		filteredEvents, err := ApplyFilters(lookupOutput,
			func(event types.Event) (bool, error) {
				return isforbiddenEvent(event)
			},
		)
		if err != nil {
			return err
		}
		PrintEvents(filteredEvents, p.PrintUrl, p.PrintRaw)
	}

	return err

}
