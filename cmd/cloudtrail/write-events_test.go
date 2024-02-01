package cloudtrail

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfiguration(t *testing.T) {

	config, err := LoadConfiguration()

	if err != nil {

		t.Errorf("Error loading configuration: %v", err)
	}

	if config == nil {
		t.Error("Configuration is nil")
	}

}

func TestFilterUsers(t *testing.T) {
	// Test Case 1
	unfilteredUsername1 := "user-1"
	cloudTrailEvent1 := `{"userIdentity": {"sessionContext": {"sessionIssuer": {"userName": "user3"}}}}`

	// Test Case 2
	cloudTrailEvent2 := `{"userIdentity": {"sessionContext": {"sessionIssuer": {"userName": "ManagedOpenShift-ControlPlane-Role"}}}}`

	// Test Case 3
	cloudTrailEvent3 := `{"userIdentity": {"sessionContext": {"sessionIssuer": {"userName": "ManagedOpenShift-ControlPlane-Role"}}}}`

	TestLookupOutputs := []*cloudtrail.LookupEventsOutput{
		{
			Events: []types.Event{
				{Username: &unfilteredUsername1, CloudTrailEvent: &cloudTrailEvent1},
			},
		},
		{
			Events: []types.Event{
				{CloudTrailEvent: &cloudTrailEvent3},
			},
		},
		{
			Events: []types.Event{
				{CloudTrailEvent: &cloudTrailEvent2},
			},
		},
	}

	expectedFilteredEvents := []types.Event{
		{Username: &unfilteredUsername1, CloudTrailEvent: &cloudTrailEvent1},
		{CloudTrailEvent: &cloudTrailEvent3},
	}

	config, err := LoadConfiguration()
	assert.NoError(t, err, "Error loading configuration")

	Ignore, err := LoadConfigList(config)
	assert.NoError(t, err, "Error loading Ignore List")
	for i, lookupOutput := range TestLookupOutputs {
		fmt.Printf("Lookup Output %d: %+v\n", i, lookupOutput)
		for j, event := range lookupOutput.Events {
			fmt.Printf("  Event %d: %+v\n", j, event)
		}
	}

	filtered, err := FilterUsers(TestLookupOutputs, Ignore)
	assert.NoError(t, err, "Error filtering events")

	// Print actual and expected events for troubleshooting
	fmt.Printf("Actual filtered events: %+v\n", *filtered)
	fmt.Printf("Expected events: %+v\n", expectedFilteredEvents)

	assert.Equal(t, len(expectedFilteredEvents), len(*filtered), "Number of filtered events mismatch")

	for i, expectedEvent := range expectedFilteredEvents {
		assert.True(t, reflect.DeepEqual((*filtered)[i], expectedEvent), "Filtered event mismatch")
	}
}
