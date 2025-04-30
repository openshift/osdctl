package jumphost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockJumphostAWSClient struct {
	jumphostAWSClient
	mock.Mock
}

func (m *mockJumphostAWSClient) DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ec2.DescribeImagesOutput), args.Error(1)
}

func (m *mockJumphostAWSClient) AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(options *ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ec2.AuthorizeSecurityGroupIngressOutput), args.Error(1)
}

func (m *mockJumphostAWSClient) DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ec2.DescribeSubnetsOutput), args.Error(1)
}

func (m *mockJumphostAWSClient) CreateKeyPair(ctx context.Context, input *ec2.CreateKeyPairInput, opts ...func(*ec2.Options)) (*ec2.CreateKeyPairOutput, error) {
	args := m.Called(ctx, input)
	return args.Get(0).(*ec2.CreateKeyPairOutput), args.Error(1)
}

func TestFindLatestJumphostAMI(t *testing.T) {
	mockClient := new(mockJumphostAWSClient)
	jumphost := &jumphostConfig{awsClient: mockClient}
	ctx := context.TODO()
	mockImages := &ec2.DescribeImagesOutput{
		Images: []types.Image{
			{
				ImageId:      aws.String("testImageID"),
				CreationDate: aws.String("2023-04-01T12:00:00Z"),
			},
		},
	}
	tests := []struct {
		name          string
		mockError     error
		expectedAMI   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "success_case_find_latest_AMI",
			mockError:   nil,
			expectedAMI: "testImageID",
			expectError: false,
		},
		{
			name:          "error_case_describeimages_fails",
			mockError:     fmt.Errorf("errorDescribeImages"),
			expectedAMI:   "",
			expectError:   true,
			errorContains: "failed to describe images in order to launch an EC2 jumphost: errorDescribeImages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.On("DescribeImages", ctx, mock.Anything).
				Return(mockImages, tt.mockError).Once()
			ami, err := jumphost.findLatestJumphostAMI(ctx)
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedAMI, ami)
				assert.EqualError(t, err, tt.errorContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAMI, ami)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestAllowJumphostSshFromIp(t *testing.T) {
	mockClient := new(mockJumphostAWSClient)
	jumphost := &jumphostConfig{awsClient: mockClient}
	ctx := context.TODO()
	groupId := "Test12345"
	mockResponse := &ec2.AuthorizeSecurityGroupIngressOutput{}
	tests := []struct {
		name          string
		mockError     error
		expectError   bool
		errorContains string
	}{
		{
			name:        "success_case_allow_ssh_access",
			mockError:   nil,
			expectError: false,
		},
		{
			name:          "error_case_authorization_fails",
			mockError:     fmt.Errorf("AuthSecError"),
			expectError:   true,
			errorContains: "AuthSecError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.On("AuthorizeSecurityGroupIngress", ctx, mock.Anything).
				Return(mockResponse, tt.mockError).Once()

			err := jumphost.allowJumphostSshFromIp(ctx, groupId)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
			mockClient.AssertExpectations(t)
		})
	}
}

func TestFindVpcId(t *testing.T) {
	mockClient := new(mockJumphostAWSClient)
	ctx := context.TODO()
	tests := []struct {
		name          string
		jumphost      *jumphostConfig
		mockResponse  *ec2.DescribeSubnetsOutput
		mockError     error
		expectedVpcId string
		expectError   bool
		errorContains string
	}{
		{
			name:     "success_case_find_vpcid",
			jumphost: &jumphostConfig{awsClient: mockClient, subnetId: "subnet-12345"},
			mockResponse: &ec2.DescribeSubnetsOutput{
				Subnets: []types.Subnet{
					{
						VpcId: aws.String("vpc-67890"),
					},
				},
			},
			mockError:     nil,
			expectedVpcId: "vpc-67890",
			expectError:   false,
		},
		{
			name:     "error_case_describesubnets_fails",
			jumphost: &jumphostConfig{awsClient: mockClient, subnetId: "subnet-12345"},
			mockResponse: &ec2.DescribeSubnetsOutput{
				Subnets: []types.Subnet{
					{
						VpcId: aws.String("vpc-67890"),
					},
				},
			},
			mockError:     fmt.Errorf("errorDescribeSubnets"),
			expectedVpcId: "",
			expectError:   true,
			errorContains: "errorDescribeSubnets",
		},
		{
			name:     "error_case_zero_subnets",
			jumphost: &jumphostConfig{awsClient: mockClient, subnetId: "subnet-12345"},
			mockResponse: &ec2.DescribeSubnetsOutput{
				Subnets: []types.Subnet{},
			},
			expectedVpcId: "",
			expectError:   true,
			errorContains: "found 0 subnets matching subnet-12345",
		},
		{
			name:          "error_case_describeSubnets_fails",
			jumphost:      &jumphostConfig{subnetId: ""},
			expectError:   true,
			errorContains: "could not determine VPC; subnet id must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockResponse != nil {
				mockClient.On("DescribeSubnets", ctx, mock.Anything).
					Return(tt.mockResponse, tt.mockError).Once()
			}

			vpcId, err := tt.jumphost.findVpcId(ctx)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedVpcId, vpcId)
				assert.EqualError(t, err, tt.errorContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVpcId, vpcId)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestAssembleNextSteps(t *testing.T) {
	tests := []struct {
		name     string
		jumphost *jumphostConfig
		expected string
	}{
		{
			name: "with_key_file",
			jumphost: &jumphostConfig{
				ec2PublicIp: "1.2.3.4",
				keyFilepath: "test-key.pem",
			},
			expected: "ssh -i test-key.pem ec2-user@1.2.3.4",
		},
		{
			name: "without_key_file",
			jumphost: &jumphostConfig{
				ec2PublicIp: "1.2.3.4",
			},
			expected: "ssh -i ${private_key} ec2-user@1.2.3.4",
		},
		{
			name:     "missing_public_ip",
			jumphost: &jumphostConfig{},
			expected: "could not determine EC2 public ip - please verify, but something likely went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.jumphost.assembleNextSteps())
		})
	}
}

func TestCreateKeyPair(t *testing.T) {
	tests := []struct {
		name         string
		mockResponse *ec2.CreateKeyPairOutput
		mockError    error
		expectError  bool
		expectFile   bool
		expectedKey  string
	}{
		{
			name: "success_key_pair_created",
			mockResponse: &ec2.CreateKeyPairOutput{
				KeyName:     aws.String("test-key"),
				KeyPairId:   aws.String("key-1234"),
				KeyMaterial: aws.String("PRIVATE-KEY-DATA"),
			},
			mockError:   nil,
			expectError: false,
			expectFile:  true,
			expectedKey: "PRIVATE-KEY-DATA",
		},
		{
			name:         "failuer_aws_error",
			mockResponse: nil,
			mockError:    errors.New("AWS error"),
			expectError:  true,
			expectFile:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mockJumphostAWSClient)
			j := &jumphostConfig{awsClient: mockClient}

			mockClient.On("CreateKeyPair", mock.Anything, mock.Anything).Return(tt.mockResponse, tt.mockError)

			err := j.createKeyPair(context.Background())

			if tt.expectError {
				assert.Error(t, err, "expected an error but got none")
			} else {
				assert.NoError(t, err, "did not expect an error but got one")
			}

			if tt.expectFile {
				assert.FileExists(t, j.keyFilepath, "key file should be created")
				data, err := os.ReadFile(j.keyFilepath)
				assert.NoError(t, err, "should be able to read the key file")
				assert.Equal(t, tt.expectedKey, string(data), "private key should match")
				os.Remove(j.keyFilepath) // Cleanup
			} else {
				assert.Empty(t, j.keyFilepath, "key file path should be empty on failure")
			}
		})
	}
}
