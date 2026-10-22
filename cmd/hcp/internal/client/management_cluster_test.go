package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestTerminalRestorePhase(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		phase    string
		terminal bool
	}{
		{phase: "Completed"},
		{phase: "InProgress"},
		{phase: "WaitingForPluginOperationsPartiallyFailed"},
		{phase: "FinalizingPartiallyFailed"},
		{phase: "Failed", terminal: true},
		{phase: "FailedValidation", terminal: true},
		{phase: "PartiallyFailed", terminal: true},
	} {
		t.Run(tt.phase, func(t *testing.T) {
			t.Parallel()
			if got := terminalRestorePhase(tt.phase); got != tt.terminal {
				t.Errorf("terminalRestorePhase(%q) = %t, want %t", tt.phase, got, tt.terminal)
			}
		})
	}
}

func TestManagementClusterClientGetVeleroSchedule(t *testing.T) {
	schedule := &unstructured.Unstructured{}
	schedule.SetGroupVersionKind(schema.GroupVersionKind{Group: "velero.io", Version: "v1", Kind: "Schedule"})
	schedule.SetNamespace("openshift-adp")
	schedule.SetName("cluster-daily")
	client := &managementClusterClient{client: fake.NewClientBuilder().WithObjects(schedule).Build()}

	got, err := client.GetVeleroSchedule(context.Background(), "openshift-adp", "cluster-daily")
	require.NoError(t, err)
	assert.Equal(t, schedule.GetName(), got.GetName())

	_, err = client.GetVeleroSchedule(context.Background(), "openshift-adp", "missing")
	assert.True(t, apierrors.IsNotFound(err))
	assert.ErrorContains(t, err, "openshift-adp")
}

func TestReadyPodName(t *testing.T) {
	readyPod := func(name string) corev1.Pod {
		return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "openshift-adp"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}}
	}
	terminatingPod := readyPod("terminating")
	now := metav1.Now()
	terminatingPod.DeletionTimestamp = &now

	for _, tt := range []struct {
		name string
		pods []corev1.Pod
		want string
	}{
		{name: "first ready running pod", pods: []corev1.Pod{readyPod("first"), readyPod("second")}, want: "first"},
		{name: "terminating pod skipped", pods: []corev1.Pod{terminatingPod, readyPod("ready")}, want: "ready"},
		{name: "pending pod skipped", pods: []corev1.Pod{{Status: corev1.PodStatus{Phase: corev1.PodPending}}}},
		{name: "not ready pod skipped", pods: []corev1.Pod{{Status: corev1.PodStatus{Phase: corev1.PodRunning}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			name, err := readyPodName(tt.pods, "openshift-adp", "app", "velero")
			if tt.want == "" {
				assert.ErrorContains(t, err, "no running Velero pod")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, name)
		})
	}
}

func TestJoinSortedMap(t *testing.T) {
	for _, tt := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "empty", values: map[string]string{}, want: ""},
		{name: "single value", values: map[string]string{"incident": "OHSS-1234"}, want: "incident=OHSS-1234"},
		{name: "sorted values", values: map[string]string{"zzz": "last", "aaa": "first", "mmm": "middle"}, want: "aaa=first,mmm=middle,zzz=last"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, joinSortedMap(tt.values))
		})
	}
}
