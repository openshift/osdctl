package cloudtrail

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

var DefaultRegion = "us-east-1"

// LookupEventsOptions struct for holding options for event lookup

// writeEventOption containers, ClusterID, StartTime, URL, Raw, Data, Printall
type writeEventsOptions struct {
<<<<<<< HEAD
	ClusterID   string
	StartTime   string
	EndTime     string
	Duration    string
	PrintUrl    bool
	PrintRaw    bool
	PrintFormat []string
=======
	ClusterID string
	StartTime string
	PrintUrl  bool
	PrintRaw  bool
	PrintAll  bool

	Username     []string
	Event        []string
	ResourceName []string
	ResourceType []string

	ExcludeUsername     []string
	ExcludeEvent        []string
	ExcludeResourceName []string
	ExcludeResourceType []string
	ArnSource           []string
>>>>>>> dc30e1c (ADD: Filters with ARN)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run(*fil)
		},
	}
	listEventsCmd.Flags().StringVarP(&ops.ClusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.StartTime, "after", "", "", "Specifies all events that occur after the specified time. Format \"YY-MM-DD,hh:mm:ss\".")
	listEventsCmd.Flags().StringVarP(&ops.EndTime, "until", "", "", "Specifies all events that occur before the specified time. Format \"YY-MM-DD,hh:mm:ss\".")
	listEventsCmd.Flags().StringVarP(&ops.Duration, "since", "", "1h", "Specifies that only events that occur within the specified time are returned. Defaults to 1h. Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\", \"d\", \"w\".")

	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")

<<<<<<< HEAD
	listEventsCmd.Flags().StringSliceVarP(&fil.Include, "include", "I", nil, "Filter events by inclusion. (i.e. \"-I username=, -I event=, -I resource-name=, -I resource-type=, -I arn=\")")
	listEventsCmd.Flags().StringSliceVarP(&fil.Exclude, "exclude", "E", nil, "Filter events by exclusion. (i.e. \"-E username=, -E event=, -E resource-name=, -E resource-type=, -E arn=\")")
=======
	// Inclusion Flags
	listEventsCmd.Flags().StringSliceVarP(&ops.Username, "username", "U", nil, "Filter events by username")
	listEventsCmd.Flags().StringSliceVarP(&ops.Event, "event", "E", nil, "Filter by event name")
	listEventsCmd.Flags().StringSliceVarP(&ops.ResourceName, "resource-name", "", nil, "Filter by resource name")
	listEventsCmd.Flags().StringSliceVarP(&ops.ResourceType, "resource-type", "t", nil, "Filter by resource type")
	listEventsCmd.Flags().StringSliceVarP(&ops.ArnSource, "arn-source", "", nil, "Filter by arn")
>>>>>>> dc30e1c (ADD: Filters with ARN)

	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

func (o *writeEventsOptions) run(filters WriteEventFilters) error {

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

	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

<<<<<<< HEAD
=======
	//StartTime
>>>>>>> dc30e1c (ADD: Filters with ARN)
	DefaultRegion := "us-east-1"

	arn, accountId, err := Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

	fmt.Printf("[INFO] Checking write event history since %v until %v for AWS Account %v as %v \n", startTime, endTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)

<<<<<<< HEAD
	queriedEvents, err := GetEvents(cloudTrailclient, startTime, endTime, true)
	if err != nil {
		return err
=======
	/*
		queriedEvents, err := ctAws.GetEvents(cloudTrailclient, startTime, true, filters)
		if err != nil {
			return err
		}
	*/

	// Assign k,v to filters
	filters := make(map[string][]string)
	filters["username"] = o.Username
	filters["event"] = o.Event
	filters["resourceName"] = o.ResourceName
	filters["resourceType"] = o.ResourceType
	filters["exclude-username"] = o.ExcludeUsername
	filters["exclude-event"] = o.ExcludeEvent
	filters["exclude-resourceName"] = o.ExcludeResourceName
	filters["exclude-resourceType"] = o.ExcludeResourceType
	filters["arn"] = o.ArnSource // Add ARN filtering

	//
	for key, values := range filters {
		var splitValues []string
		for _, value := range values {
			splitValues = append(splitValues, strings.Split(value, ",")...)
		}
		filters[key] = splitValues
>>>>>>> dc30e1c (ADD: Filters with ARN)
	}
	fmt.Println("Converted Filters:")
	for key, value := range filters {
		fmt.Printf("Key: %s, Value: %s\n", key, value)
	}

	queriedEvents, _ := ctAws.GetEvents(cloudTrailclient, startTime, true, filters)

	filteredEvents := Filters(filters, queriedEvents)

	if o.PrintFormat != nil {
		PrintFormat(filteredEvents, o.PrintUrl, o.PrintRaw, o.PrintFormat)
	} else {
		PrintEvents(filteredEvents, o.PrintUrl, o.PrintRaw)
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

		queriedEvents, err := GetEvents(defaultCloudtrailClient, startTime, endTime, true)
		fmt.Printf("[INFO] Fetching Cloudtrail Global Event History from %v Region...", defaultConfig.Region)
<<<<<<< HEAD
=======
		/*
			lookupOutput, err := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, filters)
			if err != nil {
				return err
			}*/

		lookupOutput, _ := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, filters)

		filteredEvents, err := ctUtil.ApplyFilters(lookupOutput,
			func(event types.Event) (bool, error) {
				return isIgnoredEvent(event, mergedRegex)
			},
		)
>>>>>>> dc30e1c (ADD: Filters with ARN)
		if err != nil {
			return err
		}
		filteredEvents = Filters(filters, queriedEvents)

		PrintEvents(filteredEvents, o.PrintUrl, o.PrintRaw)
	}

	return err
}
