package pkg

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	pkg "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
)

// PrintEvents prints the details of each event in the provided slice of events.
// It takes a slice of types.Event
func PrintEvents(filteredEvent []types.Event, printUrl bool, raw bool) {
	for _, event := range filteredEvent {
		if raw {
			if event.CloudTrailEvent != nil {
				fmt.Printf("%v \n\n", *event.CloudTrailEvent)
			}
		} else {
			rawEventDetails, err := pkg.ExtractUserDetails(event.CloudTrailEvent)
			if err != nil {
				fmt.Printf("[Error] Error extracting event details: %v", err)
			}
			sessionIssuer := rawEventDetails.UserIdentity.SessionContext.SessionIssuer.UserName
			if event.EventName != nil {
				fmt.Printf("%v ", *event.EventName)
			} else {
				continue
			}

			if event.EventTime != nil {
				fmt.Printf("|%v ", event.EventTime.String())
				if event.Username != nil {
					fmt.Printf("| User: %v ", *event.Username)
				}
				if sessionIssuer != "" {
					fmt.Printf("| ARN: %v\n", sessionIssuer)
				} else {
					fmt.Print("\n")
				}

				if printUrl && event.CloudTrailEvent != nil {
					if err == nil {
						fmt.Printf("EventLink: %v\n\n", generateLink(*rawEventDetails))
					} else {
						fmt.Println("EventLink: <not available>")
					}
				}

			}
		}
	}

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
func mergeRegex(regexlist []string) string {
	return strings.Join(regexlist, "|")
}

// FilterUsers filters out events based on the specified Ignore list, which contains
// regular expression patterns. It takes a slice of cloudtrail.LookupEventsOutput,
// which represents the output of AWS CloudTrail lookup events operation, and a list
// of regular expression patterns to ignore.
func FilterUsers(lookupOutputs []*cloudtrail.LookupEventsOutput, Ignore []string, allEvents bool) (*[]types.Event, error) {
	filteredEvents := []types.Event{}
	mergedRegex := mergeRegex(Ignore)

	for _, lookupOutput := range lookupOutputs {
		for _, event := range lookupOutput.Events {

			raw, err := pkg.ExtractUserDetails(event.CloudTrailEvent)
			if err != nil {
				return nil, fmt.Errorf("[ERROR] failed to to extract raw cloudtrailEvent details: %w", err)
			}
			userArn := raw.UserIdentity.SessionContext.SessionIssuer.Arn
			regexOdj := regexp.MustCompile(mergedRegex)
			matchesUsername := false
			matchesArn := false

			if !allEvents && len(Ignore) != 0 {
				if event.Username != nil {
					matchesUsername = regexOdj.MatchString(*event.Username)
				}
				if userArn != "" {
					matchesArn = regexOdj.MatchString(userArn)

				}

				if matchesUsername || matchesArn {
					continue
					// skips entry
				}

			}
			filteredEvents = append(filteredEvents, event)

		}
	}

	return &filteredEvents, nil
}
