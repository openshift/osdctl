package org

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/andygrunwald/go-jira"
	sdk "github.com/openshift-online/ocm-sdk-go"
	accountsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	v1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
)

type fakePDClient struct {
	serviceIDs []string
	incidents  map[string][]pd.Incident
}

func (f *fakePDClient) GetPDServiceIDs() ([]string, error) {
	return f.serviceIDs, nil
}

func (f *fakePDClient) GetFiringAlertsForCluster(ids []string) (map[string][]pd.Incident, error) {
	return f.incidents, nil
}

func TestGetPlanDisplayText(t *testing.T) {
	tests := []struct {
		plan     string
		expected string
	}{
		{"MOA", "ROSA"},
		{"MOA-HostedControlPlane", "HCP"},
		{"STANDARD", "STANDARD"},
	}
	for _, tc := range tests {
		got := getPlanDisplayText(tc.plan)
		if got != tc.expected {
			t.Errorf("getPlanDisplayText(%q) = %q; want %q", tc.plan, got, tc.expected)
		}
	}
}

func TestGetSupportStatusDisplayText(t *testing.T) {
	if got := getSupportStatusDisplayText(nil); got != "Fully Supported" {
		t.Errorf("expected Fully Supported for nil, got %s", got)
	}
	if got := getSupportStatusDisplayText([]*cmv1.LimitedSupportReason{{}}); got != "Limited Support" {
		t.Errorf("expected Limited Support for non-empty, got %s", got)
	}
}

func TestPrintContextJson(t *testing.T) {
	infos := []ClusterInfo{
		{
			Name:                  "cluster1",
			ID:                    "id1",
			Version:               "v1",
			CloudProvider:         "aws",
			Plan:                  "MOA",
			NodeCount:             3,
			ServiceLogs:           make([]*v1.LogEntry, 2),
			PdAlerts:              map[string][]pd.Incident{"svc": {{}}},
			JiraIssues:            []jira.Issue{{Key: "J-1"}},
			LimitedSupportReasons: nil,
		},
	}
	buf := &bytes.Buffer{}
	if err := printContextJson(buf, infos); err != nil {
		t.Fatalf("printContextJson returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"displayName": "cluster1"`) {
		t.Errorf("output JSON missing displayName: %s", out)
	}
	if !strings.Contains(out, `"plan": "ROSA"`) {
		t.Errorf("output JSON missing plan ROSA: %s", out)
	}
	if !strings.Contains(out, `"recentSLs": 2`) {
		t.Errorf("output JSON missing recentSLs: %s", out)
	}
}

func TestFetchContext_ErrorSearchSubscriptions(t *testing.T) {
	fetcher := &DefaultContextFetcher{
		SearchSubscriptions: func(orgID string, status string, managedOnly bool) ([]*accountsv1.Subscription, error) {
			return nil, errors.New("search error")
		},
	}
	_, err := fetcher.FetchContext("org", nil)
	if err == nil || !strings.Contains(err.Error(), "failed to fetch cluster subscriptions") {
		t.Errorf("expected error fetching subscriptions, got %v", err)
	}
}

func TestFetchContext_NoSubscriptions(t *testing.T) {
	fetcher := &DefaultContextFetcher{
		SearchSubscriptions: func(orgID string, status string, managedOnly bool) ([]*accountsv1.Subscription, error) {
			return []*accountsv1.Subscription{}, nil
		},
	}
	result, err := fetcher.FetchContext("org", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for no subscriptions, got %v", result)
	}
}

func TestFetchContext_ErrorCreateOCMClient(t *testing.T) {
	fetcher := &DefaultContextFetcher{
		SearchSubscriptions: func(orgID string, status string, managedOnly bool) ([]*accountsv1.Subscription, error) {
			return []*accountsv1.Subscription{{}}, nil
		},
		CreateOCMClient: func() (*sdk.Connection, error) {
			return nil, errors.New("client error")
		},
	}
	_, err := fetcher.FetchContext("org", nil)
	if err == nil || !strings.Contains(err.Error(), "failed to create OCM client") {
		t.Errorf("expected error creating OCM client, got %v", err)
	}
}
