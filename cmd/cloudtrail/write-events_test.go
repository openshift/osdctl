package cloudtrail

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
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
	unfilterdUsername1 := "user-1"
	unfilterdUsername2 := "user-2"
	unfilterdUsername3 := "user-3"
	var nilUsername3 string
	ignorededUsername1 := "ManagedOpenShift-ControlPlane-Role"
	ignorededUsername2 := "-Installer-Role"

	TestLookupOutputs := []*cloudtrail.LookupEventsOutput{
		{
			Events: []types.Event{
				{Username: &unfilterdUsername1},
				{Username: &ignorededUsername1},
			},
		},
		{
			Events: []types.Event{
				{Username: &unfilterdUsername2},
				{Username: &ignorededUsername2},
			},
		},
		{
			Events: []types.Event{
				{Username: &unfilterdUsername3},
				{Username: &nilUsername3},
			},
		},
	}
	expectedFilteredEvents := []types.Event{
		{Username: &unfilterdUsername1},
		{Username: &unfilterdUsername2},
		{Username: &unfilterdUsername3},
		{Username: &nilUsername3},
	}

	filtered, err := FilterUsers(TestLookupOutputs)
	if err != nil {
		t.Error(err)

	}

	if !reflect.DeepEqual(filtered, &expectedFilteredEvents) {
		t.Errorf("FilterUsers test failed: Expected %v, got %v", expectedFilteredEvents, *filtered)

	}

}
