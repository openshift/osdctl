package cost

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestGetTimePeriod(t *testing.T) {
	refTime := time.Now()
	defaultStart := fmt.Sprintf("%d-%02d-01", refTime.Year()-1, refTime.Month())
	defaultEnd := refTime.Format("2006-01-02")
	prevMonth := refTime.AddDate(0, -1, 0)
	var expected3MStart string
	if refTime.Month() > 3 {
		expected3MStart = refTime.AddDate(0, -3, 0).Format("2006-01-02")
	} else {
		expected3MStart = refTime.AddDate(-1, 9, 0).Format("2006-01-02")
	}
	monthNum, _ := strconv.Atoi(refTime.Format("01"))
	var expected6MStart string
	if monthNum > 6 {
		expected6MStart = refTime.AddDate(0, -6, 0).Format("2006-01-02")
	} else {
		expected6MStart = refTime.AddDate(-1, 6, 0).Format("2006-01-02")
	}

	tests := []struct {
		name          string
		timePtr       string
		expectedStart string
		expectedEnd   string
		expectDefault bool
	}{
		{
			name:          "Default empty input",
			timePtr:       "",
			expectedStart: defaultStart,
			expectedEnd:   defaultEnd,
		},
		{
			name:          "Last Month (LM)",
			timePtr:       "LM",
			expectedStart: fmt.Sprintf("%d-%02d-01", prevMonth.Year(), prevMonth.Month()),
			expectedEnd:   fmt.Sprintf("%d-%02d-01", refTime.Year(), refTime.Month()),
		},
		{
			name:          "Month to date (MTD)",
			timePtr:       "MTD",
			expectedStart: fmt.Sprintf("%d-%02d-01", refTime.Year(), refTime.Month()),
			expectedEnd:   refTime.Format("2006-01-02"),
		},
		{
			name:          "Year to date (YTD)",
			timePtr:       "YTD",
			expectedStart: fmt.Sprintf("%d-01-01", refTime.Year()),
			expectedEnd:   refTime.Format("2006-01-02"),
		},
		{
			name:          "Last 1 year (1Y)",
			timePtr:       "1Y",
			expectedStart: refTime.AddDate(-1, 0, 0).Format("2006-01-02"),
			expectedEnd:   refTime.Format("2006-01-02"),
		},
		{
			name:          "Last 3 months (3M)",
			timePtr:       "3M",
			expectedStart: expected3MStart,
			expectedEnd:   refTime.Format("2006-01-02"),
		},
		{
			name:          "Last 6 months (6M)",
			timePtr:       "6M",
			expectedStart: expected6MStart,
			expectedEnd:   refTime.Format("2006-01-02"),
		},
		{
			name:          "Lowercase mtd",
			timePtr:       "mtd",
			expectedStart: defaultStart,
			expectedEnd:   defaultEnd,
			expectDefault: true,
		},
		{
			name:          "Mixed case yTd",
			timePtr:       "yTd",
			expectedStart: defaultStart,
			expectedEnd:   defaultEnd,
			expectDefault: true,
		},

		{
			name:          "Invalid string",
			timePtr:       "asdfadsfdd",
			expectedStart: defaultStart,
			expectedEnd:   defaultEnd,
			expectDefault: true,
		},
		{
			name:          "Unsupported relative range (3Y)",
			timePtr:       "3Y",
			expectedStart: defaultStart,
			expectedEnd:   defaultEnd,
			expectDefault: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end := getTimePeriod(&tc.timePtr)

			if tc.expectDefault {
				require.Equal(t, defaultStart, start, "should fallback to default start")
				require.Equal(t, defaultEnd, end, "should fallback to default end")
			} else {
				require.Equal(t, tc.expectedStart, start)
				require.Equal(t, tc.expectedEnd, end)
			}
		})
	}
}

func Test_printCostGet(t *testing.T) {
	g := gomega.NewWithT(t)

	cost := decimal.NewFromFloat(123.45)
	unit := "USD"
	ou := &types.OrganizationalUnit{
		Id:   aws.String("ou-1234"),
		Name: aws.String("Dev-Ou"),
	}

	t.Run("CSV output", func(t *testing.T) {

		opts := &getOptions{csv: true}
		o := &getOptions{}

		err := o.printCostGet(cost, unit, opts, ou)
		g.Expect(err).ToNot(gomega.HaveOccurred())

	})

	t.Run("Recursive output", func(t *testing.T) {

		opts := &getOptions{recursive: true}
		o := &getOptions{}

		err := o.printCostGet(cost, unit, opts, ou)
		g.Expect(err).ToNot(gomega.HaveOccurred())

	})

	t.Run("JSON output", func(t *testing.T) {

		opts := &getOptions{output: "json"}
		o := &getOptions{output: "json"}

		err := o.printCostGet(cost, unit, opts, ou)
		g.Expect(err).ToNot(gomega.HaveOccurred())

	})

	t.Run("YAML output", func(t *testing.T) {

		opts := &getOptions{output: "yaml"}
		o := &getOptions{output: "yaml"}

		err := o.printCostGet(cost, unit, opts, ou)
		g.Expect(err).ToNot(gomega.HaveOccurred())

	})
}
