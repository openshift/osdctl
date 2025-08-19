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
