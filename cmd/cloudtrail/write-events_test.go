package cloudtrail

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/openshift/osdctl/cmd/cloudtrail/pkg"
	"github.com/stretchr/testify/assert"
)

func TestFilterUsers(t *testing.T) {
	// Test Case 1 (Ignored)
	testUsername1 := "user-1"
	testCloudTrailEvent1 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-kube-system-capa-controller-manager/123456789012"}}}}`

	testUsername2 := "ManagedOpenShift-ControlPlane-Role"
	testCloudTrailEvent2 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-kube-system-capa-controller-manager/123456789012"}}}}`

	// Test Case 2 (Not Ignored)
	testUsername3 := "user-2"
	testCloudTrailEvent3 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/user-2"}}}}`

	var testUsername4 string //nil username
	testCloudTrailEvent4 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/NilUsername-1"}}}}`

	// Test Case 3 (Edge Cases)

	testUsername5 := "user-5"
	testCloudTrailEvent5 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": ""}}}}`

	var testUsername6 string
	testCloudTrailEvent6 := `{"eventVersion": "1.09","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": ""}}}}`

	TestLookupOutputs := []*cloudtrail.LookupEventsOutput{
		{
			Events: []types.Event{
				{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
				{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
			},
		},
		{
			Events: []types.Event{
				{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
				{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			},
		},
		{
			Events: []types.Event{
				{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
				{Username: &testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
			},
		},
	}

	// Other Filterable Option which would be located in ~/.config/osdctl.yaml
	//{".*-Installer-Role", ".*kube-system-kube-controller.*", ".*operator.*", ".*openshift-cluster-csi-drivers.*",".*kube-system-capa-controller.*"}

	ignoreList := []string{".*kube-system-capa-controller.*"}
	emptyIgnoreList := []string{}

	// Test filtering if shouldFilter set to false
	t.Run("Filtering with shouldFilter false", func(t *testing.T) {
		expectedFilteredEvents := []types.Event{
			{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
			{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
			{Username: &testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
		}

		filtered, err := pkg.FilterUsers(TestLookupOutputs, ignoreList, false)
		assert.NoError(t, err, "Error filtering events")

		assert.Equal(t, len(expectedFilteredEvents), len(*filtered), "Number of filtered events mismatch")

	})

	// Test filtering if shouldFilter set to true
	t.Run("Filtering with shouldFilter true", func(t *testing.T) {
		expectedFilteredEvents := []types.Event{
			{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
			{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
			{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
			{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
			{Username: &testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
		}

		filtered, err := pkg.FilterUsers(TestLookupOutputs, ignoreList, true)
		assert.NoError(t, err, "Error filtering events")

		assert.Equal(t, len(expectedFilteredEvents), len(*filtered), "Number of filtered events mismatch")
	})

	// Test filtering if ~/.config/osdctl.yaml is Empty

	t.Run(("Filtering with Empty list"), func(t *testing.T) {
		expectedFilteredEvents2 := []types.Event{
			{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
			{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
			{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
			{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
			{Username: &testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
		}

		filtered2, err := pkg.FilterUsers(TestLookupOutputs, emptyIgnoreList, false)
		assert.NoError(t, err, "Error filtering events")
		assert.Equal(t, len(expectedFilteredEvents2), len(*filtered2), "Number of filtered events mismatch")

	})

}
