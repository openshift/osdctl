package dns

import (
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"time"
)

type WithCluster struct {
	Cluster *cmv1.Cluster
}

func (w WithCluster) ConfigureMakeRecommendations(cfg *MakeRecommendationsConfig) {
	cfg.Cluster = w.Cluster
}

type WithExpectedTarget string

func (w WithExpectedTarget) ConfigureVerifyCNAMERecord(cfg *VerifyCNAMERecordConfig) {
	cfg.ExpectedTarget = string(w)
}

type WithRecommender struct {
	Recommender Recommender
}

func (w WithRecommender) ConfigureDefaultAnalyzer(cfg *DefaultAnalyzerConfig) {
	cfg.Recommender = w.Recommender
}

type WithTimeout time.Duration

func (w WithTimeout) ConfigureDefaultVerifier(cfg *DefaultVerifierConfig) {
	cfg.Timeout = time.Duration(w)
}
