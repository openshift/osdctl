package cloudtrail

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

var DefaultRegion = "us-east-1"
var defaultFields = []string{"event", "time", "username", "arn"}

// LookupEventsOptions struct for holding options for event lookup
type writeEventsOptions struct {
	ClusterID   string
	StartTime   string
	EndTime     string
	Duration    string
	PrintUrl    bool
	PrintRaw    bool
	PrintFields []string
	//log         *logrus.Logger
}

const (
	cloudtrailWriteEventsExample = `
    # Time range with user and include events where username=(john.doe or system) and event=(CreateBucket or AssumeRole); print custom format
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,09:00:00 --until 2025-07-15,17:00:00 \
      -I username=john.doe -I event=CreateBucket -E event=AssumeRole -E username=system --print-format event,time,username,resource-name

    # Get all events from a specific time onwards for a 2h duration; print url
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,15:00:00 --since 2h --url

    # Get all events until the specified time since the last 2 hours; print raw-event
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,15:00:00 --since 2h --raw-event`

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
		PreRunE: func(cmd *cobra.Command, args []string) error { return ops.preRun(*fil) },
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run(*fil)
		},
	}
	listEventsCmd.Flags().StringVarP(&ops.ClusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.StartTime, "after", "", "", "Specifies all events that occur after the specified time. Format \"YY-MM-DD,hh:mm:ss\".")
	listEventsCmd.Flags().StringVarP(&ops.EndTime, "until", "", "", "Specifies all events that occur before the specified time. Format \"YY-MM-DD,hh:mm:ss\".")
	listEventsCmd.Flags().StringVarP(&ops.Duration, "since", "", "1h", "Specifies that only events that occur within the specified time are returned. Defaults to 1h.Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")

	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().StringSliceVarP(&ops.PrintFields, "print-fields", "", defaultFields, "Prints all cloudtrail write events in selected format. Can specify (username, time, event, arn, resource-name, resource-type, arn). i.e --print-format username,time,event")

	listEventsCmd.Flags().StringSliceVarP(&fil.Include, "include", "I", nil, "Filter events by inclusion. (i.e. \"-I username=, -I event=, -I resource-name=, -I resource-type=, -I arn=\")")
	listEventsCmd.Flags().StringSliceVarP(&fil.Exclude, "exclude", "E", nil, "Filter events by exclusion. (i.e. \"-E username=, -E event=, -E resource-name=, -E resource-type=, -E arn=\")")
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

func (o *writeEventsOptions) preRun(filters WriteEventFilters) error {
	err := utils.IsValidClusterKey(o.ClusterID)
	if err != nil {
		return err
	}

	if err := ValidateFilters(filters.Include); err != nil {
		return err
	}
	if err := ValidateFilters(filters.Exclude); err != nil {
		return err
	}

	if err := ValidateFormat(o.PrintFields); err != nil {
		return err
	}

	return nil

}

func (o *writeEventsOptions) run(filters WriteEventFilters) error {

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

	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	DefaultRegion := "us-east-1"
	arn, accountId, err := Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

	awsAPI := NewCloudtrailClient(cfg, true)
	printer := NewPrinter(o.PrintUrl, o.PrintRaw)

	cacheData, err := CacheInit(o.ClusterID, startTime, endTime)
	if err != nil {
		return err
	}

	generator := awsAPI.GetEvents(o.ClusterID, startTime, endTime)
	fmt.Printf("[INFO] Checking write event history since %v until %v for AWS Account %v as %v \n", startTime, endTime, accountId, arn)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)

	// If we are fetching pages (i.e, event not in cache)
	//	Missing PutCache
	var allEvents []types.Event
	var eventErrors []error
	if cacheData.WaitForData {
		for page := range generator {
			if page.errors != nil {
				eventErrors = append(eventErrors, page.errors)
				continue
			}
			allEvents = append(allEvents, page.AWSEvent...)
			if len(allEvents) > 0 {
				filteredEvents := Filters(filters, page.AWSEvent)
				if len(filteredEvents) > 0 {
					printer.PrintEvents(filteredEvents, o.PrintFields)
				}
			}

			for _, err := range eventErrors {
				fmt.Printf("[ERROR] failed to process: %v\n", err)
			}
		}
	} else {
		// if we have event in cache
		// for _, events := range cacheData {
		// filteredEvents := Filters(filters, events...)
		//	if len(filteredEvents) > 0 {
		//		printer.PrintEvents(filteredEvents, o.PrintFields)
		// }
		for page := range generator {
			filteredEvents := Filters(filters, page.AWSEvent)
			if len(filteredEvents) > 0 {
				printer.PrintEvents(filteredEvents, o.PrintFields)
			}
		}
	}

	// if covered
	// PutCache(allEvents)

	fmt.Println("")
	if DefaultRegion != cfg.Region {
		defaultConfig, err := config.LoadDefaultConfig(
			context.Background(),
			config.WithRegion(DefaultRegion))
		if err != nil {
			return err
		}

		defaultAwsAPI := NewCloudtrailClient(cfg, true)
		defaultAwsAPI.client = NewCloudTrailClientWithOptions(cfg, DefaultRegion)

		fmt.Printf("[INFO] Fetching Cloudtrail Global Event History from %v Region... \n", defaultConfig.Region)
		generator := defaultAwsAPI.GetEvents(o.ClusterID, startTime, endTime)

		for page := range generator {
			filteredEvents := Filters(filters, page.AWSEvent)
			if len(filteredEvents) > 0 {
				printer.PrintEvents(filteredEvents, o.PrintFields)
			}
		}
	}

	return err
}
