package jumphost

import (
	"context"
	"fmt"
	"log"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testLogWriter struct {
	writeFunc func(msg string)
}

func (m *testLogWriter) Write(p []byte) (n int, err error) {
	m.writeFunc(string(p))
	return len(p), nil
}

type mockAWSClient struct {
	jumphostAWSClient
	mock.Mock
}

func (m *mockAWSClient) DescribeKeyPairs(ctx context.Context, input *ec2.DescribeKeyPairsInput, opts ...func(*ec2.Options)) (*ec2.DescribeKeyPairsOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*ec2.DescribeKeyPairsOutput), args.Error(1)
}

func (m *mockAWSClient) DeleteKeyPair(ctx context.Context, input *ec2.DeleteKeyPairInput, opts ...func(*ec2.Options)) (*ec2.DeleteKeyPairOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*ec2.DeleteKeyPairOutput), args.Error(1)
}

func (m *mockAWSClient) DescribeSecurityGroups(ctx context.Context, input *ec2.DescribeSecurityGroupsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*ec2.DescribeSecurityGroupsOutput), args.Error(1)
}

func (m *mockAWSClient) DeleteSecurityGroup(ctx context.Context, input *ec2.DeleteSecurityGroupInput, opts ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*ec2.DeleteSecurityGroupOutput), args.Error(1)
}

func (m *mockAWSClient) DescribeSubnets(ctx context.Context, input *ec2.DescribeSubnetsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*ec2.DescribeSubnetsOutput), args.Error(1)
}

func (m *mockAWSClient) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ec2.DescribeInstancesOutput), args.Error(1)
}

func (m *mockAWSClient) TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ec2.TerminateInstancesOutput), args.Error(1)
}

func TestDeleteKeyPair(t *testing.T) {
	ctx := context.Background()
	mockAws := new(mockAWSClient)
	mockJumphost := &jumphostConfig{
		awsClient: mockAws,
		tags:      []types.Tag{{Key: aws.String("Name"), Value: aws.String("TestKey")}},
	}

	mockKeyName := "test-key"
	mockKeyID := "key-12345"

	tests := []struct {
		name         string
		describeResp *ec2.DescribeKeyPairsOutput
		describeErr  error
		deleteErr    error
		expectedLog  string
		expectError  error
	}{
		{
			name: "successfully_delete_key_pair",
			describeResp: &ec2.DescribeKeyPairsOutput{
				KeyPairs: []types.KeyPairInfo{
					{KeyName: aws.String(mockKeyName), KeyPairId: aws.String(mockKeyID)},
				},
			},
			describeErr: nil,
			deleteErr:   nil,
		},
		{
			name: "no_key_pairs_found",
			describeResp: &ec2.DescribeKeyPairsOutput{
				KeyPairs: []types.KeyPairInfo{},
			},
			describeErr: nil,
			deleteErr:   nil,
			expectedLog: "no key pairs found to delete",
		},
		{
			name:         "error_describing_key_pairs",
			describeResp: nil,
			describeErr:  fmt.Errorf("AWS_describe_error"),
			deleteErr:    nil,
			expectError:  fmt.Errorf("failed to describe key pair: AWS_describe_error"),
		},
		{
			name: "error_deleting_key_pair",
			describeResp: &ec2.DescribeKeyPairsOutput{
				KeyPairs: []types.KeyPairInfo{
					{KeyName: aws.String(mockKeyName), KeyPairId: aws.String(mockKeyID)},
				},
			},
			describeErr: nil,
			deleteErr:   fmt.Errorf("AWS_delete_error"),
			expectError: fmt.Errorf("failed to delete keypair: AWS_delete_error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAws.ExpectedCalls = nil
			mockAws.On("DescribeKeyPairs", ctx, mock.Anything).Return(tt.describeResp, tt.describeErr).Once()
			if tt.describeResp != nil && len(tt.describeResp.KeyPairs) > 0 {
				mockAws.On("DeleteKeyPair", ctx, mock.Anything).Return(&ec2.DeleteKeyPairOutput{}, tt.deleteErr).Once()
			}
			var logOutput string
			log.SetOutput(&testLogWriter{func(msg string) {
				logOutput = msg
			}})
			err := mockJumphost.deleteKeyPair(ctx)
			if tt.expectedLog != "" {
				assert.Contains(t, logOutput, tt.expectedLog)
			}
			if err != nil {
				assert.Error(t, err, tt.expectError)
				return
			}
			assert.NoError(t, err)

		})
	}
}

func TestDeleteSecurityGroup(t *testing.T) {
	ctx := context.Background()
	vpcId := "vpc-12345"
	groupId := "sg-67890"

	tests := []struct {
		name              string
		setupMocks        func(mockAWS *mockAWSClient)
		jumphostConfig    *jumphostConfig
		expectedErr       error
		expectedLogOutput string
	}{
		{
			name: "successful_deletion",
			setupMocks: func(mockAWS *mockAWSClient) {
				mockAWS.On("DescribeSubnets", ctx, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{{VpcId: &vpcId}},
				}, nil).Once()

				mockAWS.On("DescribeSecurityGroups", ctx, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
					SecurityGroups: []types.SecurityGroup{
						{GroupId: &groupId, GroupName: aws.String("test-group")},
					},
				}, nil).Once()

				mockAWS.On("DeleteSecurityGroup", ctx, mock.Anything).Return(&ec2.DeleteSecurityGroupOutput{}, nil).Once()
			},
			jumphostConfig: &jumphostConfig{
				tags:     []types.Tag{},
				subnetId: "subnet-12345",
			},
			expectedLogOutput: fmt.Sprintf("deleting security group: test-group (%s)", groupId),
		},
		{
			name: "no_security_groups_found",
			setupMocks: func(mockAWS *mockAWSClient) {
				mockAWS.On("DescribeSubnets", ctx, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{{VpcId: &vpcId}},
				}, nil).Once()

				mockAWS.On("DescribeSecurityGroups", ctx, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
					SecurityGroups: []types.SecurityGroup{},
				}, nil).Once()
			},
			jumphostConfig: &jumphostConfig{
				tags:     []types.Tag{},
				subnetId: "subnet-12345",
			},
			expectedLogOutput: "no security groups found to delete",
		},
		{
			name: "describeSubnets_fails",
			setupMocks: func(mockAWS *mockAWSClient) {
				mockAWS.On("DescribeSubnets", ctx, mock.Anything).Return(&ec2.DescribeSubnetsOutput{}, fmt.Errorf("failed to describe subnets"))
			},
			jumphostConfig: &jumphostConfig{
				tags:     []types.Tag{},
				subnetId: "subnet-12345",
			},
			expectedErr: fmt.Errorf("failed to describe subnets"),
		},
		{
			name: "describesecuritygroups_fails",
			setupMocks: func(mockAWS *mockAWSClient) {
				mockAWS.On("DescribeSubnets", ctx, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{{VpcId: &vpcId}},
				}, nil)

				mockAWS.On("DescribeSecurityGroups", ctx, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
					SecurityGroups: []types.SecurityGroup{},
				}, fmt.Errorf("describe_security_groups_error"))
			},
			jumphostConfig: &jumphostConfig{
				tags:     []types.Tag{},
				subnetId: "subnet-12345",
			},
			expectedErr: fmt.Errorf("failed to describe security groups: describe_security_groups_error"),
		},
		{
			name: "deletedecuritygroup_fails",
			setupMocks: func(mockAWS *mockAWSClient) {
				mockAWS.On("DescribeSubnets", ctx, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{{VpcId: &vpcId}},
				}, nil)

				mockAWS.On("DescribeSecurityGroups", ctx, mock.Anything).Return(&ec2.DescribeSecurityGroupsOutput{
					SecurityGroups: []types.SecurityGroup{
						{GroupId: &groupId, GroupName: aws.String("test-group")},
					},
				}, nil)

				mockAWS.On("DeleteSecurityGroup", ctx, mock.Anything).Return((*ec2.DeleteSecurityGroupOutput)(nil), fmt.Errorf("delete_security_group_error"))
			},
			jumphostConfig: &jumphostConfig{
				tags:     []types.Tag{},
				subnetId: "subnet-12345",
			},
			expectedErr: fmt.Errorf("failed to delete security group: delete_security_group_error"),
		},
		{
			name:       "empty_subnetid",
			setupMocks: func(mockAWS *mockAWSClient) {},
			jumphostConfig: &jumphostConfig{
				tags:     []types.Tag{},
				subnetId: "",
			},
			expectedErr: fmt.Errorf("could not determine VPC; subnet id must not be empty"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAWS := new(mockAWSClient)
			jumphost := &jumphostConfig{
				awsClient: mockAWS,
				tags:      tt.jumphostConfig.tags,
				subnetId:  tt.jumphostConfig.subnetId,
			}
			tt.setupMocks(mockAWS)
			var logOutput string
			log.SetOutput(&testLogWriter{func(msg string) {
				logOutput = msg
			}})

			err := jumphost.deleteSecurityGroup(ctx)

			if err != nil {
				assert.Error(t, err, tt.expectedErr.Error())
				return
			}
			assert.Contains(t, logOutput, tt.expectedLogOutput)
		})
	}
}
