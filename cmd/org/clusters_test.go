package org

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	accountsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/stretchr/testify/require"
)

func TestFormatClustersOutput_JSON(t *testing.T) {
	output = "json"
	defer func() { output = "" }()

	sub1, _ := accountsv1.NewSubscription().
		ClusterID("cid-1").
		ExternalClusterID("ext-1").
		DisplayName("cluster-1").
		Status("Active").
		Build()

	sub2, _ := accountsv1.NewSubscription().
		ClusterID("cid-2").
		ExternalClusterID("ext-2").
		DisplayName("cluster-2").
		Status("Inactive").
		Build()

	subs := []*accountsv1.Subscription{sub1, sub2}

	got, err := formatClustersOutput(subs)
	require.NoError(t, err)

	expected := readFixture(t, "testdata/jsonClusters.txt")

	var gotJson, expectedJson interface{}
	require.NoError(t, json.Unmarshal(got, &gotJson))
	require.NoError(t, json.Unmarshal([]byte(expected), &expectedJson))

	require.Equal(t, expectedJson, gotJson)
}

func TestFormatClustersOutput_Table(t *testing.T) {
	sub1, _ := accountsv1.NewSubscription().
		ClusterID("cid-1").
		ExternalClusterID("ext-1").
		DisplayName("cluster-1").
		Status("Active").
		Build()

	sub2, _ := accountsv1.NewSubscription().
		ClusterID("cid-2").
		ExternalClusterID("ext-2").
		DisplayName("cluster-2").
		Status("Inactive").
		Build()

	clusters := []*accountsv1.Subscription{sub1, sub2}

	actual, err := formatClustersOutput(clusters)
	require.NoError(t, err)

	expected := readFixture(t, "testdata/tableClusters.txt")
	require.Equal(t, expected, string(actual))
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	return string(data)
}

func TestSearchSubscriptions_Errors(t *testing.T) {
	awsProfile = ""
	awsAccountID = ""
	_, err := SearchSubscriptions("", "")
	require.ErrorContains(t, err, "specify either org-id")

	awsProfile = "mock"
	awsAccountID = "123"
	_, err = SearchSubscriptions("org-123", "")
	require.ErrorContains(t, err, "specify either an org id argument")
}
