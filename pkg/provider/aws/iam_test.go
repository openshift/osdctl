package aws

import (
	"errors"
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
)

func TestCheckIAMUserExists(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title        string
		setupAWSMock func(r *mock.MockClientMockRecorder)
		username     string
		exists       bool
		errExpected  bool
	}{
		{
			title: "have error calling AWS API",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.GetUser(gomock.Any()).
					Return(nil, errors.New("FakeError")).Times(1)
			},
			username:    "",
			exists:      false,
			errExpected: true,
		},
		{
			title: "specified user doesn't exist",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.GetUser(gomock.Any()).
					Return(nil, &iamTypes.NoSuchEntityException{}).Times(1)
			},
			username:    "",
			exists:      false,
			errExpected: false,
		},
		{
			title: "specified user exists",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.GetUser(gomock.Any()).
					Return(nil, nil).Times(1)
			},
			username:    "foo",
			exists:      true,
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			// after mocks is defined
			defer mocks.mockCtrl.Finish()

			exists, err := CheckIAMUserExists(mocks.mockAWSClient, &tc.username)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).Should(Not(HaveOccurred()))
				g.Expect(exists).Should(Equal(tc.exists))
			}
		})
	}
}

func TestDeleteUserAccessKeys(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title        string
		setupAWSMock func(r *mock.MockClientMockRecorder)
		errExpected  bool
	}{
		{
			title: "error calling AWS List AccessKeys",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.ListAccessKeys(gomock.Any()).
					Return(nil, errors.New("FakeError")).Times(1)
			},
			errExpected: true,
		},
		{
			title: "error calling AWS Delete AccessKey",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListAccessKeys(gomock.Any()).Return(
						&iam.ListAccessKeysOutput{
							AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
								{
									UserName:    awsSdk.String("foo"),
									AccessKeyId: awsSdk.String("bar"),
								},
							},
						}, nil).Times(1),
					r.DeleteAccessKey(gomock.Any()).
						Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			errExpected: true,
		},
		{
			title: "success delete one access key",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListAccessKeys(gomock.Any()).Return(
						&iam.ListAccessKeysOutput{
							AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
								{
									UserName:    awsSdk.String("foo"),
									AccessKeyId: awsSdk.String("bar"),
								},
							},
						}, nil).Times(1),
					r.DeleteAccessKey(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			errExpected: false,
		},
		{
			title: "Failed to delete the second access key",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListAccessKeys(gomock.Any()).Return(
						&iam.ListAccessKeysOutput{
							AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
								{
									UserName:    awsSdk.String("foo"),
									AccessKeyId: awsSdk.String("bar"),
								},
								{
									UserName:    awsSdk.String("fizz"),
									AccessKeyId: awsSdk.String("buzz"),
								},
							},
						}, nil).Times(1),
					r.DeleteAccessKey(gomock.Any()).Return(nil, nil).Times(1),
					r.DeleteAccessKey(gomock.Any()).
						Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			errExpected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			// after mocks is defined
			defer mocks.mockCtrl.Finish()

			err := DeleteUserAccessKeys(mocks.mockAWSClient, awsSdk.String(""))
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}

func TestCreateIAMUserAndAttachPolicy(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title        string
		setupAWSMock func(r *mock.MockClientMockRecorder)
		username     *string
		policyArn    *string
		errExpected  bool
	}{
		{
			title: "failed to create AWS IAM User",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.CreateUser(gomock.Any()).
					Return(nil, errors.New("FakeError")).Times(1)
			},
			username:    awsSdk.String(""),
			policyArn:   awsSdk.String(""),
			errExpected: true,
		},
		{
			title: "failed to attach user policy",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.CreateUser(gomock.Any()).Return(
						&iam.CreateUserOutput{
							User: &iamTypes.User{
								UserName: awsSdk.String("foo"),
							},
						}, nil).Times(1),
					r.AttachUserPolicy(gomock.Any()).
						Return(nil, errors.New("FakeError")).Times(1),
				)

			},
			username:    awsSdk.String("foo"),
			policyArn:   awsSdk.String("bar"),
			errExpected: true,
		},
		{
			title: "success",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.CreateUser(gomock.Any()).Return(
						&iam.CreateUserOutput{
							User: &iamTypes.User{
								UserName: awsSdk.String("foo"),
							},
						}, nil).Times(1),
					r.AttachUserPolicy(gomock.Any()).
						Return(nil, nil).Times(1),
				)

			},
			username:    awsSdk.String("foo"),
			policyArn:   awsSdk.String("bar"),
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			// after mocks is defined
			defer mocks.mockCtrl.Finish()

			err := CreateIAMUserAndAttachPolicy(mocks.mockAWSClient, tc.username, tc.policyArn)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
