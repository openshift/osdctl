package aws

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/golang/mock/gomock"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
					Return(nil, awserr.New(
						iam.ErrCodeNoSuchEntityException,
						"",
						errors.New("FakeError"),
					)).Times(1)
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

			exists, err := CheckIAMUserExists(mocks.mockAWSClient, aws.String(tc.username))
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
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
							AccessKeyMetadata: []*iam.AccessKeyMetadata{
								{
									UserName:    aws.String("foo"),
									AccessKeyId: aws.String("bar"),
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
							AccessKeyMetadata: []*iam.AccessKeyMetadata{
								{
									UserName:    aws.String("foo"),
									AccessKeyId: aws.String("bar"),
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
							AccessKeyMetadata: []*iam.AccessKeyMetadata{
								{
									UserName:    aws.String("foo"),
									AccessKeyId: aws.String("bar"),
								},
								{
									UserName:    aws.String("fizz"),
									AccessKeyId: aws.String("buzz"),
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

			err := DeleteUserAccessKeys(mocks.mockAWSClient, aws.String(""))
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
			username:    aws.String(""),
			policyArn:   aws.String(""),
			errExpected: true,
		},
		{
			title: "failed to attach user policy",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.CreateUser(gomock.Any()).Return(
						&iam.CreateUserOutput{
							User: &iam.User{
								UserName: aws.String("foo"),
							},
						}, nil).Times(1),
					r.AttachUserPolicy(gomock.Any()).
						Return(nil, errors.New("FakeError")).Times(1),
				)

			},
			username:    aws.String("foo"),
			policyArn:   aws.String("bar"),
			errExpected: true,
		},
		{
			title: "success",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.CreateUser(gomock.Any()).Return(
						&iam.CreateUserOutput{
							User: &iam.User{
								UserName: aws.String("foo"),
							},
						}, nil).Times(1),
					r.AttachUserPolicy(gomock.Any()).
						Return(nil, nil).Times(1),
				)

			},
			username:    aws.String("foo"),
			policyArn:   aws.String("bar"),
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

func TestRefreshIAMPolicy(t *testing.T) {
	g := NewGomegaWithT(t)

	exampleFederatedRole := &awsv1alpha1.AWSFederatedRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: awsv1alpha1.AWSFederatedRoleSpec{
			AWSCustomPolicy: awsv1alpha1.AWSCustomPolicy{
				Name:        "bar",
				Description: "foo",
				Statements: []awsv1alpha1.StatementEntry{
					{
						Effect:   "Allow",
						Action:   []string{"aws-portal:ViewAccount"},
						Resource: []string{"*"},
					},
				},
			},
		},
	}

	exampleFederatedRoleWithManagedPolices := &awsv1alpha1.AWSFederatedRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: awsv1alpha1.AWSFederatedRoleSpec{
			AWSCustomPolicy: awsv1alpha1.AWSCustomPolicy{
				Name:        "bar",
				Description: "foo",
				Statements: []awsv1alpha1.StatementEntry{
					{
						Effect:   "Allow",
						Action:   []string{"aws-portal:ViewAccount"},
						Resource: []string{"*"},
					},
				},
			},
			AWSManagedPolicies: []string{
				"foo",
				"bar",
			},
		},
	}

	testCases := []struct {
		title         string
		setupAWSMock  func(r *mock.MockClientMockRecorder)
		federatedRole *awsv1alpha1.AWSFederatedRole
		accountID     string
		uid           string
		errExpected   bool
	}{
		{
			title: "Failed to detach role policy",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.DetachRolePolicy(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   true,
		},
		{
			title: "Failed to delete policy",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   true,
		},
		{
			title: "Failed to create policy",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   true,
		},
		{
			title: "Failed to list current role policies",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   true,
		},
		{
			title: "Failed to attach role policy",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(
						&iam.ListAttachedRolePoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{}}, nil).Times(1),
					r.AttachRolePolicy(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   true,
		},
		{
			title: "Detach role policy noSuchEntity error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).
						Return(nil, awserr.New(iam.ErrCodeNoSuchEntityException, "",
							errors.New("FakeError"))).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(
						&iam.ListAttachedRolePoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{}}, nil).Times(1),
					r.AttachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   false,
		},
		{
			title: "Detach role policy noSuchEntity error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).
						Return(nil, awserr.New(iam.ErrCodeNoSuchEntityException, "",
							errors.New("FakeError"))).Times(1),
					r.DeletePolicy(gomock.Any()).
						Return(nil, awserr.New(iam.ErrCodeNoSuchEntityException, "",
							errors.New("FakeError"))).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(
						&iam.ListAttachedRolePoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{}}, nil).Times(1),
					r.AttachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   false,
		},
		{
			title: "Retry entity already exists error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					// Retry one time
					r.CreatePolicy(gomock.Any()).Return(nil,
						awserr.New(iam.ErrCodeEntityAlreadyExistsException, "",
							errors.New("FakeError"))).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(
						&iam.ListAttachedRolePoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{}}, nil).Times(1),
					r.AttachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   false,
		},
		{
			title: "success with none policy names",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(
						&iam.ListAttachedRolePoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{}}, nil).Times(1),
					r.AttachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   false,
		},
		{
			title: "need to remove outdated policies",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(
						&iam.ListAttachedRolePoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{
								{
									PolicyArn:  aws.String("foo"),
									PolicyName: aws.String("foo"),
								},
								{
									PolicyArn:  aws.String("bar"),
									PolicyName: aws.String("bar"),
								},
							}}, nil).Times(1),
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(2),
					r.AttachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			federatedRole: exampleFederatedRole,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   false,
		},
		{
			title: "don't need to remove managed policies",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(
						&iam.ListAttachedRolePoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{
								{
									PolicyName: aws.String("foo"),
									PolicyArn:  aws.String("foo"),
								},
								{
									PolicyName: aws.String("bar"),
									PolicyArn:  aws.String("bar"),
								},
							}}, nil).Times(1),
					r.AttachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			federatedRole: exampleFederatedRoleWithManagedPolices,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   false,
		},
		{
			title: "need to add new managed policies",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.DetachRolePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.DeletePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.CreatePolicy(gomock.Any()).Return(nil, nil).Times(1),
					r.ListAttachedRolePolicies(gomock.Any()).Return(
						&iam.ListAttachedRolePoliciesOutput{
							AttachedPolicies: []*iam.AttachedPolicy{
								{
									PolicyName: aws.String("foo"),
									PolicyArn:  aws.String("bar"),
								},
							}}, nil).Times(1),
					r.AttachRolePolicy(gomock.Any()).Return(nil, nil).Times(2),
				)
			},
			federatedRole: exampleFederatedRoleWithManagedPolices,
			accountID:     "foo",
			uid:           "bar",
			errExpected:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			// after mocks is defined
			defer mocks.mockCtrl.Finish()

			err := RefreshIAMPolicy(mocks.mockAWSClient, tc.federatedRole, tc.accountID, tc.uid)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
