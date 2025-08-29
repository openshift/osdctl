package cloudtrail

import (
	"fmt"
	"regexp"
	"strings"
	"time"

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
	opts := &permissionDeniedEventsOptions{}

	permissionDeniedCmd := &cobra.Command{
		Use:   "permission-denied-events",
		Short: "Prints cloudtrail permission-denied events to console.",
		RunE: func(cmd *cobra.Command, args []string) error {
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

	err := utils.IsValidClusterKey(p.ClusterID)
	if err != nil {
		return err
	}

	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to ocm: %w", err)
	}
	defer connection.Close()

	cluster, err := utils.GetClusterAnyStatus(connection, p.ClusterID)
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

	startTime, err := ParseDurationBefore(p.StartTime, time.Now().UTC())
	if err != nil {
		return err
	}

	arn, accountId, err := Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

	awsAPI := NewEventAPI(cfg, false, cfg.Region)
	printer := NewPrinter(p.PrintUrl, p.PrintRaw)
	requestTime := Period{StartTime: startTime, EndTime: time.Now().UTC()}
	generator := awsAPI.GetEvents(p.ClusterID, requestTime)

	fmt.Printf("[INFO] Checking Permission Denied History since %v for AWS Account %v as %v \n", startTime, accountId, arn)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)

	for page := range generator {
		filteredEvents, err := ApplyFilters(page.AWSEvent,
			func(event types.Event) (bool, error) {
				return isforbiddenEvent(event)
			},
		)
		if err != nil {
			return err
		}
		if len(filteredEvents) > 0 {
			printer.PrintEvents(filteredEvents, defaultFields)
		}
	}

	if DEFAULT_REGION != cfg.Region {
		defaultAwsAPI := NewEventAPI(cfg, true, DEFAULT_REGION)

		fmt.Printf("[INFO] Fetching Cloudtrail Global Permission Denied Event History from %v Region...", DEFAULT_REGION)
		generator := defaultAwsAPI.GetEvents(p.ClusterID, requestTime)

		for page := range generator {
			filteredEvents, err := ApplyFilters(page.AWSEvent,
				func(event types.Event) (bool, error) {
					return isforbiddenEvent(event)
				},
			)
			if err != nil {
				return err
			}
			if len(filteredEvents) > 0 {
				printer.PrintEvents(filteredEvents, defaultFields)
			}
		}
	}

	return err

}
