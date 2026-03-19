package backup

import (
	"context"
	"errors"
	"strings"
	"testing"

	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// testKubeClient is a test double for KubeClient. It embeds a controller-runtime
// fake client to satisfy Get and List, and exposes an optional execFn hook for
// Exec so individual tests can control its behaviour.
type testKubeClient struct {
	client.Client
	execFn func(ctx context.Context, namespace, pod, container string, cmd []string) (string, error)
}

func (t *testKubeClient) Exec(ctx context.Context, namespace, pod, container string, cmd []string) (string, error) {
	if t.execFn != nil {
		return t.execFn(ctx, namespace, pod, container, cmd)
	}
	return "", errors.New("Exec not configured in this test")
}

// newTestClient wraps a controller-runtime fake client in a testKubeClient.
func newTestClient(c client.Client) *testKubeClient {
	return &testKubeClient{Client: c}
}

// staticKubeClientBuilder is a test double for KubeClientBuilder. It returns
// pre-built stubs, routing calls with WithElevation to privilegedClient and
// calls without to unprivilegedClient. Either slot may be nil when the test
// does not exercise that path.
//
// Each Build call appends a buildConfig snapshot to calls so tests can assert
// on the cluster IDs and elevation reasons that Run actually forwarded.
type staticKubeClientBuilder struct {
	unprivilegedClient KubeClient
	privilegedClient   KubeClient
	unprivilegedErr    error
	privilegedErr      error
	calls              []buildConfig // snapshot of resolved options per Build call
}

func (b *staticKubeClientBuilder) Build(_ context.Context, opts ...BuildOption) (KubeClient, error) {
	cfg := &buildConfig{}
	for _, o := range opts {
		o.configureBuild(cfg)
	}
	b.calls = append(b.calls, *cfg)
	if cfg.elevated {
		return b.privilegedClient, b.privilegedErr
	}
	return b.unprivilegedClient, b.unprivilegedErr
}

// staticClusterResolver is a test double for ClusterResolver. It returns a
// fixed ClusterInfo or error, ignoring the cluster identifier argument.
type staticClusterResolver struct {
	clusterInfo ClusterInfo
	err         error
}

func (r *staticClusterResolver) Resolve(_ context.Context, _ string) (ClusterInfo, error) {
	return r.clusterInfo, r.err
}

// ── defaultBackupRunnerConfig ──────────────────────────────────────────────

func TestDefaultBackupRunnerConfig_Default(t *testing.T) {
	t.Parallel()

	cfg := newDefaultBackupRunnerConfig()

	assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
	assert.Equal(t, "velero", cfg.VeleroContainer)
	assert.Equal(t, "app.kubernetes.io/name", cfg.VeleroLabelKey)
	assert.Equal(t, "velero", cfg.VeleroLabelValue)
	assert.Equal(t, "-daily", cfg.ScheduleNameSuffix)
	assert.NotNil(t, cfg.Logger, "default Logger should be non-nil")
	assert.NotNil(t, cfg.Printer, "default Printer should be non-nil")
}

func TestDefaultBackupRunnerConfig_Options(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     []DefaultBackupRunnerOption
		assertFn func(t *testing.T, cfg defaultBackupRunnerConfig)
	}{
		{
			name: "WithADPNamespace overrides ADPNamespace",
			opts: []DefaultBackupRunnerOption{WithADPNamespace("custom-ns")},
			assertFn: func(t *testing.T, cfg defaultBackupRunnerConfig) {
				assert.Equal(t, "custom-ns", cfg.ADPNamespace)
				// other fields stay at defaults
				assert.Equal(t, "velero", cfg.VeleroContainer)
			},
		},
		{
			name: "WithVeleroContainer overrides VeleroContainer",
			opts: []DefaultBackupRunnerOption{WithVeleroContainer("sidecar")},
			assertFn: func(t *testing.T, cfg defaultBackupRunnerConfig) {
				assert.Equal(t, "sidecar", cfg.VeleroContainer)
				assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
			},
		},
		{
			name: "WithVeleroLabelKey overrides VeleroLabelKey",
			opts: []DefaultBackupRunnerOption{WithVeleroLabelKey("app")},
			assertFn: func(t *testing.T, cfg defaultBackupRunnerConfig) {
				assert.Equal(t, "app", cfg.VeleroLabelKey)
				assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
			},
		},
		{
			name: "WithVeleroLabelValue overrides VeleroLabelValue",
			opts: []DefaultBackupRunnerOption{WithVeleroLabelValue("backup-agent")},
			assertFn: func(t *testing.T, cfg defaultBackupRunnerConfig) {
				assert.Equal(t, "backup-agent", cfg.VeleroLabelValue)
				assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
			},
		},
		{
			name: "WithScheduleNameSuffix overrides ScheduleNameSuffix",
			opts: []DefaultBackupRunnerOption{WithScheduleNameSuffix("-weekly")},
			assertFn: func(t *testing.T, cfg defaultBackupRunnerConfig) {
				assert.Equal(t, "-weekly", cfg.ScheduleNameSuffix)
				assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
			},
		},
		{
			name: "WithLogger overrides Logger",
			opts: []DefaultBackupRunnerOption{WithLogger{Logger: logrus.New()}},
			assertFn: func(t *testing.T, cfg defaultBackupRunnerConfig) {
				assert.NotNil(t, cfg.Logger)
			},
		},
		{
			name: "WithPrinter overrides Printer",
			opts: []DefaultBackupRunnerOption{WithPrinter{Printer: &defaultPrinter{w: &strings.Builder{}}}},
			assertFn: func(t *testing.T, cfg defaultBackupRunnerConfig) {
				assert.NotNil(t, cfg.Printer)
			},
		},
		{
			name: "multiple options applied in order, last one wins",
			opts: []DefaultBackupRunnerOption{
				WithADPNamespace("first"),
				WithADPNamespace("second"),
			},
			assertFn: func(t *testing.T, cfg defaultBackupRunnerConfig) {
				assert.Equal(t, "second", cfg.ADPNamespace)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := newDefaultBackupRunnerConfig(tt.opts...)
			tt.assertFn(t, cfg)
		})
	}
}

// ── NewDefaultBackupRunner defaults and option overrides ───────────────────

func TestNewDefaultBackupRunner_Defaults(t *testing.T) {
	t.Parallel()

	runner := NewDefaultBackupRunner(nil)

	assert.NotNil(t, runner.logger, "default logger should be non-nil")
	assert.NotNil(t, runner.printer, "default printer should be non-nil")
}

func TestNewDefaultBackupRunner_WithLoggerAndPrinter(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	customLogger := logrus.New()
	customPrinter := &defaultPrinter{w: &sb}

	runner := NewDefaultBackupRunner(
		nil,
		WithLogger{Logger: customLogger},
		WithPrinter{Printer: customPrinter},
	)

	assert.Same(t, customLogger, runner.logger, "WithLogger should override the default logger")
	assert.Equal(t, customPrinter, runner.printer, "WithPrinter should override the default printer")
}

// ── kubeClient.Exec nil-guard ──────────────────────────────────────────────

func TestKubeClientExec_NilGuard(t *testing.T) {
	t.Parallel()

	// Client constructed without restCfg or clientset (read-only).
	k := newKubeClient(fake.NewClientBuilder().Build(), nil, nil)
	_, err := k.Exec(context.TODO(), "ns", "pod", "container", []string{"./velero", "backup", "get"})

	assert.Error(t, err)
	assert.ErrorContains(t, err, "no REST config or Clientset provided")
}

// ── buildConfig / BuildOption ──────────────────────────────────────────────

func TestBuildConfig(t *testing.T) {
	t.Parallel()

	t.Run("WithClusterID sets clusterID", func(t *testing.T) {
		t.Parallel()
		cfg := &buildConfig{}
		WithClusterID{ClusterID: "abc123"}.configureBuild(cfg)
		assert.Equal(t, "abc123", cfg.clusterID)
		assert.False(t, cfg.elevated)
		assert.Empty(t, cfg.elevationReason)
	})

	t.Run("WithElevation sets elevated and reason", func(t *testing.T) {
		t.Parallel()
		cfg := &buildConfig{}
		WithElevation{Reason: "OHSS-9999"}.configureBuild(cfg)
		assert.True(t, cfg.elevated)
		assert.Equal(t, "OHSS-9999", cfg.elevationReason)
		assert.Empty(t, cfg.clusterID)
	})

	t.Run("WithClusterID and WithElevation together accumulate correctly", func(t *testing.T) {
		t.Parallel()
		cfg := &buildConfig{}
		WithClusterID{ClusterID: "mgmt-cluster-id"}.configureBuild(cfg)
		WithElevation{Reason: "OHSS-1234"}.configureBuild(cfg)
		assert.Equal(t, "mgmt-cluster-id", cfg.clusterID)
		assert.True(t, cfg.elevated)
		assert.Equal(t, "OHSS-1234", cfg.elevationReason)
	})
}

// ── validateSchedule ───────────────────────────────────────────────────────

// newSchedule returns an *unstructured.Unstructured representing a Velero Schedule CR.
func newSchedule(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "Schedule",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func TestValidateSchedule(t *testing.T) {
	t.Parallel()

	const (
		defaultNS    = "openshift-adp"
		scheduleName = "abc123-daily"
		otherNS      = "other-ns"
		otherName    = "xyz-daily"
	)

	tests := []struct {
		name         string
		seededObjs   []unstructured.Unstructured // objects to seed into the fake client
		scheduleName string                      // schedule name to look up
		wantErr      bool
		errContains  []string
	}{
		{
			name:         "schedule exists in correct namespace",
			seededObjs:   []unstructured.Unstructured{*newSchedule(scheduleName, defaultNS)},
			scheduleName: scheduleName,
			wantErr:      false,
		},
		{
			name:         "schedule missing — client is empty",
			seededObjs:   nil,
			scheduleName: scheduleName,
			wantErr:      true,
			errContains:  []string{scheduleName, defaultNS},
		},
		{
			name:         "schedule exists in wrong namespace",
			seededObjs:   []unstructured.Unstructured{*newSchedule(scheduleName, otherNS)},
			scheduleName: scheduleName,
			wantErr:      true,
			errContains:  []string{scheduleName, defaultNS},
		},
		{
			name:         "schedule exists with wrong name",
			seededObjs:   []unstructured.Unstructured{*newSchedule(otherName, defaultNS)},
			scheduleName: scheduleName,
			wantErr:      true,
			errContains:  []string{scheduleName, defaultNS},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeBuilder := fake.NewClientBuilder()
			for i := range tt.seededObjs {
				fakeBuilder = fakeBuilder.WithObjects(&tt.seededObjs[i])
			}
			readClient := newTestClient(fakeBuilder.Build())

			runner := &defaultBackupRunner{
				cfg: newDefaultBackupRunnerConfig(),
			}

			err := runner.validateSchedule(context.Background(), readClient, tt.scheduleName)

			if tt.wantErr {
				assert.Error(t, err)
				for _, substr := range tt.errContains {
					assert.Contains(t, err.Error(), substr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ── findVeleroPod ──────────────────────────────────────────────────────────

// newVeleroPod builds a corev1.Pod with the standard Velero label selector.
// The pod has no PodReady condition; use newReadyVeleroPod for a fully ready pod.
func newVeleroPod(name, namespace string, phase corev1.PodPhase, labelValue string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": labelValue,
			},
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}
}

// newReadyVeleroPod builds a Running, Ready, non-terminating Velero pod.
func newReadyVeleroPod(name, namespace string) *corev1.Pod {
	pod := newVeleroPod(name, namespace, corev1.PodRunning, "velero")
	pod.Status.Conditions = []corev1.PodCondition{
		{Type: corev1.PodReady, Status: corev1.ConditionTrue},
	}
	return pod
}

func TestFindVeleroPod(t *testing.T) {
	t.Parallel()

	const (
		defaultNS   = "openshift-adp"
		otherNS     = "other-ns"
		veleroLabel = "velero"
		wrongLabel  = "not-velero"
		pod1Name    = "velero-abc-111"
		pod2Name    = "velero-abc-222"
	)

	tests := []struct {
		name        string
		pods        []*corev1.Pod
		wantPodName string
		wantErr     bool
		errContains []string
	}{
		{
			name:        "one running, ready pod with correct label",
			pods:        []*corev1.Pod{newReadyVeleroPod(pod1Name, defaultNS)},
			wantPodName: pod1Name,
			wantErr:     false,
		},
		{
			name: "multiple running ready pods — first is returned",
			pods: []*corev1.Pod{
				newReadyVeleroPod(pod1Name, defaultNS),
				newReadyVeleroPod(pod2Name, defaultNS),
			},
			wantPodName: pod1Name,
			wantErr:     false,
		},
		{
			name:        "no pods at all",
			pods:        nil,
			wantErr:     true,
			errContains: []string{defaultNS, "app.kubernetes.io/name=velero"},
		},
		{
			name:        "pod exists but phase is Pending",
			pods:        []*corev1.Pod{newVeleroPod(pod1Name, defaultNS, corev1.PodPending, veleroLabel)},
			wantErr:     true,
			errContains: []string{defaultNS, "app.kubernetes.io/name=velero"},
		},
		{
			name:        "pod exists but phase is Succeeded",
			pods:        []*corev1.Pod{newVeleroPod(pod1Name, defaultNS, corev1.PodSucceeded, veleroLabel)},
			wantErr:     true,
			errContains: []string{defaultNS, "app.kubernetes.io/name=velero"},
		},
		{
			name:        "pod exists but phase is Failed",
			pods:        []*corev1.Pod{newVeleroPod(pod1Name, defaultNS, corev1.PodFailed, veleroLabel)},
			wantErr:     true,
			errContains: []string{defaultNS, "app.kubernetes.io/name=velero"},
		},
		{
			name:        "pod is running but has wrong label value",
			pods:        []*corev1.Pod{newVeleroPod(pod1Name, defaultNS, corev1.PodRunning, wrongLabel)},
			wantErr:     true,
			errContains: []string{defaultNS, "app.kubernetes.io/name=velero"},
		},
		{
			name:        "pod is running with correct label but in wrong namespace",
			pods:        []*corev1.Pod{newVeleroPod(pod1Name, otherNS, corev1.PodRunning, veleroLabel)},
			wantErr:     true,
			errContains: []string{defaultNS, "app.kubernetes.io/name=velero"},
		},
		{
			// A Running pod without a PodReady=True condition should be skipped.
			name:        "pod is running but PodReady condition is False",
			pods:        []*corev1.Pod{newVeleroPod(pod1Name, defaultNS, corev1.PodRunning, veleroLabel)},
			wantErr:     true,
			errContains: []string{defaultNS, "app.kubernetes.io/name=velero"},
		},
		{
			// A terminating pod (DeletionTimestamp set) should be skipped even
			// if it is Running and Ready. A finalizer is required so the fake
			// client accepts the object with a non-nil DeletionTimestamp.
			name: "pod is running and ready but terminating",
			pods: func() []*corev1.Pod {
				pod := newReadyVeleroPod(pod1Name, defaultNS)
				pod.Finalizers = []string{"test/keep"}
				now := metav1.Now()
				pod.DeletionTimestamp = &now
				return []*corev1.Pod{pod}
			}(),
			wantErr:     true,
			errContains: []string{defaultNS, "app.kubernetes.io/name=velero"},
		},
		{
			// A terminating pod is skipped; the next ready pod is returned.
			// The terminating pod has a finalizer so the fake client accepts it.
			name: "terminating pod skipped, second ready pod returned",
			pods: func() []*corev1.Pod {
				terminating := newReadyVeleroPod(pod1Name, defaultNS)
				terminating.Finalizers = []string{"test/keep"}
				now := metav1.Now()
				terminating.DeletionTimestamp = &now
				return []*corev1.Pod{terminating, newReadyVeleroPod(pod2Name, defaultNS)}
			}(),
			wantPodName: pod2Name,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeBuilder := fake.NewClientBuilder()
			for _, p := range tt.pods {
				fakeBuilder = fakeBuilder.WithObjects(p)
			}
			execClient := newTestClient(fakeBuilder.Build())

			runner := &defaultBackupRunner{
				cfg: newDefaultBackupRunnerConfig(),
			}

			podName, err := runner.findVeleroPod(context.Background(), execClient)

			if tt.wantErr {
				assert.Error(t, err)
				for _, substr := range tt.errContains {
					assert.Contains(t, err.Error(), substr)
				}
				assert.Empty(t, podName)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPodName, podName)
			}
		})
	}
}

// ── Run ────────────────────────────────────────────────────────────────────

func TestRun(t *testing.T) {
	t.Parallel()

	const (
		clusterID    = "abc123"
		scheduleName = clusterID + "-daily"
		podName      = "velero-pod-1"
		reason       = "OHSS-9999"
	)

	// newRunningPod and newDailySchedule construct fresh objects each call to
	// avoid concurrent-map-write races when parallel subtests pass the same
	// pointer to fake.ClientBuilder.WithObjects (which mutates the object's
	// ResourceVersion during Build).
	newRunningPod := func() *corev1.Pod {
		return newReadyVeleroPod(podName, "openshift-adp")
	}
	newDailySchedule := func() *unstructured.Unstructured {
		return newSchedule(scheduleName, "openshift-adp")
	}

	tests := []struct {
		name     string
		readObjs func() []client.Object // built fresh inside each subtest
		execObjs func() []client.Object // built fresh inside each subtest
		// execFn receives the subtest *testing.T so assertions inside the
		// closure are reported on the correct subtest (not the parent TestRun).
		execFn          func(t *testing.T, ctx context.Context, namespace, pod, container string, cmd []string) (string, error)
		resolverErr     error // if set, staticClusterResolver returns this error
		unprivilegedErr error // if set, staticKubeClientBuilder.Build() returns this for unprivileged calls
		privilegedErr   error // if set, staticKubeClientBuilder.Build() returns this for elevated calls
		flags           *backupFlags
		wantErr         bool
		errContains     string
		schedSuffix     string        // non-empty activates WithScheduleNameSuffix
		wantOutput      []string      // substrings expected in printer output
		wantNoOutput    []string      // substrings that must NOT appear
		wantBuildCalls  []buildConfig // if non-nil, asserts on the Build calls received by the builder
	}{
		{
			name:     "happy path — backup ID parsed, hint uses oc get backup",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(_ *testing.T, _ context.Context, _ string, _ string, _ string, _ []string) (string, error) {
				return `Creating backup from schedule, all other filters are ignored.
Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
Run ` + "`velero backup describe " + clusterID + `-daily-20260319184212` + "`" + ` for more details.
`, nil
			},
			flags:   &backupFlags{clusterID: clusterID, reason: reason},
			wantErr: false,
			wantOutput: []string{
				clusterID + "-daily-20260319184212",
				"triggered successfully",
				"oc get backup",
				"openshift-adp",
			},
			wantNoOutput: []string{
				// raw velero output must not be forwarded to the printer
				"Creating backup from schedule",
				"Run `velero backup describe",
				// old grep-based hint must not appear
				"grep",
			},
		},
		{
			name:     "unparseable velero output — raw output printed, no error",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(_ *testing.T, _ context.Context, _ string, _ string, _ string, _ []string) (string, error) {
				return "some unexpected velero output\n", nil
			},
			flags:   &backupFlags{clusterID: clusterID, reason: reason},
			wantErr: false,
			wantOutput: []string{
				"some unexpected velero output",
				"could not parse backup ID",
			},
		},
		{
			name:        "validateSchedule fails — schedule not found",
			readObjs:    func() []client.Object { return nil }, // no schedule seeded
			execObjs:    func() []client.Object { return []client.Object{newRunningPod()} },
			flags:       &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:     true,
			errContains: scheduleName,
		},
		{
			name:        "findVeleroPod fails — no running pods",
			readObjs:    func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs:    func() []client.Object { return nil }, // no pods seeded
			flags:       &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:     true,
			errContains: "no running Velero pod found",
		},
		{
			name:     "Exec fails — error wraps schedule name",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(_ *testing.T, _ context.Context, _ string, _ string, _ string, _ []string) (string, error) {
				return "", errors.New("connection refused")
			},
			flags:       &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:     true,
			errContains: scheduleName,
		},
		{
			name:     "custom schedule suffix via config option",
			readObjs: func() []client.Object { return []client.Object{newSchedule(clusterID+"-weekly", "openshift-adp")} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, _ string, _ string, _ string, cmd []string) (string, error) {
				// Verify the correct schedule name is passed to the velero command
				assert.Contains(t, cmd[len(cmd)-1], "-weekly")
				return `Backup request "` + clusterID + `-weekly-20260319184212" submitted successfully.
`, nil
			},
			flags:       &backupFlags{clusterID: clusterID, reason: reason},
			schedSuffix: "-weekly",
			wantErr:     false,
			wantOutput: []string{
				clusterID + "-weekly-20260319184212",
				"triggered successfully",
			},
		},
		{
			name:     "single label — forwarded to velero as --labels key=value",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, _ string, _ string, _ string, cmd []string) (string, error) {
				// --labels and its value must appear somewhere in the command slice
				labelsIdx := -1
				for i, arg := range cmd {
					if arg == "--labels" {
						labelsIdx = i
						break
					}
				}
				assert.NotEqual(t, -1, labelsIdx, "--labels flag must be present")
				if labelsIdx != -1 && labelsIdx+1 < len(cmd) {
					assert.Equal(t, "incident=OHSS-1234", cmd[labelsIdx+1])
				}
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason, labels: map[string]string{"incident": "OHSS-1234"}},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "multiple labels — sorted and joined, forwarded to velero",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, _ string, _ string, _ string, cmd []string) (string, error) {
				labelsIdx := -1
				for i, arg := range cmd {
					if arg == "--labels" {
						labelsIdx = i
						break
					}
				}
				assert.NotEqual(t, -1, labelsIdx, "--labels flag must be present")
				if labelsIdx != -1 && labelsIdx+1 < len(cmd) {
					// Keys are sorted alphabetically: env before incident
					assert.Equal(t, "env=prod,incident=OHSS-5678", cmd[labelsIdx+1])
				}
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason, labels: map[string]string{"incident": "OHSS-5678", "env": "prod"}},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "no labels — --labels flag not passed to velero",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, _ string, _ string, _ string, cmd []string) (string, error) {
				for _, arg := range cmd {
					assert.NotEqual(t, "--labels", arg, "--labels must not appear when no labels are provided")
				}
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "single annotation — forwarded to velero as --annotations key=value",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, _ string, _ string, _ string, cmd []string) (string, error) {
				annIdx := -1
				for i, arg := range cmd {
					if arg == "--annotations" {
						annIdx = i
						break
					}
				}
				assert.NotEqual(t, -1, annIdx, "--annotations flag must be present")
				if annIdx != -1 && annIdx+1 < len(cmd) {
					assert.Equal(t, "ticket=OHSS-9999", cmd[annIdx+1])
				}
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason, annotations: map[string]string{"ticket": "OHSS-9999"}},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "multiple annotations — sorted and joined, forwarded to velero",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, _ string, _ string, _ string, cmd []string) (string, error) {
				annIdx := -1
				for i, arg := range cmd {
					if arg == "--annotations" {
						annIdx = i
						break
					}
				}
				assert.NotEqual(t, -1, annIdx, "--annotations flag must be present")
				if annIdx != -1 && annIdx+1 < len(cmd) {
					// Keys sorted: owner before ticket
					assert.Equal(t, "owner=sre-team,ticket=OHSS-9999", cmd[annIdx+1])
				}
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason, annotations: map[string]string{"ticket": "OHSS-9999", "owner": "sre-team"}},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "no annotations — --annotations flag not passed to velero",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, _ string, _ string, _ string, cmd []string) (string, error) {
				for _, arg := range cmd {
					assert.NotEqual(t, "--annotations", arg, "--annotations must not appear when no annotations are provided")
				}
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "labels and annotations together — both forwarded",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, _ string, _ string, _ string, cmd []string) (string, error) {
				labelsIdx, annIdx := -1, -1
				for i, arg := range cmd {
					switch arg {
					case "--labels":
						labelsIdx = i
					case "--annotations":
						annIdx = i
					}
				}
				assert.NotEqual(t, -1, labelsIdx, "--labels must be present")
				assert.NotEqual(t, -1, annIdx, "--annotations must be present")
				if labelsIdx != -1 && labelsIdx+1 < len(cmd) {
					assert.Equal(t, "env=prod", cmd[labelsIdx+1])
				}
				if annIdx != -1 && annIdx+1 < len(cmd) {
					assert.Equal(t, "ticket=OHSS-0001", cmd[annIdx+1])
				}
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags: &backupFlags{
				clusterID:   clusterID,
				reason:      reason,
				labels:      map[string]string{"env": "prod"},
				annotations: map[string]string{"ticket": "OHSS-0001"},
			},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			// Proves that ClusterResolver failures surface before any kube login.
			name:        "ClusterResolver fails — error returned before any login",
			readObjs:    func() []client.Object { return nil },
			execObjs:    func() []client.Object { return nil },
			resolverErr: errors.New("OCM unreachable"),
			flags:       &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:     true,
			errContains: "OCM unreachable",
		},
		{
			// Proves that an unprivileged login failure is surfaced immediately,
			// before any schedule validation or elevated login attempt.
			name:            "unprivileged client build fails — error returned before validation",
			readObjs:        func() []client.Object { return nil },
			execObjs:        func() []client.Object { return nil },
			unprivilegedErr: errors.New("backplane unavailable"),
			flags:           &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:         true,
			errContains:     "backplane unavailable",
		},
		{
			// Proves that the elevated login only occurs after schedule validation
			// passes: the schedule is present so validation succeeds, but the
			// elevated Build call then returns an error that Run surfaces.
			name:          "elevated client build fails — only triggered after schedule validation",
			readObjs:      func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs:      func() []client.Object { return nil },
			privilegedErr: errors.New("elevation denied"),
			flags:         &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:       true,
			errContains:   "elevation denied",
		},
		{
			// Proves that Run passes MgmtClusterID (not HCPClusterID) and the
			// correct elevation reason to both Build calls.
			name:     "Build receives correct clusterID and elevation reason",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(_ *testing.T, _ context.Context, _, _, _ string, _ []string) (string, error) {
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:   &backupFlags{clusterID: clusterID, reason: reason},
			wantErr: false,
			wantBuildCalls: []buildConfig{
				// First call: unprivileged — clusterID set, not elevated.
				{clusterID: "mgmt-cluster-id", elevated: false, elevationReason: ""},
				// Second call: elevated — clusterID and reason both set.
				{clusterID: "mgmt-cluster-id", elevated: true, elevationReason: reason},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			readBuilder := fake.NewClientBuilder()
			for _, o := range tt.readObjs() {
				readBuilder = readBuilder.WithObjects(o)
			}
			readClient := newTestClient(readBuilder.Build())

			execBuilder := fake.NewClientBuilder()
			for _, o := range tt.execObjs() {
				execBuilder = execBuilder.WithObjects(o)
			}
			var execFn func(ctx context.Context, namespace, pod, container string, cmd []string) (string, error)
			if tt.execFn != nil {
				ttExecFn := tt.execFn // capture loop var
				execFn = func(ctx context.Context, namespace, pod, container string, cmd []string) (string, error) {
					return ttExecFn(t, ctx, namespace, pod, container, cmd)
				}
			}
			execClient := &testKubeClient{Client: execBuilder.Build(), execFn: execFn}

			var out strings.Builder
			opts := []DefaultBackupRunnerOption{
				WithPrinter{Printer: &defaultPrinter{w: &out}},
			}
			if tt.schedSuffix != "" {
				opts = append(opts, WithScheduleNameSuffix(tt.schedSuffix))
			}

			clientBuilder := &staticKubeClientBuilder{
				unprivilegedClient: readClient,
				privilegedClient:   execClient,
				unprivilegedErr:    tt.unprivilegedErr,
				privilegedErr:      tt.privilegedErr,
			}
			opts = append(opts,
				WithResolver{Resolver: &staticClusterResolver{
					clusterInfo: ClusterInfo{
						HCPClusterID:  clusterID,
						MgmtClusterID: "mgmt-cluster-id",
					},
					err: tt.resolverErr,
				}},
				WithBuilder{Builder: clientBuilder},
			)
			runner := NewDefaultBackupRunner(nil, opts...)

			err := runner.Run(context.Background(), tt.flags)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				got := out.String()
				for _, substr := range tt.wantOutput {
					assert.Contains(t, got, substr)
				}
				for _, substr := range tt.wantNoOutput {
					assert.NotContains(t, got, substr)
				}
			}
			if tt.wantBuildCalls != nil {
				assert.Equal(t, tt.wantBuildCalls, clientBuilder.calls,
					"Build calls should match expected clusterID and elevation options")
			}
		})
	}
}

// ── joinSortedMap ──────────────────────────────────────────────────────────

func TestJoinSortedMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		m    map[string]string
		want string
	}{
		{
			name: "empty map returns empty string",
			m:    map[string]string{},
			want: "",
		},
		{
			name: "single entry",
			m:    map[string]string{"incident": "OHSS-1234"},
			want: "incident=OHSS-1234",
		},
		{
			name: "multiple entries sorted alphabetically by key",
			m:    map[string]string{"zzz": "last", "aaa": "first", "mmm": "middle"},
			want: "aaa=first,mmm=middle,zzz=last",
		},
		{
			name: "keys with equal prefix sorted correctly",
			m:    map[string]string{"env": "prod", "env2": "staging"},
			want: "env=prod,env2=staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, joinSortedMap(tt.m))
		})
	}
}

// ── backupFlags.AddFlags ───────────────────────────────────────────────────

func TestBackupFlags_AddFlags(t *testing.T) {
	t.Parallel()

	t.Run("--label and --annotation flags are registered and parsed", func(t *testing.T) {
		t.Parallel()

		f := &backupFlags{}
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		f.AddFlags(fs)

		err := fs.Parse([]string{
			"--cluster-id", "abc123",
			"--reason", "OHSS-1234",
			"--label", "env=prod",
			"--label", "incident=OHSS-1234",
			"--annotation", "owner=sre-team",
		})
		assert.NoError(t, err)
		assert.Equal(t, "abc123", f.clusterID)
		assert.Equal(t, "OHSS-1234", f.reason)
		assert.Equal(t, map[string]string{"env": "prod", "incident": "OHSS-1234"}, f.labels)
		assert.Equal(t, map[string]string{"owner": "sre-team"}, f.annotations)
	})

	t.Run("--label and --annotation default to nil when not provided", func(t *testing.T) {
		t.Parallel()

		f := &backupFlags{}
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		f.AddFlags(fs)

		err := fs.Parse([]string{"--cluster-id", "abc123", "--reason", "OHSS-1234"})
		assert.NoError(t, err)
		assert.Nil(t, f.labels)
		assert.Nil(t, f.annotations)
	})
}

// ── defaultPrinter ─────────────────────────────────────────────────────────

func TestDefaultPrinter(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	p := &defaultPrinter{w: &sb}

	p.Print("hello ")
	p.Printf("x=%d", 42)

	assert.Equal(t, "hello x=42", sb.String())
}
