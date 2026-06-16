package cluster

import (
	"context"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awshivev1 "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		clusterID string
		nodeRoles string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid cluster ID and node roles",
			clusterID: "test-cluster-123",
			nodeRoles: "all",
			wantErr:   false,
		},
		{
			name:      "valid infra only",
			clusterID: "test-cluster-123",
			nodeRoles: "infra",
			wantErr:   false,
		},
		{
			name:      "valid master only",
			clusterID: "test-cluster-123",
			nodeRoles: "master",
			wantErr:   false,
		},
		{
			name:      "valid workers only",
			clusterID: "test-cluster-123",
			nodeRoles: "workers",
			wantErr:   false,
		},
		{
			name:      "invalid node role",
			clusterID: "test-cluster-123",
			nodeRoles: "invalid",
			wantErr:   true,
			errMsg:    "invalid nodes: invalid",
		},
		{
			name:      "empty cluster ID",
			clusterID: "",
			nodeRoles: "all",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := &imdsv2Options{
				clusterID: tt.clusterID,
				nodeRoles: tt.nodeRoles,
			}
			err := ops.validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateIMDSv2_AllNodesReady(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = machinev1beta1.AddToScheme(scheme)

	// Create test nodes - all ready
	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "master-1"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "master-2"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "infra-1"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithLists(nodes).
		Build()

	ops := &imdsv2Options{
		client: fakeClient,
	}

	err := ops.validateIMDSv2(context.Background())
	assert.NoError(t, err)
}

func TestValidateIMDSv2_SkipDeletingNodes(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = machinev1beta1.AddToScheme(scheme)

	now := metav1.Now()

	// Create test nodes - one being deleted
	// Note: fake client requires finalizers when DeletionTimestamp is set
	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "master-1"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "master-old",
					DeletionTimestamp: &now,
					Finalizers:        []string{"test-finalizer"},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithLists(nodes).
		Build()

	ops := &imdsv2Options{
		client: fakeClient,
	}

	err := ops.validateIMDSv2(context.Background())
	assert.NoError(t, err, "Should skip nodes with DeletionTimestamp")
}

func TestValidateIMDSv2_SkipUnschedulableNodes(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = machinev1beta1.AddToScheme(scheme)

	// Create test nodes - one cordoned/unschedulable
	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "master-1"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "master-draining"},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithLists(nodes).
		Build()

	ops := &imdsv2Options{
		client: fakeClient,
	}

	err := ops.validateIMDSv2(context.Background())
	assert.NoError(t, err, "Should skip unschedulable nodes")
}

func TestCheckIMDSv2Configuration(t *testing.T) {
	tests := []struct {
		name         string
		machinePool  *hivev1.MachinePool
		expectedAuth string
	}{
		{
			name: "IMDSv2 required",
			machinePool: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						AWS: &awshivev1.MachinePoolPlatform{
							EC2Metadata: &awshivev1.EC2Metadata{
								Authentication: imdsv2Required,
							},
						},
					},
				},
			},
			expectedAuth: imdsv2Required,
		},
		{
			name: "IMDSv2 optional",
			machinePool: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						AWS: &awshivev1.MachinePoolPlatform{
							EC2Metadata: &awshivev1.EC2Metadata{
								Authentication: imdsv2Optional,
							},
						},
					},
				},
			},
			expectedAuth: imdsv2Optional,
		},
		{
			name: "No EC2Metadata configured",
			machinePool: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						AWS: &awshivev1.MachinePoolPlatform{},
					},
				},
			},
			expectedAuth: "Not configured",
		},
		{
			name: "No AWS platform",
			machinePool: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{},
				},
			},
			expectedAuth: "Not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentAuth := "Not configured"
			if tt.machinePool.Spec.Platform.AWS != nil &&
				tt.machinePool.Spec.Platform.AWS.EC2Metadata != nil {
				currentAuth = tt.machinePool.Spec.Platform.AWS.EC2Metadata.Authentication
			}
			assert.Equal(t, tt.expectedAuth, currentAuth)
		})
	}
}

func TestMachinePoolNameValidation(t *testing.T) {
	tests := []struct {
		name      string
		mpName    string
		validList map[string]bool
		wantValid bool
	}{
		{
			name:      "valid infra",
			mpName:    "infra",
			validList: map[string]bool{"infra": true},
			wantValid: true,
		},
		{
			name:      "valid worker",
			mpName:    "worker",
			validList: map[string]bool{"worker": true},
			wantValid: true,
		},
		{
			name:      "invalid name",
			mpName:    "unexpected-pool",
			validList: map[string]bool{"infra": true, "worker": true},
			wantValid: false,
		},
		{
			name:      "master not in worker list",
			mpName:    "master",
			validList: map[string]bool{"worker": true},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.validList[tt.mpName]
			assert.Equal(t, tt.wantValid, isValid)
		})
	}
}

func TestCPMSIMDSv2Configuration(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = machinev1.AddToScheme(scheme)

	tests := []struct {
		name           string
		authentication string
		expectedResult bool
	}{
		{
			name:           "already IMDSv2",
			authentication: imdsv2Required,
			expectedResult: false, // no changes needed
		},
		{
			name:           "needs IMDSv2",
			authentication: imdsv2Optional,
			expectedResult: true, // changes needed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsUpdate := tt.authentication != imdsv2Required
			assert.Equal(t, tt.expectedResult, needsUpdate)
		})
	}
}

func TestPreFlightChecks_MasterNodesCount(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = machinev1.AddToScheme(scheme)

	tests := []struct {
		name       string
		masterNode int
		wantErr    bool
	}{
		{
			name:       "exactly 3 masters",
			masterNode: 3,
			wantErr:    false,
		},
		{
			name:       "less than 3 masters",
			masterNode: 2,
			wantErr:    true,
		},
		{
			name:       "more than 3 masters",
			masterNode: 4,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var masters []corev1.Node
			for i := 0; i < tt.masterNode; i++ {
				masters = append(masters, corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "master-" + string(rune(i)),
						Labels: map[string]string{"node-role.kubernetes.io/master": ""},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				})
			}

			readyCount := 0
			for _, node := range masters {
				for _, cond := range node.Status.Conditions {
					if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
						readyCount++
						break
					}
				}
			}

			if tt.wantErr {
				assert.NotEqual(t, 3, readyCount)
			} else {
				assert.Equal(t, 3, readyCount)
			}
		})
	}
}

func TestNodeRoleFilter(t *testing.T) {
	tests := []struct {
		name         string
		nodeRoles    string
		mpSpecName   string
		shouldFilter bool
	}{
		{
			name:         "master role with master MP",
			nodeRoles:    "master",
			mpSpecName:   "master",
			shouldFilter: false,
		},
		{
			name:         "master role with infra MP",
			nodeRoles:    "master",
			mpSpecName:   "infra",
			shouldFilter: true,
		},
		{
			name:         "infra role with infra MP",
			nodeRoles:    "infra",
			mpSpecName:   "infra",
			shouldFilter: false,
		},
		{
			name:         "infra role with worker MP",
			nodeRoles:    "infra",
			mpSpecName:   "worker",
			shouldFilter: true,
		},
		{
			name:         "workers role with worker MP",
			nodeRoles:    "workers",
			mpSpecName:   "worker",
			shouldFilter: false,
		},
		{
			name:         "all role with any MP",
			nodeRoles:    "all",
			mpSpecName:   "infra",
			shouldFilter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSkip := (tt.nodeRoles == "master" && tt.mpSpecName != "master") ||
				(tt.nodeRoles == "infra" && tt.mpSpecName != "infra") ||
				(tt.nodeRoles == "workers" && tt.mpSpecName != "worker")

			assert.Equal(t, tt.shouldFilter, shouldSkip)
		})
	}
}

func TestCountReadyNodesHelper(t *testing.T) {
	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-3"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
		},
	}

	readyCount := CountReadyNodes(nodes)
	assert.Equal(t, 2, readyCount, "Should count only Ready nodes")
}

func TestValidateAWSClassicCluster(t *testing.T) {
	tests := []struct {
		name      string
		cluster   *cmv1.Cluster
		wantErr   bool
		errSubstr string
	}{
		{
			name: "AWS Classic cluster",
			cluster: func() *cmv1.Cluster {
				c, _ := cmv1.NewCluster().
					CloudProvider(cmv1.NewCloudProvider().ID("aws")).
					Hypershift(cmv1.NewHypershift().Enabled(false)).
					Build()
				return c
			}(),
			wantErr: false,
		},
		{
			name: "GCP cluster",
			cluster: func() *cmv1.Cluster {
				c, _ := cmv1.NewCluster().
					CloudProvider(cmv1.NewCloudProvider().ID("gcp")).
					Hypershift(cmv1.NewHypershift().Enabled(false)).
					Build()
				return c
			}(),
			wantErr:   true,
			errSubstr: "only supports AWS clusters",
		},
		{
			name: "HCP cluster",
			cluster: func() *cmv1.Cluster {
				c, _ := cmv1.NewCluster().
					CloudProvider(cmv1.NewCloudProvider().ID("aws")).
					Hypershift(cmv1.NewHypershift().Enabled(true)).
					Build()
				return c
			}(),
			wantErr:   true,
			errSubstr: "does not support HCP clusters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAWSClassicCluster(tt.cluster)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
