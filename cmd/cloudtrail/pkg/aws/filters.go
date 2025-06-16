package pkg

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
)

func Filters(f *WriteEventFilters, alllookupEvents []types.Event) []types.Event {
	filtered := alllookupEvents

	// Inclusive Filters
	if len(f.Username) > 0 {
		filtered = filterByString(filtered, f.Username, func(e types.Event) *string { return e.Username })
	}
	if len(f.Event) > 0 {
		filtered = filterByString(filtered, f.Event, func(e types.Event) *string { return e.EventName })
	}
	if len(f.ResourceName) > 0 {
		filtered = filterByResource(filtered, f.ResourceName, func(r types.Resource) *string { return r.ResourceName })
	}
	if len(f.ResourceType) > 0 {
		filtered = filterByResource(filtered, f.ResourceType, func(r types.Resource) *string { return r.ResourceType })
	}
	if len(f.ArnSource) > 0 {
		filtered = filterByArn(filtered, f.ArnSource)
	}

	// Exclusive Filters
	if len(f.ExcludeUsername) > 0 {
		filtered = excludeByString(filtered, f.ExcludeUsername, func(e types.Event) *string { return e.Username })
	}
	if len(f.ExcludeEvent) > 0 {
		filtered = excludeByString(filtered, f.ExcludeUsername, func(e types.Event) *string { return e.EventName })
	}
	if len(f.ExcludeResourceName) > 0 {
		filtered = excludeByResource(filtered, f.ExcludeResourceName, func(r types.Resource) *string { return r.ResourceName })
	}
	if len(f.ExcludeResourceType) > 0 {
		filtered = excludeByResource(filtered, f.ExcludeResourceType, func(r types.Resource) *string { return r.ResourceType })
	}
	if len(f.ExcludeArnSource) > 0 {
		filtered = excludeByArn(filtered, f.ExcludeArnSource)
	}

	return filtered
}

// Filter by username & event
func filterByString(rawData []types.Event, filterList []string, targetFilter func(types.Event) *string) []types.Event {
	matchSet := make(map[string]struct{}, len(filterList))
	errors := make(map[string]bool, len(filterList))
	for _, filters := range filterList {
		matchSet[filters] = struct{}{}
		errors[filters] = false
	}

	var result []types.Event
	for _, row := range rawData {
		if filter := targetFilter(row); filter != nil {
			if _, ok := matchSet[*filter]; ok {
				result = append(result, row)
				errors[*filter] = true
			}
		}
	}
	errCheck(errors)

	return result
}

// Filter by resource name & type
func filterByResource(rawData []types.Event, resourceList []string, targetFilter func(r types.Resource) *string) []types.Event {
	matchSet := make(map[string]struct{}, len(resourceList))
	errors := make(map[string]bool, len(resourceList))
	for _, filters := range resourceList {
		matchSet[filters] = struct{}{}
		errors[filters] = false
	}

	var result []types.Event
	for _, row := range rawData {
		for _, resource := range row.Resources {
			if filter := targetFilter(resource); filter != nil {
				if _, ok := matchSet[*filter]; ok {
					result = append(result, row)
					errors[*filter] = true
					break
				}
			}
		}
	}
	errCheck(errors)

	return result
}

func filterByArn(rawData []types.Event, ArnValue []string) []types.Event {
	matchSet := make(map[string]struct{}, len(ArnValue))
	errors := make(map[string]bool, len(ArnValue))
	for _, values := range ArnValue {
		matchSet[values] = struct{}{}
		errors[values] = false
	}

	var result []types.Event
	for _, row := range rawData {
		raw, err := ExtractUserDetails(row.CloudTrailEvent)
		if err != nil {
			fmt.Printf("[Error] Error extracting event details: %v", err)
		}
		if _, ok := matchSet[raw.UserIdentity.SessionContext.SessionIssuer.UserName]; ok {
			result = append(result, row)
			errors[raw.UserIdentity.SessionContext.SessionIssuer.UserName] = true
		}
	}
	errCheck(errors)
	return result
}

// Filter excludes username and event
func excludeByString(rawData []types.Event, excludeFilterList []string, targetFilter func(types.Event) *string) []types.Event {
	matchSet := make(map[string]struct{}, len(excludeFilterList))
	errors := make(map[string]bool)
	for _, filters := range excludeFilterList {
		matchSet[filters] = struct{}{}
		errors[filters] = false

	}
	var result []types.Event

	for _, row := range rawData {
		if filter := targetFilter(row); filter != nil {
			if _, ok := matchSet[*filter]; ok {
				errors[*filter] = true
				continue
			}
		}
		result = append(result, row)
	}
	errCheck(errors)

	return result
}

// Filter excludes resource name & type
func excludeByResource(rawData []types.Event, excludeResourceList []string, targetFilter func(types.Resource) *string) []types.Event {
	matchSet := make(map[string]struct{}, len(excludeResourceList))
	errors := make(map[string]bool, len(excludeResourceList))
	for _, filters := range excludeResourceList {
		matchSet[filters] = struct{}{}
		errors[filters] = false
	}

	var result []types.Event

	for _, row := range rawData {
		exclude := false
		for _, resource := range row.Resources {
			if filter := targetFilter(resource); filter != nil {
				if _, ok := matchSet[*filter]; ok {
					errors[*filter] = true
					exclude = true
					break
				}
			}
		}
		if !exclude {
			result = append(result, row)
		}
	}
	errCheck(errors)

	return result
}

func excludeByArn(rawData []types.Event, ArnValue []string) []types.Event {
	matchSet := make(map[string]struct{}, len(ArnValue))
	errors := make(map[string]bool, len(ArnValue))
	for _, values := range ArnValue {
		matchSet[values] = struct{}{}
		errors[values] = false
	}

	var result []types.Event
	for _, row := range rawData {
		raw, err := ExtractUserDetails(row.CloudTrailEvent)
		if err != nil {
			fmt.Printf("[Error] Error extracting event details: %v", err)
		}
		if _, ok := matchSet[raw.UserIdentity.SessionContext.SessionIssuer.UserName]; ok {
			errors[raw.UserIdentity.SessionContext.SessionIssuer.UserName] = true
			break
		}
		result = append(result, row)
	}
	errCheck(errors)
	return result
}

// Errorchecking for mismatch filters
func errCheck(errCheck map[string]bool) {
	for k, v := range errCheck {
		if !v {
			fmt.Printf("Warning: Filter not found %s \n", k)
		}
	}
}

/*
Some Comments
	- In api_op_LookupEvents.go. The api call can only contain 1 item in the list even though it is a list
		therefore if trying to call {ReadOnly} & {Username}. Both sets will be returned as an OR logic instead of
		AND logic. (I tried doing multiple tests of this, decided not to change the API call as I am unsure what else it is affected by)

		EDIT: The file seems to be "generated" and it was suggesting not to modify the current code

	- Moved Filter to a new filter.go file for readability.
		Values of filters are passed in. No pointers/addresses needed bc we dont have to modify the existing data.

	- Current Filters works in an "AND" Logic Operation:
		i.e When Filtering by ResourceName & ResourceType ([ResourceName1, ResourceName2], [AWS::ExampleHostedZone, AWS::ExampleCreateZone])

		It will first filter by getting all ResourceName1 & 2.
			Takes results of filter from ResourceName1 & 2.
		ResourceName1 & 2 Has AWS::ExampleHostedZone. However AWS::ExampleCreateZone Does Exist but only with ResourceName3.
			Therefore it will show an error stating [Warning: Could not find filter AWS::ExampleCreateZone] even though it does exist

	- Speed
		- O(n^2) worse case time (Assuming each filter exist in each row of data)
		- O(n log n) Average Case Time
			Each row of Data is checked with a Map O(n)
			As the filter size decreases depending on the amount of filters. On average it will decrease by half O(log n)
		- O(n) best case time
			Assuming we only have 1 filter. We only need to go through each row of data once.

	Issues:
		Cannot seem to know where the examples
*/
