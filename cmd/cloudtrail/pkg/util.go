package pkg

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	pkg "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
)

type Filter func(*cloudtrail.LookupEventsOutput) (bool, error)
type FilterBulk func([]*cloudtrail.LookupEventsOutput) []*cloudtrail.LookupEventsOutput

func ApplyFilters(records []*cloudtrail.LookupEventsOutput, filters ...Filter) ([]*cloudtrail.LookupEventsOutput, error) {
	if len(filters) == 0 {
		return records, nil
	}

	filteredRecords := make([]*cloudtrail.LookupEventsOutput, 0, len(records))
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

func ApplyBulkFilters(records []*cloudtrail.LookupEventsOutput, filters ...FilterBulk) []*cloudtrail.LookupEventsOutput {
	for _, f := range filters {
		records = f(records)
	}
	return records
}

type FilterSet func([]*cloudtrail.LookupEventsOutput, string) []*cloudtrail.LookupEventsOutput

var Filters = map[int]FilterSet{
	1: FilterForUnauthoraizedUsers,
	2: FilterForSearch,
	3: FilterForIgnoreList,
}

func FilterForUnauthoraizedUsers(events []*cloudtrail.LookupEventsOutput, value string) []*cloudtrail.LookupEventsOutput {
	filteredEvents, _ := ApplyFilters(events,
		func(event *cloudtrail.LookupEventsOutput) (bool, error) {
			return ForbiddenEvents(event, value)
		},
	)

	return ApplyBulkFilters(filteredEvents)

}

func FilterForSearch(events []*cloudtrail.LookupEventsOutput, value string) []*cloudtrail.LookupEventsOutput {
	filteredEvents, _ := ApplyFilters(events,
		func(event *cloudtrail.LookupEventsOutput) (bool, error) {
			return FilterByArnAndUsername(event, value)
		},
	)
	return ApplyBulkFilters(filteredEvents)
}

func FilterForIgnoreList(events []*cloudtrail.LookupEventsOutput, value string) []*cloudtrail.LookupEventsOutput {
	filteredEvents, _ := ApplyFilters(events,
		func(event *cloudtrail.LookupEventsOutput) (bool, error) {
			return FilterByIgnorelist(event, value)
		},
	)
	return ApplyBulkFilters(filteredEvents)
}

// PrintEvents prints the details of each event in the provided slice of events.
// It takes a slice of types.Event
func PrintEvents(filteredEvent []*cloudtrail.LookupEventsOutput, printUrl bool, raw bool) {
	var eventStringBuilder = strings.Builder{}
	for _, cloudtrailOutput := range filteredEvent {
		for i := len(cloudtrailOutput.Events) - 1; i >= 0; i-- {
			event := cloudtrailOutput.Events[i]

			if raw {
				if event.CloudTrailEvent != nil {
					fmt.Printf("%v \n", *event.CloudTrailEvent)
					return
				}
			}
			rawEventDetails, err := pkg.ExtractUserDetails(event.CloudTrailEvent)
			if err != nil {
				fmt.Printf("[Error] Error extracting event details: %v", err)
			}
			sessionIssuer := rawEventDetails.UserIdentity.SessionContext.SessionIssuer.UserName
			if event.EventName != nil {
				eventStringBuilder.WriteString(fmt.Sprintf("\n%v", *event.EventName))
			}
			if event.EventTime != nil {
				eventStringBuilder.WriteString(fmt.Sprintf(" | %v", event.EventTime.String()))
			}
			if event.Username != nil {
				eventStringBuilder.WriteString(fmt.Sprintf(" | Username: %v", *event.Username))
			}
			if sessionIssuer != "" {
				eventStringBuilder.WriteString(fmt.Sprintf(" | ARN: %v", sessionIssuer))
			}

			if printUrl && event.CloudTrailEvent != nil {
				if err == nil {
					eventStringBuilder.WriteString(fmt.Sprintf("\n%v |", generateLink(*rawEventDetails)))
				} else {
					fmt.Println("EventLink: <not available>")
				}
			}

		}
		fmt.Println(eventStringBuilder.String())

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
func MergeRegex(regexlist []string) string {
	return strings.Join(regexlist, "|")
}

// FilterUsers filters out events based on the specified Ignore list, which contains
// regular expression patterns. It takes a slice of cloudtrail.LookupEventsOutput,
// which represents the output of AWS CloudTrail lookup events operation, and a list
// of regular expression patterns to ignore.
func FilterByIgnorelist(lookupOutput *cloudtrail.LookupEventsOutput, mergedRegex string) (bool, error) {
	for _, event := range lookupOutput.Events {
		raw, err := pkg.ExtractUserDetails(event.CloudTrailEvent)
		if err != nil {
			return false, fmt.Errorf("[ERROR] failed to extract raw cloudtrailEvent details: %w", err)
		}
		userArn := raw.UserIdentity.SessionContext.SessionIssuer.Arn
		regexObj := regexp.MustCompile(mergedRegex)
		if mergedRegex == "" {
			return true, nil
		}

		if event.Username != nil && regexObj.MatchString(*event.Username) {
			return false, nil
		}

		if userArn != "" && regexObj.MatchString(userArn) {
			return false, nil
		}
	}

	return true, nil
}

// Find function

func ForbiddenEvents(lookupOutput *cloudtrail.LookupEventsOutput, value string) (bool, error) {
	if value == "" {
		return false, nil
	}

	check, err := regexp.Compile(value)
	if err != nil {
		return false, fmt.Errorf("failed to compile regex: %w", err)
	}

	for _, event := range lookupOutput.Events {
		raw, err := pkg.ExtractUserDetails(event.CloudTrailEvent)
		if err != nil {
			return false, fmt.Errorf("[ERROR] failed to extract raw CloudTrail event details: %w", err)
		}
		errorCode := raw.ErrorCode
		if errorCode != "" && check.MatchString(errorCode) {
			return true, nil
		}
	}
	return false, nil
}

func FilterByArnAndUsername(lookupOutput *cloudtrail.LookupEventsOutput, value string) (bool, error) {
	if value == "" {
		return true, nil
	}

	regexPattern := fmt.Sprintf(".*%v.*", regexp.QuoteMeta(value))
	search, err := regexp.Compile(regexPattern)
	if err != nil {
		return false, fmt.Errorf("failed to compile regex: %w", err)
	}

	for _, event := range lookupOutput.Events {
		raw, err := pkg.ExtractUserDetails(event.CloudTrailEvent)
		if err != nil {
			return false, fmt.Errorf("[ERROR] failed to extract raw CloudTrail event details: %w", err)
		}
		sessionIssuerArn := raw.UserIdentity.SessionContext.SessionIssuer.Arn
		if sessionIssuerArn != "" {
			if search.MatchString(sessionIssuerArn) {
				return true, nil
			}
		}
		if event.Username != nil {
			if search.MatchString(*event.Username) {
				return true, nil
			}
		}
	}

	return false, nil
}
