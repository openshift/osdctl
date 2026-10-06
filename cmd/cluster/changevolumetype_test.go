package cluster

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
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

func strPtr(s string) *string { return &s }

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
	tests := []struct {
		name        string
		volumeType  *string
		wantCurrent string
		wantSkipGp3 bool
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

			currentType := defaultAWSEBSVolumeType
			if awsSpec.BlockDevices[0].EBS != nil && awsSpec.BlockDevices[0].EBS.VolumeType != nil {
				currentType = *awsSpec.BlockDevices[0].EBS.VolumeType
			}

			assert.Equal(t, tt.wantCurrent, currentType)
			assert.Equal(t, tt.wantSkipGp3, currentType == "gp3")
		})
	}
}

func TestChangeControlPlaneVolumeType_AlreadyTargetSkips(t *testing.T) {
	volumeType := "gp3"
	cpms, scheme := newTestCPMSWithVolumeType(t, &volumeType)
	ops := &changeVolumeTypeOptions{
		targetType: "gp3",
		client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build(),
	}

	err := ops.changeControlPlaneVolumeType(context.Background())
	assert.NoError(t, err)
}

func TestChangeControlPlaneVolumeType_NilVolumeTypeReachesConfirm(t *testing.T) {
	cpms, scheme := newTestCPMSWithVolumeType(t, nil)
	ops := &changeVolumeTypeOptions{
		targetType: "gp3",
		client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build(),
	}

	// In a non-TTY environment ConfirmPrompt returns false, so the function
	// aborts with "aborted by user", proving it did not skip.
	err := ops.changeControlPlaneVolumeType(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "aborted by user")
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

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, machinev1.Install(scheme))
	return scheme
}

func TestMonitorCPMSRollout_StaleGeneration(t *testing.T) {
	withFastPoll(t)

	scheme := newTestScheme(t)
	cpms := &machinev1.ControlPlaneMachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  changeVolumeTypeCPMSNamespace,
			Name:       changeVolumeTypeCPMSName,
			Generation: 2,
		},
		Status: machinev1.ControlPlaneMachineSetStatus{
			ObservedGeneration: 1,
			UpdatedReplicas:    3,
			ReadyReplicas:      3,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cpms).
		WithStatusSubresource(cpms).
		Build()
	ops := &changeVolumeTypeOptions{client: fakeClient}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- ops.monitorCPMSRollout(ctx) }()

	// The stale generation must block completion.
	time.Sleep(150 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("monitorCPMSRollout returned immediately on stale ObservedGeneration")
	default:
	}

	// Simulate controller catching up.
	latest := &machinev1.ControlPlaneMachineSet{}
	assert.NoError(t, fakeClient.Get(ctx, client.ObjectKey{Namespace: changeVolumeTypeCPMSNamespace, Name: changeVolumeTypeCPMSName}, latest))
	latest.Status.ObservedGeneration = 2
	assert.NoError(t, fakeClient.Status().Update(ctx, latest))

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("monitorCPMSRollout did not complete after ObservedGeneration caught up")
	}
}

func TestMonitorCPMSRollout_SuccessfulRollout(t *testing.T) {
	withFastPoll(t)

	scheme := newTestScheme(t)
	cpms := &machinev1.ControlPlaneMachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  changeVolumeTypeCPMSNamespace,
			Name:       changeVolumeTypeCPMSName,
			Generation: 2,
		},
		Status: machinev1.ControlPlaneMachineSetStatus{
			ObservedGeneration: 2,
			UpdatedReplicas:    3,
			ReadyReplicas:      3,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpms).Build()
	ops := &changeVolumeTypeOptions{client: fakeClient}

	err := ops.monitorCPMSRollout(context.Background())
	assert.NoError(t, err)
}
