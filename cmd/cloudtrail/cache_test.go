package cloudtrail

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
)

func TestOverlap(t *testing.T) {
	// Time Period in Cache: 2000-01-01,10:00:00 -> 2000-01-01,12:00:00
	cache := Period{
		StartTime: time.Date(2000, 1, 1, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	testCases := []struct {
		name     string
		request  Period
		expected bool
	}{
		{
			name: "No Overlap",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 8, 30, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC),
			},
			expected: false,
		},
		{
			name: "Full Overlap",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 10, 30, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 11, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
		{
			name: "Sequential Overlap After cache.EndTime",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 12, 0, 1, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 14, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
		{
			name: "Sequential Overlap Before cache.StartTime",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 9, 30, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 9, 59, 59, 0, time.UTC),
			},
			expected: true,
		},
		{
			name: "Exact Overlap at cache.Start",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 14, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
		{
			name: "Sequential Overlap Before cache.EndTime",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 10, 0, 0, 0, time.UTC),
			},
			expected: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := cache.Overlap(testCase.request)
			if result != testCase.expected {
				t.Errorf("Expected %v, got %v for case: %s", testCase.expected, result, testCase.name)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	period := []Period{
		{
			StartTime: time.Date(2000, 1, 1, 10, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			StartTime: time.Date(2000, 1, 1, 14, 35, 23, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 1, 15, 22, 11, 0, time.UTC),
		},
		{
			StartTime: time.Date(2000, 1, 1, 20, 59, 10, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 2, 1, 0, 0, 0, time.UTC),
		},
	}

	testCases := []struct {
		name        string
		cachePeriod Period
		nextPeriod  *Period
		request     Period
		expected    []Period
	}{
		{
			name:        "No Overlap",
			cachePeriod: period[0],
			nextPeriod:  &period[1],
			request: Period{
				StartTime: time.Date(2020, 1, 1, 8, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2020, 1, 1, 9, 0, 0, 0, time.UTC),
			},
			expected: []Period{{
				StartTime: time.Date(2020, 1, 1, 8, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2020, 1, 1, 9, 0, 0, 0, time.UTC),
			}},
		},
		{
			name:        "Single Overlap",
			cachePeriod: period[0],
			nextPeriod:  &period[1],
			request: Period{
				StartTime: time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 11, 0, 0, 0, time.UTC),
			},
			expected: []Period{
				{
					StartTime: time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 9, 59, 59, 0, time.UTC),
				},
			},
		},
		{
			name:        "Double Overlap",
			cachePeriod: period[0],
			request: Period{
				StartTime: time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 13, 0, 0, 0, time.UTC),
			},
			expected: []Period{
				{
					StartTime: time.Date(2000, 1, 1, 9, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 9, 59, 59, 0, time.UTC),
				},
				{
					StartTime: time.Date(2000, 1, 1, 12, 0, 1, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 13, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cachePeriod := testCase.cachePeriod
			result := cachePeriod.Diff(testCase.request, testCase.nextPeriod)

			fmt.Printf("\n--- %s ---\n", testCase.name)
			fmt.Printf("Cache: %v to %v\n", testCase.cachePeriod.StartTime, testCase.cachePeriod.EndTime)
			fmt.Printf("Request: %v to %v\n", testCase.request.StartTime, testCase.request.EndTime)
			fmt.Printf("Expected %d periods, got %d periods\n", len(testCase.expected), len(result))

			for i, period := range result {

				expected := testCase.expected[i]
				if !period.StartTime.Equal(expected.StartTime) || !period.EndTime.Equal(expected.EndTime) {
					t.Errorf("Period %d mismatch:\n  Expected: %+v\n  Got:      %+v\n  Case: %s",
						i, expected, period, testCase.name)
				}
			}
		})
	}
}

func TestMultipleDiff(t *testing.T) {
	cache := Cache{
		Period: []Period{
			{
				StartTime: time.Date(2000, 1, 1, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			{
				StartTime: time.Date(2000, 1, 1, 14, 35, 23, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 15, 22, 11, 0, time.UTC),
			},
			{
				StartTime: time.Date(2000, 1, 1, 20, 59, 10, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 2, 1, 0, 0, 0, time.UTC),
			},
		},
		Event: []types.Event{{}, {}, {}},
	}

	testCases := []struct {
		name     string
		request  Period
		expected []Period
	}{
		{
			name: "Single No Time Diff",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			expected: []Period{},
		},
		{
			name: "Single Time Diff",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 10, 30, 0, 0, time.UTC),
			},
			expected: []Period{
				{
					StartTime: time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 9, 59, 59, 0, time.UTC),
				},
			},
		},
		{
			name: "Multiple Time Diff",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 2, 8, 0, 0, 0, time.UTC),
			},
			expected: []Period{
				{
					StartTime: time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 9, 59, 59, 0, time.UTC),
				},
				{
					StartTime: time.Date(2000, 1, 1, 12, 0, 1, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 14, 35, 22, 0, time.UTC),
				},
				{
					StartTime: time.Date(2000, 1, 1, 15, 22, 12, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 20, 59, 9, 0, time.UTC),
				},
				{
					StartTime: time.Date(2000, 1, 2, 1, 0, 1, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 2, 8, 0, 0, 0, time.UTC),
				},
			},
		},
		{
			name: "Double After Time Diff",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 11, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 18, 0, 0, 0, time.UTC),
			},
			expected: []Period{
				{
					StartTime: time.Date(2000, 1, 1, 12, 0, 1, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 14, 35, 22, 0, time.UTC),
				},
				{
					StartTime: time.Date(2000, 1, 1, 15, 22, 12, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 18, 0, 0, 0, time.UTC),
				},
			},
		},
		{
			name: "No Overlap Single Missing Period",
			request: Period{
				StartTime: time.Date(2000, 1, 1, 7, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
			},
			expected: []Period{
				{
					StartTime: time.Date(2000, 1, 1, 7, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := DiffMultiple(testCase.request, cache)
			fmt.Printf("\n--- %s ---\n", testCase.name)
			fmt.Printf("Request: %v to %v\n", testCase.request.StartTime, testCase.request.EndTime)
			fmt.Printf("Found %d missing periods:\n", len(result))
			for i, period := range result {
				fmt.Printf("  [%d] %v to %v\n", i, period.StartTime, period.EndTime)
			}

			if len(result) != len(testCase.expected) {
				t.Errorf("Length mismatch: expected %d periods, got %d for case: %s",
					len(testCase.expected), len(result), testCase.name)
				return
			}

			for i, period := range result {
				if i >= len(testCase.expected) {
					t.Errorf("Unexpected period at index %d: %+v", i, period)
					continue
				}

				expected := testCase.expected[i]
				if !period.StartTime.Equal(expected.StartTime) || !period.EndTime.Equal(expected.EndTime) {
					t.Errorf("Period %d mismatch:\n  Expected: %+v\n  Got:      %+v\n  Case: %s",
						i, expected, period, testCase.name)
				}
			}
		})
	}
}

func TestMerge(t *testing.T) {
	setOfTimes := []Period{
		{
			StartTime: time.Date(2000, 1, 1, 7, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
		},
		{
			StartTime: time.Date(2000, 1, 1, 11, 25, 10, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			StartTime: time.Date(2000, 1, 1, 12, 25, 59, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 1, 14, 0, 0, 0, time.UTC),
		},
	}

	testCases := []struct {
		name       string
		newPeriod  Period
		nextPeriod *Period
		result     Period
	}{
		{
			name: "No Merge",
			newPeriod: Period{
				StartTime: time.Date(2000, 1, 1, 5, 25, 59, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 6, 0, 0, 0, time.UTC)},
			nextPeriod: &setOfTimes[1],
			result:     Period{},
		},
		{
			name: "Single Merge before index 0",
			newPeriod: Period{
				StartTime: time.Date(2000, 1, 1, 5, 25, 59, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 6, 59, 59, 0, time.UTC),
			},
			nextPeriod: &setOfTimes[1],
			result: Period{
				StartTime: time.Date(2000, 1, 1, 5, 25, 59, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "Merge between index 0 & 1",
			newPeriod: Period{
				StartTime: time.Date(2000, 1, 1, 8, 0, 1, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 11, 0, 0, 0, time.UTC),
			},
			nextPeriod: &setOfTimes[1],
			result: Period{
				StartTime: time.Date(2000, 1, 1, 7, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 11, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "Exact gap between index 1 & 2, merge into a big time period",
			newPeriod: Period{
				StartTime: time.Date(2000, 1, 1, 8, 0, 1, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 11, 25, 9, 0, time.UTC),
			},
			nextPeriod: &setOfTimes[1],
			result: Period{
				StartTime: time.Date(2000, 1, 1, 7, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "Merge with last period (no next period)",
			newPeriod: Period{
				StartTime: time.Date(2000, 1, 1, 14, 0, 1, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 16, 0, 0, 0, time.UTC),
			},
			nextPeriod: &Period{},
			result: Period{
				StartTime: time.Date(2000, 1, 1, 12, 25, 59, 0, time.UTC),
				EndTime:   time.Date(2000, 1, 1, 16, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var result Period
			for _, cache := range setOfTimes {
				result = cache.Merge(testCase.newPeriod, testCase.nextPeriod)
				if result == testCase.result {
					fmt.Printf("\n--- %s ---\n", testCase.name)
					fmt.Printf("Return: %v to %v\n", result.StartTime, result.EndTime)
					break
				}
			}
			if result != testCase.result {
				t.Errorf("Expected %v, got %v for case: %s", testCase.result, result, testCase.name)
			}
		})
	}
}

func TestMultipleMerge(t *testing.T) {
	setOfTimes := []Period{
		{
			StartTime: time.Date(2000, 1, 1, 7, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 1, 8, 0, 0, 0, time.UTC),
		},
		{
			StartTime: time.Date(2000, 1, 1, 11, 0, 00, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			StartTime: time.Date(2000, 1, 1, 13, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2000, 1, 1, 14, 0, 0, 0, time.UTC),
		},
	}
	testCases := []struct {
		name       string
		period     []Period
		nextPeriod Period
		expected   []Period
	}{
		{
			name: "List of Periods merge into a big one",
			period: []Period{
				{
					StartTime: time.Date(2000, 1, 1, 8, 0, 1, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 10, 59, 59, 0, time.UTC),
				},
				{
					StartTime: time.Date(2000, 1, 1, 12, 0, 1, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 12, 59, 59, 0, time.UTC),
				},
			},
			expected: []Period{
				{
					StartTime: time.Date(2000, 1, 1, 7, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2000, 1, 1, 14, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var periodList []Period
			for _, newPeriod := range testCase.period {
				for i, cache := range setOfTimes {
					if cache.Overlap(newPeriod) {
						nextPeriod := &setOfTimes[i]
						mergePeriod := cache.Merge(newPeriod, nextPeriod)
						//hasOverlap = true
						periodList = append(periodList, mergePeriod)
					} else {
						periodList = append(periodList, newPeriod)
					}
				}
			}
		})
	}
}
