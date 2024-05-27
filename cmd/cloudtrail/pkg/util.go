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

type Filter func(types.Event) (bool, error)
type FilterBulk func([]types.Event) []types.Event

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

type FilterSet func([]*cloudtrail.LookupEventsOutput, string) *[]types.Event

var Filters = map[int]FilterSet{
	1: FilterForUnauthoraizedUsers,
	2: FilterForIgnoreList,
}

func FilterForUnauthoraizedUsers(cloudtrailOutput []*cloudtrail.LookupEventsOutput, value string) *[]types.Event {

	filteredEvents := []types.Event{}
	for _, output := range cloudtrailOutput {
		events, _ := ApplyFilters(output.Events,
			func(event types.Event) (bool, error) {
				return ForbiddenEvents(event, value)
			},
		)
		filteredEvents = append(filteredEvents, events...)

	}
	return &filteredEvents

}

func FilterForIgnoreList(cloudtrailOutput []*cloudtrail.LookupEventsOutput, value string) *[]types.Event {
	filteredEvents := []types.Event{}
	for _, output := range cloudtrailOutput {
		events, _ := ApplyFilters(output.Events,
			func(event types.Event) (bool, error) {
				return FilterByIgnorelist(event, value)
			},
		)
		filteredEvents = append(filteredEvents, events...)

	}
	return &filteredEvents
}

// PrintEvents prints the details of each event in the provided slice of events.
// It takes a slice of types.Event
// PrintEvents prints the details of each event in the provided slice of events.
// It takes a slice of types.Event
func PrintEvents(filteredEvent []types.Event, printUrl bool, raw bool) {
	var eventStringBuilder = strings.Builder{}

	for i := len(filteredEvent) - 1; i >= 0; i-- {
		if raw {
			if filteredEvent[i].CloudTrailEvent != nil {
				fmt.Printf("%v \n", *filteredEvent[i].CloudTrailEvent)
				return
			}
		}
		rawEventDetails, err := pkg.ExtractUserDetails(filteredEvent[i].CloudTrailEvent)
		if err != nil {
			fmt.Printf("[Error] Error extracting event details: %v", err)
		}
		sessionIssuer := rawEventDetails.UserIdentity.SessionContext.SessionIssuer.UserName
		if filteredEvent[i].EventName != nil {
			eventStringBuilder.WriteString(fmt.Sprintf("\n%v", *filteredEvent[i].EventName))
		}
		if filteredEvent[i].EventTime != nil {
			eventStringBuilder.WriteString(fmt.Sprintf(" | %v", filteredEvent[i].EventTime.String()))
		}
		if filteredEvent[i].Username != nil {
			eventStringBuilder.WriteString(fmt.Sprintf(" | Username: %v", *filteredEvent[i].Username))
		}
		if sessionIssuer != "" {
			eventStringBuilder.WriteString(fmt.Sprintf(" | ARN: %v", sessionIssuer))
		}

		if printUrl && filteredEvent[i].CloudTrailEvent != nil {
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

// FilterByIgnorelist filters out events based on the specified ignore list, which contains
// regular expression patterns. It returns true if the event should be kept, and false if it should be filtered out.
func FilterByIgnorelist(event types.Event, mergedRegex string) (bool, error) {
	if mergedRegex == "" {
		return true, nil
	}
	raw, err := pkg.ExtractUserDetails(event.CloudTrailEvent)
	if err != nil {
		return true, fmt.Errorf("[ERROR] failed to extract raw CloudTrail event details: %w", err)
	}
	userArn := raw.UserIdentity.SessionContext.SessionIssuer.Arn
	regexObj := regexp.MustCompile(mergedRegex)

	if event.Username != nil {
		if regexObj.MatchString(*event.Username) {
			return false, nil
		}
	}
	if userArn != "" {

		if regexObj.MatchString(userArn) {

			return false, nil
		}
	}
	if userArn == "" && event.Username == nil {
		return false, nil
	}

	return true, nil
}

func ForbiddenEvents(event types.Event, value string) (bool, error) {

	if value == "" {
		return false, nil
	}

	check, err := regexp.Compile(value)
	if err != nil {
		return false, fmt.Errorf("failed to compile regex: %w", err)
	}
	raw, err := pkg.ExtractUserDetails(event.CloudTrailEvent)
	if err != nil {
		return false, fmt.Errorf("[ERROR] failed to extract raw CloudTrail event details: %w", err)
	}
	errorCode := raw.ErrorCode
	if errorCode != "" && check.MatchString(errorCode) {
		return true, nil
	}

	return false, nil
}
