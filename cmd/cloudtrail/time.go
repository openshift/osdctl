package cloudtrail

import (
	"fmt"
	"strings"
	"time"
)

// ParseStartEndTime parses start time, end time, and duration parameters to calculate
// the actual time range for CloudTrail event queries.
//
// Parameters:
//   - start: Start time in "YYYY-MM-DD,HH:MM:SS" format (--after flag)
//   - end: End time in "YYYY-MM-DD,HH:MM:SS" format (--until flag)
//   - duration: Duration string like "2h", "30m", "1d" (--since flag)
//
// Time calculation logic:
//   - If both start and end are provided: Use exact time range
//   - If only start is provided: start + duration (forward in time)
//   - If only end is provided: end - duration (backward in time)
//   - If both start and end are no provided: Use time.Now().UTC() - duration (default 1h)
//
// Returns:
//   - startTime: Calculated start time in UTC
//   - endTime: Calculated end time in UTC
//   - error: Any parsing or validation error
func ParseStartEndTime(start, end, duration string) (time.Time, time.Time, error) {
	var startTime, endTime time.Time
	var err error

	if start == "" && end == "" {
		endTime = time.Now().UTC()
		if startTime, err = ParseDurationBefore(duration, endTime); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Failed to parse --since: %w", err)
		}
		return startTime, endTime, nil
	}

	if start != "" && end != "" {
		if startTime, err = ParseTimeAndValidate(start); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Time Format Incorrect: %w", err)
		}
		if endTime, err = ParseTimeAndValidate(end); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Time Format Incorrect: %w", err)
		}
		return startTime, endTime, nil
	}

	if start != "" {
		if startTime, err = ParseTimeAndValidate(start); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Time Format Incorrect: %w", err)
		}
		if endTime, err = ParseDurationAfter(duration, startTime); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Failed to parse --since: %w", err)
		}
		return startTime, endTime, nil
	}

	if end != "" {
		if endTime, err = ParseTimeAndValidate(end); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Time Format Incorrect: %w", err)
		}
		if startTime, err = ParseDurationBefore(duration, endTime); err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Failed to parse --since: %w", err)
		}
		return startTime, endTime, nil
	}

	if startTime.After(endTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("start time %v is after end time %v", startTime, endTime)
	}

	return time.Time{}, time.Time{}, fmt.Errorf("[ERROR] Invalid time parameter combination")
}

// parseDurationAfter parses the given startTime string as a duration and adds it from the current UTC time.
// It returns the resulting time and any parsing error encountered.
func ParseDurationAfter(input string, startTime time.Time) (time.Time, error) {
	duration, err := time.ParseDuration(input)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse time duration: %w", err)
	}
	if startTime.IsZero() {
		startTime = time.Now().UTC()
	}

	return startTime.UTC().Add(duration), nil
}

// parseDurationBefore parses the given startTime string as a duration and subtracts it from the current UTC time.
// It returns the resulting time and any parsing error encountered.
func ParseDurationBefore(input string, startTime time.Time) (time.Time, error) {
	duration, err := time.ParseDuration(input)
	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse time duration: %w", err)
	}
	if startTime.IsZero() {
		startTime = time.Now().UTC()
	}

	return startTime.UTC().Add(-duration), nil
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
