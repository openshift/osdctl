package cost

import (
	"errors"
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	costExplorerTypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	organizationTypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/onsi/gomega"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
	"go.uber.org/mock/gomock"
)

func TestReconcileCostCategories(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testCases := []struct {
		title        string
		setupAWSMock func(r *mock.MockClientMockRecorder)
		OUid         *string
		name         *string
		errExpected  bool
	}{
		{
			title: "ListCostCategoryDefinitions Returns Error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.ListCostCategoryDefinitions(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1)
			},
			OUid:        awsSdk.String("ou-9999-99999999"),
			name:        awsSdk.String("Random OU"),
			errExpected: true,
		},
		{
			title: "ListOrganizationalUnitsForParent Returns Error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListCostCategoryDefinitions(gomock.Any()).Return(
						&costexplorer.ListCostCategoryDefinitionsOutput{
							CostCategoryReferences: []costExplorerTypes.CostCategoryReference{{Name: awsSdk.String("CostCategory1")}},
							NextToken:              awsSdk.String("FakeToken"),
						}, nil).Times(1),

					r.ListCostCategoryDefinitions(gomock.Any()).Return(
						&costexplorer.ListCostCategoryDefinitionsOutput{}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			OUid:        awsSdk.String("ou-9999-99999999"),
			name:        awsSdk.String("Random OU"),
			errExpected: true,
		},
		{
			title: "ListAccountsForParent Returns Error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListCostCategoryDefinitions(gomock.Any()).Return(
						&costexplorer.ListCostCategoryDefinitionsOutput{}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken: awsSdk.String("FakeToken"),
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{
								{
									Id:   awsSdk.String("FakeID"),
									Name: awsSdk.String("FakeName"),
								},
							},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{},
						}, nil).Times(1),

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
			title: "ListAccountsForParent Succeeds",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListCostCategoryDefinitions(gomock.Any()).Return(
						&costexplorer.ListCostCategoryDefinitionsOutput{}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken: awsSdk.String("FakeToken"),
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{
								{
									Id:   awsSdk.String("FakeID"),
									Name: awsSdk.String("FakeName"),
								},
							},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []organizationTypes.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListAccountsForParent(gomock.Any()).Return(&organizations.ListAccountsForParentOutput{}, nil).Times(1),

					r.CreateCostCategoryDefinition(gomock.Any()).Return(nil, nil).Times(1),
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

			err := reconcileCostCategories(OU, mocks.mockAWSClient)

			if tc.errExpected {
				g.Expect(err).Should(gomega.HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
			}
		})
	}
}
