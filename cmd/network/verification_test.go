package network

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"

	"github.com/openshift/osd-network-verifier/pkg/data/cloud"
	"github.com/openshift/osd-network-verifier/pkg/data/cpu"
	"github.com/openshift/osd-network-verifier/pkg/output"
	"github.com/openshift/osd-network-verifier/pkg/probes/curl"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"

	"github.com/openshift/osdctl/cmd/servicelog"
)

func newTestLogger(t *testing.T) logging.Logger {
	builder := logging.NewGoLoggerBuilder()
	builder.Debug(true)
	logger, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}

	return logger
}

// newTestCluster assembles a *cmv1.Cluster while handling the error to help out with inline test-case generation
func newTestCluster(t *testing.T, cb *cmv1.ClusterBuilder) *cmv1.Cluster {
	cluster, err := cb.Build()
	if err != nil {
		t.Fatalf("failed to build cluster: %s", err)
	}

	return cluster
}

func TestNewCmdValidateEgress(t *testing.T) {
	cmd := NewCmdValidateEgress()
	assert.NotNil(t, cmd)
	assert.Equal(t, "verify-egress", cmd.Use)
}

func TestEgressVerification_ValidateInput(t *testing.T) {
	tests := []struct {
		name      string
		ev        *EgressVerification
		wantError bool
	}{
		{
			name: "valid single subnet",
			ev: &EgressVerification{
				SubnetIds: []string{"subnet-123"},
			},
			wantError: false,
		},
		{
			name: "invalid comma-separated subnets",
			ev: &EgressVerification{
				SubnetIds: []string{"subnet-123,subnet-456"},
			},
			wantError: true,
		},
		{
			name: "valid multiple subnet flags",
			ev: &EgressVerification{
				SubnetIds: []string{"subnet-123", "subnet-456"},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ev.validateInput()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEgressVerification_GetPlatform(t *testing.T) {
	tests := []struct {
		name          string
		ev            *EgressVerification
		cluster       *cmv1.Cluster
		wantPlatform  cloud.Platform
		wantError     bool
		setupMockFunc func() *cmv1.Cluster
	}{
		{
			name: "explicit platform aws-hcp",
			ev: &EgressVerification{
				platformName: "aws-hcp",
			},
			wantPlatform: cloud.AWSHCP,
			wantError:    false,
		},
		{
			name: "invalid platform",
			ev: &EgressVerification{
				platformName: "invalid-platform",
			},
			wantError: true,
		},
		{
			name: "hypershift enabled defaults to HCP",
			ev:   &EgressVerification{},
			setupMockFunc: func() *cmv1.Cluster {
				// Create a mock cluster with hypershift enabled
				cluster := cmv1.NewCluster()
				hypershift := cmv1.NewHypershift()
				hypershift.Enabled(true)
				cluster.Hypershift(hypershift)
				aws := cmv1.NewAWS()
				cluster.AWS(aws)
				cluster.CloudProvider(cmv1.NewCloudProvider().ID("aws"))
				built, err := cluster.Build()
				if err != nil {
					t.Fatal(err)
				}
				return built
			},
			wantPlatform: cloud.AWSHCP,
			wantError:    false,
		},
		{
			name: "classic cluster without hypershift",
			ev:   &EgressVerification{},
			setupMockFunc: func() *cmv1.Cluster {
				// Create a mock cluster without hypershift
				cluster := cmv1.NewCluster()
				aws := cmv1.NewAWS()
				cluster.AWS(aws)
				cluster.CloudProvider(cmv1.NewCloudProvider().ID("aws"))
				built, err := cluster.Build()
				if err != nil {
					t.Fatal(err)
				}
				return built
			},
			wantPlatform: cloud.AWSClassic,
			wantError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMockFunc != nil {
				tt.ev.cluster = tt.setupMockFunc()
			}

			platform, err := tt.ev.getPlatform()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPlatform, platform)
			}
		})
	}
}

func TestEgressVerification_DefaultValidateEgressInput(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		ev       *EgressVerification
		platform cloud.Platform
		want     *onv.ValidateEgressInput
	}{
		{
			name: "basic configuration",
			ev: &EgressVerification{
				cpuArch:       cpu.ArchX86,
				NoTls:         false,
				EgressTimeout: 5 * time.Second,
				Probe:         "curl",
			},
			platform: cloud.AWSClassic,
			want: &onv.ValidateEgressInput{
				Ctx:             ctx,
				CPUArchitecture: cpu.ArchX86,
				PlatformType:    cloud.AWSClassic,
				Probe:           curl.Probe{},
				Timeout:         5 * time.Second,
				Tags:            networkVerifierDefaultTags,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.ev.defaultValidateEgressInput(ctx, tt.platform)
			assert.NoError(t, err)
			assert.Equal(t, tt.want.CPUArchitecture, got.CPUArchitecture)
			assert.Equal(t, tt.want.PlatformType, got.PlatformType)
			assert.Equal(t, tt.want.Tags, got.Tags)
		})
	}
}

func TestGenerateServiceLog(t *testing.T) {
	testCases := []struct {
		name      string
		output    *output.Output
		clusterId string
		want      servicelog.PostCmdOptions
	}{
		{
			name: "with failures",
			output: func() *output.Output {
				o := &output.Output{}
				o.SetEgressFailures([]string{
					"https://test1.com",
					"https://test2.com",
				})
				return o
			}(),
			clusterId: "test-cluster",
			want: servicelog.PostCmdOptions{
				Template:       blockedEgressTemplateUrl,
				ClusterId:      "test-cluster",
				TemplateParams: []string{"URLS=https://test1.com,https://test2.com"},
			},
		},
		{
			name:      "no failures",
			output:    &output.Output{}, // Empty output for no failures
			clusterId: "test-cluster",
			want:      servicelog.PostCmdOptions{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := generateServiceLog(tc.output, tc.clusterId)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("generateServiceLog() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEgressVerification_GetCABundle(t *testing.T) {
	tests := []struct {
		name      string
		ev        *EgressVerification
		setupFunc func(*EgressVerification)
		want      string
		wantErr   bool
	}{
		{
			name: "no cluster",
			ev: &EgressVerification{
				cluster: nil,
			},
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc(tt.ev)
			}

			got, err := tt.ev.getCABundle(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFiltersToString(t *testing.T) {
	tests := []struct {
		name    string
		filters []types.Filter
		want    string
	}{
		{
			name: "single filter",
			filters: []types.Filter{
				{
					Name:   aws.String("tag:Name"),
					Values: []string{"test"},
				},
			},
			want: "[name: tag:Name, values: test]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filtersToString(tt.filters)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_egressVerificationGetPlatform(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		expected  cloud.Platform
		expectErr bool
	}{
		{
			name: "aws",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
				log:     newTestLogger(t),
			},
			expected:  cloud.AWSClassic,
			expectErr: false,
		},
		{
			name: "hostedcluster",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws")).Hypershift(cmv1.NewHypershift().Enabled(true))),
				log:     newTestLogger(t),
			},
			expected:  cloud.AWSHCP,
			expectErr: false,
		},
		{
			name: "override",
			e: &EgressVerification{
				cluster:      newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws")).Hypershift(cmv1.NewHypershift().Enabled(true))),
				platformName: "aws",
				log:          newTestLogger(t),
			},
			expected:  cloud.AWSClassic,
			expectErr: false,
		},
		{
			name: "gcp",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("gcp"))),
				log:     newTestLogger(t),
			},
			expected:  cloud.GCPClassic,
			expectErr: false,
		},
		{
			name: "invalid",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("foo"))),
				log:     newTestLogger(t),
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.e.getPlatform()
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %s", err)
				}
			} else {
				if test.expectErr {
					t.Errorf("expected err, got none")
				}
				if actual != test.expected {
					t.Errorf("expected platform %s, got %s", test.expected, actual)
				}
			}
		})
	}
}

func TestDefaultValidateEgressInput(t *testing.T) {
	tests := []struct {
		region    string
		expectErr bool
	}{
		{
			region:    "us-east-2",
			expectErr: false,
		},
		{
			region:    "eu-central-1",
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.region, func(t *testing.T) {
			cluster := newTestCluster(t, cmv1.NewCluster().AWS(cmv1.NewAWS()))
			e := &EgressVerification{
				cluster: cluster,
				log:     newTestLogger(t),
			}
			_, err := e.defaultValidateEgressInput(context.Background(), cloud.AWSClassic)
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %s", err)
				}
			} else {
				if test.expectErr {
					t.Errorf("expected err, got none")
				}
			}
		})
	}
}

// compareValidateEgressInput is a helper to compare selected fields of ValidateEgressInput that we care about during testing
func compareValidateEgressInput(expected, actual *onv.ValidateEgressInput) bool {
	if expected == nil && actual == nil {
		return true
	}

	if (expected == nil && actual != nil) || (expected != nil && actual == nil) {
		return false
	}

	if expected.SubnetID != actual.SubnetID ||
		!reflect.DeepEqual(expected.AWS.SecurityGroupIDs, actual.AWS.SecurityGroupIDs) {
		return false
	}

	if expected.AWS.KmsKeyID != actual.AWS.KmsKeyID {
		return false
	}

	if expected.Proxy.HttpProxy != actual.Proxy.HttpProxy ||
		expected.Proxy.HttpsProxy != actual.Proxy.HttpsProxy {
		return false
	}

	if expected.Timeout != actual.Timeout {
		return false
	}

	return true
}

func Test_egressVerificationGetSubnetIdAllSubnetsFlag(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		expected  []string
		expectErr bool
	}{
		{
			name: "all subnets flag is on",
			e: &EgressVerification{
				log:        newTestLogger(t),
				SubnetIds:  []string{"string-1", "string-2", "string-3"},
				AllSubnets: true,
			},
			expected:  []string{"string-1", "string-2", "string-3"},
			expectErr: false,
		},
		{
			name: "non-PrivateLink + BYOVPC unsupported, with --all-subnets flag",
			e: &EgressVerification{
				cluster:    newTestCluster(t, cmv1.NewCluster().AWS(cmv1.NewAWS().PrivateLink(false).SubnetIDs("subnet-1", "subnet-2", "subnet-3"))),
				log:        newTestLogger(t),
				AllSubnets: true,
			},
			expectErr: true,
		},
		{
			name: "PrivateLink + BYOVPC with --all-subnets flag",
			e: &EgressVerification{
				cluster:    newTestCluster(t, cmv1.NewCluster().AWS(cmv1.NewAWS().PrivateLink(true).SubnetIDs("subnet-abcd", "subnet-1", "subnet-2", "subnet-3"))),
				log:        newTestLogger(t),
				AllSubnets: true,
			},
			expected:  []string{"subnet-abcd", "subnet-1", "subnet-2", "subnet-3"},
			expectErr: false,
		},
		{
			name: "non-BYOVPC clusters get subnets from AWS",
			e: &EgressVerification{
				awsClient: mockEgressVerificationAWSClient{
					describeSubnetsResp: &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{
							{
								SubnetId: aws.String("subnet-abcd"),
							},
						},
					},
				},
				cluster:    newTestCluster(t, cmv1.NewCluster()),
				log:        newTestLogger(t),
				AllSubnets: true,
			},
			expected:  []string{"subnet-abcd"},
			expectErr: false,
		},
		{
			name: "non-BYOVPC clusters get subnets from AWS, all-subnets flag enabled",
			e: &EgressVerification{
				awsClient: mockEgressVerificationAWSClient{
					describeSubnetsResp: &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{
							{
								SubnetId: aws.String("subnet-abcd"),
							},
							{
								SubnetId: aws.String("subnet-1234"),
							},
							{
								SubnetId: aws.String("subnet-1267"),
							},
						},
					},
				},
				cluster:    newTestCluster(t, cmv1.NewCluster()),
				log:        newTestLogger(t),
				AllSubnets: true,
			},
			expected:  []string{"subnet-abcd", "subnet-1234", "subnet-1267"},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.e.getAwsSubnetIds(context.TODO())
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %s", err)
				}
			} else {
				for i := range actual {

					if test.expectErr {
						t.Errorf("expected err, got none")
					}
					if actual[i] != test.expected[i] {
						t.Errorf("expected subnet-id %s, got %s", test.expected, actual[i])
					}
				}
			}
		})
	}
}

func Test_generateServiceLog(t *testing.T) {
	testClusterId := "abc123"

	tests := []struct {
		name       string
		egressUrls []string
		want       servicelog.PostCmdOptions
	}{
		{
			name:       "no egress failures",
			egressUrls: nil,
		},
		{
			name:       "one egress failure",
			egressUrls: []string{"storage.googleapis.com:443"},
			want: servicelog.PostCmdOptions{
				Template:       blockedEgressTemplateUrl,
				TemplateParams: []string{"URLS=storage.googleapis.com:443"},
				ClusterId:      testClusterId,
			},
		},
		{
			name: "multiple egress failures",
			egressUrls: []string{
				"storage.googleapis.com:443",
				"console.redhat.com:443",
				"s3.amazonaws.com:443",
			},
			want: servicelog.PostCmdOptions{
				Template:       blockedEgressTemplateUrl,
				TemplateParams: []string{"URLS=storage.googleapis.com:443,console.redhat.com:443,s3.amazonaws.com:443"},
				ClusterId:      testClusterId,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := new(output.Output)
			out.SetEgressFailures(test.egressUrls)
			if got := generateServiceLog(out, testClusterId); !reflect.DeepEqual(got, test.want) {
				t.Errorf("generateServiceLog() = %v, want %v", got, test.want)
			}
		})
	}
}

const (
	rawCaBundleConfigMapTemplate string = `{
	"apiVersion": "v1",
	"kind": "ConfigMap",
	"metadata": {
		"name": "user-ca-bundle",
		"namespace": "openshift-config"
	},
	"data": {
		"%s": "%s"
	}
}`
)

func TestGetCaBundleFromSyncSet(t *testing.T) {
	tests := []struct {
		name      string
		ss        *hivev1.SyncSet
		expected  string
		expectErr bool
	}{
		{
			name: "valid",
			ss: &hivev1.SyncSet{
				Spec: hivev1.SyncSetSpec{
					SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
						Resources: []runtime.RawExtension{
							{Raw: []byte(fmt.Sprintf(rawCaBundleConfigMapTemplate, caBundleConfigMapKey, "myCABundle"))},
						},
					},
				},
			},
			expected:  "myCABundle",
			expectErr: false,
		},
		{
			name: "invalid",
			ss: &hivev1.SyncSet{
				Spec: hivev1.SyncSetSpec{
					SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
						Resources: []runtime.RawExtension{
							{Raw: []byte(fmt.Sprintf(rawCaBundleConfigMapTemplate, "somethingElse", "myCABundle"))},
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := getCaBundleFromSyncSet(test.ss)
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %v", err)
				}
			} else {
				if test.expectErr {
					t.Error("expected error, got nil")
				}

				if test.expected != actual {
					t.Errorf("expected: %s, got: %s", test.expected, actual)
				}
			}
		})
	}
}

func Test_egressVerification_setupForAwsVerification(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		expectErr bool
	}{
		{
			name:      "no ClusterId requires subnet/sg",
			e:         &EgressVerification{},
			expectErr: true,
		},
		{
			name:      "no ClusterId with subnet",
			e:         &EgressVerification{SubnetIds: []string{"subnet-a"}},
			expectErr: true,
		},
		{
			name:      "no ClusterId with security group",
			e:         &EgressVerification{SecurityGroupId: "sg-a"},
			expectErr: true,
		},
		{
			name:      "no ClusterId with platform type",
			e:         &EgressVerification{platformName: "aws"},
			expectErr: true,
		},
		{
			name: "no ClusterId with subnet and security group",
			e: &EgressVerification{
				SubnetIds:       []string{"subnet-a", "subnet-b", "subnet-c"},
				SecurityGroupId: "sg-b",
			},
			expectErr: true,
		},
		{
			name: "no ClusterId with subnet and platform type",
			e: &EgressVerification{
				SubnetIds:    []string{"subnet-a", "subnet-b", "subnet-c"},
				platformName: "aws",
			},
			expectErr: true,
		},
		{
			name: "no ClusterId with security group and platform type",
			e: &EgressVerification{
				SecurityGroupId: "sg-b",
				platformName:    "aws",
			},
			expectErr: true,
		},
		{
			name: "ClusterId optional",
			e: &EgressVerification{
				SubnetIds:       []string{"subnet-a", "subnet-b", "subnet-c"},
				SecurityGroupId: "sg-b",
				platformName:    "aws",
				log:             newTestLogger(t),
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			err := test.e.fetchCluster(ctx)
			if err != nil {
				t.Errorf("expected no err, got %s", err)
			}
		})
	}
}
