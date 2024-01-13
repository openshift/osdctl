package cost

import (
	"errors"
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	organizationTypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
)

type mockSuite struct {
	mockCtrl      *gomock.Controller
	mockAWSClient *mock.MockClient
}

func setupDefaultMocks(t *testing.T) *mockSuite {
	mocks := &mockSuite{
		mockCtrl: gomock.NewController(t),
	}

	mocks.mockAWSClient = mock.NewMockClient(mocks.mockCtrl)
	return mocks
}

func TestCreateCostCategory(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testCases := []struct {
		title        string
		setupAWSMock func(r *mock.MockClientMockRecorder)
		OUid         *string
		name         *string
		errExpected  bool
	}{
		{
			title: "ListOrganizationalUnitsForParent Returns Error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.ListOrganizationalUnitsForParent(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1)
			},
			OUid:        awsSdk.String("ou-9999-99999999"),
			name:        awsSdk.String("Random OU"),
			errExpected: true,
		},
		{
			title: "ListAccountsForParent Returns Error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListAccountsForParent(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			OUid:        awsSdk.String("ou-9999-99999999"),
			name:        awsSdk.String("Random OU"),
			errExpected: true,
		},
		{
			title: "CreateCostCategoryDefinition Fails",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListAccountsForParent(gomock.Any()).Return(
						&organizations.ListAccountsForParentOutput{
							NextToken: nil,
							Accounts:  []organizationTypes.Account{},
						}, nil).Times(1),

					r.CreateCostCategoryDefinition(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			OUid:        awsSdk.String("ou-9999-99999999"),
			name:        awsSdk.String("Random OU"),
			errExpected: true,
		},
		{
			title: "CreateCostCategoryDefinition Succeeds",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListAccountsForParent(gomock.Any()).Return(
						&organizations.ListAccountsForParentOutput{
							NextToken: nil,
							Accounts: []organizationTypes.Account{
								{
									Id:   awsSdk.String("Account ID"),
									Name: awsSdk.String("Account Name"),
								},
							},
						}, nil).Times(1),

					r.CreateCostCategoryDefinition(gomock.Any()).Return(
						&costexplorer.CreateCostCategoryDefinitionOutput{
							CostCategoryArn: awsSdk.String("Placeholder Arn"),
							EffectiveStart:  awsSdk.String("Placeholder Start"),
						}, nil).Times(1),
				)
			},
			OUid:        awsSdk.String("ou-9999-99999999"),
			name:        awsSdk.String("Random OU"),
			errExpected: false,
		},
	}

	//Go through test cases
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			defer mocks.mockCtrl.Finish()

			OU := &organizationTypes.OrganizationalUnit{Id: tc.OUid, Name: tc.name}

			err := createCostCategory(tc.OUid, OU, mocks.mockAWSClient)

			if tc.errExpected {
				g.Expect(err).Should(gomega.HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
			}
		})
	}
}
