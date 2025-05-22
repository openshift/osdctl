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

// writeEventOption containers, ClusterID, StartTime, URL, Raw, Data, Printall
type writeEventsOptions struct {
	ClusterID string
	StartTime string
	PrintUrl  bool
	PrintRaw  bool
	PrintAll  bool

	Username string
	Event    string
}

// RawEventDetails struct represents the structure of an AWS raw event

/*
Contains of:
  - Event Version
  - User Identity -> (which is also a class)
*/
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
	listEventsCmd := &cobra.Command{
		Use:   "write-events",
		Short: "Prints cloudtrail write events to console with optional filtering",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run()
		},
	}
	listEventsCmd.Flags().StringVarP(&ops.ClusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.StartTime, "since", "", "1h", "Specifies that only events that occur within the specified time are returned.Defaults to 1h.Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")
	listEventsCmd.Flags().BoolVarP(&ops.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().BoolVarP(&ops.PrintAll, "all", "A", false, "Prints all cloudtrail write events without filtering")

	listEventsCmd.Flags().StringVarP(&ops.Username, "username", "U", "", "Filter events by username")
	listEventsCmd.Flags().StringVarP(&ops.Event, "event", "E", "", "Filter by event name")
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

// ErrorChecking all key in the struct
func (o *writeEventsOptions) run() error {

	// Checking for valid cluster
	// Connection to cluster is successful
	// Check is cluster is AWS
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

	// Ask Zakaria / Research myself
	mergedRegex := ctUtil.MergeRegex(Ignore)
	if o.PrintAll {
		mergedRegex = ""
	}
	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	username := o.Username
	if username == "" {
		fmt.Println("[INFO] No username provided. Fetching all events.")
	}

	event := o.Event
	if event == "" {
		fmt.Println("[INFO] No event name provided. Fetching all events.")
	}

	//StartTime
	DefaultRegion := "us-east-1"
	startTime, err := ctUtil.ParseDurationToUTC(o.StartTime)
	if err != nil {
		return err
	}

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

	queriedEvents, err := ctAws.GetEvents(cloudTrailclient, startTime, true, username, event)
	if err != nil {
		return err
	}

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
		lookupOutput, err := ctAws.GetEvents(defaultCloudtrailClient, startTime, true, username, event)
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
