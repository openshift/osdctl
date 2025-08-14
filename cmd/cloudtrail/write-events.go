package cloudtrail

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
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
	awsAPI      *EventAPI
	printer     *Printer
	log         *logrus.Logger
	logLevel    string
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
	listEventsCmd.Flags().StringVarP(&ops.logLevel, "log-level", "", "info", "Options: \"info\", \"debug\", \"warn\", \"error\". (default=info)")

	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().StringSliceVarP(&ops.PrintFields, "print-fields", "", defaultFields, "Prints all cloudtrail write events in selected format. Can specify (username, time, event, arn, resource-name, resource-type, arn). i.e --print-format username,time,event")

	listEventsCmd.Flags().StringSliceVarP(&fil.Include, "include", "I", nil, "Filter events by inclusion. (i.e. \"-I username=, -I event=, -I resource-name=, -I resource-type=, -I arn=\")")
	listEventsCmd.Flags().StringSliceVarP(&fil.Exclude, "exclude", "E", nil, "Filter events by exclusion. (i.e. \"-E username=, -E event=, -E resource-name=, -E resource-type=, -E arn=\")")
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

// processPeriods dictates the program flow of fetching and printing Cloudtrail Events
// for a given cluster and region. The workflow is as follows:
//  1. Checks if the requested time frame already exists in the cache.
//  2. Identifies any gaps (missing periods) in the cache for the requested range.
//  3. Sorts all relevant periods in reverse chronological order.
//  4. Iterates through each period:
//     - If the period exists in the cache, filters and prints the cached events.
//     - If the period is missing, fetches events from AWS CloudTrail, filters, prints, and prepares them for caching.
//  5. Saves any newly fetched events to the cache file to optimize future queries.
func (o *writeEventsOptions) processPeriods(filters WriteEventFilters, region string) error {
	startTime, endTime, err := ParseStartEndTime(o.StartTime, o.EndTime, o.Duration)
	if err != nil {
		return err
	}
	o.log.Infof("Fetching Event History from %v until %v from %v Region...\n", startTime, endTime, region)

	requestTime := Period{StartTime: startTime, EndTime: endTime}

	cacheFile, err := CacheFileInit(o.ClusterID, DefaultRegion)
	if err != nil {
		return err
	}
	cacheData, err := Read(cacheFile, o.log)
	if err != nil {
		return err
	}
	cacheEvents := getCacheEvents(cacheData, requestTime)
	missingPeriod := DiffMultiple(requestTime, cacheData)

	allEvents := make(map[Period][]types.Event)
	var allPeriods []Period
	for period, events := range cacheEvents {
		allPeriods = append(allPeriods, period)
		allEvents[period] = events
	}

	if len(missingPeriod) == 0 {
		missingPeriod = append(missingPeriod, requestTime)
	}
	allPeriods = append(allPeriods, missingPeriod...)

	sort.Sort(sort.Reverse(Periods(allPeriods)))

	newEvents := make(map[Period][]types.Event)

	// Idea for trying to enable 2 requests per second/

	// 1. Break allPeriods into 2 lists
	// 2. First list fetches and print
	// 3. 2nd list fetches and waits
	// Issues:
	//		- Additional duplication of codes and new functions needs to be created.

	o.log.Debugf("Starting Process")
	fmt.Println(allPeriods)
	for _, period := range allPeriods {
		foundinCache := false
		for cachePeriod, events := range cacheEvents {
			if !period.StartTime.Before(cachePeriod.StartTime) && !period.EndTime.After(cachePeriod.EndTime) {
				o.log.Debugf("Page in Cache")
				filteredEvents := Filters(filters, events)
				if len(filteredEvents) > 0 {
					o.printer.PrintEvents(filteredEvents, o.PrintFields)
				}
				foundinCache = true
				break
			}
		}
		if foundinCache {
			continue
		}

		sendRequest := false
		for _, missing := range missingPeriod {
			o.log.Debugf("Check missing times")
			if missing.StartTime.Equal(period.StartTime) && missing.EndTime.Equal(period.EndTime) {
				sendRequest = true
				break
			}
		}

		if sendRequest {
			o.log.Debugf("Requesting Pages")
			var eventsForThisPeriod []types.Event
			generator := o.awsAPI.GetEvents(o.ClusterID, period)
			for page := range generator {
				eventsForThisPeriod = append(eventsForThisPeriod, page.AWSEvent...)
			}
			newEvents[period] = eventsForThisPeriod
			if len(eventsForThisPeriod) > 0 {
				filteredEvents := Filters(filters, eventsForThisPeriod)
				if len(filteredEvents) > 0 {
					o.printer.PrintEvents(filteredEvents, o.PrintFields)
				}
			}
		}
	}

	if err := Save(cacheFile, newEvents, o.log); err != nil {
		return err
	}
	return nil
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

	log := logrus.New()
	level, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		return err
	}
	log.SetLevel(level)
	log.ReportCaller = false
	log.Formatter = new(logrus.TextFormatter)
	log.Formatter.(*logrus.TextFormatter).ForceColors = true
	log.Formatter.(*logrus.TextFormatter).PadLevelText = false
	log.Formatter.(*logrus.TextFormatter).DisableQuote = true
	o.log = log

	return nil

}

func (o *writeEventsOptions) run(filters WriteEventFilters) error {
	connection, err := utils.CreateConnection()
	if err != nil {
		o.log.Error("unable to create connection to ocm: %w", err)
		return err
	}
	defer connection.Close()

	cluster, err := utils.GetClusterAnyStatus(connection, o.ClusterID)
	if err != nil {
		return err
	}
	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		o.log.Error("this command is only available for AWS clusters")
		return err
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

	o.log.Infof("Checking write event history for AWS Account %v as %v \n", accountId, arn)

	awsAPI := NewEventAPI(cfg, true)
	printer := NewPrinter(o.PrintUrl, o.PrintRaw)
	o.awsAPI = awsAPI
	o.printer = printer

	err = o.processPeriods(filters, cfg.Region)
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

		defaultAwsAPI := NewEventAPI(cfg, true)
		defaultAwsAPI.client = NewEventAPIWithOptions(cfg, DefaultRegion)
		o.awsAPI = defaultAwsAPI

		err = o.processPeriods(filters, defaultConfig.Region)
		if err != nil {
			return err
		}

	}

	return err
}
