package testdata

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	cloudtrail "github.com/openshift/osdctl/cmd/cloudtrail"
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
	testUsername3 := "user-3"
	testCloudTrailEvent3 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/user-2"}}}}`

	// Test Case 4 (Not Ignored)
	var testUsername4 string //nil username
	testCloudTrailEvent4 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:role/NilUsername-1"}}}}`

	// Test Case 5 (Edge Cases (Not Ignored))
	testUsername5 := "user-5"
	testCloudTrailEvent5 := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": ""}}}}`

	// Test Case 6 (Edge Cases (Ignored))
	var testUsername6 *string
	testCloudTrailEvent6 := `{"eventVersion": "1.09","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": ""}}}}`

	testEvents := []types.Event{

		{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
		{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},

		{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
		{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},

		{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
		{Username: testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
	}

	// Other Filterable Option which would be located in ~/.config/osdctl.yaml
	//{".*-Installer-Role", ".*kube-system-kube-controller.*", ".*operator.*", ".*openshift-cluster-csi-drivers.*",".*kube-system-capa-controller.*"}

	t.Run("Test Filtering by IgnoreList", func(t *testing.T) {

		expected := []types.Event{

			{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
			{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
		}

		ignoreList := []string{".*kube-system-capa-controller.*"}
		filtered, err := cloudtrail.ApplyFilters(testEvents,
			func(event types.Event) (bool, error) {
				return cloudtrail.IsIgnoredEvent(event, strings.Join(ignoreList, "|"))
			},
		)

		assert.Nil(t, err)
		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

	t.Run("Test Filtering by Empty IgnoreList", func(t *testing.T) {

		expected := []types.Event{
			{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
			{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
			{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
			{Username: &testUsername4, CloudTrailEvent: &testCloudTrailEvent4},
			{Username: &testUsername5, CloudTrailEvent: &testCloudTrailEvent5},
			{Username: testUsername6, CloudTrailEvent: &testCloudTrailEvent6},
		}

		ignoreList := []string{}
		filtered, err := cloudtrail.ApplyFilters(testEvents,
			func(event types.Event) (bool, error) {
				return cloudtrail.IsIgnoredEvent(event, strings.Join(ignoreList, "|"))
			},
		)
		assert.Nil(t, err)

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

	TestEvents := []types.Event{

		{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
		{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
		{Username: &testUsername3, CloudTrailEvent: &testCloudTrailEvent3},
	}

	t.Run("Test Search by PermissionDenied", func(t *testing.T) {
		expected := []types.Event{

			{Username: &testUsername1, CloudTrailEvent: &testCloudTrailEvent1},
			{Username: &testUsername2, CloudTrailEvent: &testCloudTrailEvent2},
		}

		filtered, err := cloudtrail.ApplyFilters(TestEvents,
			func(event types.Event) (bool, error) {
				return cloudtrail.IsforbiddenEvent(event)
			},
		)
		assert.Nil(t, err)

		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

	t.Run("Test for Different ErrorCode", func(t *testing.T) {
		edgeCaseUsername := "RH-SRE-xxx.openshift"
		edgeCaseCloudtrailEvent := `{"eventVersion": "1.08","userIdentity": {"sessionContext": {"sessionIssuer": {"arn": "arn:aws:iam::123456789012:user/test-12345-6-a7b8-kube-system-capa-controller-manager/123456789012"}}}, "errorCode": "TrailNotFoundException"}`

		edgeCaseEvents := []types.Event{

			{Username: &edgeCaseUsername, CloudTrailEvent: &edgeCaseCloudtrailEvent},
		}
		expected := []types.Event{}

		filtered, err := cloudtrail.ApplyFilters(edgeCaseEvents,
			func(event types.Event) (bool, error) {
				return cloudtrail.IsforbiddenEvent(event)
			},
		)
		assert.Nil(t, err)

		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

	t.Run("Test No ErrorCode", func(t *testing.T) {
		edgeCaseUsername := "RH-SRE-xxx.openshift"
		edgeCaseCloudtrailEvent := `{"eventVersion": "1.08"}`

		edgeCaseEvents := []types.Event{
			{Username: &edgeCaseUsername, CloudTrailEvent: &edgeCaseCloudtrailEvent},
		}
		expected := []types.Event{}

		filtered, err := cloudtrail.ApplyFilters(edgeCaseEvents,
			func(event types.Event) (bool, error) {
				return cloudtrail.IsforbiddenEvent(event)
			},
		)
		assert.Nil(t, err)

		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

	t.Run("Test Nil Cloudtrail Event", func(t *testing.T) {
		edgeCaseUsername := "RH-SRE-xxx.openshift"
		var edgeCaseCloudtrailEvent string

		edgeCaseEvents := []types.Event{

			{Username: &edgeCaseUsername, CloudTrailEvent: &edgeCaseCloudtrailEvent},
		}
		expected := []types.Event{}
		filtered, err := cloudtrail.ApplyFilters(edgeCaseEvents,
			func(event types.Event) (bool, error) {
				return cloudtrail.IsforbiddenEvent(event)
			},
		)
		assert.EqualErrorf(t, err, "[ERROR] failed to extract raw CloudTrail event details: cannot parse a nil input", "")

		assert.Equal(t, len(expected), len(filtered), "Filtered events do not match expected results")
	})

}
