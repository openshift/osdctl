package network

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/openshift-online/ocm-sdk-go/logging"
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
		e         *egressVerification
		expectErr bool
	}{
		{
			name: "no clusterId requires subnet/sg",
			e: &egressVerification{
				clusterId: "",
			},
			expectErr: true,
		},
		{
			name: "clusterId optional",
			e: &egressVerification{
				clusterId:       "",
				subnetId:        "subnet-a",
				securityGroupId: "sg-b",
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

func Test_egressVerificationGetSecurityGroupId(t *testing.T) {
	tests := []struct {
		name      string
		e         *egressVerification
		expected  string
		expectErr bool
	}{
		{
			name: "manual override",
			e: &egressVerification{
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
				securityGroupId: "override",
			},
			expected:  "override",
			expectErr: false,
		},
		{
			name: "zero from AWS",
			e: &egressVerification{
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
			e: &egressVerification{
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
		e         *egressVerification
		expected  string
		expectErr bool
	}{
		{
			name: "manual override",
			e: &egressVerification{
				log:      newTestLogger(t),
				subnetId: "override",
			},
			expected:  "override",
			expectErr: false,
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
				if test.expectErr {
					t.Errorf("expected err, got none")
				}
				if actual != test.expected {
					t.Errorf("expected subnet-id %s, got %s", test.expected, actual)
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
