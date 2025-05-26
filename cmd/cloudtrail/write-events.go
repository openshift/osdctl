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

<<<<<<< HEAD
<<<<<<< HEAD
	Username string
<<<<<<< HEAD
>>>>>>> 1cdcb16 (ADD: Username Filter for OSDCTL Write Events)
=======
	Event    string
>>>>>>> 89db7dc (ADD: Events Filter)
=======
	Username  string
	Event     string
	ArnSource string
>>>>>>> 63c835c (ADD: Resource Name and Type to PrintEvents)
=======
	Username     string
	Event        string
	ResourceName string
	ResourceType string
	//ArnSource string
>>>>>>> ed01b0b (ADD: Resource Type Filters)
}

<<<<<<< HEAD
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
=======
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
>>>>>>> 2e9126e (REFACTOR: Refactored AWS.go Write-events.go)

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
<<<<<<< HEAD
	listEventsCmd.Flags().StringVarP(&ops.StartTime, "after", "", "", "Specifies all events that occur after the specified time. Format \"YY-MM-DD,hh:mm:ss\".")
	listEventsCmd.Flags().StringVarP(&ops.EndTime, "until", "", "", "Specifies all events that occur before the specified time. Format \"YY-MM-DD,hh:mm:ss\".")
	listEventsCmd.Flags().StringVarP(&ops.Duration, "since", "", "1h", "Specifies that only events that occur within the specified time are returned. Defaults to 1h. Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\", \"d\", \"w\".")

	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")

	listEventsCmd.Flags().StringSliceVarP(&fil.Include, "include", "I", nil, "Filter events by inclusion. (i.e. \"-I username=, -I event=, -I resource-name=, -I resource-type=, -I arn=\")")
	listEventsCmd.Flags().StringSliceVarP(&fil.Exclude, "exclude", "E", nil, "Filter events by exclusion. (i.e. \"-E username=, -E event=, -E resource-name=, -E resource-type=, -E arn=\")")

=======
	listEventsCmd.Flags().StringVarP(&ops.StartTime, "since", "", "1h", "Specifies that only events that occur within the specified time are returned.Defaults to 1h.Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")
	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().BoolVarP(&ops.PrintAll, "all", "A", false, "Prints all cloudtrail write events without filtering")

	listEventsCmd.Flags().StringVarP(&ops.Username, "username", "U", "", "Filter events by username")
<<<<<<< HEAD
>>>>>>> 1cdcb16 (ADD: Username Filter for OSDCTL Write Events)
=======
	listEventsCmd.Flags().StringVarP(&ops.Event, "event", "E", "", "Filter by event name")
<<<<<<< HEAD
<<<<<<< HEAD
>>>>>>> 89db7dc (ADD: Events Filter)
=======
	listEventsCmd.Flags().StringVarP(&ops.ArnSource, "arn", "a", "", "Filter by arn")
>>>>>>> 63c835c (ADD: Resource Name and Type to PrintEvents)
=======
	listEventsCmd.Flags().StringVarP(&ops.ResourceName, "resource-name", "", "", "Filter by resource name")
	listEventsCmd.Flags().StringVarP(&ops.ResourceType, "resource-type", "t", "", "Filter by resource type")
	//listEventsCmd.Flags().StringVarP(&ops.ArnSource, "arn", "a", "", "Filter by arn")
>>>>>>> ed01b0b (ADD: Resource Type Filters)
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

func (o *writeEventsOptions) run(filters WriteEventFilters) error {

	// Checking for valid cluster
	// Connection to cluster is successful
	// Check is cluster is AWS
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

<<<<<<< HEAD
=======
	Ignore, err := envConfig.LoadCloudTrailConfig()
	if err != nil {
		return fmt.Errorf("[ERROR] error Loading cloudtrail configuration file: %w", err)
	}
	if len(Ignore) == 0 {
		fmt.Println("\n[WARNING] No filter list detected! If you want intend to apply user filtering for the cloudtrail events, please add cloudtrail_cmd_lists to your osdctl configuration file.")

	}

	// Ask Zakaria / Research myself
	mergedRegex := ctUtil.MergeRegex(Ignore)
	if o.PrintAll {
		mergedRegex = ""
	}
>>>>>>> 89db7dc (ADD: Events Filter)
	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
=======
	//Username

	//UserName, err := utils.IsValidUserName(o.Username)
	//if err != nil {
	//		return err/
	//	}

	UserName := o.Username
	if UserName == "" {
=======
	username := o.Username
	if username == "" {
<<<<<<< HEAD
>>>>>>> 89db7dc (ADD: Events Filter)
		fmt.Println("[INFO] No username provided. Fetching all events.")
=======
		fmt.Println("[INFO] No username provided.")
>>>>>>> 63c835c (ADD: Resource Name and Type to PrintEvents)
=======
	// Added
	filters := make(map[string]string)
	filters["username"] = o.Username
	filters["event"] = o.Event
	filters["resourceName"] = o.ResourceName
	filters["resourceType"] = o.ResourceType

	for k, v := range filters {
		if v == "" {
			fmt.Printf("[INFO] No %s provided. \n", k)
		}
>>>>>>> 2e9126e (REFACTOR: Refactored AWS.go Write-events.go)
	}

	/*
		arnSource := o.ArnSource
		fmt.Println(arnSource)
		if arnSource == "" {
			fmt.Println("[INFO] Arn not provided.")
		}
	*/

	//StartTime
>>>>>>> 1cdcb16 (ADD: Username Filter for OSDCTL Write Events)
	DefaultRegion := "us-east-1"

	arn, accountId, err := Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

<<<<<<< HEAD
	fmt.Printf("[INFO] Checking write event history since %v until %v for AWS Account %v as %v \n", startTime, endTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)

	queriedEvents, err := GetEvents(cloudTrailclient, startTime, endTime, true)
=======
	// FilterAndPrintEvents fetches events and filters them based on a regex string.
	// It then prints the filtered events.

	arn, accountId, err := ctAws.Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

	//CMD Line Prints
	fmt.Printf("[INFO] Checking write event history since %v for AWS Account %v as %v \n", startTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)

<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
	queriedEvents, err := ctAws.GetEvents(cloudTrailclient, startTime, true, UserName)
>>>>>>> 1cdcb16 (ADD: Username Filter for OSDCTL Write Events)
=======
	queriedEvents, err := ctAws.GetEvents(cloudTrailclient, startTime, true, username, event)
>>>>>>> 89db7dc (ADD: Events Filter)
=======
	queriedEvents, err := ctAws.GetEvents(cloudTrailclient, startTime, true, username, event, arnSource)
>>>>>>> 63c835c (ADD: Resource Name and Type to PrintEvents)
=======
	queriedEvents, err := ctAws.GetEvents(cloudTrailclient, startTime, true, username, event, resourceName, resourceType)
>>>>>>> ed01b0b (ADD: Resource Type Filters)
=======
	queriedEvents, err := ctAws.GetEvents(cloudTrailclient, startTime, true, filters)
>>>>>>> 2e9126e (REFACTOR: Refactored AWS.go Write-events.go)
	if err != nil {
		return err
	}

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
<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
=======
		lookupOutput, err := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, UserName)
>>>>>>> 1cdcb16 (ADD: Username Filter for OSDCTL Write Events)
=======
		lookupOutput, err := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, username, event)
>>>>>>> 89db7dc (ADD: Events Filter)
=======
		lookupOutput, err := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, username, event, arnSource)
>>>>>>> 63c835c (ADD: Resource Name and Type to PrintEvents)
=======
		lookupOutput, err := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, username, event, resourceName, resourceType)
>>>>>>> ed01b0b (ADD: Resource Type Filters)
=======
		lookupOutput, err := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, filters)
>>>>>>> 2e9126e (REFACTOR: Refactored AWS.go Write-events.go)
		if err != nil {
			return err
		}
		filteredEvents = Filters(filters, queriedEvents)

		PrintEvents(filteredEvents, o.PrintUrl, o.PrintRaw)
	}

	return err
}
