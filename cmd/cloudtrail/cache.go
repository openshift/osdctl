package cloudtrail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/sirupsen/logrus"
)

// Cache struct stores CloudTrail periods and their corresponding events,
type Cache struct {
	log      *logrus.Logger
	filename string
	Period   []Period
	Event    []types.Event
}

func NewCache(log *logrus.Logger, clusterID string) (*Cache, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	cacheDir = filepath.Join(cacheDir, "osdctl", "cloudtrail", "write-events")
	filename := filepath.Join(cacheDir, clusterID+".json")

	return &Cache{
		log:      log,
		filename: filename,
		Period:   []Period{},
		Event:    []types.Event{},
	}, nil
}

func (c *Cache) EnsureFilenameExist() error {
	cacheDir := filepath.Dir(c.filename)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		c.log.Errorf("failed to create cache directory: %v", err)
		return err
	}

	if _, err := os.Stat(c.filename); os.IsNotExist(err) {
		emptyCache := Cache{
			log:      c.log,
			filename: c.filename,
			Period:   []Period{},
			Event:    []types.Event{},
		}

		data, err := json.MarshalIndent(emptyCache, "", "  ")
		if err != nil {
			c.log.Errorf("failed to marshal empty cache: %v", err)
			return err
		}
		if err := os.WriteFile(c.filename, data, 0600); err != nil {
			c.log.Errorf("failed to create cache file: %v", err)
			return err
		}
		c.log.Debugf("Created new cache file: %s", c.filename)
		return nil

	} else if err != nil {
		c.log.Errorf("error checking cache file: %v", err)
		return err
	}

	c.log.Debugf("Cache file already exists: %s", c.filename)
	return nil
}

// CacheInit initializes the cache directory.
// Creates the directory/cache file if it doesn't exist
func (c *Cache) Read() error {
	data, err := os.ReadFile(c.filename)
	if err != nil {
		c.log.Errorf("failed to read cache file: %v", err)
		return err
	}

	if err := json.Unmarshal(data, &c); err != nil {
		c.log.Errorf("failed to unmarshal get events: %v", err)
		return err
	}

	if len(c.Event) == 0 {
		c.log.Debugf("Cache file is empty")
	}

	return nil
}

// PutCache saves the cloudtrail events to the cache file.
// Adding new time periods and events to the cache.
// Merging new data with existing overlapping data.
func (c *Cache) Save(newCacheEvents Cache) error {
	if c == nil {
		c = &Cache{
			Period: []Period{},
			Event:  []types.Event{},
		}
	}

	var allPeriods []Period
	allPeriods = append(allPeriods, c.Period...)
	allPeriods = append(allPeriods, newCacheEvents.Period...)
	sort.Sort(Periods(allPeriods))
	newPeriods := Merge(allPeriods)

	var allEvents []types.Event
	allEvents = append(allEvents, c.Event...)
	allEvents = append(allEvents, newCacheEvents.Event...)
	sort.Slice(allEvents, func(i, j int) bool {
		if allEvents[i].EventTime == nil {
			return false
		}
		if allEvents[j].EventTime == nil {
			return true
		}
		return allEvents[j].EventTime.Before(*allEvents[i].EventTime)
	})

	cache := Cache{
		log:      c.log,
		filename: c.filename,
		Period:   newPeriods,
		Event:    allEvents,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		c.log.Errorf("failed to marshal cache: %v", err)
		return err
	}
	if err := os.WriteFile(c.filename, data, 0600); err != nil {
		c.log.Errorf("failed to write to cache file: %v", err)
		return err
	}

	return nil
}

// DiffMultiple takes the requested time range and compares it to the time period in the cache.
// If it overlaps, it will be added to the list and returned to the user.
func (p Period) DiffMultiple(c []Period) ([]Period, bool) {
	var missingPeriods []Period
	hasOverlap := false
	fullOverlap := false

	for i, cachePeriod := range c {
		if cachePeriod.Overlap(p) {
			var nextPeriod *Period
			if i < len(c)-1 {
				nextPeriod = &c[i+1]
			}

			missing := cachePeriod.Diff(p, nextPeriod)
			if len(missing) == 0 {
				if cachePeriod.StartTime.Before(p.StartTime) || cachePeriod.StartTime.Equal(p.StartTime) &&
					cachePeriod.EndTime.After(p.EndTime) || cachePeriod.EndTime.Equal(p.EndTime) {
					fullOverlap = true
				}
				continue
			}

			hasOverlap = true
			for _, missingPeriod := range missing {
				if !missingPeriod.StartTime.Equal(missingPeriod.EndTime) &&
					missingPeriod.periodOverlap(missingPeriods) {
					missingPeriods = append(missingPeriods, missingPeriod)
				}

			}
		}
	}

	if !hasOverlap {
		missingPeriods = append(missingPeriods, p)
	}

	return missingPeriods, fullOverlap
}

// periodOverlap checks if the new period already exists within the
// missing period lists. Returns a bool
func (p Period) periodOverlap(missingPeriods []Period) bool {
	for _, periods := range missingPeriods {
		if p.Overlap(periods) {
			return false
		}
	}
	return true
}

func (c *Cache) FilterByPeriod(requestedPeriod Period) []types.Event {
	var eventsInCache []types.Event
	for _, period := range c.Period {
		if period.Overlap(requestedPeriod) {
			for _, event := range c.Event {
				if event.EventTime != nil {
					eventTime := *event.EventTime
					if !eventTime.Before(requestedPeriod.StartTime) && !eventTime.After(requestedPeriod.EndTime) {
						eventsInCache = append(eventsInCache, event)
					}
				}
			}
		}
	}
	return eventsInCache
}

func FilterByRegion(region string, events []types.Event) []types.Event {
	var filtered []types.Event
	for _, event := range events {
		rawEventDetails, err := ExtractUserDetails(event.CloudTrailEvent)
		if err != nil {
			continue
		}
		if rawEventDetails.EventRegion == region {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// Filter events that occur after a specific time
func FilterEventsAfter(events []types.Event, afterTime time.Time) []types.Event {
	var filtered []types.Event
	for _, event := range events {
		if event.EventTime != nil && (event.EventTime.After(afterTime) || event.EventTime.Equal(afterTime)) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// Filter events that occur before a specific time
func FilterEventsBefore(events []types.Event, beforeTime time.Time) []types.Event {
	var filtered []types.Event
	for _, event := range events {
		if event.EventTime != nil && (event.EventTime.Before(beforeTime) || event.EventTime.Equal(beforeTime)) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
