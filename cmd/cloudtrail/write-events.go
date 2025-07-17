package cloudtrail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	envConfig "github.com/openshift/osdctl/pkg/envConfig"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

var DefaultRegion = "us-east-1"

// LookupEventsOptions struct for holding options for event lookup
type writeEventsOptions struct {
	ClusterID   string
	StartTime   string
	EndTime     string
	Duration    string
	PrintUrl    bool
	PrintRaw    bool
	PrintAll    bool
	PrintFormat []string
}

const (
	cloudtrailWriteEventsExample = `
    # List all write-events in a cluster (default: last 1 hour)
    $ osdctl cloudtrail write-events -C cluster-id

  Time:
    # List all write-events from the last 3 hours
    $ osdctl cloudtrail write-events -C cluster-id --since 3h

    # List all write-events after a given date
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,15:00:00
    
    # List all write-events after a certain date-time for the next 3 hours
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,15:00:00 --since 3h
	
	# List all write-events until a given date
    $ osdctl cloudtrail write-events -C cluster-id --until 2025-07-15,15:00:00
    
    # List all write-events until date-time from the last 3 hours (default is 1h)
    $ osdctl cloudtrail write-events -C cluster-id --until 2025-07-15,18:00:00 --since 3h

    # List all write-events when given a start and end date
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,15:00:00 --until 2025-07-15,18:00:00

  Filter:
    # Include events for specific resource types
    $ osdctl cloudtrail write-events -C cluster-id -I resource-type=bucket -I resource-type=role

    # Exclude specific events and users
    $ osdctl cloudtrail write-events -C cluster-id -E event=AssumeRole -E username=system:serviceaccount

	# Include only events with username=sre-operator and event=BucketList and Exclude event=RoleUpdate
    $ osdctl cloudtrail write-events -C cluster-id -I username=sre-operator -I event=BucketList -E event=RoleUpdate

  Print format:
    # Print events in raw JSON format
    $ osdctl cloudtrail write-events -C cluster-id --raw-event

    # Print events with AWS console URLs
    $ osdctl cloudtrail write-events -C cluster-id --url

    # Print all events without filtering (including system events)
    $ osdctl cloudtrail write-events -C cluster-id --all

    # Allow only certain fields to be displayed
    $ osdctl cloudtrail write-events -C cluster-id --print-format event,time,username

  Combined examples:
    # Search for specific events in the last 2 hours with custom format
    $ osdctl cloudtrail write-events -C cluster-id --since 2h -I event=CreateBucket --print-format event,time,username,resource-name

    # Get events after a specific time with filtering and URLs
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,09:00:00 -E username=system -I event=Delete --url

    # Get events before a specific time from the last 4 hours in raw format
    $ osdctl cloudtrail write-events -C cluster-id --until 2025-07-15,17:00:00 --since 4h --raw-event

    # Get events between a specific time frame
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-13,17:00:00 --until 2025-07-15,17:00:00 
    `
	cloudtrailWriteEventsDescription = `
	Lists AWS CloudTrail write events for a specific OpenShift/ROSA cluster with advanced 
	filtering capabilities to help investigate cluster-related activities.

	The command automatically authenticates with OpenShift Cluster Manager (OCM) and assumes 
	the appropriate AWS role for the target cluster to access CloudTrail logs.

	By default, the command filters out system and service account events using patterns 
	from the osdctl configuration file. `
)

func newCmdWriteEvents() *cobra.Command {
	ops := &writeEventsOptions{}
	fil := &WriteEventFilters{}
	listEventsCmd := &cobra.Command{
		Use:     "write-events",
		Short:   "Prints cloudtrail write events to console with advanced filtering options",
		Long:    cloudtrailWriteEventsDescription,
		Example: cloudtrailWriteEventsExample,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run(*fil)
		},
	}
	listEventsCmd.Flags().StringVarP(&ops.ClusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.StartTime, "after", "", "", "Specifies that only events that occur within the specified time are returned. Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\", \"d\", \"w\".")
	listEventsCmd.Flags().StringVarP(&ops.EndTime, "until", "", time.Now().UTC().Format("2006-01-02,15:04:05"), "Specifies all events that occur before the specified time. Format \"YY-MM-DD,hh:mm:ss\".")
	listEventsCmd.Flags().StringVarP(&ops.Duration, "since", "", "1h", "Specifies that only events that occur within the specified time are returned.")

	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().BoolVarP(&ops.PrintAll, "all", "A", false, "Prints all cloudtrail write events without filtering")
	listEventsCmd.Flags().StringSliceVarP(&ops.PrintFormat, "print-format", "", nil, "Prints all cloudtrail write events in selected format. Can specify (username, time, event, arn, resource-name, resource-type, arn). i.e --print-format username,time,event")

	listEventsCmd.Flags().StringSliceVarP(&fil.Include, "include", "I", nil, "Filter events by inclusion. (i.e. \"-I username=, -I event=, -I resource-name=, -I resource-type=, -I arn=\")")
	listEventsCmd.Flags().StringSliceVarP(&fil.Exclude, "exclude", "E", nil, "Filter events by exclusion. (i.e. \"-E username=, -E event=, -E resource-name=, -E resource-type=, -E arn=\")")
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

func (o *writeEventsOptions) run(filters WriteEventFilters) error {

	if err := utils.IsValidClusterKey(o.ClusterID); err != nil {
		return err
	}
	if err := ValidateFilters(filters.Include); err != nil {
		return err
	}
	if err := ValidateFilters(filters.Exclude); err != nil {
		return err
	}
	if err := ValidateFormat(o.PrintFormat); err != nil {
		return err
	}

	startTime, endTime, err := ParseStartEndTime(o.StartTime, o.EndTime, o.Duration)
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

	mergedRegex := strings.Join(Ignore, "|")
	if o.PrintAll {
		mergedRegex = ""
	}

	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	DefaultRegion := "us-east-1"

	arn, accountId, err := Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

	fmt.Printf("[INFO] Checking write event history since %v until %v for AWS Account %v as %v \n", startTime, endTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)

	err = GetEventsByPage(cloudTrailclient, startTime, endTime, true,
		func(pageEvents []types.Event) error {
			// Step 1: Apply user filters (include/exclude)
			filteredEvents := Filters(filters, pageEvents)

			// Step 2: Apply ignore filters (system events)
			cleanEvents, err := ApplyFilters(filteredEvents,
				func(event types.Event) (bool, error) {
					return IsIgnoredEvent(event, mergedRegex)
				},
			)
			if err != nil {
				return err
			}

			// Step 3: Print this page's events
			if len(cleanEvents) > 0 {
				if o.PrintFormat != nil {
					PrintFormat(cleanEvents, o.PrintUrl, o.PrintRaw, o.PrintFormat)
				} else {
					PrintEvents(cleanEvents, o.PrintUrl, o.PrintRaw)
				}
			}

			return nil
		})

	if err != nil {
		return err
	}

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
		fmt.Printf("[INFO] Fetching Cloudtrail Global Event History from %v Region... \n", defaultConfig.Region)

		err = GetEventsByPage(defaultCloudtrailClient, startTime, endTime, true,
			func(pageEvents []types.Event) error {
				filteredEvents := Filters(filters, pageEvents)
				cleanEvents, err := ApplyFilters(filteredEvents,
					func(event types.Event) (bool, error) {
						return IsIgnoredEvent(event, mergedRegex)
					},
				)
				if err != nil {
					return err
				}

				if len(cleanEvents) > 0 {
					if o.PrintFormat != nil {
						PrintFormat(cleanEvents, o.PrintUrl, o.PrintRaw, o.PrintFormat)
					} else {
						PrintEvents(cleanEvents, o.PrintUrl, o.PrintRaw)
					}
				}

				return nil
			})

		if err != nil {
			return err
		}
	}
	return err
}
