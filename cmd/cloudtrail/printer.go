package cloudtrail

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
)

// Printer struct handles the formatting and output of CloudTrail events.
type Printer struct {
	printUrl bool
	printRaw bool
}

// NewPrinter creates a new Printer instance with the specified output options.
// Parameters:
//   - printUrl: If true, generates and includes AWS Console links for events
//   - printRaw: If true, displays events in raw JSON format
func NewPrinter(printUrl, printRaw bool) *Printer {
	return &Printer{
		printUrl: printUrl,
		printRaw: printRaw,
	}
}

// PrintEvents prints the filtered CloudTrail events in a human-readable format.
// Allows to print cloudtrail event url link or its raw JSON format.
// Allows to print cloutrail event resource name & type.
func (o *Printer) PrintEvents(filterEvents []types.Event, printFields []string) {
	var eventStringBuilder = strings.Builder{}
	tableFilter := map[string]struct{}{}

	for _, field := range printFields {
		tableFilter[field] = struct{}{}
	}

	for i := range filterEvents {
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

		if o.printUrl && filterEvents[i].CloudTrailEvent != nil {
			if err == nil {
				eventStringBuilder.WriteString(fmt.Sprintf("%v", generateLink(*rawEventDetails)))
			} else {
				fmt.Println("EventLink: <not available>")
			}
		}

	}
	fmt.Print(eventStringBuilder.String())
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
