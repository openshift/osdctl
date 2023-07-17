package mgmt

import (
	"fmt"
	"math/rand"
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	organizationTypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/golang/mock/gomock"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
)

func TestIsOwned(t *testing.T) {
	var genericAWSError = fmt.Errorf("Generic AWS error")
	testData := []struct {
		testname         string
		tags             organizations.ListTagsForResourceOutput
		expectedIsOwned  bool
		expectErr        error
		expectedAWSError error
	}{
		{
			testname: "test for unowned account",
			tags: organizations.ListTagsForResourceOutput{
				Tags: []organizationTypes.Tag{},
			},
			expectedIsOwned:  false,
			expectErr:        nil,
			expectedAWSError: nil,
		},
		{
			testname: "test for owned account",
			tags: organizations.ListTagsForResourceOutput{
				Tags: []organizationTypes.Tag{
					{
						Key:   awsSdk.String("claimed"),
						Value: awsSdk.String("true"),
					},
				},
			},
			expectedIsOwned:  true,
			expectErr:        nil,
			expectedAWSError: nil,
		},
		{
			testname: "test for owned account, encounter aws error",
			tags: organizations.ListTagsForResourceOutput{
				Tags: []organizationTypes.Tag{
					{
						Key:   awsSdk.String("claimed"),
						Value: awsSdk.String("true"),
					},
				},
			},
			expectedIsOwned:  false,
			expectErr:        genericAWSError,
			expectedAWSError: genericAWSError,
		},
	}
	for _, test := range testData {
		t.Run(test.testname, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
			accountID := "11111"

			mockAWSClient.EXPECT().ListTagsForResource(
				&organizations.ListTagsForResourceInput{
					ResourceId: &accountID,
				},
			).Return(&test.tags, test.expectedAWSError)

			var awsC awsprovider.Client = mockAWSClient
			isOwned, err := isOwned(accountID, &awsC)

			if isOwned != test.expectedIsOwned {
				t.Errorf("expected isOwned to be %v, got %v", test.expectedIsOwned, isOwned)
			}

			if err != test.expectErr {
				t.Errorf("expected error to be %v, got %v", test.expectErr, err)
			}
		})
	}
}

func TestFindUntaggedAccount(t *testing.T) {
	var genericAWSError = fmt.Errorf("Generic AWS error")

	testData := []struct {
		name                             string
		accountsList                     []string
		tags                             map[string]string
		suspendCheck                     bool
		accountStatus                    string
		callerIdentityAccount            string
		expectedGetCallerIdentityErr     error
		expectedAccountId                string
		expectErr                        error
		expectedListAccountsForParentErr error
	}{
		{
			name:              "test for untagged account present",
			accountsList:      []string{"111111111111"},
			expectedAccountId: "111111111111",
			tags:              map[string]string{},
			suspendCheck:      true,
			accountStatus:     "ACTIVE",
		},
		{
			name:                  "test for only payer account present",
			accountsList:          []string{"222222222222"},
			callerIdentityAccount: "222222222222",
			expectErr:             ErrNoUntaggedAccounts,
		},
		{
			name:         "test for only partially tagged accounts present",
			accountsList: []string{"111111111111"},
			tags: map[string]string{
				"claimed": "true",
			},
			expectErr: ErrNoUntaggedAccounts,
		},
		{
			name:         "test for no untagged accounts present",
			accountsList: []string{},
			expectErr:    ErrNoUntaggedAccounts,
		},
		{
			name:         "test for only tagged accounts present",
			accountsList: []string{"111111111111"},
			tags: map[string]string{
				"owner":   "randuser",
				"claimed": "true",
			},
			expectErr: ErrNoUntaggedAccounts,
		},
		{
			name:                             "test for AWS list accounts error",
			accountsList:                     []string{},
			expectErr:                        genericAWSError,
			expectedListAccountsForParentErr: genericAWSError,
		},
		{
			name:                         "test for AWS get caller identity error",
			accountsList:                 []string{"111111111111"},
			expectErr:                    genericAWSError,
			expectedGetCallerIdentityErr: genericAWSError,
		},
		{
			name:          "test for suspended account error",
			accountsList:  []string{"111111111111"},
			tags:          map[string]string{},
			suspendCheck:  true,
			accountStatus: "SUSPENDED",
			expectErr:     ErrNoUntaggedAccounts,
		},
	}

	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {

			mocks := setupDefaultMocks(t)

			mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
			rootOuId := "abc"
			o := &accountAssignOptions{}
			o.awsClient = mockAWSClient

			awsOutputAccounts := &organizations.ListAccountsForParentOutput{}

			if test.accountsList != nil {
				var accountsList []organizationTypes.Account
				for _, a := range test.accountsList {
					account := organizationTypes.Account{
						Id: &a,
					}
					accountsList = append(accountsList, account)
				}
				awsOutputAccounts.Accounts = accountsList
			}

			if test.tags != nil {
				awsOutputTags := &organizations.ListTagsForResourceOutput{}
				var tags []organizationTypes.Tag
				for key, value := range test.tags {
					tag := organizationTypes.Tag{
						Key:   &key,
						Value: &value,
					}
					tags = append(tags, tag)
				}
				awsOutputTags.Tags = tags

				mockAWSClient.EXPECT().ListTagsForResource(
					&organizations.ListTagsForResourceInput{
						ResourceId: &test.accountsList[0],
					}).Return(
					awsOutputTags,
					test.expectedListAccountsForParentErr,
				)
			}

			if test.suspendCheck {
				mockAWSClient.EXPECT().DescribeAccount(
					&organizations.DescribeAccountInput{
						AccountId: &test.accountsList[0],
					},
				).Return(
					&organizations.DescribeAccountOutput{
						Account: &organizationTypes.Account{
							Id:     &test.accountsList[0],
							Status: organizationTypes.AccountStatus(test.accountStatus),
						},
					}, nil,
				)
			}

			mockAWSClient.EXPECT().ListAccountsForParent(gomock.Any()).Return(
				awsOutputAccounts,
				test.expectedListAccountsForParentErr,
			)

			if test.expectedListAccountsForParentErr == nil && len(test.accountsList) > 0 {
				mockAWSClient.EXPECT().GetCallerIdentity(gomock.Any()).Return(
					&sts.GetCallerIdentityOutput{Account: &test.callerIdentityAccount},
					test.expectedGetCallerIdentityErr,
				)
			}

			returnValue, err := o.findUntaggedAccount(rootOuId)
			if test.expectErr != err {
				t.Errorf("expected error %s and got %s", test.expectErr, err)
			}
			if returnValue != test.expectedAccountId {
				t.Errorf("expected %s is %s", test.expectedAccountId, returnValue)
			}
		})
	}
}

func TestCreateAccount(t *testing.T) {
	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

	seed := int64(1)
	randStr := RandomString(rand.New(rand.NewSource(seed)), 6)
	accountName := "osd-creds-mgmt+" + randStr
	email := accountName + "@redhat.com"

	createId := "car-random1234"

	mockAWSClient.EXPECT().CreateAccount(&organizations.CreateAccountInput{
		AccountName: &accountName,
		Email:       &email,
	}).Return(&organizations.CreateAccountOutput{
		CreateAccountStatus: &organizationTypes.CreateAccountStatus{Id: &createId},
	}, nil)

	expectedOutput := "SUCCEEDED"

	awsDescribeOutput := &organizations.DescribeCreateAccountStatusOutput{
		CreateAccountStatus: &organizationTypes.CreateAccountStatus{
			State: organizationTypes.CreateAccountState(expectedOutput),
		}}

	mockAWSClient.EXPECT().DescribeCreateAccountStatus(&organizations.DescribeCreateAccountStatusInput{
		CreateAccountRequestId: &createId,
	}).Return(awsDescribeOutput, nil)

	o := &accountAssignOptions{}
	o.awsClient = mockAWSClient
	returnVal, err := o.createAccount(seed)
	if err != nil {
		t.Error("failed to create account")
	}
	if returnVal.CreateAccountStatus.State != organizationTypes.CreateAccountState(expectedOutput) {
		t.Error("failed to create account")
	}
}

func TestTagAccount(t *testing.T) {

	mocks := setupDefaultMocks(t)

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
	accountID := "111111111111"

	awsOutputTag := &organizations.TagResourceOutput{}

	mockAWSClient.EXPECT().TagResource(gomock.Any()).Return(
		awsOutputTag,
		nil,
	)

	o := &accountAssignOptions{}
	o.awsClient = mockAWSClient
	err := o.tagAccount(accountID)
	if err != nil {
		t.Errorf("failed to tag account")
	}
}

func TestMoveAccount(t *testing.T) {

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

	o := &accountAssignOptions{}
	o.awsClient = mockAWSClient
	err := o.moveAccount(accountId, destOu, rootOu)
	if err != nil {
		t.Errorf("failed to move account")
	}
}
