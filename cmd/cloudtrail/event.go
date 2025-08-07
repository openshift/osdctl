package cloudtrail

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
)

type EventResult struct {
	AWSEvent []types.Event
	errors   error
}

type EventAPI struct {
	client    *cloudtrail.Client
	writeOnly bool
}

func NewCloudtrailClient(cfg aws.Config, writeOnly bool) *EventAPI {
	return &EventAPI{
		client:    cloudtrail.NewFromConfig(cfg),
		writeOnly: writeOnly,
	}
}

func NewCloudTrailClientWithOptions(cfg aws.Config, region string) *cloudtrail.Client {
	return cloudtrail.New(cloudtrail.Options{
		Region:      region,
		Credentials: cfg.Credentials,
		HTTPClient:  cfg.HTTPClient,
	})
}

// NewEventAPI
// (cfg aws.Config, writeOnly bool)

func (a *EventAPI) GetEvents(clusterID string, missingPeriod []Period) <-chan EventResult {
	var alllookupEvents []types.Event

	pageChan := make(chan EventResult)

	for _, missing := range missingPeriod {

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
	}

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
