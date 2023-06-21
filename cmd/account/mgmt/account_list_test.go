package accmgmt

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"

	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/golang/mock/gomock"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"

	"k8s.io/apimachinery/pkg/runtime"
)

type mocks struct {
	mockCtrl *gomock.Controller
}

func setupDefaultMocks(t *testing.T, localObjects []runtime.Object) *mocks {
	mocks := &mocks{
		mockCtrl: gomock.NewController(t),
	}

	return mocks
}

func TestListUsername(t *testing.T) {
	var genericAWSError error = fmt.Errorf("Generic AWS Error")

	testData := []struct {
		name             string
		expectedUsername string
		tags             map[string]string
		expectErr        error
		expectedAWSError error
	}{
		{
			name:             "test for owner tag present",
			expectedUsername: "tuser",
			expectErr:        nil,
			expectedAWSError: nil,
			tags: map[string]string{
				"owner": "tuser",
			},
		},
		{
			name:             "test for no owner tag present",
			expectedUsername: "",
			expectErr:        ErrNoOwnerTag,
			expectedAWSError: nil,
			tags: map[string]string{
				"claimed":  "true",
				"asldkjfa": "alskdjfaksjd",
			},
		},
		{
			name:             "test for no tags present",
			expectedUsername: "",
			expectErr:        ErrNoTagsOnAccount,
			expectedAWSError: nil,
			tags:             map[string]string{},
		},
		{
			name:             "test for AWS error catching",
			expectedUsername: "",
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

			o := &accountListOptions{}
			o.awsClient = mockAWSClient
			printValue, err := o.listUserName(accountID)
			if test.expectErr != err {
				t.Errorf("expected error %s and got %s", test.expectErr, err)
			}
			if printValue != test.expectedUsername {
				t.Errorf("expected %s is %s", test.expectedUsername, printValue)
			}
		})
	}
}

func TestListAccountsByUser(t *testing.T) {

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
			expectErr:           ErrNoResources,
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

			o := &accountListOptions{}
			o.awsClient = mockAWSClient
			returnValue, err := o.listAccountsByUser(userName)
			if test.expectErr != err {
				t.Errorf("expected error %s and got %s", test.expectErr, err)
			}
			if len(returnValue) != len(test.expectedAccountList) {
				t.Errorf("expected length of accounts list is %s instead of %s", test.expectedAccountList, returnValue)
			}
		})
	}
}

func TestListAllAccounts(t *testing.T) {

	var genericAWSError error = fmt.Errorf("Generic AWS Error")

	testData := []struct {
		name             string
		accountsList     []string
		expectedMap      map[string][]string
		tags             map[string]string
		expectErr        error
		expectedAWSError error
	}{
		{
			name:         "test for accounts present with owner tags",
			accountsList: []string{"111111111111"},
			tags: map[string]string{
				"owner":   "randuser",
				"claimed": "true",
			},
			expectedMap: map[string][]string{
				"randuser": {"111111111111"},
			},
			expectErr:        nil,
			expectedAWSError: nil,
		},
		{
			name:         "test for accounts present without owner tags",
			accountsList: []string{"111111111111"},
			tags: map[string]string{
				"claimed": "true",
			},
			expectedMap:      map[string][]string{},
			expectErr:        ErrAccountsWithNoOwner,
			expectedAWSError: nil,
		},
		{
			name:             "test for accounts present with no tags",
			accountsList:     []string{"111111111111"},
			tags:             map[string]string{},
			expectedMap:      map[string][]string{},
			expectErr:        ErrAccountsWithNoOwner,
			expectedAWSError: nil,
		},
		{
			name:             "test for no accounts present",
			accountsList:     []string{},
			tags:             nil,
			expectedMap:      map[string][]string{},
			expectErr:        ErrNoAccountsForParent,
			expectedAWSError: nil,
		},
		{
			name:             "test for AWS error catching",
			accountsList:     nil,
			tags:             nil,
			expectedMap:      map[string][]string{},
			expectErr:        genericAWSError,
			expectedAWSError: genericAWSError,
		},
	}

	for _, test := range testData {
		t.Run(test.name, func(t *testing.T) {

			mocks := setupDefaultMocks(t, []runtime.Object{})
			mockAWSClient := mock.NewMockClient(mocks.mockCtrl)
			OuId := "ou-abcd-efghlmno"

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

				mockAWSClient.EXPECT().ListTagsForResource(gomock.Any()).Return(
					awsOutputTags,
					test.expectedAWSError,
				)
			}

			mockAWSClient.EXPECT().ListAccountsForParent(gomock.Any()).Return(
				awsOutputAccounts,
				test.expectedAWSError,
			)

			o := &accountListOptions{}
			o.awsClient = mockAWSClient
			returnValue, err := o.listAllAccounts(OuId)
			if test.expectErr != err {
				t.Errorf("expected error %s and got %s", test.expectErr, err)
			}
			if !reflect.DeepEqual(returnValue, test.expectedMap) {
				t.Errorf("expected %s is %s", test.expectedMap, returnValue)
			}
		})
	}
}
