package cloudtrail

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const DEFAULT_REGION = "us-east-1"

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
	Cache       bool

	awsAPI   *EventAPI
	printer  *Printer
	log      *logrus.Logger
	logLevel string

	missingPeriod []Period
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
	listEventsCmd.Flags().StringVarP(&ops.logLevel, "log-level", "l", "info", "Options: \"info\", \"debug\", \"warn\", \"error\". (default=info)")
	listEventsCmd.Flags().BoolVarP(&ops.Cache, "cache", "", true, "Enable/Disable cache file for write-events")

	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().StringSliceVarP(&ops.PrintFields, "print-fields", "", defaultFields, "Prints all cloudtrail write events in selected format. Can specify (username, time, event, arn, resource-name, resource-type, arn). i.e --print-format username,time,event")

	listEventsCmd.Flags().StringSliceVarP(&fil.Include, "include", "I", nil, "Filter events by inclusion. (i.e. \"-I username=, -I event=, -I resource-name=, -I resource-type=, -I arn=\")")
	listEventsCmd.Flags().StringSliceVarP(&fil.Exclude, "exclude", "E", nil, "Filter events by exclusion. (i.e. \"-E username=, -E event=, -E resource-name=, -E resource-type=, -E arn=\")")
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

func (o *writeEventsOptions) getPages(filters WriteEventFilters, region string, requestedPeriod Period) error {
	cache, err := NewCache(o.log, o.ClusterID)
	if err != nil {
		return err
	}
	err = cache.EnsureFilenameExist()
	if err != nil {
		return err
	}

	err = cache.Read()
	if err != nil {
		return err
	}

	var fullCacheOverlap bool

	if len(o.missingPeriod) == 0 {
		o.missingPeriod, fullCacheOverlap = requestedPeriod.DiffMultiple(cache.Period)
	}

	cacheEvents := cache.FilterByPeriod(requestedPeriod)
	cacheEvents = FilterByRegion(region, cacheEvents)

	sort.Sort(sort.Reverse(Periods(o.missingPeriod)))

	newCacheData := Cache{
		Period: []Period{},
		Event:  []types.Event{},
	}

	for i := 0; i < len(o.missingPeriod); i++ {
		currentPeriod := o.missingPeriod[i]

		if fullCacheOverlap {
			o.log.Debugf("Retrieve all events from cache")
			events := FilterEventsBefore(FilterEventsAfter(
				cacheEvents, requestedPeriod.StartTime),
				requestedPeriod.EndTime)
			o.printer.PrintEvents(Filters(filters, events), o.PrintFields)
			o.missingPeriod = []Period{}
			break
		}

		if i == 0 && !requestedPeriod.EndTime.Equal(currentPeriod.EndTime) {
			o.log.Debugf("Retrieving 1st Period Event in Cache \n")
			cachedBefore := FilterEventsBefore(
				FilterEventsAfter(cacheEvents, currentPeriod.EndTime),
				requestedPeriod.EndTime,
			)
			o.printer.PrintEvents(Filters(filters, cachedBefore), o.PrintFields)
		}

		var missingEvents []types.Event
		generator := o.awsAPI.GetEvents(o.ClusterID, currentPeriod)
		for page := range generator {
			o.log.Debug("\n Retrieving Pages \n")
			if page.errors != nil {
				o.log.Errorf("Error fetching events: %v", page.errors)
				continue
			}
			missingEvents = append(missingEvents, page.AWSEvent...)
		}

		fetchedEvents := Filters(filters, missingEvents)
		o.printer.PrintEvents(fetchedEvents, o.PrintFields)

		// if startTimePeriod is out of range, create a period starting
		// at the requested end time to establish the final boundary.
		var startTimePeriod time.Time
		if i < len(o.missingPeriod)-1 {
			startTimePeriod = o.missingPeriod[i+1].EndTime.Add(time.Second)
			currentPeriod.StartTime = currentPeriod.StartTime.Add(-time.Second)
		} else {
			startTimePeriod = requestedPeriod.StartTime
		}

		o.log.Debugf("\n Checking Cache %v, %v", startTimePeriod, currentPeriod.StartTime)

		cachedBetween := FilterEventsBefore(FilterEventsAfter(
			cacheEvents, startTimePeriod),
			currentPeriod.StartTime,
		)
		o.printer.PrintEvents(Filters(filters, cachedBetween), o.PrintFields)

		newCacheData.Period = append(newCacheData.Period, currentPeriod)
		newCacheData.Event = append(newCacheData.Event, missingEvents...)
	}

	o.log.Debugf("Saving into Cache")
	if err := cache.Save(newCacheData); err != nil {
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

	arn, accountId, err := Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}
	startTime, endTime, err := ParseStartEndTime(o.StartTime, o.EndTime, o.Duration)
	if err != nil {
		return err
	}

	o.log.Infof("Checking write event history for AWS Account %v as %v from %v until %v from %v Region...\n", accountId, arn, startTime, endTime, cfg.Region)

	o.awsAPI = NewEventAPI(cfg, true, cfg.Region)
	o.printer = NewPrinter(o.PrintUrl, o.PrintRaw)

	requestedPeriod := Period{StartTime: startTime, EndTime: endTime}

	err = o.getPages(filters, cfg.Region, requestedPeriod)
	if err != nil {
		return err
	}

	fmt.Println("")
	if DEFAULT_REGION != cfg.Region {

		o.log.Infof("Retrieving from %s...", DEFAULT_REGION)
		defaultAwsAPI := NewEventAPI(cfg, true, DEFAULT_REGION)
		o.awsAPI = defaultAwsAPI

		err = o.getPages(filters, DEFAULT_REGION, requestedPeriod)
		if err != nil {
			return err
		}
	}

	return nil
}
