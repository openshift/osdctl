package pkg

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	pkg "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
	"github.com/stretchr/testify/assert"
)

func filterByEventName(eventName string) Filter {
	return func(e types.Event) (bool, error) {
		if e.EventName != nil {
			return *e.EventName == eventName, nil
		}
		return false, nil
	}
}

func filterByUsername(username string) Filter {
	return func(e types.Event) (bool, error) {
		if e.Username != nil {
			return *e.Username == username, nil
		}
		return false, nil
	}
}

func capturePrintEventsOutput(filterEvents []types.Event, printUrl bool, printRaw bool) string {
	var eventStringBuilder strings.Builder
	for i := 0; i < len(filterEvents); i++ {
		if printRaw && filterEvents[i].CloudTrailEvent != nil {
			eventStringBuilder.WriteString(fmt.Sprintf("EventName: %v \n", *filterEvents[i].EventName))
			eventStringBuilder.WriteString(fmt.Sprintf("%v \n", *filterEvents[i].CloudTrailEvent))
		} else {
			if filterEvents[i].CloudTrailEvent == nil || *filterEvents[i].CloudTrailEvent == "" {
				eventStringBuilder.WriteString("[Error] Error extracting event details: CloudTrailEvent is nil or empty")
				continue
			}
			rawEventDetails, err := pkg.ExtractUserDetails(filterEvents[i].CloudTrailEvent)
			if err != nil {
				eventStringBuilder.WriteString(fmt.Sprintf("[Error] Error extracting event details: %v", err))
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
					eventStringBuilder.WriteString("EventLink: <not available>")
				}
			}
		}
	}
	return eventStringBuilder.String()
}

func TestApplyFilters(t *testing.T) {
	mockEvents := []types.Event{
		{EventName: aws.String("EventName1"), Username: aws.String("user1"), EventTime: aws.Time(time.Now())},
		{EventName: aws.String("EventName2"), Username: aws.String("user2"), EventTime: aws.Time(time.Now())},
		{EventName: aws.String("EventName3"), Username: aws.String("user1"), EventTime: aws.Time(time.Now())},
	}
	t.Run("Apply filter to return specific events", func(t *testing.T) {
		filter := filterByEventName("EventName2")
		filteredEvents, err := ApplyFilters(mockEvents, filter)
		assert.NoError(t, err)
		assert.Len(t, filteredEvents, 1)
		assert.Equal(t, "EventName2", *filteredEvents[0].EventName)
	})
	t.Run("Apply no filters to return all events", func(t *testing.T) {
		filteredEvents, err := ApplyFilters(mockEvents)
		assert.NoError(t, err)
		assert.Len(t, filteredEvents, len(mockEvents))
	})
	t.Run("Apply multiple filters", func(t *testing.T) {
		filter1 := filterByEventName("EventName1")
		filter2 := filterByUsername("user1")
		filteredEvents, err := ApplyFilters(mockEvents, filter1, filter2)
		assert.NoError(t, err)
		assert.Len(t, filteredEvents, 1)
		assert.Equal(t, "EventName1", *filteredEvents[0].EventName)
		assert.Equal(t, "user1", *filteredEvents[0].Username)
	})
	t.Run("Apply filter that returns error", func(t *testing.T) {
		errorFilter := func(e types.Event) (bool, error) { return false, fmt.Errorf("filter error") }
		_, err := ApplyFilters(mockEvents, errorFilter)
		assert.Error(t, err)
		assert.EqualError(t, err, "filter error")
	})
}

func TestPrintEvents(t *testing.T) {
	mockEvents := []types.Event{
		{
			EventName: aws.String("EventName1"),
			Username:  aws.String("user1"),
			EventTime: aws.Time(time.Now()),
			CloudTrailEvent: aws.String(`{
				"EventVersion": "1.08",
				"EventId": "1234",
				"EventRegion": "us-west-2",
				"UserIdentity": {
					"SessionContext": {
						"SessionIssuer": {
							"UserName": "user1"
						}
					}
				}
			}`),
		},
		{
			EventName: aws.String("EventName2"),
			Username:  aws.String("user2"),
			EventTime: aws.Time(time.Now()),
			CloudTrailEvent: aws.String(`{
				"EventVersion": "1.08",
				"EventId": "5678",
				"EventRegion": "us-west-2",
				"UserIdentity": {
					"SessionContext": {
						"SessionIssuer": {
							"UserName": "user2"
						}
					}
				}
			}`),
		},
	}
	t.Run("Test PrintEvents with printRaw = true", func(t *testing.T) {
		output := capturePrintEventsOutput(mockEvents, false, true)
		assert.Contains(t, output, "EventName1")
		assert.Contains(t, output, "EventName2")
	})
	t.Run("Test PrintEvents with printUrl = true", func(t *testing.T) {
		output := capturePrintEventsOutput(mockEvents, true, false)
		assert.Contains(t, output, "EventName1")
		assert.Contains(t, output, "EventName2")
		assert.Contains(t, output, "https://")
	})
	t.Run("Test PrintEvents without URL or raw output", func(t *testing.T) {
		output := capturePrintEventsOutput(mockEvents, false, false)
		assert.Contains(t, output, "EventName1")
		assert.Contains(t, output, "EventName2")
		assert.NotContains(t, output, "https://")
	})
}
