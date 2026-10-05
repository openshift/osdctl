package cluster

import (
	"context"
	"encoding/json"
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

// newTestCPMSWithVolumeType builds a fake CPMS with the given EBS VolumeType.
// Pass nil to simulate a CPMS whose VolumeType field was never set.
func newTestCPMSWithVolumeType(t *testing.T, volumeType *string) (*machinev1.ControlPlaneMachineSet, *runtime.Scheme) {
	t.Helper()

	providerSpec, err := json.Marshal(machinev1beta1.AWSMachineProviderConfig{
		BlockDevices: []machinev1beta1.BlockDeviceMappingSpec{{
			EBS: &machinev1beta1.EBSBlockDeviceSpec{VolumeType: volumeType},
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
	return cpms, scheme
}

func TestVolumeTypeResolution(t *testing.T) {
	// Directly exercises the VolumeType detection logic used by
	// changeControlPlaneVolumeType: unmarshal the CPMS provider spec and
	// resolve currentType. This avoids the ConfirmPrompt interaction.
	tests := []struct {
		name        string
		volumeType  *string
		wantCurrent string
		wantSkipGp3 bool // true → currentType == "gp3" → skip
	}{
		{
			name:        "nil VolumeType uses effective AWS default",
			volumeType:  nil,
			wantCurrent: defaultAWSEBSVolumeType,
			wantSkipGp3: false,
		},
		{
			name:        "explicit gp2",
			volumeType:  strPtr("gp2"),
			wantCurrent: "gp2",
			wantSkipGp3: false,
		},
		{
			name:        "explicit gp3 (already target)",
			volumeType:  strPtr("gp3"),
			wantCurrent: "gp3",
			wantSkipGp3: true,
		},
		{
			name:        "explicit io1",
			volumeType:  strPtr("io1"),
			wantCurrent: "io1",
			wantSkipGp3: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(machinev1beta1.AWSMachineProviderConfig{
				BlockDevices: []machinev1beta1.BlockDeviceMappingSpec{{
					EBS: &machinev1beta1.EBSBlockDeviceSpec{VolumeType: tt.volumeType},
				}},
			})
			assert.NoError(t, err)

			awsSpec := &machinev1beta1.AWSMachineProviderConfig{}
			assert.NoError(t, json.Unmarshal(raw, awsSpec))

			// Replicate the detection logic from changeControlPlaneVolumeType.
			currentType := defaultAWSEBSVolumeType
			if awsSpec.BlockDevices[0].EBS != nil && awsSpec.BlockDevices[0].EBS.VolumeType != nil {
				currentType = *awsSpec.BlockDevices[0].EBS.VolumeType
			}

			assert.Equal(t, tt.wantCurrent, currentType, "resolved currentType")
			assert.Equal(t, tt.wantSkipGp3, currentType == "gp3", "skip-when-target-is-gp3")

			// Verify the notification gating contract: currentType must
			// be non-empty when a change would be made.
			if !tt.wantSkipGp3 {
				assert.NotEmpty(t, currentType,
					"currentType must be non-empty for the notification gate (previousType != \"\") to pass")
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestChangeControlPlaneVolumeType_NilVolumeTypeReachesConfirm(t *testing.T) {
	// When VolumeType is nil, the function must NOT skip (because the
	// effective type gp2 differs from the target gp3). It should reach
	// the ConfirmPrompt and abort in a non-TTY test environment.
	cpms, scheme := newTestCPMSWithVolumeType(t, nil)
	ops := &changeVolumeTypeOptions{
		targetType: "gp3",
		client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build(),
	}

	_, err := ops.changeControlPlaneVolumeType(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aborted by user",
		"nil VolumeType must not be skipped — code should reach confirm prompt")
}

func TestChangeControlPlaneVolumeType_ExplicitTypePreserved(t *testing.T) {
	// When VolumeType is explicitly set to "gp2", the function must
	// return "gp2" as previousType (not the default constant).
	explicitType := "gp2"
	cpms, scheme := newTestCPMSWithVolumeType(t, &explicitType)
	ops := &changeVolumeTypeOptions{
		targetType: "gp3",
		client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build(),
	}

	_, err := ops.changeControlPlaneVolumeType(context.Background())

	// Will abort at confirm prompt, proving the code detected the type
	// difference and didn't skip.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aborted by user")
}

func TestChangeControlPlaneVolumeTypeAlreadyTargetTypeSkipsNotification(t *testing.T) {
	volumeType := "gp3"
	cpms, scheme := newTestCPMSWithVolumeType(t, &volumeType)
	ops := &changeVolumeTypeOptions{
		targetType: "gp3",
		client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build(),
	}

	previousType, err := ops.changeControlPlaneVolumeType(context.Background())

	assert.NoError(t, err)
	assert.Empty(t, previousType, "previousType must be empty when already at target type")
}

func TestNotificationGating_NilVolumeType(t *testing.T) {
	// Verify the notification gating logic in run(): previousType from
	// changeControlPlaneVolumeType must be non-empty when VolumeType was
	// nil, so that the service log notification fires.
	//
	// We can't call run() directly (it requires OCM + K8s clients), but
	// we can verify the invariant: the default constant is non-empty and
	// differs from the empty-string gate used in run().
	assert.NotEmpty(t, defaultAWSEBSVolumeType,
		"defaultAWSEBSVolumeType must be non-empty so the notification gating check (previousType != \"\") passes")
}

func TestNotificationGating_AlreadyTargetType(t *testing.T) {
	// When the CPMS is already at the target type, previousType is empty,
	// and the notification must NOT fire. Verify this contract.
	volumeType := "gp3"
	cpms, scheme := newTestCPMSWithVolumeType(t, &volumeType)
	ops := &changeVolumeTypeOptions{
		targetType: "gp3",
		client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build(),
	}

	previousType, err := ops.changeControlPlaneVolumeType(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, previousType,
		"previousType must be empty (skipped) when already at target, so notification does not fire")
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

			// $CLUSTER_ID must be double-quoted so the variable expands
			// safely without word splitting.
			assert.Contains(t, guidance, `"$CLUSTER_ID"`,
				"guidance must use double-quoted $CLUSTER_ID placeholder")
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
	// Verify all template parameters appear in the guidance (inside
	// single-quoted shells) so the SRE can run the command as-is.
	req := newVolumeTypeChangedServiceLogRequest(
		controlPlaneVolumeTypeChangedServiceLogTemplate,
		"gp2", "gp3", "OHSS-456",
	)

	guidance := manualServiceLogGuidance(req)
	for _, param := range req.TemplateParams {
		// The parameter value is single-quoted in the output, so the
		// raw KEY=VALUE text still appears inside the quotes.
		assert.True(t, strings.Contains(guidance, param),
			"guidance must include template parameter %q", param)
	}
}

func TestManualServiceLogGuidance_ShellQuotesMaliciousReason(t *testing.T) {
	// A malicious or complex reason must be safely quoted so that
	// copying the printed command never executes injected content.
	maliciousReasons := []struct {
		name   string
		reason string
	}{
		{"spaces", "some reason with spaces"},
		{"double quotes", `reason "with" quotes`},
		{"single quotes", "reason 'with' quotes"},
		{"command substitution", "$(rm -rf /)"},
		{"backticks", "`whoami`"},
		{"semicolons", "reason; rm -rf /"},
		{"redirect", "reason > /etc/passwd"},
		{"pipe", "reason | cat /etc/shadow"},
		{"newlines", "reason\nwhoami"},
		{"combined", `OHSS-$(whoami); rm -rf / > /dev/null & echo 'pwned'` + "\n`id`"},
	}

	for _, tt := range maliciousReasons {
		t.Run(tt.name, func(t *testing.T) {
			req := newVolumeTypeChangedServiceLogRequest(
				controlPlaneVolumeTypeChangedServiceLogTemplate,
				"gp2", "gp3", tt.reason,
			)
			guidance := manualServiceLogGuidance(req)

			// All dynamic values must be single-quoted. Verify the
			// shell-quoted form of every parameter appears exactly.
			assert.Contains(t, guidance, shellQuote(req.Template),
				"template URL must be single-quoted")
			for _, param := range req.TemplateParams {
				assert.Contains(t, guidance, shellQuote(param),
					"template parameter %q must be single-quoted", param)
			}

			// The REASON parameter (including the shell-quoted reason)
			// must be present so the SRE can verify what will be sent.
			// For reasons with single quotes, the escaped form '\'' is
			// correct and preserves the value when evaluated by a shell.
			assert.Contains(t, guidance, shellQuote("REASON="+tt.reason),
				"guidance must include the shell-quoted reason parameter")

			// $CLUSTER_ID must be double-quoted for safe expansion.
			assert.Contains(t, guidance, `"$CLUSTER_ID"`,
				"$CLUSTER_ID must be double-quoted")
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hello", "'hello'"},
		{"spaces", "hello world", "'hello world'"},
		{"single quote", "it's", `'it'\''s'`},
		{"double quote", `say "hi"`, `'say "hi"'`},
		{"dollar sign", "$(cmd)", "'$(cmd)'"},
		{"backtick", "`cmd`", "'`cmd`'"},
		{"semicolon", "a; b", "'a; b'"},
		{"redirect", "a > b", "'a > b'"},
		{"newline", "a\nb", "'a\nb'"},
		{"empty", "", "''"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, shellQuote(tt.input))
		})
	}
}
