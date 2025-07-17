package cloudtrail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
)

type pageProcessor func(events []types.Event) error

// GetEvents etrieve CloudTrail events using the provided client and time range.
// It paginates through all available events, and returns all.
func GetEvents(cloudtailClient *cloudtrail.Client, startTime time.Time, endTime time.Time, writeOnly bool) ([]types.Event, error) {

	alllookupEvents := []types.Event{}

	input := cloudtrail.LookupEventsInput{
		StartTime: &startTime,
		EndTime:   &endTime,
	}

	if writeOnly {
		input.LookupAttributes = []types.LookupAttribute{
			{AttributeKey: "ReadOnly",
				AttributeValue: aws.String("false")},
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
	}

	return alllookupEvents, nil
}

func GetEventsByPage(cloudtailClient *cloudtrail.Client, startTime time.Time, endTime time.Time, writeOnly bool, processor pageProcessor) error {

	input := cloudtrail.LookupEventsInput{
		StartTime: &startTime,
		EndTime:   &endTime,
	}

	if writeOnly {
		input.LookupAttributes = []types.LookupAttribute{
			{AttributeKey: "ReadOnly",
				AttributeValue: aws.String("false")},
		}

		paginator := cloudtrail.NewLookupEventsPaginator(cloudtailClient, &input, func(c *cloudtrail.LookupEventsPaginatorOptions) {})
		for paginator.HasMorePages() {
			lookupOutput, err := paginator.NextPage(context.TODO())
			if err != nil {
				return fmt.Errorf("[WARNING] paginator error on page: %w", err)
			}

			if len(lookupOutput.Events) == 0 {
				continue
			}

			// Process this page using the callback
			if err := processor(lookupOutput.Events); err != nil {
				return fmt.Errorf("error processing page: %w", err)
			}

		}
	}
	return nil
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

// generateLink generates a hyperlink to aws cloudTrail event
// based on the provided RawEventDetails.
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

// parseDurationToUTC parses the given startTime string as a duration and subtracts it from the current UTC time.
// It returns the resulting time and any parsing error encountered.
func ParseDurationToUTC(input string, referenceTime time.Time, isForward ...bool) (time.Time, error) {
	duration, err := time.ParseDuration(input)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse time duration: %w", err)
	}
	if referenceTime.IsZero() {
		referenceTime = time.Now().UTC()
	}
	if isForward[0] {
		return referenceTime.Add(duration), nil
	}

	return referenceTime.Add(-duration), nil
}

// parseTimeAndValidate takes YY-MM-DD,hh:mm:ss format, splits the year and time and convert it to current UTC time.
// It returns the parsed time and any parsing error encountered.
func ParseTimeAndValidate(timeStr string) (time.Time, error) {
	parts := strings.Split(timeStr, ",")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format. Expected format: YYYY-MM-DD,HH:MM:SS")
	}

	formattedTimeStr := parts[0] + " " + parts[1]
	layout := "2006-01-02 15:04:05"
	parsedTime, err := time.Parse(layout, formattedTimeStr)

	if err != nil {
		return time.Time{}, err
	}
	return parsedTime.UTC(), nil
}

func ParseStartEndTime(start, end, duration string) (time.Time, time.Time, error) {
	var startTime, endTime time.Time
	var err error
	if start != "" && end != "" {
		if startTime, err = ParseTimeAndValidate(start); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Time Format Incorrect: %w", err)
		}
		if endTime, err = ParseTimeAndValidate(end); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Time Format Incorrect: %w", err)
		}
	} else if start != "" {
		if startTime, err = ParseTimeAndValidate(start); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Time Format Incorrect: %w", err)
		}
		if endTime, err = ParseDurationToUTC(duration, startTime, true); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Failed to parse --since: %w", err)
		}
	} else if end != "" {
		if endTime, err = ParseTimeAndValidate(end); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Time Format Incorrect: %w", err)
		}
		if startTime, err = ParseDurationToUTC(duration, endTime, false); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Failed to parse --since: %w", err)
		}
	}

	return startTime, endTime, nil
}

// ValidateTable checks for the string list given and returns error
// if it does not match.
func ValidateFormat(table []string) error {
	allowedKeys := map[string]struct{}{
		"username":      {},
		"event":         {},
		"resource-name": {},
		"resource-type": {},
		"arn":           {},
		"time":          {},
	}

	for _, column := range table {
		if _, ok := allowedKeys[strings.ToLower(column)]; !ok {
			return fmt.Errorf("invalid table column: %s (allowed: username, event, resource-name, resource-type, arn, time, url, region)", column)
		}
	}

	return nil
}
