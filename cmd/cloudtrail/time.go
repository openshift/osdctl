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

// Period struct is a struct that consist of the Start and End time for the Cache
type Period struct {
	StartTime time.Time
	EndTime   time.Time
}

// Periods is a slice of Period structs.
// It implements the sort.Interface so that a slice of Periods can be sorted by StartTime.
type Periods []Period

// Len returns the number of periods in the slice.
func (p Periods) Len() int {
	return len(p)
}

// Less reports whether the period at index i should sort before the period at index j.
// Periods are sorted by their StartTime in ascending order.
func (p Periods) Less(i, j int) bool {
	return p[i].StartTime.Before(p[j].StartTime)
}

// Swap swaps the periods at indices i and j.
func (p Periods) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// Overlap returns a boolean value under 2 conditions
// Returns:
//   - True; if p1 and p2 overlaps
//   - True; if p1 and p2 is sequential (i.e +/-1s difference)
//   - False; if there is no overlap between p1 and p2
func (p1 *Period) Overlap(p2 Period) bool {
	if !p1.EndTime.Before(p2.StartTime) && !p1.StartTime.After(p2.EndTime) {
		return true
	}

	if p1.EndTime.Equal(p2.StartTime) || p1.EndTime.Add(time.Second).Equal(p2.StartTime) {
		return true
	}

	if p2.EndTime.Equal(p1.StartTime) || p2.EndTime.Add(time.Second).Equal(p1.StartTime) {
		return true
	}

	return false
}

// Merge checks to see if the period overlaps the new period.
// If it overlaps it will merge the periods and return a new period.
// Input parameter has to be sorted before the function is called
func Merge(allPeriods []Period) []Period {
	mergedPeriod := []Period{}

	for _, period := range allPeriods {
		if len(mergedPeriod) == 0 {
			mergedPeriod = append(mergedPeriod, period)
			continue
		}
		prev := &mergedPeriod[len(mergedPeriod)-1]
		if prev.Overlap(period) {
			if period.StartTime.Before(prev.StartTime) {
				prev.StartTime = period.StartTime
			}
			if period.EndTime.After(prev.EndTime) {
				prev.EndTime = period.EndTime
			}
		} else {
			mergedPeriod = append(mergedPeriod, period)
		}
	}
	return mergedPeriod
}

// Diff returns the missing time Period if there is an overlap
// If req.start is before p.start; StartTime: req.StartTime, EndTime: p.StartTime - 1s
// If req.end is after p.end; StartTime: p.EndTime, EndTime + 1: req.StartTime
func (p *Period) Diff(req Period, nextPeriod *Period) []Period {
	var result []Period
	if !p.Overlap(req) {
		return []Period{req}
	}
	if p.StartTime.Equal(req.StartTime) && p.EndTime.Equal(req.EndTime) {
		return []Period{}
	}
	if req.StartTime.Before(p.StartTime) {
		result = append(result, Period{StartTime: req.StartTime, EndTime: p.StartTime.Add(-time.Second)})
	}

	if req.EndTime.After(p.EndTime) {
		remainingTime := Period{StartTime: p.EndTime.Add(time.Second), EndTime: req.EndTime}

		if nextPeriod != nil && nextPeriod.Overlap(remainingTime) {
			remainingTime.EndTime = nextPeriod.StartTime.Add(-time.Second)
		}
		if !remainingTime.StartTime.After(remainingTime.EndTime) {
			result = append(result, remainingTime)
		}
	}
	return result
}
