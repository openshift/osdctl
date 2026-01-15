package cluster

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/cmd/cluster/internal/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestVerifyDNSOptions_ShouldSkipUniqueFQDN(t *testing.T) {
	tests := []struct {
		name           string
		creationDate   time.Time
		expectedResult bool
	}{
		{
			name:           "cluster created before cutoff date",
			creationDate:   time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
			expectedResult: true,
		},
		{
			name:           "cluster created on cutoff date",
			creationDate:   time.Date(2025, time.March, 10, 0, 0, 0, 0, time.UTC),
			expectedResult: false,
		},
		{
			name:           "cluster created after cutoff date",
			creationDate:   time.Date(2025, time.March, 11, 0, 0, 0, 0, time.UTC),
			expectedResult: false,
		},
		{
			name:           "cluster created way before cutoff date",
			creationDate:   time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			expectedResult: true,
		},
		{
			name:           "cluster created way after cutoff date",
			creationDate:   time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cluster, _ := cmv1.NewCluster().
				CreationTimestamp(tt.creationDate).
				Build()

			opts := &verifyDNSOptions{}
			result := opts.shouldSkipUniqueFQDN(cluster)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestVerifyDNSOptions_BuildTestCases(t *testing.T) {
	g := NewGomegaWithT(t)

	tests := []struct {
		name                string
		clusterName         string
		clusterID           string
		baseDomain          string
		consoleURL          string
		isPrivateLink       bool
		creationDate        time.Time
		expectedTestCount   int
		expectUniqueSkipped bool
	}{
		{
			name:                "HCP cluster without PrivateLink created after cutoff",
			clusterName:         "test-cluster",
			clusterID:           "abc123",
			baseDomain:          "example.com",
			consoleURL:          "https://console-openshift-console.apps.rosa.test-cluster.example.com",
			isPrivateLink:       false,
			creationDate:        time.Date(2025, time.March, 11, 0, 0, 0, 0, time.UTC),
			expectedTestCount:   7,
			expectUniqueSkipped: false,
		},
		{
			name:                "HCP cluster with PrivateLink created before cutoff",
			clusterName:         "test-cluster",
			clusterID:           "abc123",
			baseDomain:          "example.com",
			consoleURL:          "https://console-openshift-console.apps.rosa.test-cluster.example.com",
			isPrivateLink:       true,
			creationDate:        time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
			expectedTestCount:   7,
			expectUniqueSkipped: true,
		},
		{
			name:                "HCP cluster without PrivateLink created before cutoff",
			clusterName:         "my-cluster",
			clusterID:           "xyz789",
			baseDomain:          "test.io",
			consoleURL:          "https://console-openshift-console.apps.rosa.my-cluster.test.io",
			isPrivateLink:       false,
			creationDate:        time.Date(2024, time.December, 1, 0, 0, 0, 0, time.UTC),
			expectedTestCount:   7,
			expectUniqueSkipped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cluster := createTestHCPCluster(
				tt.clusterName,
				tt.clusterID,
				tt.baseDomain,
				tt.consoleURL,
				tt.isPrivateLink,
				tt.creationDate,
			)

			opts := &verifyDNSOptions{}
			testCases := opts.buildTestCases(cluster)

			// Verify the correct number of test cases were created
			assert.Equal(t, tt.expectedTestCount, len(testCases))

			// Verify console test case
			g.Expect(testCases).Should(HaveKey("console"))
			consoleTest := testCases["console"]
			assert.Equal(t, tt.consoleURL, consoleTest.name)
			assert.Equal(t, dns.RecordType("A"), consoleTest.recordType)

			// Verify default ingress test case
			g.Expect(testCases).Should(HaveKey("default_ingress"))
			defaultIngressTest := testCases["default_ingress"]
			expectedDefaultIngress := "apps.rosa." + tt.clusterName + "." + tt.baseDomain
			assert.Equal(t, expectedDefaultIngress, defaultIngressTest.name)
			assert.Equal(t, dns.RecordTypeCNAME, defaultIngressTest.recordType)
			assert.Equal(t, tt.clusterName+"."+tt.baseDomain, defaultIngressTest.expectedTarget)

			// Verify default ingress challenge test case
			g.Expect(testCases).Should(HaveKey("default_ingress_challenge"))
			defaultIngressChallengeTest := testCases["default_ingress_challenge"]
			expectedChallenge := "_acme-challenge.apps.rosa." + tt.clusterName + "." + tt.baseDomain
			assert.Equal(t, expectedChallenge, defaultIngressChallengeTest.name)
			assert.Equal(t, dns.RecordTypeCNAME, defaultIngressChallengeTest.recordType)

			// Verify unique FQDN test case
			g.Expect(testCases).Should(HaveKey("unique"))
			uniqueTest := testCases["unique"]
			expectedUnique := tt.clusterID + ".rosa." + tt.clusterName + "." + tt.baseDomain
			assert.Equal(t, expectedUnique, uniqueTest.name)
			assert.Equal(t, dns.RecordTypeCNAME, uniqueTest.recordType)
			assert.Equal(t, tt.expectUniqueSkipped, uniqueTest.skip)

			// Verify unique challenge test case
			g.Expect(testCases).Should(HaveKey("unique_challenge"))
			uniqueChallengeTest := testCases["unique_challenge"]
			expectedUniqueChallenge := "_acme-challenge." + tt.clusterID + ".rosa." + tt.clusterName + "." + tt.baseDomain
			assert.Equal(t, expectedUniqueChallenge, uniqueChallengeTest.name)
			assert.Equal(t, dns.RecordTypeCNAME, uniqueChallengeTest.recordType)
			assert.Equal(t, tt.expectUniqueSkipped, uniqueChallengeTest.skip)

			// Verify API and OAuth test cases based on PrivateLink
			g.Expect(testCases).Should(HaveKey("api"))
			apiTest := testCases["api"]
			expectedAPI := "api." + tt.clusterName + "." + tt.baseDomain
			assert.Equal(t, expectedAPI, apiTest.name)
			if tt.isPrivateLink {
				assert.Equal(t, dns.RecordTypeCNAME, apiTest.recordType)
			} else {
				assert.Equal(t, dns.RecordTypeA, apiTest.recordType)
			}

			g.Expect(testCases).Should(HaveKey("oauth"))
			oauthTest := testCases["oauth"]
			expectedOAuth := "oauth." + tt.clusterName + "." + tt.baseDomain
			assert.Equal(t, expectedOAuth, oauthTest.name)
			if tt.isPrivateLink {
				assert.Equal(t, dns.RecordTypeCNAME, oauthTest.recordType)
			} else {
				assert.Equal(t, dns.RecordTypeA, oauthTest.recordType)
			}
		})
	}
}

func TestVerifyDNSOptions_Complete(t *testing.T) {
	tests := []struct {
		name        string
		clusterID   string
		expectError bool
	}{
		{
			name:        "valid cluster ID",
			clusterID:   "test-cluster-123",
			expectError: false,
		},
		{
			name:        "empty cluster ID",
			clusterID:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := &verifyDNSOptions{
				clusterID: tt.clusterID,
			}

			err := opts.complete(nil)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "cluster-id is required")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRecommender_MakeRecommendations(t *testing.T) {
	tests := []struct {
		name                    string
		results                 []dns.VerifyResult
		clusterID               string
		expectedRecommendations int
		shouldContain           []string
	}{
		{
			name: "console failure",
			results: []dns.VerifyResult{
				{
					Name:   "console-openshift-console.apps.rosa.test.example.com",
					Status: dns.VerifyResultStatusFail,
				},
			},
			clusterID:               "test-cluster",
			expectedRecommendations: 1,
			shouldContain:           []string{"console", "CIO"},
		},
		{
			name: "default ingress failure",
			results: []dns.VerifyResult{
				{
					Name:   "apps.rosa.test.example.com",
					Status: dns.VerifyResultStatusFail,
				},
			},
			clusterID:               "test-cluster",
			expectedRecommendations: 1,
			shouldContain:           []string{"default ingress", "Route 53"},
		},
		{
			name: "default ingress challenge failure",
			results: []dns.VerifyResult{
				{
					Name:   "_acme-challenge.apps.rosa.test.example.com",
					Status: dns.VerifyResultStatusFail,
				},
			},
			clusterID:               "test-cluster",
			expectedRecommendations: 1,
			shouldContain:           []string{"default ingress", "Route 53"},
		},
		{
			name: "API/OAuth failure",
			results: []dns.VerifyResult{
				{
					Name:   "api.test.example.com",
					Status: dns.VerifyResultStatusFail,
				},
			},
			clusterID:               "test-cluster",
			expectedRecommendations: 1,
			shouldContain:           []string{"API", "external-dns"},
		},
		{
			name: "no failures",
			results: []dns.VerifyResult{
				{
					Name:   "test.example.com",
					Status: dns.VerifyResultStatusPass,
				},
			},
			clusterID:               "test-cluster",
			expectedRecommendations: 0,
			shouldContain:           []string{},
		},
		{
			name: "skipped tests",
			results: []dns.VerifyResult{
				{
					Name:   "test.example.com",
					Status: dns.VerifyResultStatusSkip,
				},
			},
			clusterID:               "test-cluster",
			expectedRecommendations: 0,
			shouldContain:           []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &recommender{}
			cluster, _ := cmv1.NewCluster().ID(tt.clusterID).Build()
			cfg := dns.MakeRecommendationsConfig{
				Cluster: cluster,
			}

			recommendations := r.MakeRecommendations(tt.results, dns.WithCluster{Cluster: cluster})

			assert.Equal(t, tt.expectedRecommendations, len(recommendations))
			for _, expected := range tt.shouldContain {
				if len(recommendations) > 0 {
					assert.Contains(t, recommendations[0], expected)
				}
			}

			// Verify the config is properly set
			assert.NotNil(t, cfg.Cluster)
		})
	}
}

// MockVerifier is a mock implementation of the Verifier interface
type MockVerifier struct {
	mock.Mock
}

func (m *MockVerifier) VerifyARecord(ctx context.Context, path string) dns.VerifyResult {
	args := m.Called(ctx, path)
	return args.Get(0).(dns.VerifyResult)
}

func (m *MockVerifier) VerifyCNAMERecord(ctx context.Context, name string, opts ...dns.VerifyCNAMERecordOption) dns.VerifyResult {
	args := m.Called(ctx, name, opts)
	return args.Get(0).(dns.VerifyResult)
}

// MockAnalyzer is a mock implementation of the Analyzer interface
type MockAnalyzer struct {
	mock.Mock
}

func (m *MockAnalyzer) Analyze(cluster *cmv1.Cluster, results []dns.VerifyResult) dns.DNSVerificationReport {
	args := m.Called(cluster, results)
	return args.Get(0).(dns.DNSVerificationReport)
}

// Helper functions

func createTestHCPCluster(name, id, baseDomain, consoleURL string, isPrivateLink bool, creationDate time.Time) *cmv1.Cluster {
	clusterBuilder := cmv1.NewCluster().
		Name(name).
		ID(id).
		DNS(cmv1.NewDNS().BaseDomain(baseDomain)).
		Console(cmv1.NewClusterConsole().URL(consoleURL)).
		Hypershift(cmv1.NewHypershift().Enabled(true)).
		CreationTimestamp(creationDate).
		AWS(cmv1.NewAWS().PrivateLink(isPrivateLink))

	cluster, _ := clusterBuilder.Build()
	return cluster
}

func TestNewCmdVerifyDNS(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	streams := genericclioptions.IOStreams{}
	cmd := NewCmdVerifyDNS(streams)

	// Verify command is created correctly
	g.Expect(cmd).ShouldNot(BeNil())
	g.Expect(cmd.Use).Should(ContainSubstring("verify-dns"))
	g.Expect(cmd.Short).Should(ContainSubstring("DNS resolution"))

	// Verify flags are registered
	clusterIDFlag := cmd.Flags().Lookup("cluster-id")
	g.Expect(clusterIDFlag).ShouldNot(BeNil())

	verboseFlag := cmd.Flags().Lookup("verbose")
	g.Expect(verboseFlag).ShouldNot(BeNil())
}
