package cost

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go-v2/aws"
	costexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	types2 "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/onsi/gomega"
	"github.com/openshift/osdctl/pkg/provider/aws/mock"
	"go.uber.org/mock/gomock"
)

func TestPrintCostList(t *testing.T) {
	tests := []struct {
		name      string
		cost      decimal.Decimal
		unit      string
		ou        *types.OrganizationalUnit
		ops       *listOptions
		isChild   bool
		expected  string
		expectErr bool
		isJSON    bool
	}{
		{
			name:    "Successful JSON Output",
			cost:    decimal.NewFromFloat(100.50),
			unit:    "USD",
			ou:      &types.OrganizationalUnit{Id: stringToStringPtr("ou-123"), Name: stringToStringPtr("TestOU")},
			ops:     &listOptions{csv: false, output: "json"},
			isChild: true,
			expected: `{
    "ouid": "ou-123",
    "ouname": "TestOU",
    "costUSD": "100.5"
}`,
			expectErr: false,
			isJSON:    true,
		},
		{
			name:      "Successful CSV Output",
			cost:      decimal.NewFromFloat(200.75),
			unit:      "USD",
			ou:        &types.OrganizationalUnit{Id: stringToStringPtr("ou-456"), Name: stringToStringPtr("Finance")},
			ops:       &listOptions{csv: true},
			isChild:   true,
			expected:  "ou-456,Finance,200.75,USD\n",
			expectErr: false,
			isJSON:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			printCostList(tt.cost, tt.unit, tt.ou, tt.ops, tt.isChild)
			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			if tt.isJSON {
				assert.JSONEq(t, tt.expected, output)
			} else {
				assert.Equal(t, tt.expected, output)
			}
		})
	}
}

func stringToStringPtr(s string) *string {
	return &s
}

func TestGetSum(t *testing.T) {
	tests := []struct {
		name      string
		ouCost    OUCost
		expected  decimal.Decimal
		expectErr bool
	}{
		{
			name: "Valid Sum Calculation",
			ouCost: OUCost{
				Costs: []AccountCost{
					{Cost: decimal.NewFromFloat(100.50), Unit: "USD"},
					{Cost: decimal.NewFromFloat(200.25), Unit: "USD"},
				},
			},
			expected:  decimal.NewFromFloat(300.75),
			expectErr: false,
		},
		{
			name: "Different Currency Units Should Fail",
			ouCost: OUCost{
				Costs: []AccountCost{
					{Cost: decimal.NewFromFloat(100.50), Unit: "USD"},
					{Cost: decimal.NewFromFloat(200.25), Unit: "EUR"},
				},
			},
			expected:  decimal.Zero,
			expectErr: true,
		},
		{
			name: "Empty Cost List Should Return Zero",
			ouCost: OUCost{
				Costs: []AccountCost{},
			},
			expected:  decimal.Zero,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sum, _, err := tt.ouCost.getSum()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, sum.Equal(tt.expected), "expected %v, got %v", tt.expected, sum)
			}
		})
	}
}

func TestGetCost(t *testing.T) {
	g := gomega.NewWithT(t)

	t.Run("success case with 2 accounts", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(
			&organizations.ListOrganizationalUnitsForParentOutput{}, nil).AnyTimes()

		mockAWS.EXPECT().ListAccountsForParent(gomock.Any()).Return(
			&organizations.ListAccountsForParentOutput{
				Accounts: []types.Account{
					{Id: aws.String("111111111111")},
					{Id: aws.String("222222222222")},
				},
			}, nil).AnyTimes()

		mockAWS.EXPECT().GetCostAndUsage(gomock.Any()).Return(
			&costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types2.ResultByTime{
					{
						TimePeriod: &types2.DateInterval{
							Start: aws.String("2025-01-01"),
							End:   aws.String("2025-01-31"),
						},
						Total: map[string]types2.MetricValue{
							"NetUnblendedCost": {
								Amount: aws.String("100.00"),
								Unit:   aws.String("USD"),
							},
						},
					},
				},
			}, nil).Times(2)

		ouCost := &OUCost{
			OU: &types.OrganizationalUnit{
				Id:   aws.String("ou-root"),
				Name: aws.String("RootOU"),
			},
			options: &listOptions{
				start: "2025-01-01",
				end:   "2025-01-31",
			},
		}

		err := ouCost.getCost(mockAWS)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(ouCost.Costs).To(gomega.HaveLen(2))
		for _, c := range ouCost.Costs {
			g.Expect(c.Cost.InexactFloat64()).To(gomega.Equal(100.0))
			g.Expect(c.AccountID).To(gomega.BeElementOf("111111111111", "222222222222"))
		}
	})

	t.Run("error: ListOrganizationalUnitsForParent fails", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(nil, errors.New("OU list error"))

		ouCost := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-root")},
			options: &listOptions{
				start: "2025-01-01", end: "2025-01-31",
			},
		}

		err := ouCost.getCost(mockAWS)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("OU list error"))
	})

	t.Run("error: ListAccountsForParent fails", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(
			&organizations.ListOrganizationalUnitsForParentOutput{}, nil)

		mockAWS.EXPECT().ListAccountsForParent(gomock.Any()).Return(nil, errors.New("account list error"))

		ouCost := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-root")},
			options: &listOptions{
				start: "2025-01-01", end: "2025-01-31",
			},
		}

		err := ouCost.getCost(mockAWS)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("account list error"))
	})

	t.Run("error: GetCostAndUsage fails for account", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(
			&organizations.ListOrganizationalUnitsForParentOutput{}, nil)

		mockAWS.EXPECT().ListAccountsForParent(gomock.Any()).Return(
			&organizations.ListAccountsForParentOutput{
				Accounts: []types.Account{{Id: aws.String("111111111111")}},
			}, nil)

		mockAWS.EXPECT().GetCostAndUsage(gomock.Any()).Return(nil, errors.New("cost fetch error"))

		ouCost := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-root")},
			options: &listOptions{
				start: "2025-01-01", end: "2025-01-31",
			},
		}

		err := ouCost.getCost(mockAWS)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("cost fetch error"))
	})

	t.Run("partial cost success: one account fails", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(
			&organizations.ListOrganizationalUnitsForParentOutput{}, nil)

		mockAWS.EXPECT().ListAccountsForParent(gomock.Any()).Return(
			&organizations.ListAccountsForParentOutput{
				Accounts: []types.Account{
					{Id: aws.String("111111111111")},
					{Id: aws.String("222222222222")},
				},
			}, nil)

		mockAWS.EXPECT().GetCostAndUsage(gomock.Any()).DoAndReturn(
			func(input *costexplorer.GetCostAndUsageInput) (*costexplorer.GetCostAndUsageOutput, error) {
				if input.Filter.Dimensions.Values[0] == "111111111111" {
					return &costexplorer.GetCostAndUsageOutput{
						ResultsByTime: []types2.ResultByTime{
							{
								TimePeriod: &types2.DateInterval{
									Start: aws.String("2025-01-01"),
									End:   aws.String("2025-01-31"),
								},
								Total: map[string]types2.MetricValue{
									"NetUnblendedCost": {
										Amount: aws.String("50.00"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					}, nil
				}
				return nil, errors.New("cost fetch error for 222")
			}).Times(2)

		ouCost := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-root")},
			options: &listOptions{
				start: "2025-01-01", end: "2025-01-31",
			},
		}

		err := ouCost.getCost(mockAWS)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(ouCost.Costs).To(gomega.HaveLen(1))
		g.Expect(ouCost.Costs[0].AccountID).To(gomega.Equal("111111111111"))
		g.Expect(ouCost.Costs[0].Cost.InexactFloat64()).To(gomega.Equal(50.0))
	})
}
