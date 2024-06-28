package pkg

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	pkg "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
)

type Filter func(types.Event) (bool, error)

func ApplyFilters(records []types.Event, filters ...Filter) ([]types.Event, error) {
	if len(filters) == 0 {
		return records, nil
	}

	filteredRecords := make([]types.Event, 0, len(records))
	for _, r := range records {
		keep := true
		for _, f := range filters {
			filtered, err := f(r)
			if err != nil {
				return nil, err
			}
			if !filtered {
				keep = false
				break
			}
		}

		if keep {
			filteredRecords = append(filteredRecords, r)
		}
	}

	return filteredRecords, nil
}

// PrintEvents prints the details of each event in the provided slice of events.
// It takes a slice of types.Event
func PrintEvents(filterEvents []types.Event, printUrl bool, printRaw bool) {
	var eventStringBuilder = strings.Builder{}

	for i := len(filterEvents) - 1; i >= 0; i-- {
		if printRaw {
			if filterEvents[i].CloudTrailEvent != nil {
				fmt.Printf("%v \n", *filterEvents[i].CloudTrailEvent)
				return
			}
		}
		rawEventDetails, err := pkg.ExtractUserDetails(filterEvents[i].CloudTrailEvent)
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

// generateLink generates a hyperlink to aws cloudTrail event.
func generateLink(raw pkg.RawEventDetails) (url_link string) {
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
func ParseDurationToUTC(input string) (time.Time, error) {
	duration, err := time.ParseDuration(input)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse time duration: %w", err)
	}

	return time.Now().UTC().Add(-duration), nil
}

// Join all individual patterns into a single string separated by the "|" operator
func MergeRegex(regexlist []string) string {
	return strings.Join(regexlist, "|")
}
