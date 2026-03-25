package resize

import (
	"strings"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/openshift/osdctl/pkg/utils"
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
			r := &Infra{
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

func TestValidateInstanceSize(t *testing.T) {
	tests := []struct {
		instanceSize string
		nodeType     string
		expectErr    bool
	}{
		{
			instanceSize: "r5.2xlarge",
			nodeType:     "infra",
			expectErr:    false,
		},
		{
			instanceSize: "m5.4xlarge",
			nodeType:     "infra",
			expectErr:    true,
		},
		{
			instanceSize: "r5.4xlarge",
			nodeType:     "infra",
			expectErr:    false,
		},
		{
			instanceSize: "m5.2xlarge",
			nodeType:     "controlplane",
			expectErr:    false,
		},
		{
			instanceSize: "r5.4xlarge",
			nodeType:     "controlplane",
			expectErr:    true,
		},
		{
			instanceSize: "m5.4xlarge",
			nodeType:     "controlplane",
			expectErr:    false,
		},
		{
			instanceSize: "m6i.4xlarge",
			nodeType:     "controlplane",
			expectErr:    false,
		},
		{
			instanceSize: "m6i.8xlarge",
			nodeType:     "controlplane",
			expectErr:    false,
		},
		{
			instanceSize: "m6i.4xlarge",
			nodeType:     "infra",
			expectErr:    true,
		},
		{
			instanceSize: "r6i.4xlarge",
			nodeType:     "infra",
			expectErr:    false,
		},
		{
			instanceSize: "r6i.8xlarge",
			nodeType:     "infra",
			expectErr:    false,
		},
		{
			instanceSize: "r6i.4xlarge",
			nodeType:     "controlplane",
			expectErr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.instanceSize, func(t *testing.T) {
			actual := validateInstanceSize(test.instanceSize, test.nodeType)
			if actual != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %v", actual)
				}
			} else {
				if test.expectErr {
					t.Error("expected err, got nil")
				}
			}
		})
	}
}

func TestConvertProviderIDtoInstanceID(t *testing.T) {
	tests := []struct {
		providerID string
		expected   string
	}{
		{
			providerID: "aws:///us-east-1a/i-0a1b2c3d4e5f6g7h8",
			expected:   "i-0a1b2c3d4e5f6g7h8",
		},
		{
			providerID: "gce://some-string/europe-west4-a/my-cluster-name-n65hp-infra-a-4fbrd",
			expected:   "my-cluster-name-n65hp-infra-a-4fbrd",
		},
	}

	for _, test := range tests {
		t.Run(test.providerID, func(t *testing.T) {
			actual := convertProviderIDtoInstanceID(test.providerID)
			if test.expected != actual {
				t.Errorf("expected: %s, got %s", test.expected, actual)
			}
		})
	}
}

// TestHiveOcmUrlValidation tests the early validation of --hive-ocm-url flag in the infra resize command
func TestHiveOcmUrlValidation(t *testing.T) {
	tests := []struct {
		name        string
		hiveOcmUrl  string
		expectErr   bool
		errContains string
	}{
		{
			name:       "Valid hive-ocm-url (production)",
			hiveOcmUrl: "production",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (staging)",
			hiveOcmUrl: "staging",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (integration)",
			hiveOcmUrl: "integration",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (full URL)",
			hiveOcmUrl: "https://api.openshift.com",
			expectErr:  false,
		},
		{
			name:        "Invalid hive-ocm-url",
			hiveOcmUrl:  "invalid-environment",
			expectErr:   true,
			errContains: "invalid OCM_URL",
		},
		{
			name:       "Empty hive-ocm-url (flag omitted)",
			hiveOcmUrl: "",
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate Infra.New() behavior: validate only when value is provided
			var err error
			if tt.hiveOcmUrl != "" {
				_, err = utils.ValidateAndResolveOcmUrl(tt.hiveOcmUrl)
			}

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got nil", tt.errContains)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}
