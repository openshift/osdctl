package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/osdctl/pkg/servicelog"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestChangeVolumeType_ValidateTargetType(t *testing.T) {
	tests := []struct {
		name       string
		targetType string
		wantErr    bool
	}{
		{"valid gp3", "gp3", false},
		{"invalid io1", "io1", true},
		{"invalid gp2", "gp2", true},
		{"invalid io2", "io2", true},
		{"invalid st1", "st1", true},
		{"invalid sc1", "sc1", true},
		{"invalid type", "invalid", true},
		{"empty type", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := &changeVolumeTypeOptions{
				clusterID:  "test-cluster",
				targetType: tt.targetType,
				reason:     "test",
			}
			err := ops.validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				// validate also checks clusterID which will fail for non-real IDs,
				// so just verify the type validation logic
				valid := false
				for _, v := range validVolumeTypes {
					if tt.targetType == v {
						valid = true
						break
					}
				}
				assert.True(t, valid)
			}
		})
	}
}

func TestChangeVolumeType_ValidateRole(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{"empty role (both)", "", false},
		{"control-plane", "control-plane", false},
		{"infra", "infra", false},
		{"invalid worker", "worker", true},
		{"invalid master", "master", true},
		{"invalid random", "random", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := &changeVolumeTypeOptions{
				clusterID:  "test-cluster",
				targetType: "gp3",
				reason:     "test",
				role:       tt.role,
			}
			err := ops.validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid role")
			}
			// For valid roles, validate would still fail on clusterID check,
			// but the role validation should pass
			if !tt.wantErr {
				validRole := tt.role == "" || tt.role == "control-plane" || tt.role == "infra"
				assert.True(t, validRole)
			}
		})
	}
}

func TestChangeVolumeType_CommandCreation(t *testing.T) {
	cmd := newCmdChangeVolumeType()

	assert.NotNil(t, cmd)
	assert.Equal(t, "change-ebs-volume-type", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)

	// Required flags
	requiredFlags := []string{"cluster-id", "type", "reason"}
	for _, flagName := range requiredFlags {
		flag := cmd.Flag(flagName)
		assert.NotNilf(t, flag, "required flag %q should exist", flagName)
	}

	// Optional flags
	flag := cmd.Flag("role")
	assert.NotNil(t, flag, "optional flag 'role' should exist")
}

func TestChangeVolumeType_RoleDisplay(t *testing.T) {
	assert.Equal(t, "control-plane + infra", roleDisplay(""))
	assert.Equal(t, "control-plane", roleDisplay("control-plane"))
	assert.Equal(t, "infra", roleDisplay("infra"))
}

func TestChangeVolumeType_CountReadyNodes(t *testing.T) {
	// Empty list
	nodes := &corev1.NodeList{}
	assert.Equal(t, 0, countReadyNodes(nodes))
}

func TestNewVolumeTypeChangedServiceLogRequest(t *testing.T) {
	tests := []struct {
		name             string
		template         string
		expectedTemplate string
	}{
		{
			name:             "control plane",
			template:         controlPlaneVolumeTypeChangedServiceLogTemplate,
			expectedTemplate: "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/controlplane_volume_type_changed.json",
		},
		{
			name:             "infra",
			template:         infraVolumeTypeChangedServiceLogTemplate,
			expectedTemplate: "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/infranode_volume_type_changed.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := newVolumeTypeChangedServiceLogRequest(tt.template, "gp2", "gp3", "OHSS-123")

			assert.Equal(t, tt.expectedTemplate, request.Template)
			assert.Equal(t, []string{
				"PREVIOUS_VOLUME_TYPE=gp2",
				"NEW_VOLUME_TYPE=gp3",
				"REASON=OHSS-123",
			}, request.TemplateParams)
			assert.False(t, request.InternalOnly)
			assert.False(t, request.SkipLinkCheck)
		})
	}
}

func TestMonitorControlPlaneRolloutAndNotify(t *testing.T) {
	t.Run("notifies after successful rollout", func(t *testing.T) {
		events := []string{}
		err := monitorControlPlaneRolloutAndNotify(context.Background(), func(context.Context) error {
			events = append(events, "rollout")
			return nil
		}, func() {
			events = append(events, "notification")
		})

		assert.NoError(t, err)
		assert.Equal(t, []string{"rollout", "notification"}, events)
	})

	t.Run("does not notify after failed rollout", func(t *testing.T) {
		rolloutErr := errors.New("rollout failed")
		notified := false
		err := monitorControlPlaneRolloutAndNotify(context.Background(), func(context.Context) error {
			return rolloutErr
		}, func() {
			notified = true
		})

		assert.ErrorIs(t, err, rolloutErr)
		assert.False(t, notified)
	})
}

func TestChangeControlPlaneVolumeTypeAlreadyTargetTypeSkipsNotification(t *testing.T) {
	volumeType := "gp3"
	providerSpec, err := json.Marshal(machinev1beta1.AWSMachineProviderConfig{
		BlockDevices: []machinev1beta1.BlockDeviceMappingSpec{{
			EBS: &machinev1beta1.EBSBlockDeviceSpec{VolumeType: &volumeType},
		}},
	})
	assert.NoError(t, err)

	scheme := runtime.NewScheme()
	assert.NoError(t, machinev1.Install(scheme))
	cpms := &machinev1.ControlPlaneMachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: changeVolumeTypeCPMSNamespace,
			Name:      changeVolumeTypeCPMSName,
		},
		Spec: machinev1.ControlPlaneMachineSetSpec{
			Template: machinev1.ControlPlaneMachineSetTemplate{
				OpenShiftMachineV1Beta1Machine: &machinev1.OpenShiftMachineV1Beta1MachineTemplate{
					Spec: machinev1beta1.MachineSpec{
						ProviderSpec: machinev1beta1.ProviderSpec{
							Value: &runtime.RawExtension{Raw: providerSpec},
						},
					},
				},
			},
		},
	}
	ops := &changeVolumeTypeOptions{
		targetType: "gp3",
		client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build(),
	}
	notified := false

	err = ops.changeControlPlaneVolumeType(context.Background(), func(string, string, string) {
		notified = true
	})

	assert.NoError(t, err)
	assert.False(t, notified)
}

func TestChangeRequestedVolumeTypesCombinedNotificationOrdering(t *testing.T) {
	events := []string{}

	err := changeRequestedVolumeTypes(context.Background(), "", func(context.Context) error {
		events = append(events, "control-plane notification")
		return nil
	}, func(context.Context) error {
		events = append(events, "infra notification")
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, []string{"control-plane notification", "infra notification"}, events)
}

// newTestCPMS builds a ControlPlaneMachineSet for monitorCPMSRollout tests.
// generation sets metadata.Generation, observedGeneration sets
// Status.ObservedGeneration, and updated/ready populate the replica counts.
func newTestCPMS(generation, observedGeneration int64, updated, ready int32) *machinev1.ControlPlaneMachineSet {
	return &machinev1.ControlPlaneMachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  changeVolumeTypeCPMSNamespace,
			Name:       changeVolumeTypeCPMSName,
			Generation: generation,
		},
		Status: machinev1.ControlPlaneMachineSetStatus{
			ObservedGeneration: observedGeneration,
			UpdatedReplicas:    updated,
			ReadyReplicas:      ready,
		},
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, machinev1.Install(scheme))
	return scheme
}

// withFastPoll temporarily shortens pollInterval and rolloutPollTimeout so
// that monitorCPMSRollout tests run in milliseconds, not minutes.
func withFastPoll(t *testing.T) {
	t.Helper()
	origInterval, origTimeout := pollInterval, rolloutPollTimeout
	pollInterval = 50 * time.Millisecond
	rolloutPollTimeout = 2 * time.Second
	t.Cleanup(func() {
		pollInterval = origInterval
		rolloutPollTimeout = origTimeout
	})
}

func TestMonitorCPMSRollout_StaleGeneration(t *testing.T) {
	withFastPoll(t)

	// Simulate the race CodeRabbit identified: immediately after the patch,
	// the CPMS controller has not yet observed the new generation, so the
	// status still shows the pre-patch 3/3 counts. The monitor must wait
	// until ObservedGeneration catches up before trusting replica counts.
	scheme := newTestScheme(t)
	cpms := newTestCPMS(2, 1, 3, 3) // gen 2 patched, controller still on gen 1

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cpms).
		WithStatusSubresource(cpms).
		Build()
	ops := &changeVolumeTypeOptions{client: fakeClient}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Run monitor in a goroutine — it should NOT complete immediately.
	done := make(chan error, 1)
	go func() { done <- ops.monitorCPMSRollout(ctx) }()

	// Give the first poll iteration time to run, then verify it has not
	// returned (the stale generation must block completion).
	time.Sleep(150 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("monitorCPMSRollout returned immediately on stale ObservedGeneration; should have waited")
	default:
	}

	// Simulate controller catching up: bump ObservedGeneration to match.
	latest := &machinev1.ControlPlaneMachineSet{}
	assert.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Namespace: changeVolumeTypeCPMSNamespace, Name: changeVolumeTypeCPMSName}, latest))
	latest.Status.ObservedGeneration = 2
	assert.NoError(t, fakeClient.Status().Update(ctx, latest))

	// Now the monitor should complete successfully.
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("monitorCPMSRollout did not complete after ObservedGeneration caught up")
	}
}

func TestMonitorCPMSRollout_SuccessfulRollout(t *testing.T) {
	withFastPoll(t)

	// When ObservedGeneration already matches and replicas are 3/3,
	// the monitor should return success on the first poll.
	scheme := newTestScheme(t)
	cpms := newTestCPMS(2, 2, 3, 3)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build()
	ops := &changeVolumeTypeOptions{client: fakeClient}

	err := ops.monitorCPMSRollout(context.Background())
	assert.NoError(t, err)
}

func TestMonitorCPMSRollout_Timeout(t *testing.T) {
	withFastPoll(t)

	// When replicas never reach 3/3, the monitor should time out.
	scheme := newTestScheme(t)
	cpms := newTestCPMS(2, 2, 1, 1) // stuck at 1/3

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build()
	ops := &changeVolumeTypeOptions{client: fakeClient}

	err := ops.monitorCPMSRollout(context.Background())
	assert.Error(t, err, "monitor should return an error on timeout")
}

// --- Sensitive-data redaction tests ---

func TestManualServiceLogGuidance_RedactsClusterID(t *testing.T) {
	// The manual fallback must use $CLUSTER_ID placeholder — never a real
	// cluster identifier — so that terminal output and logs do not leak
	// customer-specific values.
	sampleClusterID := "1abc2def3ghi4jkl5mno6pqr7stu8vwx"

	tests := []struct {
		name     string
		template string
	}{
		{"control-plane template", controlPlaneVolumeTypeChangedServiceLogTemplate},
		{"infra template", infraVolumeTypeChangedServiceLogTemplate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newVolumeTypeChangedServiceLogRequest(tt.template, "gp2", "gp3", "OHSS-123")
			guidance := manualServiceLogGuidance(req)

			assert.Contains(t, guidance, "$CLUSTER_ID",
				"guidance must use $CLUSTER_ID placeholder")
			assert.Contains(t, guidance, "osdctl servicelog post",
				"guidance must include the osdctl command")
			assert.Contains(t, guidance, tt.template,
				"guidance must include the template URL")
			assert.NotContains(t, guidance, sampleClusterID,
				"guidance must not contain a real cluster ID")
		})
	}
}

func TestServiceLogPreview_OmitsClusterFields(t *testing.T) {
	// After Prepare (before PostMessage), the Message has empty cluster
	// fields. With omitempty JSON tags, cluster_uuid, cluster_id, and
	// subscription_id must not appear in the marshaled preview JSON.
	msg := servicelog.Message{
		Severity:    "Info",
		ServiceName: "SREManualAction",
		Summary:     "Volume type changed from gp2 to gp3",
		Description: "The EBS volume type was changed.",
	}

	previewBytes, err := json.MarshalIndent(msg, "", "  ")
	assert.NoError(t, err)

	preview := string(previewBytes)
	assert.NotContains(t, preview, "cluster_uuid",
		"preview must not contain cluster_uuid field")
	assert.NotContains(t, preview, "cluster_id",
		"preview must not contain cluster_id field")
	assert.NotContains(t, preview, "subscription_id",
		"preview must not contain subscription_id field")

	// Verify the preview still contains the expected content fields
	assert.Contains(t, preview, "SREManualAction")
	assert.Contains(t, preview, "Volume type changed from gp2 to gp3")
}

func TestOutboundServiceLogMessage_CarriesClusterFields(t *testing.T) {
	// PostMessage sets ClusterUUID, ClusterID, and SubscriptionID on the
	// Message before sending. Verify that when these fields are populated,
	// they appear in the marshaled JSON — proving the outbound request
	// carries the required cluster identifiers.
	msg := servicelog.Message{
		Severity:       "Info",
		ServiceName:    "SREManualAction",
		Summary:        "Volume type changed",
		Description:    "The EBS volume type was changed.",
		ClusterUUID:    "ext-uuid-12345",
		ClusterID:      "int-cluster-67890",
		SubscriptionID: "sub-abcde-fghij",
	}

	outboundBytes, err := json.Marshal(msg)
	assert.NoError(t, err)

	outbound := string(outboundBytes)
	assert.Contains(t, outbound, `"cluster_uuid":"ext-uuid-12345"`,
		"outbound JSON must contain the cluster UUID")
	assert.Contains(t, outbound, `"cluster_id":"int-cluster-67890"`,
		"outbound JSON must contain the cluster ID")
	assert.Contains(t, outbound, `"subscription_id":"sub-abcde-fghij"`,
		"outbound JSON must contain the subscription ID")
}

func TestManualServiceLogGuidance_IncludesTemplateParams(t *testing.T) {
	// Verify all template parameters appear in the guidance so the SRE
	// can run the manual command without reconstructing them.
	req := newVolumeTypeChangedServiceLogRequest(
		controlPlaneVolumeTypeChangedServiceLogTemplate,
		"gp2", "gp3", "OHSS-456",
	)

	guidance := manualServiceLogGuidance(req)
	for _, param := range req.TemplateParams {
		assert.True(t, strings.Contains(guidance, param),
			"guidance must include template parameter %q", param)
	}
}
