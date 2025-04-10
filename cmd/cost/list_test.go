package cost

import (
	"bytes"
	"errors"
	"os"
	"sort"
	"testing"

	//"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go-v2/aws"
	costexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	types2 "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/onsi/gomega"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
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
		isJSON    bool // New flag to check if output is JSON
	}{
		{
			name:    "Successful JSON Output",
			cost:    decimal.NewFromFloat(100.50),
			unit:    "USD",
			ou:      &types.OrganizationalUnit{Id: stringToStringptr("ou-123"), Name: stringToStringptr("TestOU")},
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
			ou:        &types.OrganizationalUnit{Id: stringToStringptr("ou-456"), Name: stringToStringptr("Finance")},
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
				assert.Equal(t, tt.expected, output) // Correct assertion for CSV
			}
		})
	}
}

func stringToStringptr(s string) *string {
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
func TestListCostsUnderOU(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	t.Run("success case with CSV output", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		// Recursive mocking of ListOrganizationalUnitsForParent
		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(
			&organizations.ListOrganizationalUnitsForParentOutput{
				OrganizationalUnits: []types.OrganizationalUnit{
					{Id: aws.String("ou-1"), Name: aws.String("Child1")},
					{Id: aws.String("ou-2"), Name: aws.String("Child2")},
				},
			}, nil).Times(1)

		// No more child OUs
		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(
			&organizations.ListOrganizationalUnitsForParentOutput{
				OrganizationalUnits: []types.OrganizationalUnit{},
			}, nil).AnyTimes()

		// Accounts under OUs
		mockAWS.EXPECT().ListAccountsForParent(gomock.Any()).Return(
			&organizations.ListAccountsForParentOutput{
				Accounts: []types.Account{
					{Id: aws.String("123")},
					{Id: aws.String("456")},
				},
			}, nil).AnyTimes()

		// Cost response
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
								Amount: aws.String("123.45"),
								Unit:   aws.String("USD"),
							},
						},
					},
				},
			}, nil).AnyTimes()

		options := &listOptions{
			start: "2025-01-01",
			end:   "2025-01-31",
			csv:   true,
		}
		rootOU := &types.OrganizationalUnit{
			Id:   aws.String("ou-root"),
			Name: aws.String("RootOU"),
		}

		err := listCostsUnderOU(rootOU, mockAWS, options)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	})

	t.Run("error when listing OUs", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		// Fail immediately
		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(nil, errors.New("ListOU error")).AnyTimes()

		options := &listOptions{
			start: "2025-01-01",
			end:   "2025-01-31",
			csv:   true,
		}
		rootOU := &types.OrganizationalUnit{
			Id:   aws.String("ou-root"),
			Name: aws.String("RootOU"),
		}

		err := listCostsUnderOU(rootOU, mockAWS, options)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("ListOU error"))
	})

	t.Run("error when listing accounts", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		// Return no child OUs
		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(
			&organizations.ListOrganizationalUnitsForParentOutput{
				OrganizationalUnits: []types.OrganizationalUnit{},
			}, nil).AnyTimes()

		// Fail account listing
		mockAWS.EXPECT().ListAccountsForParent(gomock.Any()).Return(nil, errors.New("ListAccounts error")).AnyTimes()

		options := &listOptions{
			start: "2025-01-01",
			end:   "2025-01-31",
			csv:   true,
		}
		rootOU := &types.OrganizationalUnit{
			Id:   aws.String("ou-root"),
			Name: aws.String("RootOU"),
		}

		err := listCostsUnderOU(rootOU, mockAWS, options)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("ListAccounts error"))
	})

	t.Run("error when getting cost and usage", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockAWS := mock.NewMockClient(mockCtrl)

		// No child OUs
		mockAWS.EXPECT().ListOrganizationalUnitsForParent(gomock.Any()).Return(
			&organizations.ListOrganizationalUnitsForParentOutput{
				OrganizationalUnits: []types.OrganizationalUnit{},
			}, nil).AnyTimes()

		// Return dummy accounts
		mockAWS.EXPECT().ListAccountsForParent(gomock.Any()).Return(
			&organizations.ListAccountsForParentOutput{
				Accounts: []types.Account{
					{Id: aws.String("123")},
				},
			}, nil).AnyTimes()

		// Cost fetch fails
		mockAWS.EXPECT().GetCostAndUsage(gomock.Any()).Return(nil, errors.New("CostUsage error")).AnyTimes()

		options := &listOptions{
			start: "2025-01-01",
			end:   "2025-01-31",
			csv:   true,
		}
		rootOU := &types.OrganizationalUnit{
			Id:   aws.String("ou-root"),
			Name: aws.String("RootOU"),
		}

		err := listCostsUnderOU(rootOU, mockAWS, options)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("CostUsage error"))
	})
}

func getCostTestVersion(
	o *OUCost,
	mockAccounts func(*types.OrganizationalUnit, awsprovider.Client) ([]*string, error),
	mockCost func(account *string, unit *string, awsClient awsprovider.Client, cost *decimal.Decimal) error,
	awsClient awsprovider.Client,
) error {
	accounts, err := mockAccounts(o.OU, awsClient)
	if err != nil {
		return err
	}

	for _, account := range accounts {
		accCost := AccountCost{
			AccountID: *account,
			Unit:      "",
			Cost:      decimal.Zero,
		}
		err = mockCost(account, &accCost.Unit, awsClient, &accCost.Cost)
		if err != nil {
			return err
		}
		o.Costs = append(o.Costs, accCost)
	}

	sort.Slice(o.Costs, func(i, j int) bool {
		return o.Costs[j].Cost.LessThan(o.Costs[i].Cost)
	})

	return nil
}

func TestGetCost(t *testing.T) {
	g := gomega.NewWithT(t)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockAWS := mock.NewMockClient(mockCtrl)

	t.Run("success case", func(t *testing.T) {
		o := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-abc")},
			options: &listOptions{
				start: "2025-01-01",
				end:   "2025-01-31",
				time:  "monthly",
			},
		}

		mockAccounts := func(_ *types.OrganizationalUnit, _ awsprovider.Client) ([]*string, error) {
			return []*string{aws.String("111"), aws.String("222")}, nil
		}

		mockCost := func(account *string, unit *string, _ awsprovider.Client, cost *decimal.Decimal) error {
			if *account == "111" {
				*unit = "USD"
				*cost = decimal.NewFromFloat(120.00)
			} else {
				*unit = "USD"
				*cost = decimal.NewFromFloat(80.00)
			}
			return nil
		}

		err := getCostTestVersion(o, mockAccounts, mockCost, mockAWS)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(o.Costs).To(gomega.HaveLen(2))
		g.Expect(o.Costs[0].AccountID).To(gomega.Equal("111")) // Sorted by descending cost
		g.Expect(o.Costs[1].AccountID).To(gomega.Equal("222"))
	})

	t.Run("error from getAccountsRecursive", func(t *testing.T) {
		o := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-err")},
			options: &listOptions{
				start: "2025-01-01",
				end:   "2025-01-31",
				time:  "monthly",
			},
		}

		mockAccounts := func(_ *types.OrganizationalUnit, _ awsprovider.Client) ([]*string, error) {
			return nil, errors.New("mock account fetch error")
		}

		mockCost := func(account *string, unit *string, _ awsprovider.Client, cost *decimal.Decimal) error {
			return nil
		}

		err := getCostTestVersion(o, mockAccounts, mockCost, mockAWS)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("mock account fetch error"))
	})

	t.Run("error from getAccountCost", func(t *testing.T) {
		o := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-cost")},
			options: &listOptions{
				start: "2025-01-01",
				end:   "2025-01-31",
				time:  "monthly",
			},
		}

		mockAccounts := func(_ *types.OrganizationalUnit, _ awsprovider.Client) ([]*string, error) {
			return []*string{aws.String("999")}, nil
		}

		mockCost := func(account *string, unit *string, _ awsprovider.Client, cost *decimal.Decimal) error {
			return errors.New("mock cost error")
		}

		err := getCostTestVersion(o, mockAccounts, mockCost, mockAWS)
		g.Expect(err).To(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("mock cost error"))
	})

	t.Run("empty account list", func(t *testing.T) {
		o := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-empty")},
			options: &listOptions{
				start: "2025-01-01",
				end:   "2025-01-31",
				time:  "monthly",
			},
		}

		mockAccounts := func(_ *types.OrganizationalUnit, _ awsprovider.Client) ([]*string, error) {
			return []*string{}, nil
		}

		mockCost := func(account *string, unit *string, _ awsprovider.Client, cost *decimal.Decimal) error {
			return nil
		}

		err := getCostTestVersion(o, mockAccounts, mockCost, mockAWS)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(o.Costs).To(gomega.HaveLen(0))
	})

	t.Run("account with zero cost", func(t *testing.T) {
		o := &OUCost{
			OU: &types.OrganizationalUnit{Id: aws.String("ou-zero")},
			options: &listOptions{
				start: "2025-01-01",
				end:   "2025-01-31",
				time:  "monthly",
			},
		}

		mockAccounts := func(_ *types.OrganizationalUnit, _ awsprovider.Client) ([]*string, error) {
			return []*string{aws.String("000")}, nil
		}

		mockCost := func(account *string, unit *string, _ awsprovider.Client, cost *decimal.Decimal) error {
			*unit = "USD"
			*cost = decimal.Zero
			return nil
		}

		err := getCostTestVersion(o, mockAccounts, mockCost, mockAWS)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(o.Costs).To(gomega.HaveLen(1))
		g.Expect(o.Costs[0].Cost.IsZero()).To(gomega.BeTrue())
	})
}
