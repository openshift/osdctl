package cloudtrail

import (
	"fmt"
	"regexp"
	"strings"

	"slices"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/sirupsen/logrus"
)

// Filter is a function type that takes a CloudTrail event and returns a boolean indicating
// whether the event passes the filter, and an error if the filter evaluation fails.
type Filter func(types.Event) (bool, error)

// WriteEventFilters defines the structure for filters used in write-events.go
type WriteEventFilters struct {
	Include []string
	Exclude []string
}

// ApplyFilters takes the filteredEvents slice and applies an additional filter function.
// The filter function here is an inline function that calls isIgnoredEvent(event, mergedRegex).
// Only events for which isIgnoredEvent returns true (i.e., not ignored by the regex) are returned.
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

// isIgnoredEvent filters out events based on the specified ignore list, which contains
// regular expression patterns. It returns true if the event should be kept, and false if it should be filtered out.
func IsIgnoredEvent(event types.Event, mergedRegex string, log *logrus.Logger) (bool, error) {
	if mergedRegex == "" {
		return true, nil
	}
	raw, err := ExtractUserDetails(event.CloudTrailEvent)
	if err != nil {
		log.Error("[ERROR] failed to extract raw CloudTrail event details: %w", err)
		return true, err
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

// Filters applies inclusion and exclusion filters to all Cloudtrail Events
// applies inclusion filters then exclusion filters.
func Filters(f WriteEventFilters, alllookupEvents []types.Event) []types.Event {
	filtered := alllookupEvents

	if len(f.Include) > 0 {
		filtered = inclusionFilter(filtered, f.Include)
	}
	if len(f.Exclude) > 0 {
		filtered = exclusionFilter(filtered, f.Exclude)
	}
	return filtered
}

// inclusionFilter filter events by inclusion criteria.
// Only events that match all specified filter keys and at least one value per key are included.
func inclusionFilter(rawData []types.Event, inclusionFilters []string) []types.Event {
	keyValuePair := parseFilters(inclusionFilters)

	filterFunc := map[string]func(data types.Event, values []string) bool{
		"username": func(data types.Event, values []string) bool {
			if data.Username != nil {
				if slices.Contains(values, *data.Username) {
					return true
				}
			}
			return false
		},
		"event": func(data types.Event, values []string) bool {
			if data.EventName != nil {
				if slices.Contains(values, *data.EventName) {
					return true
				}
			}
			return false
		},
		"resource-name": func(data types.Event, values []string) bool {
			for _, resource := range data.Resources {
				if resource.ResourceName != nil {
					if slices.Contains(values, *resource.ResourceName) {
						return true
					}
				}
			}
			return false
		},
		"resource-type": func(data types.Event, values []string) bool {
			for _, resource := range data.Resources {
				if resource.ResourceType != nil {
					if slices.Contains(values, *resource.ResourceType) {
						return true
					}
				}
			}
			return false
		},
		"arn": func(data types.Event, values []string) bool {
			rawEventDetails, err := ExtractUserDetails(data.CloudTrailEvent)
			if err != nil {
				fmt.Printf("failed to extract event details: %v\n", err)
				return false
			}
			val := rawEventDetails.UserIdentity.SessionContext.SessionIssuer.UserName
			return slices.Contains(values, val)
		},
	}

	var result []types.Event
	for _, data := range rawData {
		found := true
		for key, values := range keyValuePair {
			if fn, ok := filterFunc[key]; ok {
				if !fn(data, values) {
					found = false
					break
				}
			}
		}
		if found {
			result = append(result, data)
		}
	}
	return result
}

// exclusionFilter filters events by exclusion criteria.
// All events that match any exclusion filter are removed.
func exclusionFilter(rawData []types.Event, exclusionFilters []string) []types.Event {
	keyValuePair := parseFilters(exclusionFilters)

	filterFunc := map[string]func(data types.Event, values []string) bool{
		"username": func(data types.Event, values []string) bool {
			if data.Username != nil {
				if slices.Contains(values, *data.Username) {
					return true
				}
			}
			return false
		},
		"event": func(data types.Event, values []string) bool {
			if data.EventName != nil {
				if slices.Contains(values, *data.EventName) {
					return true
				}
			}
			return false
		},
		"resource-name": func(data types.Event, values []string) bool {
			for _, resource := range data.Resources {
				if resource.ResourceName != nil {
					if slices.Contains(values, *resource.ResourceName) {
						return true
					}
				}
			}
			return false
		},
		"resource-type": func(data types.Event, values []string) bool {
			for _, resource := range data.Resources {
				if resource.ResourceType != nil {
					if slices.Contains(values, *resource.ResourceType) {
						return true
					}
				}
			}
			return false
		},
		"arn": func(data types.Event, values []string) bool {
			rawEventDetails, err := ExtractUserDetails(data.CloudTrailEvent)
			if err != nil {
				fmt.Printf("failed to extract event details: %v\n", err)
				return false
			}
			val := rawEventDetails.UserIdentity.SessionContext.SessionIssuer.UserName
			return slices.Contains(values, val)
		},
	}

	var result []types.Event
	for _, data := range rawData {
		found := false
		for key, values := range keyValuePair {
			if fn, ok := filterFunc[key]; ok {
				if fn(data, values) {
					found = true
					break
				}
			}
		}
		if !found {
			result = append(result, data)
		}
	}
	return result
}

// parseFilters parses a slice of filter strings in the format "key=value" into a map.
func parseFilters(filters []string) map[string][]string {
	keyValuePair := make(map[string][]string)
	for _, filter := range filters {
		kv := strings.SplitN(filter, "=", 2)
		key := kv[0]
		value := kv[1]
		keyValuePair[key] = append(keyValuePair[key], value)
	}
	return keyValuePair
}

// ValidateFilters checks that all filters are in the correct "key=value" format
// Returns an error immediately if a filter is invalid.
func ValidateFilters(filters []string) error {
	var allowedFilterKeys = map[string]struct{}{
		"username":      {},
		"event":         {},
		"resource-name": {},
		"resource-type": {},
		"arn":           {},
	}

	for _, filter := range filters {
		kv := strings.SplitN(filter, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("invalid filter format: %s (expected key=value)", filter)
		}
		key := kv[0]
		if _, ok := allowedFilterKeys[key]; !ok {
			return fmt.Errorf("invalid filter key: %s (allowed: username, event, resource-name, resource-type, arn)", key)
		}
	}
	return nil
}
