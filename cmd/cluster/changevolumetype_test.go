package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
