package rhobs

import (
	"strings"
	"testing"
	"time"
)

// --- logsClusterExtId ---

func TestLogsClusterExtId(t *testing.T) {
	tests := []struct {
		name              string
		clusterExternalId string
		mcExternalId      string
		isHostedCluster   bool
		want              string
	}{
		{
			name:              "non-HCP cluster uses its own external ID",
			clusterExternalId: "cluster-ext-id",
			mcExternalId:      "",
			isHostedCluster:   false,
			want:              "cluster-ext-id",
		},
		{
			name:              "MC cluster uses its own external ID",
			clusterExternalId: "mc-ext-id",
			mcExternalId:      "",
			isHostedCluster:   false,
			want:              "mc-ext-id",
		},
		{
			name:              "HCP cluster with MC external ID uses MC's UUID",
			clusterExternalId: "hcp-ext-id",
			mcExternalId:      "mc-ext-id",
			isHostedCluster:   true,
			want:              "mc-ext-id",
		},
		{
			name:              "HCP cluster without MC external ID falls back to own UUID",
			clusterExternalId: "hcp-ext-id",
			mcExternalId:      "",
			isHostedCluster:   true,
			want:              "hcp-ext-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &RhobsFetcher{
				clusterExternalId: tt.clusterExternalId,
				mcExternalId:      tt.mcExternalId,
				IsHostedCluster:   tt.isHostedCluster,
			}
			got := f.logsClusterExtId()
			if got != tt.want {
				t.Errorf("logsClusterExtId() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- resolveLogsNamespace ---

func TestResolveLogsNamespace(t *testing.T) {
	tests := []struct {
		name                   string
		isHostedCluster        bool
		hcpNamespace           string
		namespaceExplicitlySet bool
		defaultNamespace       string
		want                   string
	}{
		{
			name:                   "non-HCP cluster keeps default namespace",
			isHostedCluster:        false,
			hcpNamespace:           "",
			namespaceExplicitlySet: false,
			defaultNamespace:       "default",
			want:                   "default",
		},
		{
			name:                   "HCP cluster without HcpNamespace keeps default",
			isHostedCluster:        true,
			hcpNamespace:           "",
			namespaceExplicitlySet: false,
			defaultNamespace:       "default",
			want:                   "default",
		},
		{
			name:                   "HCP cluster uses HCP control-plane namespace",
			isHostedCluster:        true,
			hcpNamespace:           "ocm-production-abc123-ns",
			namespaceExplicitlySet: false,
			defaultNamespace:       "default",
			want:                   "ocm-production-abc123-ns",
		},
		{
			name:                   "HCP cluster respects explicit namespace override",
			isHostedCluster:        true,
			hcpNamespace:           "ocm-production-abc123-ns",
			namespaceExplicitlySet: true,
			defaultNamespace:       "custom-ns",
			want:                   "custom-ns",
		},
		{
			name:                   "non-HCP cluster respects explicit namespace override",
			isHostedCluster:        false,
			hcpNamespace:           "",
			namespaceExplicitlySet: true,
			defaultNamespace:       "openshift-monitoring",
			want:                   "openshift-monitoring",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &RhobsFetcher{
				IsHostedCluster: tt.isHostedCluster,
				HcpNamespace:    tt.hcpNamespace,
			}
			got := resolveLogsNamespace(f, tt.namespaceExplicitlySet, tt.defaultNamespace)
			if got != tt.want {
				t.Errorf("resolveLogsNamespace() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- GetGrafanaLogsUrl ---

func TestGetGrafanaLogsUrl(t *testing.T) {
	f := &RhobsFetcher{
		RhobsCell:  "https://eu-central-1-0.rhobs.api.openshift.com",
		ocmEnvName: "production",
	}

	now := time.Now()
	start := now.Add(-5 * time.Minute)

	tests := []struct {
		name           string
		lokiExpr       string
		isGoingForward bool
	}{
		{
			name:           "HCP corrected query - backward",
			lokiExpr:       `{k8s_namespace_name="ocm-production-abc123-ns"} | json json_kind="kind" | json_kind != "Event" | openshift_cluster_id = "mc-ext-uuid"`,
			isGoingForward: false,
		},
		{
			name:           "MC query - forward",
			lokiExpr:       `{k8s_namespace_name="default"} | json json_kind="kind" | json_kind != "Event" | openshift_cluster_id = "mc-ext-uuid"`,
			isGoingForward: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, err := f.GetGrafanaLogsUrl(tt.lokiExpr, start, now, tt.isGoingForward)
			if err != nil {
				t.Fatalf("GetGrafanaLogsUrl returned error: %v", err)
			}
			if gotURL == "" {
				t.Fatal("expected non-empty URL")
			}
			if !strings.Contains(gotURL, "grafana.app-sre.devshift.net") {
				t.Errorf("URL should point to Grafana, got: %s", gotURL)
			}
			if !strings.Contains(gotURL, "explore") {
				t.Errorf("URL should be an explore URL, got: %s", gotURL)
			}
		})
	}
}

// TestGetGrafanaLogsUrl_EncodesLokiExpr verifies that the Loki expression ends up
// URL-encoded in the produced Grafana URL (not literally embedded as plain text).
func TestGetGrafanaLogsUrl_EncodesLokiExpr(t *testing.T) {
	f := &RhobsFetcher{
		RhobsCell:  "https://eu-central-1-0.rhobs.api.openshift.com",
		ocmEnvName: "production",
	}

	expr := `{k8s_namespace_name="ocm-production-abc123-ns"} | openshift_cluster_id = "mc-uuid"`
	now := time.Now()

	gotURL, err := f.GetGrafanaLogsUrl(expr, now.Add(-5*time.Minute), now, false)
	if err != nil {
		t.Fatalf("GetGrafanaLogsUrl returned error: %v", err)
	}

	// The raw expression must not appear verbatim: it should be JSON/URL-encoded.
	if strings.Contains(gotURL, `{k8s_namespace_name=`) {
		t.Errorf("Loki expression appears unencoded in URL: %s", gotURL)
	}
}

// TestHcpLogsQueryDiffersFromStandard verifies the two code-paths produce different
// Grafana URLs — confirming that the HCP fix actually changes the output.
func TestHcpLogsQueryDiffersFromStandard(t *testing.T) {
	f := &RhobsFetcher{
		RhobsCell:  "https://us-east-1-1.rhobs.api.openshift.com",
		ocmEnvName: "production",
	}

	now := time.Now()
	start := now.Add(-5 * time.Minute)

	brokenExpr := `{k8s_namespace_name="default"} | json json_kind="kind" | json_kind != "Event" | openshift_cluster_id = "hcp-ext-uuid"`
	fixedExpr := `{k8s_namespace_name="ocm-production-abc123-ns"} | json json_kind="kind" | json_kind != "Event" | openshift_cluster_id = "mc-ext-uuid"`

	brokenURL, err := f.GetGrafanaLogsUrl(brokenExpr, start, now, false)
	if err != nil {
		t.Fatalf("broken URL generation failed: %v", err)
	}
	fixedURL, err := f.GetGrafanaLogsUrl(fixedExpr, start, now, false)
	if err != nil {
		t.Fatalf("fixed URL generation failed: %v", err)
	}

	if brokenURL == fixedURL {
		t.Error("broken and fixed Loki expressions should produce different Grafana URLs")
	}
}
