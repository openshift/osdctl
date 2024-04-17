package network

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift-online/ocm-sdk-go/logging"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osd-network-verifier/pkg/output"
	"github.com/openshift/osd-network-verifier/pkg/proxy"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"
	"github.com/openshift/osdctl/cmd/servicelog"
	"k8s.io/apimachinery/pkg/runtime"
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

type mockEgressVerificationAWSClient struct {
	describeSecurityGroupsResp *ec2.DescribeSecurityGroupsOutput
	describeSubnetsResp        *ec2.DescribeSubnetsOutput
}

func (m mockEgressVerificationAWSClient) DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return m.describeSubnetsResp, nil
}

func (m mockEgressVerificationAWSClient) DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	return m.describeSecurityGroupsResp, nil
}

func Test_egressVerificationSetup(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		expectErr bool
	}{
		{
			name: "no ClusterId requires subnet/sg",
			e: &EgressVerification{
				ClusterId: "",
			},
			expectErr: true,
		},
		{
			name: "ClusterId optional",
			e: &EgressVerification{
				ClusterId:       "",
				SubnetIds:       []string{"subnet-a", "subnet-b", "subnet-c"},
				SecurityGroupId: "sg-b",
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.e.setup(context.TODO())
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

func Test_egressVerificationGenerateAWSValidateEgressInput(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		region    string
		expected  *onv.ValidateEgressInput
		expectErr bool
	}{
		{
			name: "GCP Unsupported",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("gcp"))),
				log:     newTestLogger(t),
			},
			expectErr: true,
		},
		{
			name: "Cluster-wide proxy requires cacert when there is an additional trust bundle",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().
					CloudProvider(cmv1.NewCloudProvider().ID("aws")).
					Product(cmv1.NewProduct().ID("rosa")).
					AdditionalTrustBundle("REDACTED").
					Proxy(cmv1.NewProxy().HTTPProxy("http://my.proxy:80").HTTPSProxy("https://my.proxy:443")),
				),
				log: newTestLogger(t),
			},
			region:    "us-east-2",
			expectErr: true,
		},
		{
			name: "Transparent cluster-wide proxy",
			e: &EgressVerification{
				awsClient: mockEgressVerificationAWSClient{
					describeSecurityGroupsResp: &ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []types.SecurityGroup{
							{
								GroupId: aws.String("sg-abcd"),
							},
						},
					},
					describeSubnetsResp: &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{
							{
								SubnetId: aws.String("subnet-abcd"),
							},
						},
					},
				},
				cluster: newTestCluster(t, cmv1.NewCluster().
					CloudProvider(cmv1.NewCloudProvider().ID("aws")).
					Product(cmv1.NewProduct().ID("rosa")).
					Proxy(cmv1.NewProxy().HTTPProxy("http://my.proxy:80").HTTPSProxy("https://my.proxy:443")),
				),
				log: newTestLogger(t),
			},
			region: "us-east-2",
			expected: &onv.ValidateEgressInput{
				SubnetID: "subnet-abcd",
				Proxy: proxy.ProxyConfig{
					HttpProxy:  "http://my.proxy:80",
					HttpsProxy: "https://my.proxy:443",
				},
				AWS: onv.AwsEgressConfig{
					SecurityGroupId: "sg-abcd",
				},
			},
			expectErr: false,
		},
		{
			name: "cluster specific KMS key forward",
			e: &EgressVerification{
				awsClient: mockEgressVerificationAWSClient{
					describeSecurityGroupsResp: &ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []types.SecurityGroup{
							{
								GroupId: aws.String("sg-abcd"),
							},
						},
					},
					describeSubnetsResp: &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{
							{
								SubnetId: aws.String("subnet-abcd"),
							},
						},
					},
				},
				cluster: newTestCluster(t, cmv1.NewCluster().
					CloudProvider(cmv1.NewCloudProvider().ID("aws")).
					Product(cmv1.NewProduct().ID("rosa")).
					AWS(cmv1.NewAWS().KMSKeyArn("some-KMS-key-ARN")),
				),
				log: newTestLogger(t),
			},
			region: "us-east-2",
			expected: &onv.ValidateEgressInput{
				SubnetID: "subnet-abcd",
				AWS: onv.AwsEgressConfig{
					SecurityGroupId: "sg-abcd",
					KmsKeyID:        "some-KMS-key-ARN",
				},
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.e.generateAWSValidateEgressInput(context.TODO(), test.region)
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %s", err)
				}
			} else {
				if test.expectErr {
					t.Errorf("expected err, got none")
				}
				for i := range actual {
					if !compareValidateEgressInput(test.expected, actual[i]) {
						t.Errorf("expected %v, got %v", test.expected, actual[i])
					}
				}
			}
		})
	}
}

func Test_egressVerificationGetPlatformType(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		expected  string
		expectErr bool
	}{
		{
			name: "aws",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
				log:     newTestLogger(t),
			},
			expected:  "aws",
			expectErr: false,
		},
		{
			name: "hostedcluster",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws")).Hypershift(cmv1.NewHypershift().Enabled(true))),
				log:     newTestLogger(t),
			},
			expected:  "hostedcluster",
			expectErr: false,
		},
		{
			name: "override",
			e: &EgressVerification{
				cluster:      newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws")).Hypershift(cmv1.NewHypershift().Enabled(true))),
				PlatformType: "aws",
				log:          newTestLogger(t),
			},
			expected:  "aws",
			expectErr: false,
		},
		{
			name: "invalid",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("gcp"))),
				log:     newTestLogger(t),
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.e.getPlatformType()
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

func Test_egressVerificationGetSecurityGroupId(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		expected  string
		expectErr bool
	}{
		{
			name: "manual override",
			e: &EgressVerification{
				awsClient: mockEgressVerificationAWSClient{
					describeSecurityGroupsResp: &ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []types.SecurityGroup{
							{
								GroupId: aws.String("sg-abcd"),
							},
						},
					},
				},
				log:             newTestLogger(t),
				SecurityGroupId: "override",
			},
			expected:  "override",
			expectErr: false,
		},
		{
			name: "zero from AWS",
			e: &EgressVerification{
				awsClient: mockEgressVerificationAWSClient{
					describeSecurityGroupsResp: &ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []types.SecurityGroup{},
					},
				},
				log: newTestLogger(t),
			},
			expectErr: true,
		},
		{
			name: "one from AWS",
			e: &EgressVerification{
				awsClient: mockEgressVerificationAWSClient{
					describeSecurityGroupsResp: &ec2.DescribeSecurityGroupsOutput{
						SecurityGroups: []types.SecurityGroup{
							{
								GroupId: aws.String("sg-abcd"),
							},
						},
					},
				},
				log: newTestLogger(t),
			},
			expected:  "sg-abcd",
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.e.getSecurityGroupId(context.TODO())
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %s", err)
				}
			} else {
				if test.expectErr {
					t.Errorf("expected err, got none")
				}
				if actual != test.expected {
					t.Errorf("expected sg-id %s, got %s", test.expected, actual)
				}
			}
		})
	}
}

func Test_egressVerificationGetSubnetId(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		expected  string
		expectErr bool
	}{
		{
			name: "manual override",
			e: &EgressVerification{
				log:       newTestLogger(t),
				SubnetIds: []string{"override"},
			},
			expected:  "override",
			expectErr: false,
		},
		{
			name: "non-PrivateLink + BYOVPC unsupported",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().AWS(cmv1.NewAWS().PrivateLink(false).SubnetIDs("subnet-abcd"))),
				log:     newTestLogger(t),
			},
			expectErr: true,
		},
		{
			name: "PrivateLink + BYOVPC picks the first subnet",
			e: &EgressVerification{
				cluster: newTestCluster(t, cmv1.NewCluster().AWS(cmv1.NewAWS().PrivateLink(true).SubnetIDs("subnet-abcd"))),
				log:     newTestLogger(t),
			},
			expected:  "subnet-abcd",
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
				cluster: newTestCluster(t, cmv1.NewCluster()),
				log:     newTestLogger(t),
			},
			expected:  "subnet-abcd",
			expectErr: false,
		},
		{
			name: "non-BYOVPC clusters error if no subnets found in AWS",
			e: &EgressVerification{
				awsClient: mockEgressVerificationAWSClient{
					describeSubnetsResp: &ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{},
					},
				},
				cluster: newTestCluster(t, cmv1.NewCluster()),
				log:     newTestLogger(t),
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.e.getSubnetIds(context.TODO())
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %s", err)
				}
			} else {
				for i := range actual {

					if test.expectErr {
						t.Errorf("expected err, got none")
					}
					if actual[i] != test.expected {
						t.Errorf("expected subnet-id %s, got %s", test.expected, actual[i])
					}
				}
			}
		})
	}
}

func TestDefaultValidateEgressInput(t *testing.T) {
	customTags := map[string]string{
		"a": "b",
	}

	tests := []struct {
		region         string
		withCustomTags bool
		expectErr      bool
	}{
		{
			region:         "us-east-2",
			withCustomTags: false,
			expectErr:      false,
		},
		{
			region:         "eu-central-1",
			withCustomTags: true,
			expectErr:      false,
		},
		{
			region:         "us-central-1",
			withCustomTags: false,
			expectErr:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.region, func(t *testing.T) {
			cluster := newTestCluster(t, cmv1.NewCluster().AWS(cmv1.NewAWS()))
			if test.withCustomTags {
				cluster = newTestCluster(t, cmv1.NewCluster().AWS(cmv1.NewAWS().Tags(customTags)))
			}
			actual, err := defaultValidateEgressInput(context.TODO(), cluster, test.region)
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %s", err)
				}
			} else {
				if test.expectErr {
					t.Errorf("expected err, got none")
				}
				if test.withCustomTags {
					for k := range customTags {
						if v, ok := actual.Tags[k]; !ok {
							t.Errorf("expected %v to contain %v", actual.Tags, k)
							if v != customTags[k] {
								t.Errorf("expected %v to contain %v: %v", actual.Tags, k, customTags[k])
							}
						}
					}
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
		expected.AWS.SecurityGroupId != actual.AWS.SecurityGroupId {
		return false
	}

	if expected.AWS.KmsKeyID != actual.AWS.KmsKeyID {
		return false
	}

	if expected.Proxy.HttpProxy != actual.Proxy.HttpProxy ||
		expected.Proxy.HttpsProxy != actual.Proxy.HttpsProxy {
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
			actual, err := test.e.getSubnetIds(context.TODO())
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
