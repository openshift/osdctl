package dns

import (
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

type Analyzer interface {
	Analyze(*cmv1.Cluster, []VerifyResult) DNSVerificationReport
}

type Recommender interface {
	MakeRecommendations([]VerifyResult, ...MakeRecommendationsOption) []string
}

type MakeRecommendationsConfig struct {
	Cluster *cmv1.Cluster
}

func (c *MakeRecommendationsConfig) Option(opts ...MakeRecommendationsOption) {
	for _, o := range opts {
		o.ConfigureMakeRecommendations(c)
	}
}

type MakeRecommendationsOption interface {
	ConfigureMakeRecommendations(*MakeRecommendationsConfig)
}

func NewDefaultAnalyzer(opts ...DefaultAnalyzerOption) *DefaultAnalyzer {
	var cfg DefaultAnalyzerConfig

	cfg.Option(opts...)
	cfg.Default()

	return &DefaultAnalyzer{
		cfg: cfg,
	}
}

type DefaultAnalyzer struct {
	cfg DefaultAnalyzerConfig
}

type DefaultAnalyzerConfig struct {
	Recommender Recommender
}

func (c *DefaultAnalyzerConfig) Option(opts ...DefaultAnalyzerOption) {
	for _, o := range opts {
		o.ConfigureDefaultAnalyzer(c)
	}
}

func (c *DefaultAnalyzerConfig) Default() {
	if c.Recommender == nil {
		c.Recommender = &NoopRecommender{}
	}
}

type NoopRecommender struct{}

func (r *NoopRecommender) MakeRecommendations([]VerifyResult, ...MakeRecommendationsOption) []string {
	return []string{}
}

type DefaultAnalyzerOption interface {
	ConfigureDefaultAnalyzer(*DefaultAnalyzerConfig)
}

func (a *DefaultAnalyzer) Analyze(cluster *cmv1.Cluster, results []VerifyResult) DNSVerificationReport {
	return DNSVerificationReport{
		Cluster: ClusterInfo{
			Name:   cluster.Name(),
			ID:     cluster.ID(),
			Region: cluster.Region().ID(),
		},
		Results:         results,
		Summary:         a.populateSummary(results),
		Recommendations: a.generateRecommendations(results),
	}
}

type DNSVerificationReport struct {
	Cluster         ClusterInfo    `json:"cluster"`
	Results         []VerifyResult `json:"results"`
	Summary         summaryStats   `json:"summary"`
	Recommendations []string       `json:"recommendations,omitempty"`
}

type ClusterInfo struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Region string `json:"region"`
}

func (a *DefaultAnalyzer) populateSummary(results []VerifyResult) summaryStats {
	summary := summaryStats{}
	for _, res := range results {
		summary.Total++
		switch res.Status {
		case VerifyResultStatusPass:
			summary.Passed++
		case VerifyResultStatusFail:
			summary.Failed++
		case VerifyResultStatusSkip:
			summary.Skipped++
		}
	}
	return summary
}

type summaryStats struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

func (a *DefaultAnalyzer) generateRecommendations(results []VerifyResult) []string {
	return a.cfg.Recommender.MakeRecommendations(results)
}
