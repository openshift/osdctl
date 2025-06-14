package cloudtrail

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ctUtil "github.com/openshift/osdctl/cmd/cloudtrail/pkg"
	ctAws "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
	envConfig "github.com/openshift/osdctl/pkg/envConfig"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

var DefaultRegion = "us-east-1"

// LookupEventsOptions struct for holding options for event lookup
type writeEventsOptions struct {
	ClusterID string
	StartTime string
	PrintUrl  bool
	PrintRaw  bool
	PrintAll  bool
}

type RawEventDetails struct {
	EventVersion string `json:"eventVersion"`
	UserIdentity struct {
		AccountId      string `json:"accountId"`
		SessionContext struct {
			SessionIssuer struct {
				Type     string `json:"type"`
				UserName string `json:"userName"`
				Arn      string `json:"arn"`
			} `json:"sessionIssuer"`
		} `json:"sessionContext"`
	} `json:"userIdentity"`
	EventRegion string `json:"awsRegion"`
	EventId     string `json:"eventID"`
}

func newCmdWriteEvents() *cobra.Command {
	ops := &writeEventsOptions{}
	fil := &ctAws.WriteEventFilters{}
	listEventsCmd := &cobra.Command{
		Use:   "write-events",
		Short: "Prints cloudtrail write events to console with optional filtering",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run(fil)
		},
	}
	listEventsCmd.Flags().StringVarP(&ops.ClusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.StartTime, "since", "", "1h", "Specifies that only events that occur within the specified time are returned. Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\", \"d\", \"w\".")
	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().BoolVarP(&ops.PrintAll, "all", "A", false, "Prints all cloudtrail write events without filtering")

	// Inclusion Flags
	listEventsCmd.Flags().StringSliceVarP(&fil.Username, "username", "U", nil, "Filter events by usernames. Sample Code follows the same format for remaining filters")
	listEventsCmd.Flags().StringSliceVarP(&fil.Event, "event", "E", nil, "Filter by event names. (e.g. --event \"event1,event2\")")
	listEventsCmd.Flags().StringSliceVarP(&fil.ResourceName, "resource-name", "n", nil, "Filter by resource names")
	listEventsCmd.Flags().StringSliceVarP(&fil.ResourceType, "resource-type", "t", nil, "Filter by resource types")
	listEventsCmd.Flags().StringSliceVarP(&fil.ArnSource, "arn", "", nil, "Filter by arn")

	// Exclusion Flags
	listEventsCmd.Flags().StringSliceVar(&fil.ExcludeUsername, "exclude-username", nil, "Exclude events by username")
	listEventsCmd.Flags().StringSliceVar(&fil.ExcludeEvent, "exclude-event", nil, "Exclude events by event name")
	listEventsCmd.Flags().StringSliceVar(&fil.ExcludeResourceName, "exclude-resource-name", nil, "Exclude events by resource name")
	listEventsCmd.Flags().StringSliceVar(&fil.ExcludeResourceType, "exclude-resource-type", nil, "Exclude events by resource type")
	listEventsCmd.Flags().StringSliceVarP(&fil.ExcludeArnSource, "exclude-arn", "", nil, "Exclude by arn")
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

// FilterByIgnorelist filters out events based on the specified ignore list, which contains
// regular expression patterns. It returns true if the event should be kept, and false if it should be filtered out.
func isIgnoredEvent(event types.Event, mergedRegex string) (bool, error) {
	if mergedRegex == "" {
		return true, nil
	}
	raw, err := ctAws.ExtractUserDetails(event.CloudTrailEvent)
	if err != nil {
		return true, fmt.Errorf("[ERROR] failed to extract raw CloudTrail event details: %w", err)
	}
	userArn := raw.UserIdentity.SessionContext.SessionIssuer.Arn
	regexObj := regexp.MustCompile(mergedRegex)

	if event.Username != nil {
		if regexObj.MatchString(*event.Username) {
			return false, nil
		}
	}
	if userArn != "" {

		if regexObj.MatchString(userArn) {

			return false, nil
		}
	}
	if userArn == "" && event.Username == nil {
		return false, nil
	}

	return true, nil
}

func (o *writeEventsOptions) run(filters *ctAws.WriteEventFilters) error {

	err := utils.IsValidClusterKey(o.ClusterID)
	if err != nil {
		return err
	}
	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to ocm: %w", err)
	}
	defer connection.Close()

	cluster, err := utils.GetClusterAnyStatus(connection, o.ClusterID)
	if err != nil {
		return err
	}
	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("[ERROR] this command is only available for AWS clusters")
	}

	Ignore, err := envConfig.LoadCloudTrailConfig()
	if err != nil {
		return fmt.Errorf("[ERROR] error Loading cloudtrail configuration file: %w", err)
	}
	if len(Ignore) == 0 {
		fmt.Println("\n[WARNING] No filter list detected! If you want intend to apply user filtering for the cloudtrail events, please add cloudtrail_cmd_lists to your osdctl configuration file.")

	}

	mergedRegex := ctUtil.MergeRegex(Ignore)
	if o.PrintAll {
		mergedRegex = ""
	}
	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	DefaultRegion := "us-east-1"
	startTime, err := ctUtil.ParseDurationToUTC(o.StartTime)
	if err != nil {
		return err
	}

	arn, accountId, err := ctAws.Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

	fmt.Printf("[INFO] Checking write event history since %v for AWS Account %v as %v \n", startTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)

	fmt.Printf("ClusterID: %s \n", o.ClusterID)

	queriedEvents, _ := ctAws.GetEvents(cloudTrailclient, startTime, true, filters)

	filteredEvents, err := ctUtil.ApplyFilters(queriedEvents,
		func(event types.Event) (bool, error) {
			return isIgnoredEvent(event, mergedRegex)
		},
	)
	if err != nil {
		return err
	}

	ctUtil.PrintEvents(filteredEvents, o.PrintUrl, o.PrintRaw)
	fmt.Println("")

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
		fmt.Printf("[INFO] Fetching Cloudtrail Global Event History from %v Region...", defaultConfig.Region)

		lookupOutput, err := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, filters)
		if err != nil {
			return err
		}

		filteredEvents, err := ctUtil.ApplyFilters(lookupOutput,
			func(event types.Event) (bool, error) {
				return isIgnoredEvent(event, mergedRegex)
			},
		)
		if err != nil {
			return err
		}
		ctUtil.PrintEvents(filteredEvents, o.PrintUrl, o.PrintRaw)
	}

	return err
}
