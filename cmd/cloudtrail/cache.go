package cloudtrail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
)

// Period struct is a struct that consist of the Start and End time for the Cache
type Period struct {
	StartTime time.Time
	EndTime   time.Time
}

// Cache struct consists of a list of periods and a list of events
type Cache struct {
	Period []Period
	Event  [][]types.Event // Each Event[i] is a batch for Period[i]
}

// Overlap returns a boolean value under 2 conditions
// Returns:
//   - True; if Cache Period and Request Period Overlaps
//   - True; if Cache Period and Request Period is Sequential (i.e +1s difference)
//   - False; if there is no overlap
func (cache *Period) Overlap(request Period) bool {
	if !cache.EndTime.Before(request.StartTime) && !cache.StartTime.After(request.EndTime) {
		return true
	}

	if cache.EndTime.Equal(request.StartTime) || cache.EndTime.Add(time.Second).Equal(request.StartTime) {
		return true
	}

	if request.EndTime.Equal(cache.StartTime) || request.EndTime.Add(time.Second).Equal(cache.StartTime) {
		return true
	}

	return false
}

// Merge checks to see if the cache period overlaps the new period.
// If it overlaps it will merge the periods and return a new period.
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

	/*
		if !cache.Overlap(request) {
			return Period{}
		}
		if cache.StartTime.Equal(request.StartTime) && cache.EndTime.Equal(request.EndTime) {
			return Period{}
		}

		start := cache.StartTime
		if request.StartTime.Before(start) {
			start = request.StartTime
		}
		end := cache.EndTime
		if request.EndTime.Equal(nextPeriod.StartTime.Add(-time.Second)) {
			end = nextPeriod.EndTime
		} else if request.EndTime.After(cache.EndTime) {
			end = request.EndTime
		}
	*/
	return mergedPeriod
}

// Diff returns the missing time Period if there is an overlap
// If StartTime of request is before cache; StartTime: r.StartTime, EndTime: cache.StartTime - 1s
// If EndTime of request is before cache; StartTime: c.EndTime, EndTime + 1: r.StartTime
func (cache *Period) Diff(request Period, nextPeriod *Period) []Period {
	var result []Period
	if !cache.Overlap(request) {
		return []Period{request}
	}
	if cache.StartTime.Equal(request.StartTime) && cache.EndTime.Equal(request.EndTime) {
		return []Period{}
	}
	if request.StartTime.Before(cache.StartTime) {
		result = append(result, Period{StartTime: request.StartTime, EndTime: cache.StartTime.Add(-time.Second)})
	}

	if request.EndTime.After(cache.EndTime) {
		remainingTime := Period{StartTime: cache.EndTime.Add(time.Second), EndTime: request.EndTime}

		if nextPeriod != nil && nextPeriod.Overlap(remainingTime) {
			remainingTime.EndTime = nextPeriod.StartTime.Add(-time.Second)
		}
		if remainingTime.StartTime.After(remainingTime.EndTime) {
			result = append(result, remainingTime)
		} else {
			result = append(result, remainingTime)
		}
	}
	return result
}

// CacheInit initializes the cache directory.
// Creates the directory/cache file if it doesn't exist
func Read(clusterID string) (Cache, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return Cache{}, err
	}
	cacheDir = filepath.Join(cacheDir, "osdctl", "cloudtrail", "write-events")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return Cache{}, fmt.Errorf("failed to create cache directory: %w", err)
	}
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%s-cache.json", clusterID))

	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		return Cache{
			Period: []Period{},
			Event:  [][]types.Event{},
		}, nil
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return Cache{}, fmt.Errorf("[ERROR] failed to read cache file: %w", err)
	}

	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return Cache{}, fmt.Errorf("[ERROR] failed to unmarshal get events: %w", err)
	}

	if len(cache.Period) == 0 {
		return Cache{
			Period: []Period{},
			Event:  [][]types.Event{},
		}, nil
	}

	return cache, nil
}

// PutCache saves the cloudtrail events to the cache file.
// Adding new time periods and events to the cache.
// Merging new data with existing overlapping data.
func Save(cache string, newEvents map[Period][]types.Event) error {
	var existingCache Cache
	if data, err := os.ReadFile(cache); err == nil {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &existingCache); err != nil {
				fmt.Printf("[WARNING] Corrupted cache file, starting fresh: %v\n", err)
			}
		}
	}

	var allPeriods []Period
	allPeriods = append(allPeriods, existingCache.Period...)
	for period := range newEvents {
		allPeriods = append(allPeriods, period)
	}
	sort.Sort(Periods(allPeriods))

	newPeriods := Merge(allPeriods)

	var allEvents []types.Event
	for _, eventBatch := range existingCache.Event {
		allEvents = append(allEvents, eventBatch...)
	}
	for _, events := range newEvents {
		allEvents = append(allEvents, events...)
	}

	var finalPeriods []Period
	var finalEvents [][]types.Event
	for _, period := range newPeriods {
		var eventsForThisPeriod []types.Event
		for _, event := range allEvents {
			if event.EventTime != nil {
				t := *event.EventTime
				if !t.Before(period.StartTime) && !t.After(period.EndTime) {
					eventsForThisPeriod = append(eventsForThisPeriod, event)
				}
			}
		}
		finalPeriods = append(finalPeriods, period)
		finalEvents = append(finalEvents, eventsForThisPeriod)

	}

	newCacheData := Cache{
		Period: finalPeriods,
		Event:  finalEvents,
	}

	data, err := json.MarshalIndent(newCacheData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}
	if err := os.WriteFile(cache, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// DiffMultiple takes the requested time range and compares it to the time period in the cache.
// If it overlaps, it will be added to the list and returned to the user.
func DiffMultiple(timeRange Period, cacheData Cache) []Period {
	var missingPeriods []Period
	hasAnyOverlap := false

	for i, cachePeriod := range cacheData.Period {
		if cachePeriod.Overlap(timeRange) {
			hasAnyOverlap = true
			nextPeriod := nextPeriod(cacheData, i)
			missing := cachePeriod.Diff(timeRange, nextPeriod)
			if len(missing) == 0 {
				continue
			}
			for _, period := range missing {
				if !period.StartTime.Equal(period.EndTime) && periodOverlap(missingPeriods, period) {
					missingPeriods = append(missingPeriods, period)
				}

			}
		}
	}

	if !hasAnyOverlap {
		missingPeriods = append(missingPeriods, timeRange)
	}
	return missingPeriods
}

// nextPeriod returns the next available period.
// if already at the last index, returns nil
func nextPeriod(cache Cache, i int) *Period {
	var nextPeriod *Period
	if i+1 < len(cache.Period) {
		nextPeriod = &cache.Period[i+1]
	} else {
		nextPeriod = nil
	}
	return nextPeriod
}

// periodOverlap checks if the new period already exists withing the
// missing period lists. Returns a bool
func periodOverlap(missingPeriods []Period, newPeriod Period) bool {
	for _, periods := range missingPeriods {
		if periods.Overlap(newPeriod) {
			return false
		}
	}
	return true
}

// getCacheEvents returns all events within the requested time period.
// returns a map of key value pairs (Period: []Events)
func getCacheEvents(cache Cache, requestPeriod Period) map[Period][]types.Event {
	periodEvents := make(map[Period][]types.Event)
	for i, period := range cache.Period {
		if period.Overlap(requestPeriod) {
			var eventsForThisPeriod []types.Event
			for _, event := range cache.Event[i] {
				if event.EventTime != nil {
					eventTime := *event.EventTime
					if !eventTime.Before(period.StartTime) && !eventTime.After(period.EndTime) {
						eventsForThisPeriod = append(eventsForThisPeriod, event)
					}
				}

			}
			if len(eventsForThisPeriod) > 0 {
				periodEvents[period] = eventsForThisPeriod
			}
		}
	}
	return periodEvents
}

/*
Shouldnt be in getCache

Cache.go should only be saving/read.


Get and Save
Overwrite every single time.



*/

// Save cluster ID, list of events / list of cluster periods
//. Keeping cache simple
//. Retrieve all the data.
//	Get all data and calculate when we are using it in the main flow.
//

// sorting Len() Swap() Less()

type Periods []Period

func (p Periods) Len() int {
	return len(p)
}

func (p Periods) Less(i, j int) bool {
	return p[i].StartTime.Before(p[j].StartTime)
}

func (p Periods) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}
