package cloudtrail

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/openshift/osdctl/cmd/cloudtrail/pkg"
	"github.com/stretchr/testify/assert"
)

func TestIgnoreListFilter(t *testing.T) {
	// Test Case 1 (Ignored)
	testUsername1 := "user-1"
	testCloudTrailEvent1 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-kube-system-capa-controller-manager/123456789012"}}}}`

	// Test Case 2 (Ignored)
	testUsername2 := "ManagedOpenShift-ControlPlane-Role"
	testCloudTrailEvent2 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-kube-system-capa-controller-manager/123456789012"}}}}`

	// Test Case 3 (Not Ignored)
	testUsername3 := "user-2"
	testCloudTrailEvent3 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/user-2"}}}}`

	// Test Case 4 (Not Ignored)
	var testUsername4 string //nil username
	testCloudTrailEvent4 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/NilUsername-1"}}}}`

	// Test Case 5 (Edge Cases (Not Ignored))
	testUsername5 := "user-5"
	testCloudTrailEvent5 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": ""}}}}`

	// Test Case 5 (Edge Cases (Ignored))
	var testUsername6 *string
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
				{Username: testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
			},
		},
	}

	// Other Filterable Option which would be located in ~/.config/osdctl.yaml
	//{".*-Installer-Role", ".*kube-system-kube-controller.*", ".*operator.*", ".*openshift-cluster-csi-drivers.*",".*kube-system-capa-controller.*"}

	ignoreList := []string{".*kube-system-capa-controller.*"}
	mergedList := pkg.MergeRegex(ignoreList)
	var emptyMergedList string

	// Test filtering if shouldFilter set to false
	t.Run("Filtering with shouldFilter false", func(t *testing.T) {
		expectedFilteredEvents := []types.Event{
			{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
			{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
	t.Run("Filtering with IgnoreList", func(t *testing.T) {

		expected := []*cloudtrail.LookupEventsOutput{
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

		filtered := pkg.Filters[3](TestLookupOutputs, mergedList)
		assert.Equal(t, expected, filtered, "Number of filtered events mismatch")

	})

	// Test filtering if shouldFilter set to true
	t.Run("Filtering with shouldFilter true", func(t *testing.T) {
		expectedFilteredEvents := []types.Event{
			{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
			{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
			{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
			{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
			{Username: testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
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
			{Username: testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
		}

		filtered2, err := pkg.FilterUsers(TestLookupOutputs, emptyIgnoreList, false)
		assert.NoError(t, err, "Error filtering events")
		assert.Equal(t, len(expectedFilteredEvents2), len(*filtered2), "Number of filtered events mismatch")
	t.Run("Filtering with Empty IgnoreList", func(t *testing.T) {

		expected := []*cloudtrail.LookupEventsOutput{
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

		filtered := pkg.Filters[3](TestLookupOutputs, emptyMergedList)
		assert.Equal(t, expected, filtered, "Number of filtered events mismatch")

	})

}
func TestSearchFilter(t *testing.T) {

	// Test Case 1 (Found)
	testUsername1 := "user-1"
	testCloudTrailEvent1 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-/123456772RH-SRE."}}}}`

	// Test Case 2 (Found)
	testUsername2 := "RH-SRE.xxx.openshift"
	testCloudTrailEvent2 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-/123456772"}}}}`

	// Test Case 3 (Nil Username) (Not Found)
	var testUsername3 string //nil username
	testCloudTrailEvent3 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/NilUsername-1"}}}}`

	// Test case 4 (Does not match search case exactly)(Not Found)
	testUsername4 := "RH-"
	testCloudTrailEvent4 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-/RH-23456772"}}}}`

	// Test case 5 (Nil Session Issuer)(Found)
	testUsername5 := "RH-SRE.George.openshift"
	testCloudTrailEvent5 := `{"eventVersion": "1.08"}`

	TestLookupOutputs := []*cloudtrail.LookupEventsOutput{
		{
			Events: []types.Event{
				{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
				{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
				{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
			},
		},
		{
			Events: []types.Event{
				{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
				{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			},
		},
	}

	t.Run("Test Search by Username", func(t *testing.T) {
		expected := []*cloudtrail.LookupEventsOutput{
			{
				Events: []types.Event{
					{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
					{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
				},
			},
		}
		searchValue := "RH-SRE"

		filtered := pkg.Filters[2](TestLookupOutputs, searchValue)
		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

	t.Run("Test Search by Arn", func(t *testing.T) {
		expected := []*cloudtrail.LookupEventsOutput{
			{
				Events: []types.Event{
					{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
					{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
				},
			},
		}
		searchValue := "RH-SRE"

		filtered := pkg.Filters[2](TestLookupOutputs, searchValue)
		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

	t.Run("Test for Empty search Input", func(t *testing.T) {
		expected := []*cloudtrail.LookupEventsOutput{
			{
				Events: []types.Event{
					{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
					{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
					{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
				},
			},
			{
				Events: []types.Event{
					{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
				},
			},
		}
		searchValue := ""

		filtered := pkg.Filters[2](TestLookupOutputs, searchValue)
		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

	t.Run("Non Existent Event", func(t *testing.T) {
		expected := []*cloudtrail.LookupEventsOutput{}
		searchValue := "NotThere"

		filtered := pkg.Filters[2](TestLookupOutputs, searchValue)
		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

	t.Run("Nil Session Issuer", func(t *testing.T) {
		expected := []*cloudtrail.LookupEventsOutput{
			{
				Events: []types.Event{
					{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
				},
			},
		}
		searchValue := ".George."

		filtered := pkg.Filters[2](TestLookupOutputs, searchValue)
		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})
}

func TestPermissonDeniedFilter(t *testing.T) {
	// Test Case 1 (Ignored)
	testUsername1 := "RH-SRE-xxx.openshift"
	testCloudTrailEvent1 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-kube-system-capa-controller-manager/RH-SRE-xxx.openshift"}}}, "errorCode": "Client.UnauthorizedOperation"}`

	testUsername2 := "ManagedOpenShift-ControlPlane-Role"
	testCloudTrailEvent2 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-kube-system-capa-controller-manager/123456789012"}}}, "errorCode": "Client.UnauthorizedOperation"}`

	var testUsername3 string //nil username
	testCloudTrailEvent3 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/NilUsername-1"}}}}`

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
			},
		},
	}

	t.Run("Test Search by PermissionDenied", func(t *testing.T) {
		expected := []*cloudtrail.LookupEventsOutput{
			{
				Events: []types.Event{
					{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
					{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
				},
			},
		}

		search = ".*Client.UnauthorizedOperation.*"

		filtered := pkg.Filters[1](TestLookupOutputs, search)
		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

}
