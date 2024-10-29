package mgmt

import (
	"fmt"
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	organizationTypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	resourceGroupsTaggingApiTypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	stsTypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	awsInternal "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
	"github.com/spf13/viper"
	"go.uber.org/mock/gomock"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestAssumeRoleForAccount(t *testing.T) {
	viper.Set(awsInternal.ProxyConfigKey, "a-random-proxy")
	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

	accountId := "111111111111"
	accessKeyID := awsSdk.String("randAccessKeyId")
	secretAccessKey := awsSdk.String("randSecretAccessKey")
	sessionToken := awsSdk.String("randSessionToken")

	awsAssumeRoleOutput := &sts.AssumeRoleOutput{
		Credentials: &stsTypes.Credentials{
			AccessKeyId:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			SessionToken:    sessionToken,
		},
	}

	mockAWSClient.EXPECT().AssumeRole(gomock.Any()).Return(
		awsAssumeRoleOutput,
		nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	returnVal, err := o.assumeRoleForAccount(accountId)
	if err != nil {
		t.Errorf("failed to assume role")
	}
	if returnVal == nil {
		t.Error("no awsclient returned")
	}
}

func TestListUsersFromAccount(t *testing.T) {

	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

	awsOutput := &iam.ListUsersOutput{
		Users: []iamTypes.User{
			{
				UserName: awsSdk.String("user")},
		}}

	mockAWSClient.EXPECT().ListUsers(gomock.Any()).Return(
		awsOutput,
		nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	returnVal, err := listUsersFromAccount(mockAWSClient)
	if err != nil {
		t.Errorf("failed to list iam users")
	}
	if len(returnVal) < 1 {
		t.Errorf("empty iam users list")
	}
}
func TestCheckForHiveNameTags(t *testing.T) {
	var genericAWSError = fmt.Errorf("Generic AWS Error")

	testData := []struct {
		name             string
		tags             map[string]string
		expectedUsername string
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
			expectedUsername: "tuser",
		},
		{
			name:             "test for hive name tag present",
			expectErr:        ErrHiveNameProvided,
			expectedAWSError: nil,
			tags: map[string]string{
				"owner": "hivesomething",
			},
			expectedUsername: "",
		},
		{
			name:             "test for no owner tag present",
			expectErr:        ErrNoOwnerTag,
			expectedAWSError: nil,
			tags: map[string]string{
				"claimed":  "true",
				"asldkjfa": "alskdjfaksjd",
			},
			expectedUsername: "",
		},
		{
			name:             "test for no tags present",
			expectErr:        ErrNoTagsOnAccount,
			expectedAWSError: nil,
			tags:             map[string]string{},
			expectedUsername: "",
		},
		{
			name:             "test for AWS error catching",
			expectErr:        genericAWSError,
			expectedAWSError: genericAWSError,
			tags:             map[string]string{},
			expectedUsername: "",
		},
	}

	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {
			mocks := setupDefaultMocks(t)

			mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
			accountID := "111111111111"

			awsOutput := &organizations.ListTagsForResourceOutput{}
			if test.expectedAWSError == nil {
				var tags []organizationTypes.Tag
				for key, value := range test.tags {
					tag := organizationTypes.Tag{
						Key:   awsSdk.String(key),
						Value: awsSdk.String(value),
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
			returnVal, err := o.checkForHiveNameTag(accountID)
			if test.expectErr != err {
				t.Errorf("expected error %s and got %s", test.expectErr, err)
			}
			if returnVal != test.expectedUsername {
				t.Errorf("expected %s is %s", test.expectedUsername, returnVal)
			}
		})
	}
}

func TestUnassignMoveAccount(t *testing.T) {

	mocks := setupDefaultMocks(t)

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

	var genericAWSError = fmt.Errorf("Generic AWS Error")

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
			mocks := setupDefaultMocks(t)

			mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

			userName := "auser"

			awsOutput := &resourcegroupstaggingapi.GetResourcesOutput{}
			if test.expectedAWSError == nil {
				var resources []resourceGroupsTaggingApiTypes.ResourceTagMapping
				for _, r := range test.resources {
					resource := resourceGroupsTaggingApiTypes.ResourceTagMapping{
						ResourceARN: &r,
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

	mocks := setupDefaultMocks(t)

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

	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedAccessKeyID := awsSdk.String("expectedAccessKeyID")

	mockAWSClient.EXPECT().ListAccessKeys(&iam.ListAccessKeysInput{UserName: &userName}).Return(
		&iam.ListAccessKeysOutput{
			AccessKeyMetadata: []iamTypes.AccessKeyMetadata{
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

	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedCertificateId := awsSdk.String("expectedCertificateId")

	mockAWSClient.EXPECT().ListSigningCertificates(&iam.ListSigningCertificatesInput{UserName: &userName}).Return(
		&iam.ListSigningCertificatesOutput{
			Certificates: []iamTypes.SigningCertificate{
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

	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedPolicyName := "ExpectedPolicyName"
	mockAWSClient.EXPECT().ListUserPolicies(
		&iam.ListUserPoliciesInput{UserName: &userName},
	).Return(
		&iam.ListUserPoliciesOutput{
			PolicyNames: []string{
				expectedPolicyName,
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
	err := o.deleteUserPolicies(userName)
	if err != nil {
		t.Errorf("failed to delete user policies")
	}
}

func TestDeleteAccountPolicies(t *testing.T) {

	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

	expectedPolicyArn := "ExpectedPolicyArn"
	mockAWSClient.EXPECT().ListPolicies(
		&iam.ListPoliciesInput{Scope: "Local"},
	).Return(
		&iam.ListPoliciesOutput{
			Policies: []iamTypes.Policy{
				{Arn: &expectedPolicyArn},
			},
		},
		nil,
	)
	mockAWSClient.EXPECT().DeletePolicy(
		&iam.DeletePolicyInput{
			PolicyArn: &expectedPolicyArn,
		},
	).Return(
		nil, nil,
	)

	err := deleteAccountPolicies(mockAWSClient)
	if err != nil {
		t.Errorf("failed to delete user policies")
	}
}

func TestDeleteAttachedPolicies(t *testing.T) {

	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedPolicyArn := "ExpectedPolicyArn"
	mockAWSClient.EXPECT().ListAttachedUserPolicies(
		&iam.ListAttachedUserPoliciesInput{UserName: &userName},
	).Return(
		&iam.ListAttachedUserPoliciesOutput{
			AttachedPolicies: []iamTypes.AttachedPolicy{
				{
					PolicyArn:  &expectedPolicyArn,
					PolicyName: awsSdk.String("ExpectedPolicyName"),
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

func TestDeleteRoles(t *testing.T) {
	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

	roleName := awsSdk.String("randomRoleName")

	awsRolesOutput := &iam.ListRolesOutput{
		Roles: []iamTypes.Role{
			{
				RoleName: roleName,
			},
		},
	}

	mockAWSClient.EXPECT().ListRoles(gomock.Any()).Return(
		awsRolesOutput,
		nil,
	)

	awsListAttachedRolePolOutput := &iam.ListAttachedRolePoliciesOutput{
		AttachedPolicies: []iamTypes.AttachedPolicy{
			{
				PolicyArn: awsSdk.String("randomPol"),
			},
		},
	}

	mockAWSClient.EXPECT().ListAttachedRolePolicies(gomock.Any()).Return(
		awsListAttachedRolePolOutput,
		nil,
	)

	mockAWSClient.EXPECT().DetachRolePolicy(gomock.Any()).Return(
		&iam.DetachRolePolicyOutput{},
		nil,
	)

	awsDeleteRolesOutput := &iam.DeleteRoleOutput{}
	mockAWSClient.EXPECT().DeleteRole(
		&iam.DeleteRoleInput{
			RoleName: roleName,
		},
	).Return(
		awsDeleteRolesOutput,
		nil,
	)

	o := &accountUnassignOptions{}
	o.awsClient = mockAWSClient
	err := deleteRoles(mockAWSClient)
	if err != nil {
		t.Errorf("failed to delete roles")
	}
}

func TestDeleteGroups(t *testing.T) {

	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	userName := "randuser"

	expectedGroupName := "expectedGroupName"

	mockAWSClient.EXPECT().ListGroupsForUser(
		&iam.ListGroupsForUserInput{UserName: &userName},
	).Return(
		&iam.ListGroupsForUserOutput{
			Groups: []iamTypes.Group{
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

	mocks := setupDefaultMocks(t)

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

	mocks := setupDefaultMocks(t)

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

func TestConflictingOptions(t *testing.T) {
	s := genericclioptions.IOStreams{}
	g := globalflags.GlobalOptions{}
	cmd := newCmdAccountAssign(s, &g)
	o := &accountUnassignOptions{}
	o.payerAccount = "fake account"
	o.accountID = "123456"
	o.username = "testuser"
	err := o.complete(cmd, []string{})
	if err == nil {
		t.Errorf("An error should have been raised")
	}
}
