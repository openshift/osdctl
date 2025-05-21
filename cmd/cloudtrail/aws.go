package cloudtrail

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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

// QueryOptions defines the start time for querying CloudTrail events.
type QueryOptions struct {
	StartTime time.Time
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

// Whoami retrieves the AWS account ARN and account ID for the current caller
// using the provided STS client.
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
<<<<<<< HEAD:cmd/cloudtrail/aws.go
=======

// getWriteEvents retrieves cloudtrail events since the specified time
// using the provided cloudtrail client and starttime from since flag.
func GetEvents(cloudtailClient *cloudtrail.Client, startTime time.Time, writeOnly bool, userName string) ([]types.Event, error) {

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
	/*
		if userName != "" {
			input.LookupAttributes = append(input.LookupAttributes, types.LookupAttribute{
				AttributeKey:   "Username",
				AttributeValue: aws.String(userName),
			})
		}

		fmt.Println("")
		fmt.Printf("testing %v", input.LookupAttributes)
	*/
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

	// If a username is provided, filter the results by username
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

	return alllookupEvents, nil
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
>>>>>>> 1cdcb16 (ADD: Username Filter for OSDCTL Write Events):cmd/cloudtrail/pkg/aws/aws.go
