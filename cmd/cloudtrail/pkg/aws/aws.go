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
func GetEvents(since time.Time, cloudtailClient *cloudtrail.Client) ([]*cloudtrail.LookupEventsOutput, error) {
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
