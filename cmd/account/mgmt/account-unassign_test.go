package mgmt

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/organizations"

	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/golang/mock/gomock"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestCheckForHiveNameTage(t *testing.T) {
	var genericAWSError error = fmt.Errorf("Generic AWS Error")

	testData := []struct {
		name             string
		tags             map[string]string
		expectErr        error
		expectedAWSError error
	}{
		{
			name:             "test for both tags present",
			expectErr:        nil,
			expectedAWSError: nil,
			tags: map[string]string{
				"owner":   "tuser",
				"claimed": "true",
			},
		},
		{
			name:             "test for hive name tag present",
			expectErr:        ErrHiveNameProvided,
			expectedAWSError: nil,
			tags: map[string]string{
				"owner": "hivesomething",
			},
		},
		{
			name:             "test for no tags present",
			expectErr:        ErrNoTagsOnAccount,
			expectedAWSError: nil,
			tags:             map[string]string{},
		},
		{
			name:             "test for AWS error catching",
			expectErr:        genericAWSError,
			expectedAWSError: genericAWSError,
			tags:             map[string]string{},
		},
	}

	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {
			mocks := setupDefaultMocks(t, []runtime.Object{})

			mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
			accountID := "111111111111"

			awsOutput := &organizations.ListTagsForResourceOutput{}
			if test.expectedAWSError == nil {
				tags := []*organizations.Tag{}
				for key, value := range test.tags {
					tag := &organizations.Tag{
						Key:   aws.String(key),
						Value: aws.String(value),
					}
					tags = append(tags, tag)
				}
				awsOutput.Tags = tags
			}

			mockAWSClient.EXPECT().ListTagsForResource(gomock.Any()).Return(
				awsOutput,
				test.expectedAWSError,
			)

			o := &accountUnassignOptions{}
			o.awsClient = mockAWSClient
			err := o.checkForHiveNameTag(accountID)
			if test.expectErr != err {
				t.Errorf("expected error %s and got %s", test.expectErr, err)
			}
		})
	}
}

func TestUnassignMoveAccount(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

	accountId := "111111111111"
	destOu := "abc-vnjfdshs"
	rootOu := "abc"

	awsOutputMove := &organizations.MoveAccountOutput{}

	mockAWSClient.EXPECT().MoveAccount(gomock.Any()).Return(
		awsOutputMove,
		nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.moveAccount(accountId, rootOu, destOu)
	if err != nil {
		t.Errorf("failed to move account")
	}
}

func TestListAccountsFromUser(t *testing.T) {

	var genericAWSError error = fmt.Errorf("Generic AWS Error")

	testData := []struct {
		name                string
		expectedAccountList []string
		resources           []string
		expectErr           error
		expectedAWSError    error
	}{
		{
			name:                "test for resources present",
			expectedAccountList: []string{"111111111111"},
			expectErr:           nil,
			expectedAWSError:    nil,
			resources:           []string{"randomresourcearn"},
		},
		{
			name:                "test for no resources present",
			expectedAccountList: nil,
			expectErr:           ErrNoAccountsForUser,
			expectedAWSError:    nil,
			resources:           []string{},
		},
		{
			name:                "test for AWS error catching",
			expectedAccountList: nil,
			expectErr:           genericAWSError,
			expectedAWSError:    genericAWSError,
			resources:           []string{},
		},
	}

	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {
			mocks := setupDefaultMocks(t, []runtime.Object{})

			mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

			userName := "auser"

			awsOutput := &resourcegroupstaggingapi.GetResourcesOutput{}
			if test.expectedAWSError == nil {
				resources := []*resourcegroupstaggingapi.ResourceTagMapping{}
				for _, r := range test.resources {
					resource := &resourcegroupstaggingapi.ResourceTagMapping{
						ResourceARN: aws.String(r),
					}
					resources = append(resources, resource)
				}
				awsOutput.ResourceTagMappingList = resources
			}

			mockAWSClient.EXPECT().GetResources(gomock.Any()).Return(
				awsOutput,
				test.expectedAWSError,
			)

			o := &accountUnassignOptions{}
			o.awsClient = mockAWSClient
			returnValue, err := o.listAccountsFromUser(userName)
			if test.expectErr != err {
				t.Errorf("expected error %s and got %s", test.expectErr, err)
			}
			if len(returnValue) != len(test.expectedAccountList) {
				t.Errorf("expected length of accounts list is %s instead of %s", test.expectedAccountList, returnValue)
			}
		})
	}
}

func TestDeleteProfile(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"
	awsOutput := &iam.DeleteLoginProfileOutput{}
	mockAWSClient.EXPECT().DeleteLoginProfile(gomock.Any()).Return(
		awsOutput,
		nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.deleteLoginProfile(userName)
	if err != nil {
		t.Errorf("failed to delete login profile")
	}
}

func TestDeleteAccessKey(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedAccessKeyID := aws.String("expectedAccessKeyID")

	mockAWSClient.EXPECT().ListAccessKeys(&iam.ListAccessKeysInput{UserName: &userName}).Return(
		&iam.ListAccessKeysOutput{
			AccessKeyMetadata: []*iam.AccessKeyMetadata{
				{
					AccessKeyId: expectedAccessKeyID,
				},
			},
		},
		nil,
	)
	mockAWSClient.EXPECT().DeleteAccessKey(
		&iam.DeleteAccessKeyInput{
			AccessKeyId: expectedAccessKeyID,
			UserName:    &userName,
		}).Return(
		&iam.DeleteAccessKeyOutput{},
		nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.deleteAccessKeys(userName)
	if err != nil {
		t.Errorf("failed to delete access keys")
	}
}

func TestDeleteSigningCert(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedCertificateId := aws.String("expectedCertificateId")

	mockAWSClient.EXPECT().ListSigningCertificates(&iam.ListSigningCertificatesInput{UserName: &userName}).Return(
		&iam.ListSigningCertificatesOutput{
			Certificates: []*iam.SigningCertificate{
				{
					CertificateId: expectedCertificateId,
				},
			},
		},
		nil,
	)

	mockAWSClient.EXPECT().DeleteSigningCertificate(
		&iam.DeleteSigningCertificateInput{
			CertificateId: expectedCertificateId,
			UserName:      &userName,
		},
	).Return(
		&iam.DeleteSigningCertificateOutput{},
		nil,
	)
	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.deleteSigningCert(userName)
	if err != nil {
		t.Errorf("failed to delete signing certificates")
	}
}

func TestDeletePolicies(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedPolicyName := "ExpectedPolicyName"
	mockAWSClient.EXPECT().ListUserPolicies(
		&iam.ListUserPoliciesInput{UserName: &userName},
	).Return(
		&iam.ListUserPoliciesOutput{
			PolicyNames: []*string{
				&expectedPolicyName,
			},
		},
		nil,
	)
	mockAWSClient.EXPECT().DeleteUserPolicy(
		&iam.DeleteUserPolicyInput{
			UserName:   &userName,
			PolicyName: &expectedPolicyName,
		},
	).Return(
		nil, nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.deletePolicies(userName)
	if err != nil {
		t.Errorf("failed to delete user policies")
	}
}

func TestDeleteAttachedPolicies(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedPolicyArn := "ExpectedPolicyArn"
	mockAWSClient.EXPECT().ListAttachedUserPolicies(
		&iam.ListAttachedUserPoliciesInput{UserName: &userName},
	).Return(
		&iam.ListAttachedUserPoliciesOutput{
			AttachedPolicies: []*iam.AttachedPolicy{
				{
					PolicyArn:  &expectedPolicyArn,
					PolicyName: aws.String("ExpectedPolicyName"),
				},
			},
		},
		nil,
	)
	mockAWSClient.EXPECT().DetachUserPolicy(
		&iam.DetachUserPolicyInput{
			UserName:  &userName,
			PolicyArn: &expectedPolicyArn,
		},
	).Return(
		nil, nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.deleteAttachedPolicies(userName)
	if err != nil {
		t.Errorf("failed to detach policies")
	}
}

func TestDeleteGroups(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedGroupName := "expectedGroupName"

	mockAWSClient.EXPECT().ListGroupsForUser(
		&iam.ListGroupsForUserInput{UserName: &userName},
	).Return(
		&iam.ListGroupsForUserOutput{
			Groups: []*iam.Group{
				{
					GroupName: &expectedGroupName,
				},
			},
		},
		nil,
	)

	mockAWSClient.EXPECT().RemoveUserFromGroup(
		&iam.RemoveUserFromGroupInput{
			GroupName: &expectedGroupName,
			UserName:  &userName,
		},
	).Return(
		nil, nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.deleteGroups(userName)
	if err != nil {
		t.Errorf("failed to delete groups")
	}
}

func TestDeleteUser(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	awsOutput := &iam.DeleteUserOutput{}
	mockAWSClient.EXPECT().DeleteUser(gomock.Any()).Return(
		awsOutput,
		nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.deleteUser(userName)
	if err != nil {
		t.Errorf("failed to delete iam user")
	}
}

func TestUntagAccount(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

	accountId := "111111111111"

	mockAWSClient.EXPECT().UntagResource(gomock.Any()).Return(
		&organizations.UntagResourceOutput{},
		nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := o.untagAccount(accountId)
	if err != nil {
		t.Errorf("failed to untag aws account")
	}
}
