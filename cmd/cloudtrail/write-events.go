package cloudtrail

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
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
type LookupEventsOptions struct {
	clusterID      string
	startTime      string
	printEventUrl  bool
	printRawEvents bool
	printAllEvents bool
}

// RawEventDetails struct represents the structure of an AWS raw event
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
	ops := &LookupEventsOptions{}
	listEventsCmd := &cobra.Command{
		Use:   "write-events",
		Short: "Prints cloudtrail write events to console with optional filtering",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run()
		},
	}
	listEventsCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.startTime, "since", "", "1h", "Specifies that only events that occur within the specified time are returned.Defaults to 1h.Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")
	listEventsCmd.Flags().BoolVarP(&ops.printEventUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.printRawEvents, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().BoolVarP(&ops.printAllEvents, "all", "A", false, "Prints all cloudtrail write events without filtering")
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

// parseDurationToUTC parses the given startTime string as a duration and subtracts it from the current UTC time.
// It returns the resulting time and any parsing error encountered.
func parseDurationToUTC(input string) (time.Time, error) {
	duration, err := time.ParseDuration(input)
	if err != nil {
		return time.Time{}, fmt.Errorf("[ERROR] unable to parse time duration: %w", err)
	}

	return time.Now().UTC().Add(-duration), nil
}

// whoami retrieves caller identity information
func whoami(stsClient sts.Client) (accountArn string, accountId string, err error) {
	ctx := context.TODO()
	callerIdentityOutput, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", "", err
	}

	userArn, err := arn.Parse(*callerIdentityOutput.Arn)
	if err != nil {
		return "", "", err
	}

	return userArn.String(), userArn.AccountID, nil
}

// getWriteEvents retrieves cloudtrail events since the specified time
// using the provided cloudtrail client and starttime from since flag.
func getWriteEvents(since time.Time, cloudtailClient *cloudtrail.Client) ([]*cloudtrail.LookupEventsOutput, error) {
	starttime := since
	allookupOutputs := []*cloudtrail.LookupEventsOutput{}
	input := cloudtrail.LookupEventsInput{
		StartTime: &starttime,
		EndTime:   aws.Time(time.Now()),
		LookupAttributes: []types.LookupAttribute{
			{AttributeKey: "ReadOnly",
				AttributeValue: aws.String("false")},
		},
	}

	paginator := cloudtrail.NewLookupEventsPaginator(cloudtailClient, &input, func(c *cloudtrail.LookupEventsPaginatorOptions) {})
	for paginator.HasMorePages() {

		lookupOutput, err := paginator.NextPage(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("[WARNING] paginator error: \n%w", err)
		}

		allookupOutputs = append(allookupOutputs, lookupOutput)

		input.NextToken = lookupOutput.NextToken
		if lookupOutput.NextToken == nil {
			break
		}

	}

	return allookupOutputs, nil
}

// Extracts Raw cloudtrailEvent Details
func extractUserDetails(cloudTrailEvent *string) (*RawEventDetails, error) {
	if cloudTrailEvent == nil || *cloudTrailEvent == "" {
		return &RawEventDetails{}, fmt.Errorf("[ERROR] cannot parse a nil input")
	}
	var res RawEventDetails
	err := json.Unmarshal([]byte(*cloudTrailEvent), &res)
	if err != nil {
		return &RawEventDetails{}, fmt.Errorf("[ERROR] could not marshal event.CloudTrailEvent: %w", err)
	}

	const supportedEventVersionMajor = 1
	const minSupportedEventVersionMinor = 8

	var responseMajor, responseMinor int
	if _, err := fmt.Sscanf(res.EventVersion, "%d.%d", &responseMajor, &responseMinor); err != nil {
		return &RawEventDetails{}, fmt.Errorf("[ERROR]failed to parse CloudTrail event version: %w", err)
	}
	if responseMajor != supportedEventVersionMajor || responseMinor < minSupportedEventVersionMinor {
		return &RawEventDetails{}, fmt.Errorf("[ERROR] unexpected event version (got %s, expected compatibility with %d.%d)", res.EventVersion, supportedEventVersionMajor, minSupportedEventVersionMinor)
	}
	return &res, nil
}

// generateLink generates a hyperlink to aws cloudTrail event.
func generateLink(raw RawEventDetails) (url_link string) {
	str1 := "https://"
	str2 := ".console.aws.amazon.com/cloudtrailv2/home?region="
	str3 := "#/events/"

	eventRegion := raw.EventRegion
	eventId := raw.EventId

	var url = str1 + eventRegion + str2 + eventRegion + str3 + eventId
	url_link = url

	return url_link
}

// Join all individual patterns into a single string separated by the "|" operator
func mergeRegex(regexlist []string) string {
	return strings.Join(regexlist, "|")
}

// FilterUsers filters out events based on the specified Ignore list, which contains
// regular expression patterns. It takes a slice of cloudtrail.LookupEventsOutput,
// which represents the output of AWS CloudTrail lookup events operation, and a list
// of regular expression patterns to ignore.
func filterUsers(lookupOutputs []*cloudtrail.LookupEventsOutput, Ignore []string, allEvents bool) (*[]types.Event, error) {
	filteredEvents := []types.Event{}
	mergedRegex := mergeRegex(Ignore)

	for _, lookupOutput := range lookupOutputs {
		for _, event := range lookupOutput.Events {

			raw, err := extractUserDetails(event.CloudTrailEvent)
			if err != nil {
				return nil, fmt.Errorf("[ERROR] failed to to extract raw cloudtrailEvent details: %w", err)
			}
			userArn := raw.UserIdentity.SessionContext.SessionIssuer.Arn
			regexOdj := regexp.MustCompile(mergedRegex)
			matchesUsername := false
			matchesArn := false

			if !allEvents && len(Ignore) != 0 {
				if event.Username != nil {
					matchesUsername = regexOdj.MatchString(*event.Username)
				}
				if userArn != "" {
					matchesArn = regexOdj.MatchString(userArn)

				}

				if matchesArn || matchesUsername || (*event.Username == "" && userArn == "") {

					continue
					// skips entry
				}

			}
			filteredEvents = append(filteredEvents, event)

		}
	}

	return &filteredEvents, nil
}

// PrintEvents prints the details of each event in the provided slice of events.
// It takes a slice of types.Event
func printEvents(filteredEvent []types.Event, printUrl bool, raw bool) {
	var eventStringBuilder = strings.Builder{}

	for i := len(filteredEvent) - 1; i >= 0; i-- {
		if raw {
			if filteredEvent[i].CloudTrailEvent != nil {
				fmt.Printf("%v \n", *filteredEvent[i].CloudTrailEvent)
				return
			}
		}
		rawEventDetails, err := extractUserDetails(filteredEvent[i].CloudTrailEvent)
		if err != nil {
			fmt.Printf("[Error] Error extracting event details: %v", err)
		}
		sessionIssuer := rawEventDetails.UserIdentity.SessionContext.SessionIssuer.UserName
		if filteredEvent[i].EventName != nil {
			eventStringBuilder.WriteString(fmt.Sprintf("\n%v", *filteredEvent[i].EventName))
		}
		if filteredEvent[i].EventTime != nil {
			eventStringBuilder.WriteString(fmt.Sprintf(" | %v", filteredEvent[i].EventTime.String()))
		}
		if filteredEvent[i].Username != nil {
			eventStringBuilder.WriteString(fmt.Sprintf(" | Username: %v", *filteredEvent[i].Username))
		}
		if sessionIssuer != "" {
			eventStringBuilder.WriteString(fmt.Sprintf(" | ARN: %v", sessionIssuer))
		}

		if printUrl && filteredEvent[i].CloudTrailEvent != nil {
			if err == nil {
				eventStringBuilder.WriteString(fmt.Sprintf("\n%v |", generateLink(*rawEventDetails)))
			} else {
				fmt.Println("EventLink: <not available>")
			}
		}

	}
	fmt.Println(eventStringBuilder.String())
}

func (o *LookupEventsOptions) run() error {
	err := utils.IsValidClusterKey(o.clusterID)
	if err != nil {
		return err
	}
	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to ocm: %w", err)
	}
	defer connection.Close()

	cluster, err := utils.GetClusterAnyStatus(connection, o.clusterID)
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
		fmt.Println("\n[WARNING] No filter list DETECTED!!")
	}

	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	startTime, err := parseDurationToUTC(o.startTime)
	if err != nil {
		return err
	}

	fetchFilterPrintEvents := func(client cloudtrail.Client, startTime time.Time, o *LookupEventsOptions) error {
		lookupOutput, err := getWriteEvents(startTime, &client)
		if err != nil {
			return err
		}
		filteredEvents, err := filterUsers(lookupOutput, Ignore, o.printAllEvents)
		if err != nil {
			return err
		}

		printEvents(*filteredEvents, o.printEventUrl, o.printRawEvents)
		fmt.Println("")
		return err
	}

	arn, accountId, err := whoami(*sts.NewFromConfig(cfg))
	fmt.Printf("[INFO] Checking write event history since %v for AWS Account %v as %v \n", startTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)
	if err := fetchFilterPrintEvents(*cloudTrailclient, startTime, o); err != nil {
		return err
	}

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
		if err := fetchFilterPrintEvents(*defaultCloudtrailClient, startTime, o); err != nil {
			return err
		}
	}

	return err
}
