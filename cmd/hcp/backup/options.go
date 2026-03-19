package backup

import logrus "github.com/sirupsen/logrus"

// The With* types below are concrete implementations of DefaultBackupRunnerOption,
// used to override defaultBackupRunnerConfig defaults at construction time.

// WithADPNamespace overrides the namespace where the ADP / Velero resources live.
type WithADPNamespace string

func (v WithADPNamespace) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.ADPNamespace = string(v)
}

// WithVeleroContainer overrides the name of the container inside the Velero pod to exec into.
type WithVeleroContainer string

func (v WithVeleroContainer) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.VeleroContainer = string(v)
}

// WithVeleroLabelKey overrides the pod label key used to locate the Velero pod.
type WithVeleroLabelKey string

func (v WithVeleroLabelKey) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.VeleroLabelKey = string(v)
}

// WithVeleroLabelValue overrides the pod label value used to locate the Velero pod.
type WithVeleroLabelValue string

func (v WithVeleroLabelValue) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.VeleroLabelValue = string(v)
}

// WithScheduleNameSuffix overrides the suffix appended to the cluster ID to form the schedule name.
type WithScheduleNameSuffix string

func (v WithScheduleNameSuffix) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.ScheduleNameSuffix = string(v)
}

// WithLogger overrides the logrus.Logger used for diagnostic output.
// By default a new logger writing to os.Stderr is created; callers may redirect
// it (e.g. to cmd.ErrOrStderr()) before passing it here.
type WithLogger struct{ Logger *logrus.Logger }

func (v WithLogger) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.Logger = v.Logger
}

// WithPrinter overrides the Printer used for user-facing output (backup results, status hints).
// The printer writes to os.Stdout by default.
type WithPrinter struct{ Printer Printer }

func (v WithPrinter) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.Printer = v.Printer
}

// WithResolver overrides the ClusterResolver used to translate a raw cluster
// identifier (from --cluster-id) into canonical OCM IDs. Primarily useful in
// tests to avoid real OCM calls.
type WithResolver struct{ Resolver ClusterResolver }

func (v WithResolver) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.Resolver = v.Resolver
}

// WithBuilder overrides the KubeClientBuilder used to create KubeClient
// instances. Primarily useful in tests to avoid real backplane logins.
type WithBuilder struct{ Builder KubeClientBuilder }

func (v WithBuilder) ConfigureDefaultBackupRunner(c *defaultBackupRunnerConfig) {
	c.Builder = v.Builder
}
