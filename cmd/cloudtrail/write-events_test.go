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
	// Test Case 1 (Ignored)
	testUsername1 := "user-1"
	testCloudTrailEvent1 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/ManagedOpenShift-ControlPlane-Role"}}}}`

	// Test Case 2 (Not Ignored)
	testUsername2 := "user-2"
	testCloudTrailEvent2 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/user-2"}}}}`

	// Test Case 3 (Ignored)
	testUsername3 := "ManagedOpenShift-ControlPlane-Role"
	testCloudTrailEvent3 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/ManagedOpenShift-ControlPlane-Role"}}}}`

	// Test Case 3 (Not Ignored (nil Username))
	var testUsername4 string
	testCloudTrailEvent4 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/NilUsername-1"}}}}`

	TestLookupOutputs := []*cloudtrail.LookupEventsOutput{
		{
			Events: []types.Event{
				{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
			},
		},
		{
			Events: []types.Event{
				{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
			},
		},
		{
			Events: []types.Event{
				{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
				{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			},
		},
	}

	//Expected Results
	expectedFilteredEvents := []types.Event{
		{Username: &testCloudTrailEvent2, CloudTrailEvent: &testCloudTrailEvent2},
		{Username: &testCloudTrailEvent4, CloudTrailEvent: &testCloudTrailEvent4},
	}

	// Mock Ignore slice of string
	// Other Filterable Option which would be located in ~/.config/osdctl
	//{".*-Installer-Role", ".*kube-system-kube-controller.*", ".*operator.*", ".*openshift-cluster-csi-drivers.*",".*kube-system-capa-controller.*"}

	Ignore := []string{".*-ControlPlane-Role"}
	shouldFilter := true

	filtered, err := FilterUsers(TestLookupOutputs, Ignore, shouldFilter)
	assert.NoError(t, err, "Error filtering events")

	assert.Equal(t, len(expectedFilteredEvents), len(*filtered), "Number of filtered events mismatch")
}
