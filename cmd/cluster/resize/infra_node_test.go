package resize

import (
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
)

// newTestCluster assembles a *cmv1.Cluster while handling the error to help out with inline test-case generation
func newTestCluster(t *testing.T, cb *cmv1.ClusterBuilder) *cmv1.Cluster {
	cluster, err := cb.Build()
	if err != nil {
		t.Fatalf("failed to build cluster: %s", err)
	}

	return cluster
}

func TestResize_embiggenMachinePool(t *testing.T) {
	tests := []struct {
		name      string
		cluster   *cmv1.Cluster
		mp        *hivev1.MachinePool
		override  string
		expected  string
		expectErr bool
	}{
		{
			name:    "AWS r5.xlarge --> r5.2xlarge",
			cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
			mp: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1aws.MachinePoolPlatform{
							InstanceType: "r5.xlarge",
						},
					},
				},
			},
			expected:  "r5.2xlarge",
			expectErr: false,
		},
		{
			name:    "GCP custom-4-32768-ext --> custom-8-65536-ext",
			cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("gcp"))),
			mp: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						GCP: &hivev1gcp.MachinePool{
							InstanceType: "custom-4-32768-ext",
						},
					},
				},
			},
			expected:  "custom-8-65536-ext",
			expectErr: false,
		},
		{
			name:    "AWS r5.2xlarge --> r5.xlarge with override",
			cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
			mp: &hivev1.MachinePool{
				Spec: hivev1.MachinePoolSpec{
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1aws.MachinePoolPlatform{
							InstanceType: "r5.2xlarge",
						},
					},
				},
			},
			override:  "r5.xlarge",
			expected:  "r5.xlarge",
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := &Resize{
				cluster:      test.cluster,
				instanceType: test.override,
			}
			actual, err := r.embiggenMachinePool(test.mp)
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %v", err)
				}
			} else {
				if test.expectErr {
					t.Error("expected err, got nil")
				}

				actualInstanceType, err := getInstanceType(actual)
				if err != nil {
					t.Error(err)
				}

				if test.expected != actualInstanceType {
					t.Errorf("expected: %s, got %s", test.expected, actualInstanceType)
				}
			}
		})
	}
}
