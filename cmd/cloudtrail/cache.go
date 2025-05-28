package cloudtrail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
)

type CacheDetails struct {
	Period []Period
	Event  [][]types.Event
}

type Period struct {
	StartTime time.Time
	EndTime   time.Time
}

type CacheResult struct {
	Event        []types.Event
	MissingTimes []Period
	WaitForData  bool
}

func (p *Period) Len() int {
	return 0
}

// Overlap returns a boolean value if the requested time exists in cache
func (cache *Period) Overlap(request Period) bool {
	return !cache.EndTime.Before(request.StartTime) && !cache.StartTime.After(request.EndTime)
}

// Merge is used for putcache
// Used the start end time to collect all periods
// Merge the events and remove duplications
func (cache *Period) Merge(request Period) Period {
	if !cache.Overlap(request) {
		return Period{}
	}
	start := cache.StartTime
	if request.StartTime.Before(start) {
		start = request.StartTime
	}
	end := cache.EndTime
	if request.EndTime.After(end) {
		end = request.EndTime
	}
	return Period{StartTime: start, EndTime: end}
}

// Diff returns at most 2 periods
// Returning period can be before/after the requested period
func (cache *Period) Diff(request Period) []Period {
	var result []Period
	// No overlap: return p as is
	if !cache.Overlap(request) {
		result = append(result, *cache)
		return result
	}
	// If b starts after p, add the left gap
	if request.StartTime.After(cache.StartTime) {
		result = append(result, Period{StartTime: request.StartTime, EndTime: cache.StartTime})
	}
	// If b ends before p, add the right gap
	if cache.EndTime.Before(request.EndTime) {
		result = append(result, Period{StartTime: cache.EndTime, EndTime: cache.EndTime})
	}
	return result
}

func newCacheResult() *CacheResult {
	return &CacheResult{
		Event:        []types.Event{},
		MissingTimes: []Period{},
		WaitForData:  true,
	}
}

// CacheInit initializes the cache directory structure and returns the
// cache file for the specified AWS account ID
func CacheInit(clusterID string, startTime, endTime time.Time) (CacheResult, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return CacheResult{}, err
	}
	cache = filepath.Join(cache, "osdctl", "cloudtrail", "write-events")
	if err := os.MkdirAll(cache, 0755); err != nil {
		return CacheResult{}, fmt.Errorf("failed to create cache directory: %w", err)
	}
	cacheFile := filepath.Join(cache, fmt.Sprintf("%s-cache.json", clusterID))

	cacheResult := newCacheResult()
	err = cacheResult.getCache(cacheFile, startTime, endTime)
	if err != nil {
		return CacheResult{}, err
	}

	return *cacheResult, nil
}

func (c *CacheResult) getCache(cacheFile string, startTime, endTime time.Time) error {
	var missingPeriods []Period
	var allEvents []types.Event

	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		c.Update(allEvents, []Period{{StartTime: startTime, EndTime: endTime}}, true)
		return nil
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return fmt.Errorf("[ERROR] failed to read cache file: %w", err)
	}

	var cache CacheDetails
	if err := json.Unmarshal(data, &cache); err != nil {
		return fmt.Errorf("[ERROR] failed to unmarshal get events: %w", err)
	}

	if len(cache.Period) == 0 {
		c.Update(allEvents, []Period{{StartTime: startTime, EndTime: endTime}}, true)
		return nil
	}

	requested := Period{StartTime: startTime, EndTime: endTime}
	covered := false

	for i, cached := range cache.Period {
		if cached.Overlap(requested) {
			allEvents = append(allEvents, getCacheEvents(cache.Event[i], startTime, endTime)...)
			missingPeriod := cached.Diff(requested)
			for _, missing := range missingPeriod {
				missingPeriods = append(missingPeriods, Period{StartTime: missing.StartTime, EndTime: missing.EndTime})
			}
			covered = true
		}
	}
	if !covered {
		missingPeriods = append(missingPeriods, Period{StartTime: startTime, EndTime: endTime})
	}
	c.Update(allEvents, missingPeriods, len(missingPeriods) > 0)
	return nil
}

// PutCache saves the cloudtrail events to the cache file.
// Adding new time periods and events to the cache.
// Merging new data with existing overlapping data.
func PutCache(cacheFile string, startTime, endTime time.Time, events []types.Event, overlap bool) error {
	var existingCache CacheDetails
	if data, err := os.ReadFile(cacheFile); err == nil {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &existingCache); err != nil {
				fmt.Printf("[WARNING] Corrupted cache file, starting fresh: %v\n", err)
				existingCache = CacheDetails{}
			}
		}
	}

	if !overlap {
		newCacheTime := Period{
			StartTime: startTime,
			EndTime:   endTime,
		}

		existingCache.Period = append(existingCache.Period, newCacheTime)
		existingCache.Event = append(existingCache.Event, events)
	}

	if overlap {
		for i, period := range existingCache.Period {
			cacheStart := period.StartTime
			cacheEnd := period.EndTime

			rangesOverlap := !cacheEnd.Before(startTime) && !cacheStart.After(endTime)

			if rangesOverlap {
				mergeStart := startTime
				if cacheStart.Before(startTime) {
					mergeStart = cacheStart
				}

				mergeEnd := endTime
				if cacheEnd.After(endTime) {
					mergeEnd = cacheEnd
				}

				existingCache.Period[i] = Period{
					StartTime: mergeStart,
					EndTime:   mergeEnd,
				}
				existingCache.Event[i] = events
				break
			}
		}
	}

	data, err := json.MarshalIndent(existingCache, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal put events: %w", err)
	}
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Update is a helper function that updates CacheResults with new data.
func (c *CacheResult) Update(events []types.Event, missing []Period, waitForData bool) {
	c.Event = events
	c.MissingTimes = missing
	c.WaitForData = waitForData
}

// getCachedEvent filters cached events to only include events within
// the specified time frames.
func getCacheEvents(cacheEvents []types.Event, startTime, endTime time.Time) []types.Event {
	var allEvents []types.Event
	for _, event := range cacheEvents {
		if event.EventTime != nil {
			eventTime := *event.EventTime
			if !eventTime.Before(startTime) && !eventTime.After(endTime) {
				allEvents = append(allEvents, event)
			}
		}
	}
	return allEvents
}
