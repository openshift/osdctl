package cloudtrail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/labstack/gommon/log"
	"github.com/sirupsen/logrus"
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
//   - True; if Cache Period and Request Period Overlaps
//   - True; if Cache Period and Request Period is Sequential (i.e +/-1s difference)
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

// getCacheFilePath returns the full cache file path and ensures the directory exists.
// File name includes cluster id and region
func CacheFileInit(cache, region string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	cacheDir = filepath.Join(cacheDir, "osdctl", "cloudtrail", "write-events")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Error("failed to create cache directory: %w", err)
		return "", err
	}
	cacheFile := filepath.Join(cacheDir, cache+"-"+region+"-cache.json")
	return cacheFile, nil
}

// CacheInit initializes the cache directory.
// Creates the directory/cache file if it doesn't exist
func Read(cacheFile string, log *logrus.Logger) (Cache, error) {
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		log.Infof("Creating new file")
		emptyCache := Cache{
			Period: []Period{},
			Event:  [][]types.Event{},
		}
		data, err := json.MarshalIndent(emptyCache, "", "  ")
		if err != nil {
			log.Error("failed to marshal empty cache: %w", err)
			return Cache{}, err
		}
		if err := os.WriteFile(cacheFile, data, 0644); err != nil {
			log.Error("failed to create cache file: %w", err)
			return Cache{}, err
		}
		return emptyCache, nil
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		log.Error("failed to read cache file: %w", err)
		return Cache{}, err
	}

	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		log.Error("failed to unmarshal get events: %w", err)
		return Cache{}, err
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
func Save(cacheFile string, newEvents map[Period][]types.Event, log *logrus.Logger) error {
	var existingCache Cache
	if data, err := os.ReadFile(cacheFile); err == nil {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &existingCache); err != nil {
				log.Warnf("[WARNING] Corrupted cache file, starting fresh: %v\n", err)
			}
		}
	}

	var allPeriods []Period
	allPeriods = append(allPeriods, existingCache.Period...)
	for period := range newEvents {
		allPeriods = append(allPeriods, period)
	}

	// Periods now in order
	// NewPeriod consist of merged overlapping periods
	sort.Sort(Periods(allPeriods))
	newPeriods := Merge(allPeriods)

	// allEvents have all events
	// allevents = [[events],[events]]
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
		log.Error("failed to marshal cache: %w", err)
		return err
	}
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		log.Error("failed to marshal cache: %w", err)
		return err
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
					if !eventTime.Before(requestPeriod.StartTime) && !eventTime.After(requestPeriod.EndTime) {
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
