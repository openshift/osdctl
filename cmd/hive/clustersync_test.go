package hive

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	mockk8s "github.com/openshift/osdctl/cmd/hive/clusterdeployment/mock/k8s"
)


// Mock data for ClusterDeployment and ClusterSync that are used across all test cases
var (
	cdList = hivev1.ClusterDeploymentList{
		Items: []hivev1.ClusterDeployment{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-cluster",
					Namespace: "uhc-production-1234",
				},
				Status: hivev1.ClusterDeploymentStatus{
					Conditions: []hivev1.ClusterDeploymentCondition{
						{
							Type:   "Hibernating",
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
		},
	}
	csList = v1alpha1.ClusterSyncList{
		Items: []v1alpha1.ClusterSync{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-clustersync",
					Namespace: "uhc-production-1234",
				},
				Status: v1alpha1.ClusterSyncStatus{
					Conditions: []v1alpha1.ClusterSyncCondition{
						{
							Type:               "Ready",
							Status:             corev1.ConditionFalse,
							Reason:             "Failure",
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
					SyncSets: []v1alpha1.SyncStatus{
						{
							Name:           "syncset1",
							Result:         "Failure",
							FailureMessage: "Failed to sync syncset1",
						},
					},
					SelectorSyncSets: []v1alpha1.SyncStatus{
						{
							Name:           "selectorsyncset1",
							Result:         "Failure",
							FailureMessage: "Failed to sync selectorsyncset1",
						},
					},
				},
			},
		},
	}
)

// Central function to mock the List method and set expectations
func setupMockClient(mockClient *mockk8s.MockClient, returnErr error, isEmpty bool) {
	callTimes := 1
	if returnErr == nil {
		callTimes = 2 // We expect two calls only if there's no error
	}
	mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			switch v := list.(type) {
			case *hivev1.ClusterDeploymentList:
				*v = cdList
			case *v1alpha1.ClusterSyncList:
				if isEmpty {
					*v = v1alpha1.ClusterSyncList{} // Return empty ClusterSyncList for the empty case
				} else {
					*v = csList // Assuming csList is populated normally
				}
			}
			return returnErr
		}).Times(callTimes) // Expect List to be called n times based on the error condition
}

func TestNewCmdClusterSyncFailures(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	mockClient := mockk8s.NewMockClient(mockCtrl)

	setupMockClient(mockClient, nil, false)

	cmd := NewCmdClusterSyncFailures(streams, mockClient)
	assert.Equal(t, "clustersync-failures [flags]", cmd.Use, "Command use should be 'clustersync-failures [flags]'")
	assert.Contains(t, cmd.Long, "Helps investigate ClusterSyncs", "Command long description should be set")
	assert.Contains(t, cmd.Example, "$ osdctl hive csf", "Command example should be set")

	cmd.Flags().Set("limited-support", "true")
	includeLimitedSupport := cmd.Flags().Lookup("limited-support").Value.String()
	assert.Equal(t, "true", includeLimitedSupport, "Flag 'limited-support' should be parsed correctly")

	cmd.Flags().Set("hibernating", "true")
	includeHibernating := cmd.Flags().Lookup("hibernating").Value.String()
	assert.Equal(t, "true", includeHibernating, "Flag 'hibernating' should be parsed correctly")

	cmd.Flags().Set("syncsets", "false")
	includeFailingSyncSets := cmd.Flags().Lookup("syncsets").Value.String()
	assert.Equal(t, "false", includeFailingSyncSets, "Flag 'syncsets' should be parsed correctly")

	cmd.Flags().Set("no-headers", "true")
	noHeaders := cmd.Flags().Lookup("no-headers").Value.String()
	assert.Equal(t, "true", noHeaders, "Flag 'no-headers' should be parsed correctly")

	cmd.Flags().Set("output", "yaml")
	output := cmd.Flags().Lookup("output").Value.String()
	assert.Equal(t, "yaml", output, "Flag 'output' should be parsed correctly")

	cmd.Flags().Set("cluster-id", "1234")
	cmd.Flags().Set("limited-support", "true")
	cmd.Flags().Set("hibernating", "false")
	cmd.Flags().Set("syncsets", "true")
	cmd.Flags().Set("output", "json")

	err := cmd.Execute()
	assert.NoError(t, err, "Command should execute without error")

	err = cmd.Flags().Set("invalid-flag", "true")
	assert.Error(t, err, "Setting invalid flag should return an error")
}

func testListFailingClusterSyncs(t *testing.T, returnErr error, expectedResults []failingClusterSync, isEmpty bool) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockClient := mockk8s.NewMockClient(mockCtrl)

	setupMockClient(mockClient, returnErr, isEmpty)

	options := &clusterSyncFailuresOptions{
		kubeCli: mockClient,
	}

	result, err := options.listFailingClusterSyncs()

	if returnErr != nil {
		assert.Error(t, err)
		assert.Nil(t, result)
	} else {
		assert.NoError(t, err)
		if isEmpty {
			// If isEmpty is true, the result should be an empty slice
			assert.Len(t, result, 0)
		} else {
			assert.NotNil(t, result)
			assert.Len(t, result, len(expectedResults))
			for i, expected := range expectedResults {
				assert.Equal(t, expected.Name, result[i].Name)
				assert.Equal(t, expected.Namespace, result[i].Namespace)
				assert.Equal(t, expected.LimitedSupport, result[i].LimitedSupport)
				assert.Equal(t, expected.Hibernating, result[i].Hibernating)
				assert.Contains(t, result[i].FailingSyncSets, expected.FailingSyncSets)
				assert.Contains(t, result[i].ErrorMessage, expected.ErrorMessage)
			}
		}
	}
}

func TestListFailingClusterSyncs(t *testing.T) {

	testCases := []struct {
		name           string
		errorToReturn  error
		expectedResult []failingClusterSync
		isEmpty        bool
	}{
		{
			name:          "Success scenario with expected results",
			errorToReturn: nil,
			expectedResult: []failingClusterSync{
				{
					Name:            "example_clustersync",
					Namespace:       "uhc-production-1234",
					Timestamp:       time.Now().Format(time.RFC3339),
					LimitedSupport:  false,
					Hibernating:     true,
					FailingSyncSets: "selectorsyncset1 syncset1 ",
					ErrorMessage:    "Failed to sync selectorsyncset1\n\nFailed to sync syncset1\n\n",
				},
			},
			isEmpty: false,
		},
		{
			name:           "Empty_results_scenario(List_returns_no_items)",
			errorToReturn:  nil,
			expectedResult: []failingClusterSync{}, // Expecting empty results
			isEmpty:        true,
		},
		{
			name:           "Error_scenario",
			errorToReturn:  errors.New("failed to list ClusterSync resources due to network timeout"), // Triggering error condition
			expectedResult: nil,
			isEmpty:        false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testListFailingClusterSyncs(t, tc.errorToReturn, tc.expectedResult, tc.isEmpty)
		})
	}
}

func TestSortBy(t *testing.T) {

	tests := []struct {
		name      string
		sortField string
		sortOrder string
		expected  []failingClusterSync
	}{
		// Test sorting by name in ascending order
		{
			name:      "sort_by_name_ascending_order",
			sortField: "name",
			sortOrder: "asc",
			expected: []failingClusterSync{
				{Name: "alpha", Timestamp: "2022-01-01T00:00:00Z", FailingSyncSets: "syncset1"},
				{Name: "beta", Timestamp: "2023-02-01T00:00:00Z", FailingSyncSets: "syncset2"},
				{Name: "zeta", Timestamp: "2023-01-01T00:00:00Z", FailingSyncSets: "syncset3"},
			},
		},
		// Test sorting by name in descending order
		{
			name:      "sort_by_name_descending_order",
			sortField: "name",
			sortOrder: "desc",
			expected: []failingClusterSync{
				{Name: "zeta", Timestamp: "2023-01-01T00:00:00Z", FailingSyncSets: "syncset3"},
				{Name: "beta", Timestamp: "2023-02-01T00:00:00Z", FailingSyncSets: "syncset2"},
				{Name: "alpha", Timestamp: "2022-01-01T00:00:00Z", FailingSyncSets: "syncset1"},
			},
		},
		// Test sorting by timestamp in ascending order
		{
			name:      "sort_by_timestamp_ascending_order",
			sortField: "timestamp",
			sortOrder: "asc",
			expected: []failingClusterSync{
				{Name: "alpha", Timestamp: "2022-01-01T00:00:00Z", FailingSyncSets: "syncset1"},
				{Name: "zeta", Timestamp: "2023-01-01T00:00:00Z", FailingSyncSets: "syncset3"},
				{Name: "beta", Timestamp: "2023-02-01T00:00:00Z", FailingSyncSets: "syncset2"},
			},
		},
		// Test sorting by timestamp in descending order
		{
			name:      "sort_by_timestamp_descending_order",
			sortField: "timestamp",
			sortOrder: "desc",
			expected: []failingClusterSync{
				{Name: "beta", Timestamp: "2023-02-01T00:00:00Z", FailingSyncSets: "syncset2"},
				{Name: "zeta", Timestamp: "2023-01-01T00:00:00Z", FailingSyncSets: "syncset3"},
				{Name: "alpha", Timestamp: "2022-01-01T00:00:00Z", FailingSyncSets: "syncset1"},
			},
		},
		// Test sorting by failingSyncSets in ascending order
		{
			name:      "sort_by_failingSyncSets_ascending_order",
			sortField: "failingsyncsets",
			sortOrder: "asc",
			expected: []failingClusterSync{
				{Name: "alpha", Timestamp: "2022-01-01T00:00:00Z", FailingSyncSets: "syncset1"},
				{Name: "beta", Timestamp: "2023-02-01T00:00:00Z", FailingSyncSets: "syncset2"},
				{Name: "zeta", Timestamp: "2023-01-01T00:00:00Z", FailingSyncSets: "syncset3"},
			},
		},
		// Test sorting by failingSyncSets in descending order
		{
			name:      "sort_by_failingSyncSets_descending_order",
			sortField: "failingsyncsets",
			sortOrder: "desc",
			expected: []failingClusterSync{
				{Name: "zeta", Timestamp: "2023-01-01T00:00:00Z", FailingSyncSets: "syncset3"},
				{Name: "beta", Timestamp: "2023-02-01T00:00:00Z", FailingSyncSets: "syncset2"},
				{Name: "alpha", Timestamp: "2022-01-01T00:00:00Z", FailingSyncSets: "syncset1"},
			},
		},
		// Test invalid sort field
		{
			name:      "invalid_sort_order",
			sortField: "invalid",
			sortOrder: "asc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &clusterSyncFailuresOptions{
				sortField: tt.sortField,
				sortOrder: tt.sortOrder,
			}

			failingClusterSyncList := []failingClusterSync{
				{Name: "zeta", Timestamp: "2023-01-01T00:00:00Z", FailingSyncSets: "syncset3"},
				{Name: "alpha", Timestamp: "2022-01-01T00:00:00Z", FailingSyncSets: "syncset1"},
				{Name: "beta", Timestamp: "2023-02-01T00:00:00Z", FailingSyncSets: "syncset2"},
			}

			err := options.sortBy(failingClusterSyncList)
			if tt.name == "invalid_sort_order" {
				assert.Error(t, err)
				assert.Equal(t, "Specify one of the following fields as a sort argument: name, timestamp, failingsyncsets.", err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, failingClusterSyncList)
			}
		})
	}
}
