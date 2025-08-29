package cloudtrail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
)

// RawEventDetails represents the structure of relevant fields extracted from a CloudTrail event JSON.
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
	ErrorCode   string `json:"errorCode"`
}

type EventResult struct {
	AWSEvent []types.Event
	errors   error
}

type EventAPI struct {
	client    *cloudtrail.Client
	writeOnly bool
}

func NewEventAPI(cfg aws.Config, writeOnly bool, region string) *EventAPI {
	var client *cloudtrail.Client

	if region != "" {
		client = cloudtrail.New(cloudtrail.Options{
			Region:      region,
			Credentials: cfg.Credentials,
			HTTPClient:  cfg.HTTPClient,
		})
	} else {
		client = cloudtrail.NewFromConfig(cfg)
	}

	return &EventAPI{
		client:    client,
		writeOnly: writeOnly,
	}
}

func (a *EventAPI) GetEvents(clusterID string, missing Period) <-chan EventResult {
	var alllookupEvents []types.Event

	pageChan := make(chan EventResult)

	input := cloudtrail.LookupEventsInput{
		StartTime: &missing.StartTime,
		EndTime:   &missing.EndTime,
	}

	if a.writeOnly {
		input.LookupAttributes = []types.LookupAttribute{
			{AttributeKey: "ReadOnly",
				AttributeValue: aws.String("false")},
		}
	}
	paginator := cloudtrail.NewLookupEventsPaginator(a.client, &input, func(c *cloudtrail.LookupEventsPaginatorOptions) {})

	go func() {
		defer close(pageChan)

		for paginator.HasMorePages() {
			lookupOutput, err := paginator.NextPage(context.Background())
			if err != nil {
				pageChan <- EventResult{
					AWSEvent: nil,
					errors:   err,
				}
			}
			alllookupEvents = append(alllookupEvents, lookupOutput.Events...)

			pageChan <- EventResult{
				AWSEvent: lookupOutput.Events,
				errors:   nil,
			}

		}
	}()

	return pageChan
}

// ExtractUserDetails parses a CloudTrail event JSON string and extracts user identity details.
func ExtractUserDetails(cloudTrailEvent *string) (*RawEventDetails, error) {
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

// PrintEvents prints the filtered CloudTrail events in a human-readable format.
// Allows to print cloudtrail event url link or its raw JSON format.
// Allows to print cloutrail event resource name & type.
func PrintEvents(filterEvents []types.Event, printUrl bool, printRaw bool) {
	var eventStringBuilder = strings.Builder{}

	for i := len(filterEvents) - 1; i >= 0; i-- {
		if printRaw {
			if filterEvents[i].CloudTrailEvent != nil {
				fmt.Printf("%v \n", *filterEvents[i].CloudTrailEvent)
				return
			}
		}
		rawEventDetails, err := ExtractUserDetails(filterEvents[i].CloudTrailEvent)
		if err != nil {
			fmt.Printf("[Error] Error extracting event details: %v", err)
		}
		sessionIssuer := rawEventDetails.UserIdentity.SessionContext.SessionIssuer.UserName
		if filterEvents[i].EventName != nil {
			eventStringBuilder.WriteString(fmt.Sprintf("\n%v", *filterEvents[i].EventName))
		}
		if filterEvents[i].EventTime != nil {
			eventStringBuilder.WriteString(fmt.Sprintf(" | %v", filterEvents[i].EventTime.String()))
		}
		if filterEvents[i].Username != nil {
			eventStringBuilder.WriteString(fmt.Sprintf(" | Username: %v", *filterEvents[i].Username))
		}
		if sessionIssuer != "" {
			eventStringBuilder.WriteString(fmt.Sprintf(" | ARN: %v", sessionIssuer))
		}

		for _, resource := range filterEvents[i].Resources {
			if resource.ResourceName != nil {
				eventStringBuilder.WriteString(fmt.Sprintf("| Resource Name: %v", *resource.ResourceName))
			}
			if resource.ResourceType != nil {
				eventStringBuilder.WriteString(fmt.Sprintf(" | Resource Type: %v", *resource.ResourceType))
			}
		}

		if printUrl && filterEvents[i].CloudTrailEvent != nil {
			if err == nil {
				eventStringBuilder.WriteString(fmt.Sprintf("\n%v |", generateLink(*rawEventDetails)))
			} else {
				fmt.Println("EventLink: <not available>")
			}
		}

	}
	fmt.Println(eventStringBuilder.String())
}

// PrintFormat allows the user to specify which fields to print.
// Allows to print cloudtrail event url link
func PrintFormat(filterEvents []types.Event, printUrl bool, printRaw bool, table []string) {
	var eventStringBuilder = strings.Builder{}
	tableFilter := map[string]struct{}{}

	for _, field := range table {
		tableFilter[field] = struct{}{}
	}

	for i := len(filterEvents) - 1; i >= 0; i-- {

		rawEventDetails, err := ExtractUserDetails(filterEvents[i].CloudTrailEvent)
		if err != nil {
			fmt.Printf("[Error] Error extracting event details: %v", err)
		}
		sessionIssuer := rawEventDetails.UserIdentity.SessionContext.SessionIssuer.UserName
		eventStringBuilder.WriteString("\n")
		if _, ok := tableFilter["event"]; ok && filterEvents[i].EventName != nil {
			eventStringBuilder.WriteString(fmt.Sprintf("%v | ", *filterEvents[i].EventName))
		}
		if _, ok := tableFilter["time"]; ok && filterEvents[i].EventTime != nil {
			eventStringBuilder.WriteString(fmt.Sprintf("%v | ", filterEvents[i].EventTime.String()))
		}
		if _, ok := tableFilter["username"]; ok && filterEvents[i].Username != nil {
			eventStringBuilder.WriteString(fmt.Sprintf("Username: %v | ", *filterEvents[i].Username))
		}
		if _, ok := tableFilter["arn"]; ok && sessionIssuer != "" {
			eventStringBuilder.WriteString(fmt.Sprintf("ARN: %v | ", sessionIssuer))
		}

		for _, resource := range filterEvents[i].Resources {
			if _, ok := tableFilter["resource-name"]; ok && resource.ResourceName != nil {
				eventStringBuilder.WriteString(fmt.Sprintf("Resource Name: %v | ", *resource.ResourceName))
			}
			if _, ok := tableFilter["resource-type"]; ok && resource.ResourceType != nil {
				eventStringBuilder.WriteString(fmt.Sprintf("Resource Type: %v | ", *resource.ResourceType))
			}
		}

		if printUrl && filterEvents[i].CloudTrailEvent != nil {
			if err == nil {
				eventStringBuilder.WriteString(fmt.Sprintf("%v", generateLink(*rawEventDetails)))
			} else {
				fmt.Println("EventLink: <not available>")
			}
		}

	}
	fmt.Println(eventStringBuilder.String())
}
