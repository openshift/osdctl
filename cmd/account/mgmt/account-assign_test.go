package mgmt

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/golang/mock/gomock"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestFindUntaggedAccount(t *testing.T) {
	var genericAWSError error = fmt.Errorf("Generic AWS error")

	testData := []struct {
		name              string
		accountsList      []string
		tags              map[string]string
		expectedAccountId string
		expectErr         error
		expectedAWSError  error
	}{
		{
			name:              "test for untagged account present",
			accountsList:      []string{"111111111111"},
			expectedAccountId: "111111111111",
			tags:              map[string]string{},
			expectErr:         nil,
			expectedAWSError:  nil,
		},
		{
			name:              "test for only partially tagged accounts present",
			accountsList:      []string{"111111111111"},
			expectedAccountId: "",
			tags: map[string]string{
				"claimed": "true",
			},
			expectErr:        ErrNoUntaggedAccounts,
			expectedAWSError: nil,
		},
		{
			name:              "test for only tagged accounts present",
			accountsList:      []string{"111111111111"},
			expectedAccountId: "",
			tags: map[string]string{
				"owner":   "randuser",
				"claimed": "true",
			},
			expectErr:        ErrNoUntaggedAccounts,
			expectedAWSError: nil,
		},
		{
			name:              "test for new account created",
			accountsList:      []string{},
			expectedAccountId: "111111111111",
			tags:              nil,
			expectErr:         nil,
			expectedAWSError:  nil,
		},
		{
			name:              "test for AWS list accounts error",
			accountsList:      []string{},
			expectedAccountId: "",
			tags:              nil,
			expectErr:         genericAWSError,
			expectedAWSError:  genericAWSError,
		},
	}

	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {

			mocks := setupDefaultMocks(t, []runtime.Object{})

			mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
			rootOuId := "abc"
			seed := int64(1)

			awsOutputAccounts := &organizations.ListAccountsForParentOutput{}

			if test.accountsList != nil {
				accountsList := []*organizations.Account{}
				for _, a := range test.accountsList {
					account := &organizations.Account{
						Id: aws.String(a),
					}
					accountsList = append(accountsList, account)
				}
				awsOutputAccounts.Accounts = accountsList
			}

			if test.name == "test for new account created" {
				rand.Seed(seed)
				randStr := RandomString(6)
				accountName := "osd-creds-mgmt" + "+" + randStr
				email := accountName + "@redhat.com"
				awsOutputCreate := &organizations.CreateAccountOutput{}
				mockAWSClient.EXPECT().CreateAccount(&organizations.CreateAccountInput{
					AccountName: aws.String(accountName),
					Email:       aws.String(email),
				}).Return(
					awsOutputCreate,
					nil,
				)
			}

			if test.tags != nil {
				awsOutputTags := &organizations.ListTagsForResourceOutput{}
				tags := []*organizations.Tag{}
				for key, value := range test.tags {
					tag := &organizations.Tag{
						Key:   aws.String(key),
						Value: aws.String(value),
					}
					tags = append(tags, tag)
				}
				awsOutputTags.Tags = tags

				mockAWSClient.EXPECT().ListTagsForResource(
					&organizations.ListTagsForResourceInput{
						ResourceId: &test.accountsList[0],
					}).Return(
					awsOutputTags,
					test.expectedAWSError,
				)
			}

			mockAWSClient.EXPECT().ListAccountsForParent(gomock.Any()).Return(
				awsOutputAccounts,
				test.expectedAWSError,
			)

			o := &accountAssignOptions{}
			o.awsClient = mockAWSClient
			returnValue, err := o.findUntaggedAccount(seed, rootOuId)
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
	mocks := setupDefaultMocks(t, []runtime.Object{})

	mockAWSClient := mock.NewMockClient(mocks.mockCtrl)

	seed := int64(1)

	randStr := RandomString(6)
	accountName := "osd-creds-mgmt" + "+" + randStr
	email := accountName + "@redhat.com"

	createId := "car-random1234"

	mockAWSClient.EXPECT().CreateAccount(&organizations.CreateAccountInput{
		AccountName: &accountName,
		Email:       &email,
	}).Return(&organizations.CreateAccountOutput{
		CreateAccountStatus: &organizations.CreateAccountStatus{Id: &createId},
	}, nil)

	awsDescribeOutput := &organizations.DescribeCreateAccountStatusOutput{}

	mockAWSClient.EXPECT().DescribeCreateAccountStatus(&organizations.DescribeCreateAccountStatusInput{
		CreateAccountRequestId: &createId,
	}).Return(awsDescribeOutput, nil)

	o := &accountAssignOptions{}
	o.awsClient = mockAWSClient
	returnVal, err := o.createAccount(seed)
	if err != nil {
		t.Error("failed to create account")
	}
	if returnVal.CreateAccountStatus.State != aws.String("SUCCEEDED") {
		t.Error("failed to create account")
	}
}

func TestTagAccount(t *testing.T) {

	mocks := setupDefaultMocks(t, []runtime.Object{})

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

	o := &accountAssignOptions{}
	o.awsClient = mockAWSClient
	err := o.moveAccount(accountId, destOu, rootOu)
	if err != nil {
		t.Errorf("failed to move account")
	}
}
