package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

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
	ErrorCode   string `json:"errorCode"`
}

type QueryOptions struct {
	StartTime time.Time
}

// Extracts Raw cloudtrailEvent Details
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

// whoami retrieves caller identity information
func Whoami(stsClient sts.Client) (accountArn string, accountId string, err error) {
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
func GetEvents(cloudtailClient *cloudtrail.Client, startTime time.Time, writeOnly bool, filters map[string]string) ([]types.Event, error) {

	alllookupEvents := []types.Event{}
	input := cloudtrail.LookupEventsInput{
		StartTime: &startTime,
		EndTime:   aws.Time(time.Now()),
	}

	if writeOnly {
		input.LookupAttributes = []types.LookupAttribute{
			{AttributeKey: "ReadOnly",
				AttributeValue: aws.String("false")},
		}
	}

	fmt.Println("")
	fmt.Printf("testing %v", input.LookupAttributes)
	paginator := cloudtrail.NewLookupEventsPaginator(cloudtailClient, &input, func(c *cloudtrail.LookupEventsPaginatorOptions) {})
	for paginator.HasMorePages() {

		lookupOutput, err := paginator.NextPage(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("[WARNING] paginator error: \n%w", err)
		}
		alllookupEvents = append(alllookupEvents, lookupOutput.Events...)

		input.NextToken = lookupOutput.NextToken
		if lookupOutput.NextToken == nil {
			break
		}

	}
	/*
		if userName != "" {
			filteredEvents := []types.Event{}
			for _, event := range alllookupEvents {
				if event.Username != nil && *event.Username == userName {
					filteredEvents = append(filteredEvents, event)
				}
			}
			if len(filteredEvents) == 0 {
				fmt.Printf("\nNo events found for user %s", userName)
			}
			alllookupEvents = filteredEvents
		}

		if event != "" {
			filteredEvents := []types.Event{}
			for _, events := range alllookupEvents {
				if events.EventName != nil && *events.EventName == event {
					filteredEvents = append(filteredEvents, events)
				}
			}
			if len(filteredEvents) == 0 {
				fmt.Printf("\nNo events found for %s", event)
			}
			alllookupEvents = filteredEvents
		}

		if resourceName != "" {
			filteredEvents := []types.Event{}
			for _, event := range alllookupEvents {
				for _, resource := range event.Resources {
					if resource.ResourceName != nil && *resource.ResourceName == resourceName {
						filteredEvents = append(filteredEvents, event)
						break // Stop checking other resources for this event
					}
				}
			}

			if len(filteredEvents) == 0 {
				fmt.Printf("\nNo events found for resource name %s\n", resourceName)
			}

			alllookupEvents = filteredEvents
		}

		if resourceType != "" {
			filteredEvents := []types.Event{}
			for _, event := range alllookupEvents {
				for _, resource := range event.Resources {
					if resource.ResourceType != nil && *resource.ResourceType == resourceType {
						filteredEvents = append(filteredEvents, event)
						break // Stop checking other resources for this event

					}
				}
			}

			if len(filteredEvents) == 0 {
				fmt.Printf("\nNo events found for resource type %s\n", resourceType)
			}

			alllookupEvents = filteredEvents
		}
	*/

	alllookupEvents = Filters(filters, alllookupEvents)

	return alllookupEvents, nil
}

func Filters(filters map[string]string, alllookupEvents []types.Event) (results []types.Event) {

	for k, v := range filters {
		if v != "" {
			filteredEvents := []types.Event{}
			for _, event := range alllookupEvents {
				switch k {
				case "username":
					if event.Username != nil && *event.Username == v {
						filteredEvents = append(filteredEvents, event)
					}
				case "event":
					if event.EventName != nil && *event.EventName == v {
						filteredEvents = append(filteredEvents, event)
					}
				case "resourceName":
					for _, resource := range event.Resources {
						if resource.ResourceName != nil && *resource.ResourceName == v {
							filteredEvents = append(filteredEvents, event)
							break // Stop checking other resources for this event
						}
					}
				case "resourceType":
					for _, resource := range event.Resources {
						if resource.ResourceType != nil && *resource.ResourceType == v {
							filteredEvents = append(filteredEvents, event)
							break // Stop checking other resources for this event
						}
					}
				}
			}
			if len(filteredEvents) == 0 {
				fmt.Printf("\nNo events found for %s with value: %s", k, v)
			}
			alllookupEvents = filteredEvents
		}
	}
	return alllookupEvents
}

func GetEventsP(cloudtailClient *cloudtrail.Client, startTime time.Time, writeOnly bool) ([]types.Event, error) {

	alllookupEvents := []types.Event{}
	input := cloudtrail.LookupEventsInput{
		StartTime: &startTime,
		EndTime:   aws.Time(time.Now()),
	}

	if writeOnly {
		input.LookupAttributes = []types.LookupAttribute{
			{AttributeKey: "ReadOnly",
				AttributeValue: aws.String("false")},
		}
	}

	paginator := cloudtrail.NewLookupEventsPaginator(cloudtailClient, &input, func(c *cloudtrail.LookupEventsPaginatorOptions) {})
	for paginator.HasMorePages() {

		lookupOutput, err := paginator.NextPage(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("[WARNING] paginator error: \n%w", err)
		}
		alllookupEvents = append(alllookupEvents, lookupOutput.Events...)

		input.NextToken = lookupOutput.NextToken
		if lookupOutput.NextToken == nil {
			break
		}

	}

	return alllookupEvents, nil
}
