package network

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osd-network-verifier/pkg/data/cloud"
	"github.com/openshift/osd-network-verifier/pkg/output"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"
	"github.com/openshift/osdctl/cmd/servicelog"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"testing"
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

// Testing multiple subnets

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
