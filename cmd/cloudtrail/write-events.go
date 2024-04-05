package cloudtrail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	config "github.com/openshift/osdctl/pkg/envConfig"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// LookupEventsoptions struct for holding options for event lookup
type LookupEventsOptions struct {
	clusterID      string
	startTime      string
	printEventUrl  bool
	printRawEvents bool
	printAllEvents bool
}

// RawEventDetails struct for holding raw event details
type RawEventDetails struct {
	EventVersion string `json:"eventVersion"`
	UserIdentity struct {
		AccountId      string `json:"accountId"`
		Type           string `json:"type"`
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
		Short: "Prints out all cloudtrail write events to console",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
	}
	listEventsCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.startTime, "since", "", "", "Start time of the lookup i.e (\"2h\" for starttime 2 hours ago")
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
		return time.Time{}, err
	}

	return time.Now().UTC().Add(-duration), nil
}

// whoami retrieves caller identity information
func whoami(stsClient sts.Client) (Arn string, AccountId string, err error) {
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
	ctx := context.TODO()
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

	pages := 0
	paginator := cloudtrail.LookupEventsPaginator{}
	hasPages := paginator.HasMorePages()

	for pageIter := 1; hasPages; pageIter++ {
		pages += pageIter
	}

	maxPages := pages
	for counter := 0; counter <= maxPages; counter++ {

		lookupOutput, err := cloudtailClient.LookupEvents(ctx, &input)
		if err != nil {
			return nil, err
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
		return &RawEventDetails{}, fmt.Errorf("cannot parse a nil input")
	}
	var res RawEventDetails
	err := json.Unmarshal([]byte(*cloudTrailEvent), &res)
	if err != nil {
		return &RawEventDetails{}, fmt.Errorf("could not marshal event.CloudTrailEvent: %w", err)
	}

	const supportedEventVersionMajor = 1
	const minSupportedEventVersionMinor = 8

	var responseMajor, responseMinor int
	if _, err := fmt.Sscanf(res.EventVersion, "%d.%d", &responseMajor, &responseMinor); err != nil {
		return &RawEventDetails{}, fmt.Errorf("failed to parse CloudTrail event version: %w", err)
	}
	if responseMajor != supportedEventVersionMajor || responseMinor < minSupportedEventVersionMinor {
		return &RawEventDetails{}, fmt.Errorf("unexpected event version (got %s, expected compatibility with %d.%d)", res.EventVersion, supportedEventVersionMajor, minSupportedEventVersionMinor)
	}
	return &res, nil
}

// GenerateLink generates a hyperlink for the given URL and display text based of value pairs in cloudTrail Event.
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
func filterUsers(lookupOutputs []*cloudtrail.LookupEventsOutput, Ignore []string, shouldFilter bool) (*[]types.Event, error) {
	filteredEvents := []types.Event{}
	mergedRegex := mergeRegex(Ignore)

	for _, lookupOutput := range lookupOutputs {
		for _, event := range lookupOutput.Events {
			raw, err := extractUserDetails(event.CloudTrailEvent)
			if err != nil {
				return nil, err
			}
			userArn := raw.UserIdentity.SessionContext.SessionIssuer.Arn
			regexOdj := regexp.MustCompile(mergedRegex)
			matchesUsrname := false
			matchesArn := false

			if shouldFilter {
				if event.Username != nil {
					matchesUsrname = regexOdj.MatchString(*event.Username)
				}
				if userArn != "" {
					matchesArn = regexOdj.MatchString(userArn)

				}

				if matchesUsrname || matchesArn {
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
	for _, event := range filteredEvent {

		if raw {
			if event.CloudTrailEvent != nil {
				fmt.Printf("\n%s	\n", *event.CloudTrailEvent)
			}
		} else {
			if event.EventName != nil {
				fmt.Printf("%s |", *event.EventName)
			} else {
				fmt.Println("<not available> |")
			}

			if event.EventTime != nil {
				fmt.Printf("%s |", event.EventTime.String())
			} else {
				fmt.Println("<not available> |")
			}

			if event.Username != nil {
				fmt.Printf("User: %s |\n", *event.Username)
			} else {
				fmt.Println("User: <not available> |")
			}

			if printUrl && event.CloudTrailEvent != nil {
				details, err := extractUserDetails(event.CloudTrailEvent)
				if err == nil {
					fmt.Printf("EventLink: %s\n\n", generateLink(*details))
				} else {
					fmt.Println("EventLink: <not available>")
				}
			}

		}

	}

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
		return errors.New("this command is only available for AWS clusters")
	}

	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	cloudtrailClient := cloudtrail.NewFromConfig(cfg)
	arn, accountId, err := whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

	starttime, err := parseDurationToUTC(o.startTime)
	if err != nil {
		return err
	}

	lookupOutput, err := getWriteEvents(starttime, cloudtrailClient)
	if err != nil {
		return err
	}
	Ignore, err := config.LoadCTConfig()
	if err != nil {
		return err
	}

	fmt.Printf("Checking write event history since %s for AWS Account %s as %s \n", starttime, accountId, arn)

	fmt.Printf("\n[+] Fetching %s Event History...\n", cfg.Region)

	filteredEvents, err := filterUsers(lookupOutput, Ignore, o.printAllEvents)
	if err != nil {
		return err
	}

	printEvents(*filteredEvents, o.printEventUrl, o.printRawEvents)

	fmt.Println("")
	return err
}
