package pkg

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
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

func TestApplyFilters(t *testing.T) {
	baseEvents := []types.Event{
		{EventName: aws.String("EventName1"), Username: aws.String("user1"), EventTime: aws.Time(time.Now())},
		{EventName: aws.String("EventName2"), Username: aws.String("user2"), EventTime: aws.Time(time.Now())},
		{EventName: aws.String("EventName3"), Username: aws.String("user1"), EventTime: aws.Time(time.Now())},
		{EventName: nil, Username: aws.String("user3"), EventTime: aws.Time(time.Now())}, // nil EventName
	}

	tests := []struct {
		name               string
		filters            []Filter
		inputEvents        []types.Event
		expectedLength     int
		expectedEventNames []*string
		expectError        bool
		errorMessage       string
	}{
		{
			name:               "apply_filter_to_return_specific_events",
			filters:            []Filter{filterByEventName("EventName2")},
			inputEvents:        baseEvents,
			expectedLength:     1,
			expectedEventNames: []*string{aws.String("EventName2")},
		},
		{
			name:               "apply_no_filters_to_return_all_events",
			filters:            nil,
			inputEvents:        baseEvents,
			expectedLength:     len(baseEvents),
			expectedEventNames: []*string{aws.String("EventName1"), aws.String("EventName2"), aws.String("EventName3"), nil},
		},
		{
			name:               "apply_multiple_filters",
			filters:            []Filter{filterByEventName("EventName1"), filterByUsername("user1")},
			inputEvents:        baseEvents,
			expectedLength:     1,
			expectedEventNames: []*string{aws.String("EventName1")},
		},
		{
			name: "apply_filter_that_returns_error",
			filters: []Filter{
				func(e types.Event) (bool, error) {
					return false, fmt.Errorf("filter error")
				},
			},
			inputEvents:        baseEvents,
			expectError:        true,
			errorMessage:       "filter error",
			expectedLength:     0,
			expectedEventNames: []*string{},
		},
		{
			name:               "handle_empty_event_list",
			filters:            nil,
			inputEvents:        []types.Event{},
			expectedLength:     0,
			expectedEventNames: []*string{},
		},
		{
			name:               "event_with_nil_eventname_should_be_skipped_by_filter",
			filters:            []Filter{filterByEventName("AnyEvent")},
			inputEvents:        []types.Event{{EventName: nil, Username: aws.String("user1"), EventTime: aws.Time(time.Now())}},
			expectedLength:     0,
			expectedEventNames: []*string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make([]types.Event, len(tt.inputEvents))
			copy(events, tt.inputEvents)

			filteredEvents, err := ApplyFilters(events, tt.filters...)

			if tt.expectError {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.errorMessage)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, filteredEvents, tt.expectedLength)

			for i, event := range filteredEvents {
				assert.Equal(t, tt.expectedEventNames[i], event.EventName)
			}
		})
	}
}

func TestPrintEvents(t *testing.T) {
	mockEvents := []types.Event{
		{
			EventName: aws.String("LoginEvent"),
			Username:  aws.String("test-user"),
			EventTime: aws.Time(time.Date(2023, 11, 10, 12, 0, 0, 0, time.UTC)),
			CloudTrailEvent: aws.String(`{
				"EventVersion": "1.08",
				"EventId": "abcd1234",
				"UserIdentity": {
					"SessionContext": {
						"SessionIssuer": {
							"UserName": "arn:aws:iam::123456789012:user/test-user"
						}
					}
				}
			}`),
		},
	}

	tests := []struct {
		name      string
		printRaw  bool
		printUrl  bool
		events    []types.Event
		assertion func(output string)
	}{
		{
			name:     "print_raw_event_only",
			printRaw: true,
			events:   mockEvents,
			assertion: func(output string) {
				assert.Contains(t, output, `"EventVersion": "1.08"`)
				assert.Contains(t, output, `"EventId": "abcd1234"`)
				assert.NotContains(t, output, "Username:")
			},
		},
		{
			name:     "print_formatted_output_with_url",
			printUrl: true,
			events:   mockEvents,
			assertion: func(output string) {
				assert.Contains(t, output, "LoginEvent")
				assert.Contains(t, output, "test-user")
				assert.Contains(t, output, "arn:aws:iam")
				assert.Contains(t, output, "https://")
			},
		},
		{
			name:   "print_formatted_without_url",
			events: mockEvents,
			assertion: func(output string) {
				assert.Contains(t, output, "LoginEvent")
				assert.Contains(t, output, "test-user")
				assert.NotContains(t, output, "https://")
			},
		},
		{
			name: "invalid_cloudtrail_json",
			events: []types.Event{
				{
					EventName:       aws.String("InvalidEvent"),
					Username:        aws.String("broken-user"),
					EventTime:       aws.Time(time.Now()),
					CloudTrailEvent: aws.String(`{invalid json`),
				},
			},
			assertion: func(output string) {
				assert.Contains(t, output, "[Error] Error extracting event details")
				assert.Contains(t, output, "InvalidEvent")
			},
		},
		{
			name: "unsupported_event_version",
			events: []types.Event{
				{
					EventName: aws.String("OldEvent"),
					Username:  aws.String("legacy-user"),
					EventTime: aws.Time(time.Now()),
					CloudTrailEvent: aws.String(`{
						"EventVersion": "1.01",
						"EventId": "xx",
						"UserIdentity": {
							"SessionContext": {
								"SessionIssuer": {
									"UserName": "arn:aws:iam::111111111111:user/legacy"
								}
							}
						}
					}`),
				},
			},
			assertion: func(output string) {
				assert.Contains(t, output, "[Error] Error extracting event details")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureOutput(func() {
				PrintEvents(tt.events, tt.printUrl, tt.printRaw)
			})
			tt.assertion(output)
		})
	}
}

func captureOutput(f func()) string {
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = stdout
	_, _ = io.Copy(writer, r)
	writer.Flush()
	return buf.String()
}
