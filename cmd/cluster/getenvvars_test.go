package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCmdGetEnvVars(t *testing.T) {
	cmd := newCmdGetEnvVars()

	assert.NotNil(t, cmd)
	assert.Equal(t, "get-env-vars --cluster-id <cluster-identifier>", cmd.Use)
	assert.Equal(t, "Print a cluster's ID/management namespaces, optionally as env variables", cmd.Short)

	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("cluster-id"), "Command should have a cluster-id flag")
	assert.NotNil(t, flags.Lookup("output"))

	// Check default values
	output, _ := flags.GetString("output")
	assert.Equal(t, "text", output)
}

type testCase struct {
	name               string
	cluster            formattedOutputCluster
	expectedTextOutput string
	expectedJSONOutput string
	expectedEnvOutput  string
}

func TestPrintOutput(t *testing.T) {
	testCases := []testCase{
		{
			name: "Classic ROSA",
			cluster: formattedOutputCluster{
				Name:          "test-classic",
				ID:            "1234",
				ExternalID:    "1-2-3-4",
				HiveNamespace: "uhc-staging-1234",
			},
			expectedTextOutput: "" +
				"Name:               test-classic\n" +
				"ID:                 1234\n" +
				"External ID:        1-2-3-4\n" +
				"Hive namespace:     uhc-staging-1234\n",
			expectedJSONOutput: `{"name":"test-classic","id":"1234","external_id":"1-2-3-4","hive_namespace":"uhc-staging-1234"}`,
			expectedEnvOutput: "" +
				"export CLUSTER_NAME=test-classic\n" +
				"export CLUSTER_ID=1234\n" +
				"export CLUSTER_UUID=1-2-3-4\n" +
				"export HIVE_NAMESPACE=uhc-staging-1234\n",
		},
		{
			name: "ROSA w/ HCP",
			cluster: formattedOutputCluster{
				Name:                   "test-rosa-hcp",
				ID:                     "1234",
				ExternalID:             "1-2-3-4",
				HCPNamespace:           "ocm-staging-1234-test-rosa-hcp",
				HostedClusterNamespace: "ocm-staging-1234",
				KlusterletNamespace:    "klusterlet-1234",
			},
			expectedTextOutput: "" +
				"Name:                   test-rosa-hcp\n" +
				"ID:                     1234\n" +
				"External ID:            1-2-3-4\n" +
				"HCP namespace:          ocm-staging-1234-test-rosa-hcp\n" +
				"HC namespace:           ocm-staging-1234\n" +
				"Klusterlet namespace:   klusterlet-1234\n",
			expectedJSONOutput: `{"name":"test-rosa-hcp","id":"1234","external_id":"1-2-3-4","hcp_namespace":"ocm-staging-1234-test-rosa-hcp","hosted_cluster_namespace":"ocm-staging-1234","klusterlet_namespace":"klusterlet-1234"}`,
			expectedEnvOutput: "" +
				"export CLUSTER_NAME=test-rosa-hcp\n" +
				"export CLUSTER_ID=1234\n" +
				"export CLUSTER_UUID=1-2-3-4\n" +
				"export HCP_NAMESPACE=ocm-staging-1234-test-rosa-hcp\n" +
				"export HC_NAMESPACE=ocm-staging-1234\n" +
				"export KLUSTERLET_NAMESPACE=klusterlet-1234\n",
		},
	}

	for _, tc := range testCases {
		testTextOutput(t, tc)
		testJSONOutput(t, tc)
		testEnvOutput(t, tc)
	}
}

func testTextOutput(t *testing.T, tc testCase) {
	t.Helper()
	assert.Equal(t, tc.expectedTextOutput, tc.cluster.String())
}

func testJSONOutput(t *testing.T, tc testCase) {
	t.Helper()
	assert.JSONEq(t, tc.expectedJSONOutput, tc.cluster.json())
}

func testEnvOutput(t *testing.T, tc testCase) {
	t.Helper()
	assert.Equal(t, tc.expectedEnvOutput, tc.cluster.env())
}
