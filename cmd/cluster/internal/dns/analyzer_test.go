package dns

import (
	"testing"

	. "github.com/onsi/gomega"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
)

func TestDefaultAnalyzer_Analyze(t *testing.T) {
	g := NewGomegaWithT(t)

	tests := []struct {
		name                    string
		results                 []VerifyResult
		clusterName             string
		clusterID               string
		clusterRegion           string
		expectedSummaryTotal    int
		expectedSummaryPassed   int
		expectedSummaryFailed   int
		expectedSummarySkipped  int
		expectedRecommendations int
		useRecommender          bool
	}{
		{
			name: "all tests passed",
			results: []VerifyResult{
				{Name: "test1.example.com", Type: RecordTypeA, Status: VerifyResultStatusPass},
				{Name: "test2.example.com", Type: RecordTypeCNAME, Status: VerifyResultStatusPass},
			},
			clusterName:             "test-cluster",
			clusterID:               "abc123",
			clusterRegion:           "us-east-1",
			expectedSummaryTotal:    2,
			expectedSummaryPassed:   2,
			expectedSummaryFailed:   0,
			expectedSummarySkipped:  0,
			expectedRecommendations: 0,
			useRecommender:          false,
		},
		{
			name: "mixed test results",
			results: []VerifyResult{
				{Name: "test1.example.com", Type: RecordTypeA, Status: VerifyResultStatusPass},
				{Name: "test2.example.com", Type: RecordTypeCNAME, Status: VerifyResultStatusFail, ErrorMessage: "lookup failed"},
				{Name: "test3.example.com", Type: RecordTypeA, Status: VerifyResultStatusSkip, SkipReason: "cluster too old"},
			},
			clusterName:             "test-cluster",
			clusterID:               "abc123",
			clusterRegion:           "us-west-2",
			expectedSummaryTotal:    3,
			expectedSummaryPassed:   1,
			expectedSummaryFailed:   1,
			expectedSummarySkipped:  1,
			expectedRecommendations: 0,
			useRecommender:          false,
		},
		{
			name: "all tests failed",
			results: []VerifyResult{
				{Name: "test1.example.com", Type: RecordTypeA, Status: VerifyResultStatusFail, ErrorMessage: "no such host"},
				{Name: "test2.example.com", Type: RecordTypeCNAME, Status: VerifyResultStatusFail, ErrorMessage: "lookup failed"},
			},
			clusterName:             "test-cluster",
			clusterID:               "abc123",
			clusterRegion:           "eu-west-1",
			expectedSummaryTotal:    2,
			expectedSummaryPassed:   0,
			expectedSummaryFailed:   2,
			expectedSummarySkipped:  0,
			expectedRecommendations: 0,
			useRecommender:          false,
		},
		{
			name:                    "empty results",
			results:                 []VerifyResult{},
			clusterName:             "test-cluster",
			clusterID:               "abc123",
			clusterRegion:           "us-east-1",
			expectedSummaryTotal:    0,
			expectedSummaryPassed:   0,
			expectedSummaryFailed:   0,
			expectedSummarySkipped:  0,
			expectedRecommendations: 0,
			useRecommender:          false,
		},
		{
			name: "with recommender",
			results: []VerifyResult{
				{Name: "test1.example.com", Type: RecordTypeA, Status: VerifyResultStatusFail, ErrorMessage: "no such host"},
			},
			clusterName:             "test-cluster",
			clusterID:               "abc123",
			clusterRegion:           "us-east-1",
			expectedSummaryTotal:    1,
			expectedSummaryPassed:   0,
			expectedSummaryFailed:   1,
			expectedSummarySkipped:  0,
			expectedRecommendations: 1,
			useRecommender:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cluster := createTestCluster(tt.clusterName, tt.clusterID, tt.clusterRegion)

			var analyzer *DefaultAnalyzer
			if tt.useRecommender {
				analyzer = NewDefaultAnalyzer(WithRecommender{
					Recommender: &mockRecommender{recommendations: []string{"test recommendation"}},
				})
			} else {
				analyzer = NewDefaultAnalyzer()
			}

			report := analyzer.Analyze(cluster, tt.results)

			// Verify cluster info
			assert.Equal(t, tt.clusterName, report.Cluster.Name)
			assert.Equal(t, tt.clusterID, report.Cluster.ID)
			assert.Equal(t, tt.clusterRegion, report.Cluster.Region)

			// Verify results
			assert.Equal(t, len(tt.results), len(report.Results))
			assert.Equal(t, tt.results, report.Results)

			// Verify summary
			assert.Equal(t, tt.expectedSummaryTotal, report.Summary.Total)
			assert.Equal(t, tt.expectedSummaryPassed, report.Summary.Passed)
			assert.Equal(t, tt.expectedSummaryFailed, report.Summary.Failed)
			assert.Equal(t, tt.expectedSummarySkipped, report.Summary.Skipped)

			// Verify recommendations
			if tt.useRecommender {
				g.Expect(len(report.Recommendations)).Should(Equal(tt.expectedRecommendations))
			} else {
				g.Expect(len(report.Recommendations)).Should(BeZero())
			}
		})
	}
}

func TestDefaultAnalyzer_PopulateSummary(t *testing.T) {
	tests := []struct {
		name            string
		results         []VerifyResult
		expectedTotal   int
		expectedPassed  int
		expectedFailed  int
		expectedSkipped int
	}{
		{
			name: "all passed",
			results: []VerifyResult{
				{Status: VerifyResultStatusPass},
				{Status: VerifyResultStatusPass},
				{Status: VerifyResultStatusPass},
			},
			expectedTotal:   3,
			expectedPassed:  3,
			expectedFailed:  0,
			expectedSkipped: 0,
		},
		{
			name: "all failed",
			results: []VerifyResult{
				{Status: VerifyResultStatusFail},
				{Status: VerifyResultStatusFail},
			},
			expectedTotal:   2,
			expectedPassed:  0,
			expectedFailed:  2,
			expectedSkipped: 0,
		},
		{
			name: "all skipped",
			results: []VerifyResult{
				{Status: VerifyResultStatusSkip},
			},
			expectedTotal:   1,
			expectedPassed:  0,
			expectedFailed:  0,
			expectedSkipped: 1,
		},
		{
			name: "mixed results",
			results: []VerifyResult{
				{Status: VerifyResultStatusPass},
				{Status: VerifyResultStatusFail},
				{Status: VerifyResultStatusSkip},
				{Status: VerifyResultStatusPass},
			},
			expectedTotal:   4,
			expectedPassed:  2,
			expectedFailed:  1,
			expectedSkipped: 1,
		},
		{
			name:            "empty results",
			results:         []VerifyResult{},
			expectedTotal:   0,
			expectedPassed:  0,
			expectedFailed:  0,
			expectedSkipped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			analyzer := NewDefaultAnalyzer()
			summary := analyzer.populateSummary(tt.results)

			assert.Equal(t, tt.expectedTotal, summary.Total)
			assert.Equal(t, tt.expectedPassed, summary.Passed)
			assert.Equal(t, tt.expectedFailed, summary.Failed)
			assert.Equal(t, tt.expectedSkipped, summary.Skipped)
		})
	}
}

func TestNewDefaultAnalyzer(t *testing.T) {
	t.Run("creates analyzer with default recommender", func(t *testing.T) {
		t.Parallel()
		analyzer := NewDefaultAnalyzer()

		assert.NotNil(t, analyzer)
		assert.NotNil(t, analyzer.cfg.Recommender)
		assert.IsType(t, &NoopRecommender{}, analyzer.cfg.Recommender)
	})

	t.Run("creates analyzer with custom recommender", func(t *testing.T) {
		t.Parallel()
		customRecommender := &mockRecommender{}
		analyzer := NewDefaultAnalyzer(WithRecommender{Recommender: customRecommender})

		assert.NotNil(t, analyzer)
		assert.NotNil(t, analyzer.cfg.Recommender)
		assert.Equal(t, customRecommender, analyzer.cfg.Recommender)
	})
}

func TestNoopRecommender_MakeRecommendations(t *testing.T) {
	t.Parallel()
	recommender := &NoopRecommender{}
	results := []VerifyResult{
		{Status: VerifyResultStatusFail},
	}

	recommendations := recommender.MakeRecommendations(results)

	assert.NotNil(t, recommendations)
	assert.Empty(t, recommendations)
}

func TestWithRecommender(t *testing.T) {
	t.Parallel()
	customRecommender := &mockRecommender{}
	opt := WithRecommender{Recommender: customRecommender}
	cfg := &DefaultAnalyzerConfig{}

	opt.ConfigureDefaultAnalyzer(cfg)

	assert.Equal(t, customRecommender, cfg.Recommender)
}

func TestWithCluster(t *testing.T) {
	t.Parallel()
	cluster := createTestCluster("test", "id123", "us-east-1")
	opt := WithCluster{Cluster: cluster}
	cfg := &MakeRecommendationsConfig{}

	opt.ConfigureMakeRecommendations(cfg)

	assert.Equal(t, cluster, cfg.Cluster)
}

// Helper functions and mocks

func createTestCluster(name, id, region string) *cmv1.Cluster {
	cluster, _ := cmv1.NewCluster().
		Name(name).
		ID(id).
		Region(cmv1.NewCloudRegion().ID(region)).
		Build()
	return cluster
}

type mockRecommender struct {
	recommendations []string
}

func (m *mockRecommender) MakeRecommendations(results []VerifyResult, opts ...MakeRecommendationsOption) []string {
	return m.recommendations
}
