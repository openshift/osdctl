package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type testVeleroClient struct {
	client.Client
	execFn func(ctx context.Context, request VeleroBackupRequest) (string, error)
}

func (t *testVeleroClient) GetSchedule(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	schedule := &unstructured.Unstructured{}
	schedule.SetGroupVersionKind(schema.GroupVersionKind{Group: "velero.io", Version: "v1", Kind: "Schedule"})
	if err := t.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, schedule); err != nil {
		return nil, fmt.Errorf("getting Velero schedule %q in namespace %q: %w", name, namespace, err)
	}
	return schedule, nil
}

func (t *testVeleroClient) CreateFromSchedule(ctx context.Context, request VeleroBackupRequest) (string, error) {
	if t.execFn != nil {
		return t.execFn(ctx, request)
	}
	return "", errors.New("backup creation not configured in this test")
}

type managementCall struct {
	clusterID string
	reason    string
	elevated  bool
}

type testManagementClusterClient struct {
	reader    *testVeleroClient
	creator   *testVeleroClient
	backupErr error
}

func (c *testManagementClusterClient) GetVeleroSchedule(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return c.reader.GetSchedule(ctx, namespace, name)
}

func (c *testManagementClusterClient) CreateVeleroBackup(ctx context.Context, request VeleroBackupRequest) (string, error) {
	if c.backupErr != nil {
		return "", c.backupErr
	}
	return c.creator.CreateFromSchedule(ctx, request)
}

func (c *testManagementClusterClient) RestoreVeleroBackup(context.Context, VeleroRestoreRequest) (VeleroRestoreResult, error) {
	return VeleroRestoreResult{}, errors.New("restore not configured in this test")
}

func (c *testManagementClusterClient) ListVolumeSnapshotContents(context.Context) (*unstructured.UnstructuredList, error) {
	return nil, errors.New("snapshot inventory not configured in this test")
}

func (c *testManagementClusterClient) RemapVolumeSnapshotContents(context.Context, VolumeSnapshotRemapRequest) error {
	return errors.New("VSC remapping not configured in this test")
}

type testBackupOCMClient struct {
	clusterInfo ClusterInfo
	resolverErr error
	client      ManagementClusterClient
	elevated    ManagementClusterClient
	clientErr   error
	elevatedErr error
	calls       []managementCall
}

func (c *testBackupOCMClient) Resolve(_ context.Context, _ string) (ClusterInfo, error) {
	return c.clusterInfo, c.resolverErr
}

func (c *testBackupOCMClient) ManagementCluster(clusterID string, _ ...internalclient.ManagementClusterClientOption) (ManagementClusterClient, error) {
	c.calls = append(c.calls, managementCall{clusterID: clusterID})
	if c.clientErr != nil {
		return nil, c.clientErr
	}
	return c.client, nil
}

func (c *testBackupOCMClient) ElevatedManagementCluster(clusterID, reason string, _ ...internalclient.ManagementClusterClientOption) (ManagementClusterClient, error) {
	c.calls = append(c.calls, managementCall{clusterID: clusterID, reason: reason, elevated: true})
	if c.elevatedErr != nil {
		return nil, c.elevatedErr
	}
	return c.elevated, nil
}

type backupFlags struct {
	clusterID   string
	reason      string
	labels      map[string]string
	annotations map[string]string
}

func (f *backupFlags) request() BackupRequest {
	return BackupRequest{
		ClusterID:   f.clusterID,
		Reason:      f.reason,
		Labels:      f.labels,
		Annotations: f.annotations,
	}
}

func TestBackupRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request BackupRequest
		wantErr string
	}{
		{
			name:    "missing cluster ID",
			request: BackupRequest{Reason: "OHSS-1234"},
			wantErr: "cluster ID is required",
		},
		{
			name:    "missing reason",
			request: BackupRequest{ClusterID: "cluster-id"},
			wantErr: "reason is required",
		},
		{
			name:    "valid request",
			request: BackupRequest{ClusterID: "cluster-id", Reason: "OHSS-1234"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// ── defaultBackupCreatorConfig ─────────────────────────────────────────────

func TestDefaultBackupCreatorConfig_Default(t *testing.T) {
	t.Parallel()

	var cfg DefaultBackupCreatorConfig
	cfg.Defaults()

	assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
	assert.Equal(t, "velero", cfg.VeleroContainer)
	assert.Equal(t, "app.kubernetes.io/name", cfg.VeleroLabelKey)
	assert.Equal(t, "velero", cfg.VeleroLabelValue)
	assert.Equal(t, "-daily", cfg.ScheduleNameSuffix)
}

func TestDefaultBackupCreatorConfig_Options(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     []DefaultBackupCreatorOption
		assertFn func(t *testing.T, cfg DefaultBackupCreatorConfig)
	}{
		{
			name: "WithADPNamespace overrides ADPNamespace",
			opts: []DefaultBackupCreatorOption{WithADPNamespace("custom-ns")},
			assertFn: func(t *testing.T, cfg DefaultBackupCreatorConfig) {
				assert.Equal(t, "custom-ns", cfg.ADPNamespace)
				// other fields stay at defaults
				assert.Equal(t, "velero", cfg.VeleroContainer)
			},
		},
		{
			name: "WithVeleroContainer overrides VeleroContainer",
			opts: []DefaultBackupCreatorOption{WithVeleroContainer("sidecar")},
			assertFn: func(t *testing.T, cfg DefaultBackupCreatorConfig) {
				assert.Equal(t, "sidecar", cfg.VeleroContainer)
				assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
			},
		},
		{
			name: "WithVeleroLabelKey overrides VeleroLabelKey",
			opts: []DefaultBackupCreatorOption{WithVeleroLabelKey("app")},
			assertFn: func(t *testing.T, cfg DefaultBackupCreatorConfig) {
				assert.Equal(t, "app", cfg.VeleroLabelKey)
				assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
			},
		},
		{
			name: "WithVeleroLabelValue overrides VeleroLabelValue",
			opts: []DefaultBackupCreatorOption{WithVeleroLabelValue("backup-agent")},
			assertFn: func(t *testing.T, cfg DefaultBackupCreatorConfig) {
				assert.Equal(t, "backup-agent", cfg.VeleroLabelValue)
				assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
			},
		},
		{
			name: "WithScheduleNameSuffix overrides ScheduleNameSuffix",
			opts: []DefaultBackupCreatorOption{WithScheduleNameSuffix("-weekly")},
			assertFn: func(t *testing.T, cfg DefaultBackupCreatorConfig) {
				assert.Equal(t, "-weekly", cfg.ScheduleNameSuffix)
				assert.Equal(t, "openshift-adp", cfg.ADPNamespace)
			},
		},
		{
			name: "multiple options applied in order, last one wins",
			opts: []DefaultBackupCreatorOption{
				WithADPNamespace("first"),
				WithADPNamespace("second"),
			},
			assertFn: func(t *testing.T, cfg DefaultBackupCreatorConfig) {
				assert.Equal(t, "second", cfg.ADPNamespace)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg DefaultBackupCreatorConfig
			cfg.Options(tt.opts...)
			cfg.Defaults()
			tt.assertFn(t, cfg)
		})
	}
}

// ── NewDefaultBackupCreator defaults and option overrides ──────────────────

func TestNewDefaultBackupCreator_Defaults(t *testing.T) {
	t.Parallel()

	creator := NewDefaultBackupCreator(nil)

	assert.Equal(t, "openshift-adp", creator.cfg.ADPNamespace)
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

func TestGetSchedule(t *testing.T) {
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
		wantNotFound bool
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
			wantNotFound: true,
			errContains:  []string{scheduleName, defaultNS},
		},
		{
			name:         "schedule exists in wrong namespace",
			seededObjs:   []unstructured.Unstructured{*newSchedule(scheduleName, otherNS)},
			scheduleName: scheduleName,
			wantErr:      true,
			wantNotFound: true,
			errContains:  []string{scheduleName, defaultNS},
		},
		{
			name:         "schedule exists with wrong name",
			seededObjs:   []unstructured.Unstructured{*newSchedule(otherName, defaultNS)},
			scheduleName: scheduleName,
			wantErr:      true,
			wantNotFound: true,
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
			managementClient := &testVeleroClient{Client: fakeBuilder.Build()}
			schedule, err := managementClient.GetSchedule(context.Background(), defaultNS, tt.scheduleName)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantNotFound {
					assert.True(t, apierrors.IsNotFound(err))
				}
				for _, substr := range tt.errContains {
					assert.Contains(t, err.Error(), substr)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, schedule)
			}
		})
	}
}

// newReadyVeleroPod builds a Running, Ready, non-terminating Velero pod.
func newReadyVeleroPod(name, namespace string) *corev1.Pod {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"app.kubernetes.io/name": "velero"}}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	pod.Status.Conditions = []corev1.PodCondition{
		{Type: corev1.PodReady, Status: corev1.ConditionTrue},
	}
	return pod
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
		execFn              func(t *testing.T, ctx context.Context, request VeleroBackupRequest) (string, error)
		resolverErr         error // if set, OCM cluster resolution returns this error
		readErr             error // if set, schedule validation returns this
		manageErr           error // if set, backup creation returns this
		flags               *backupFlags
		wantErr             bool
		errContains         string
		schedSuffix         string           // non-empty activates WithScheduleNameSuffix
		wantOutput          []string         // validated by Cobra adapter tests
		wantNoOutput        []string         // validated by Cobra adapter tests
		wantManagementCalls []managementCall // if non-nil, asserts on management-cluster initialization
	}{
		{
			name:     "happy path — backup ID parsed, hint uses oc get backup",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(_ *testing.T, _ context.Context, _ VeleroBackupRequest) (string, error) {
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
			execFn: func(_ *testing.T, _ context.Context, _ VeleroBackupRequest) (string, error) {
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
			name:        "schedule is not found before backup creation",
			readObjs:    func() []client.Object { return nil }, // no schedule seeded
			execObjs:    func() []client.Object { return []client.Object{newRunningPod()} },
			flags:       &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:     true,
			errContains: scheduleName,
			wantManagementCalls: []managementCall{
				{clusterID: "mgmt-cluster-id"},
			},
		},
		{
			name:     "Exec fails — error wraps schedule name",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(_ *testing.T, _ context.Context, _ VeleroBackupRequest) (string, error) {
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
			execFn: func(t *testing.T, _ context.Context, request VeleroBackupRequest) (string, error) {
				assert.Contains(t, request.Schedule, "-weekly")
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
			name:     "single label forwarded to the management client",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, request VeleroBackupRequest) (string, error) {
				assert.Equal(t, map[string]string{"incident": "OHSS-1234"}, request.Labels)
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason, labels: map[string]string{"incident": "OHSS-1234"}},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "multiple labels forwarded to the management client",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, request VeleroBackupRequest) (string, error) {
				assert.Equal(t, map[string]string{"incident": "OHSS-5678", "env": "prod"}, request.Labels)
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason, labels: map[string]string{"incident": "OHSS-5678", "env": "prod"}},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "no labels forwarded to the management client",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, request VeleroBackupRequest) (string, error) {
				assert.Empty(t, request.Labels)
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "single annotation forwarded to the management client",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, request VeleroBackupRequest) (string, error) {
				assert.Equal(t, map[string]string{"ticket": "OHSS-9999"}, request.Annotations)
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason, annotations: map[string]string{"ticket": "OHSS-9999"}},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "multiple annotations forwarded to the management client",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, request VeleroBackupRequest) (string, error) {
				assert.Equal(t, map[string]string{"ticket": "OHSS-9999", "owner": "sre-team"}, request.Annotations)
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:      &backupFlags{clusterID: clusterID, reason: reason, annotations: map[string]string{"ticket": "OHSS-9999", "owner": "sre-team"}},
			wantErr:    false,
			wantOutput: []string{"triggered successfully"},
		},
		{
			name:     "no annotations forwarded to the management client",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(t *testing.T, _ context.Context, request VeleroBackupRequest) (string, error) {
				assert.Empty(t, request.Annotations)
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
			execFn: func(t *testing.T, _ context.Context, request VeleroBackupRequest) (string, error) {
				assert.Equal(t, map[string]string{"env": "prod"}, request.Labels)
				assert.Equal(t, map[string]string{"ticket": "OHSS-0001"}, request.Annotations)
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
			// Proves that an unprivileged client initialization failure is surfaced immediately,
			// before any schedule validation or elevated login attempt.
			name:        "read-only access fails — error returned before validation",
			readObjs:    func() []client.Object { return nil },
			execObjs:    func() []client.Object { return nil },
			readErr:     errors.New("backplane unavailable"),
			flags:       &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:     true,
			errContains: "backplane unavailable",
		},
		{
			// Proves that elevated client initialization only occurs after schedule validation
			// passes: the schedule is present so validation succeeds, but the
			// elevated access call then returns an error that Run surfaces.
			name:        "elevated access fails — only triggered after schedule validation",
			readObjs:    func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs:    func() []client.Object { return nil },
			manageErr:   errors.New("elevation denied"),
			flags:       &backupFlags{clusterID: clusterID, reason: reason},
			wantErr:     true,
			errContains: "elevation denied",
		},
		{
			// Proves schedule validation occurs before backup creation and the
			// supplied reason is forwarded to elevated client initialization.
			name:     "management clients initialize in order with elevation reason",
			readObjs: func() []client.Object { return []client.Object{newDailySchedule()} },
			execObjs: func() []client.Object { return []client.Object{newRunningPod()} },
			execFn: func(_ *testing.T, _ context.Context, _ VeleroBackupRequest) (string, error) {
				return `Backup request "` + clusterID + `-daily-20260319184212" submitted successfully.
`, nil
			},
			flags:   &backupFlags{clusterID: clusterID, reason: reason},
			wantErr: false,
			wantManagementCalls: []managementCall{
				{clusterID: "mgmt-cluster-id"},
				{clusterID: "mgmt-cluster-id", elevated: true, reason: reason},
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
			readClient := &testVeleroClient{Client: readBuilder.Build()}

			execBuilder := fake.NewClientBuilder()
			for _, o := range tt.execObjs() {
				execBuilder = execBuilder.WithObjects(o)
			}
			var execFn func(ctx context.Context, request VeleroBackupRequest) (string, error)
			if tt.execFn != nil {
				ttExecFn := tt.execFn // capture loop var
				execFn = func(ctx context.Context, request VeleroBackupRequest) (string, error) {
					return ttExecFn(t, ctx, request)
				}
			}
			execClient := &testVeleroClient{Client: execBuilder.Build(), execFn: execFn}

			opts := []DefaultBackupCreatorOption{}
			if tt.schedSuffix != "" {
				opts = append(opts, WithScheduleNameSuffix(tt.schedSuffix))
			}

			managementClient := &testManagementClusterClient{
				reader:  readClient,
				creator: execClient,
			}
			ocmClient := &testBackupOCMClient{
				clusterInfo: ClusterInfo{HCPClusterID: clusterID, MgmtClusterID: "mgmt-cluster-id"},
				resolverErr: tt.resolverErr,
				client:      managementClient,
				elevated:    managementClient,
				clientErr:   tt.readErr,
				elevatedErr: tt.manageErr,
			}
			creator := NewDefaultBackupCreator(ocmClient, opts...)

			_, err := creator.CreateBackup(context.Background(), tt.flags.request())

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
			if tt.wantManagementCalls != nil {
				assert.Equal(t, tt.wantManagementCalls, ocmClient.calls,
					"management clients should initialize in the expected order with the expected reason")
			}
		})
	}
}
