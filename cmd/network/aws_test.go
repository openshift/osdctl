package network

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osd-network-verifier/pkg/data/cloud"
	"github.com/openshift/osd-network-verifier/pkg/proxy"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"
	"testing"
	"time"
)

type mockEgressVerificationAWSClient struct {
	describeSecurityGroupsResp *ec2.DescribeSecurityGroupsOutput
	describeSubnetsResp        *ec2.DescribeSubnetsOutput
	describeRouteTablesResp    *ec2.DescribeRouteTablesOutput
}

func (m mockEgressVerificationAWSClient) DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(options *ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return m.describeSubnetsResp, nil
}

func (m mockEgressVerificationAWSClient) DescribeSecurityGroups(context.Context, *ec2.DescribeSecurityGroupsInput, ...func(options *ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	return m.describeSecurityGroupsResp, nil
}
func (m mockEgressVerificationAWSClient) DescribeRouteTables(context.Context, *ec2.DescribeRouteTablesInput, ...func(options *ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	return m.describeRouteTablesResp, nil
}

func Test_egressVerification_setupForAws(t *testing.T) {
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

			_, err = test.e.setupForAws(ctx)
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

func Test_egressVerification_GenerateAWSValidateEgressInput(t *testing.T) {
	tests := []struct {
		name      string
		e         *EgressVerification
		region    string
		expected  *onv.ValidateEgressInput
		expectErr bool
	}{
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
					describeRouteTablesResp: &ec2.DescribeRouteTablesOutput{
						RouteTables: []types.RouteTable{
							{
								RouteTableId: aws.String("rt-id"),
								Routes: []types.Route{
									{
										GatewayId: aws.String("gateway"),
									},
								},
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
					SecurityGroupIDs: []string{"sg-abcd"},
				},
			},
			expectErr: false,
		},
		{
			name: "Cluster specific KMS key forward",
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
					describeRouteTablesResp: &ec2.DescribeRouteTablesOutput{
						RouteTables: []types.RouteTable{
							{
								RouteTableId: aws.String("rt-id"),
								Routes: []types.Route{
									{
										GatewayId: aws.String("gateway"),
									},
								},
							},
						},
					},
				},

				cluster: newTestCluster(t, cmv1.NewCluster().
					CloudProvider(cmv1.NewCloudProvider().ID("aws")).
					Product(cmv1.NewProduct().ID("rosa")).
					AWS(cmv1.NewAWS().KMSKeyArn("some-KMS-key-ARN")),
				),
				log:           newTestLogger(t),
				EgressTimeout: 42 * time.Second,
			},
			region: "us-east-2",
			expected: &onv.ValidateEgressInput{
				SubnetID: "subnet-abcd",
				AWS: onv.AwsEgressConfig{
					SecurityGroupIDs: []string{"sg-abcd"},
					KmsKeyID:         "some-KMS-key-ARN",
				},
				Timeout: 42 * time.Second,
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := test.e.generateAWSValidateEgressInput(context.Background(), cloud.AWSClassic)
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

func Test_egressVerification_GetAwsSecurityGroupId(t *testing.T) {
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
				cluster:         newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
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
				log:     newTestLogger(t),
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
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
				log:     newTestLogger(t),
				cluster: newTestCluster(t, cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws"))),
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

func Test_egressVerification_GetAwsSubnetId(t *testing.T) {
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
					if actual[i] != test.expected {
						t.Errorf("expected subnet-id %s, got %s", test.expected, actual[i])
					}
				}
			}
		})
	}
}
