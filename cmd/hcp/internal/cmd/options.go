package cmd

import (
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	"github.com/sirupsen/logrus"
)

// The With* types below are concrete implementations of DefaultBackupCreatorOption,
// used to override defaultBackupCreatorConfig defaults at construction time.

// WithADPNamespace overrides the namespace where the ADP / Velero resources live.
type WithADPNamespace string

func (v WithADPNamespace) ConfigureDefaultBackupCreator(c *DefaultBackupCreatorConfig) {
	c.ADPNamespace = string(v)
}

// WithVeleroContainer overrides the name of the container inside the Velero pod to exec into.
type WithVeleroContainer string

func (v WithVeleroContainer) ConfigureDefaultBackupCreator(c *DefaultBackupCreatorConfig) {
	c.VeleroContainer = string(v)
}

// WithVeleroLabelKey overrides the pod label key used to locate the Velero pod.
type WithVeleroLabelKey string

func (v WithVeleroLabelKey) ConfigureDefaultBackupCreator(c *DefaultBackupCreatorConfig) {
	c.VeleroLabelKey = string(v)
}

// WithVeleroLabelValue overrides the pod label value used to locate the Velero pod.
type WithVeleroLabelValue string

func (v WithVeleroLabelValue) ConfigureDefaultBackupCreator(c *DefaultBackupCreatorConfig) {
	c.VeleroLabelValue = string(v)
}

// WithScheduleNameSuffix overrides the suffix appended to the cluster ID to form the schedule name.
type WithScheduleNameSuffix string

func (v WithScheduleNameSuffix) ConfigureDefaultBackupCreator(c *DefaultBackupCreatorConfig) {
	c.ScheduleNameSuffix = string(v)
}

// WithResolver overrides the ClusterResolver used to translate a raw cluster
// identifier (from --cluster-id) into canonical OCM IDs. Primarily useful in
// tests to avoid real OCM calls.
type WithResolver struct {
	Resolver internalclient.ClusterResolver
}

func (v WithResolver) ConfigureDefaultBackupCreator(c *DefaultBackupCreatorConfig) {
	c.Resolver = v.Resolver
}

type WithLogger struct {
	Logger *logrus.Logger
}

func (w WithLogger) ConfigureDefaultSnapshotCopier(c *DefaultSnapshotCopierConfig) {
	c.Logger = w.Logger
}

type WithClock struct {
	Clock Clock
}

func (w WithClock) ConfigureDefaultSnapshotCopier(c *DefaultSnapshotCopierConfig) {
	c.Clock = w.Clock
}

type WithAWSAccountClientProvider struct {
	AWSClient internalclient.AWSAccountClientProvider
}

func (w WithAWSAccountClientProvider) ConfigureDefaultBackupDiscoverer(c *DefaultBackupDiscovererConfig) {
	c.AWSClient = w.AWSClient
}

func (w WithAWSAccountClientProvider) ConfigureDefaultDNSReleaser(c *DefaultDNSReleaserConfig) {
	c.AWSClient = w.AWSClient
}

func (w WithAWSAccountClientProvider) ConfigureDefaultTrustPolicyUpdater(c *DefaultTrustPolicyUpdaterConfig) {
	c.AWSClient = w.AWSClient
}
