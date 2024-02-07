package cloudtrail

import (
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
	cloudTrailEvent1 := `{"userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/ManagedOpenShift-ControlPlane-Role"}}}}`

	// Test Case 2
	unfilteredUsername2 := "user-2"
	cloudTrailEvent2 := `{"userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/user-2"}}}}`

	// Test Case 3 (Ignored)
	ignoredser1 := "ManagedOpenShift-ControlPlane-Role"
	cloudTrailEvent3 := `{"userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/ManagedOpenShift-ControlPlane-Role"}}}}`

	TestLookupOutputs := []*cloudtrail.LookupEventsOutput{
		{
			Events: []types.Event{
				{Username: &unfilteredUsername1, CloudTrailEvent: &cloudTrailEvent1},
			},
		},
		{
			Events: []types.Event{
				{Username: &unfilteredUsername2, CloudTrailEvent: &cloudTrailEvent2},
			},
		},
		{
			Events: []types.Event{
				{Username: &ignoredser1, CloudTrailEvent: &cloudTrailEvent3},
			},
		},
	}

	expectedFilteredEvents := []types.Event{
		{Username: &unfilteredUsername2, CloudTrailEvent: &cloudTrailEvent2},
	}

	Ignore := []string{".*-ControlPlane-Role"}

	filtered, err := FilterUsers(TestLookupOutputs, Ignore)
	assert.NoError(t, err, "Error filtering events")

	assert.Equal(t, len(expectedFilteredEvents), len(*filtered), "Number of filtered events mismatch")
}
