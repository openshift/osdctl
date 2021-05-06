package cost

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"

	//"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
	"testing"
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
			OUid:        aws.String("ou-9999-99999999"),
			name:        aws.String("Random OU"),
			errExpected: true,
		},
		{
			title: "ListOrganizationalUnitsForParent Returns Error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListCostCategoryDefinitions(gomock.Any()).Return(
						&costexplorer.ListCostCategoryDefinitionsOutput{
							CostCategoryReferences: []*costexplorer.CostCategoryReference{{Name: aws.String("CostCategory1")}},
							NextToken:              aws.String("FakeToken"),
						}, nil).Times(1),

					r.ListCostCategoryDefinitions(gomock.Any()).Return(
						&costexplorer.ListCostCategoryDefinitionsOutput{}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			OUid:        aws.String("ou-9999-99999999"),
			name:        aws.String("Random OU"),
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
							NextToken: aws.String("FakeToken"),
							OrganizationalUnits: []*organizations.OrganizationalUnit{
								{
									Id:   aws.String("FakeID"),
									Name: aws.String("FakeName"),
								},
							},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []*organizations.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []*organizations.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []*organizations.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListAccountsForParent(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			OUid:        aws.String("ou-9999-99999999"),
			name:        aws.String("Random OU"),
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
							NextToken: aws.String("FakeToken"),
							OrganizationalUnits: []*organizations.OrganizationalUnit{
								{
									Id:   aws.String("FakeID"),
									Name: aws.String("FakeName"),
								},
							},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []*organizations.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []*organizations.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListOrganizationalUnitsForParent(gomock.Any()).Return(
						&organizations.ListOrganizationalUnitsForParentOutput{
							NextToken:           nil,
							OrganizationalUnits: []*organizations.OrganizationalUnit{},
						}, nil).Times(1),

					r.ListAccountsForParent(gomock.Any()).Return(&organizations.ListAccountsForParentOutput{}, nil).Times(1),

					r.CreateCostCategoryDefinition(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			OUid:        aws.String("ou-9999-99999999"),
			name:        aws.String("Random OU"),
			errExpected: false,
		},
	}

	//Go through test cases
	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			defer mocks.mockCtrl.Finish()

			OU := &organizations.OrganizationalUnit{Id: tc.OUid, Name: tc.name}

			err := reconcileCostCategories(OU, mocks.mockAWSClient)

			if tc.errExpected {
				g.Expect(err).Should(gomega.HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
			}
		})
	}
}
