package network

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift-online/ocm-sdk-go/logging"
	"github.com/openshift/osd-network-verifier/pkg/proxy"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"
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
				SubnetId:        []string{"subnet-a"},
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
				for i, _ := range actual {
					if !compareValidateEgressInput(test.expected, actual[i]) {
						t.Errorf("expected %v, got %v", test.expected, actual[i])
					}
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
				log:      newTestLogger(t),
				SubnetId: []string{"override"},
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
			actual, err := test.e.getSubnetId(context.TODO())
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %s", err)
				}
			} else {
				for i, _ := range actual {

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
	tests := []struct {
		region    string
		expectErr bool
	}{
		{
			region:    "us-east-2",
			expectErr: false,
		},
		{
			region:    "us-central-1",
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.region, func(t *testing.T) {
			_, err := defaultValidateEgressInput(context.TODO(), test.region)
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
		expected.AWS.SecurityGroupId != actual.AWS.SecurityGroupId {
		return false
	}

	if expected.Proxy.HttpProxy != actual.Proxy.HttpProxy ||
		expected.Proxy.HttpsProxy != actual.Proxy.HttpsProxy {
		return false
	}

	return true
}
