package cost

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

var mockNowFunc = time.Now

func TestGetTimePeriod(t *testing.T) {
	refTime := time.Now()
	mockNowFunc = func() time.Time { return refTime }

	tests := []struct {
		name          string
		timePtr       string
		expectedStart string
		expectedEnd   string
	}{
		{
			name:          "Default",
			timePtr:       "",
			expectedStart: fmt.Sprintf("%d-%02d-%02d", refTime.Year()-1, refTime.Month(), 01),
			expectedEnd:   fmt.Sprintf("%d-%02d-%02d", refTime.Year(), refTime.Month(), refTime.Day()),
		},
		{
			name:          "LM",
			timePtr:       "LM",
			expectedStart: fmt.Sprintf("%d-%02d-%02d", refTime.Year(), refTime.Month()-1, 01),
			expectedEnd:   fmt.Sprintf("%d-%02d-%02d", refTime.Year(), refTime.Month(), 01),
		},
		{
			name:          "MTD",
			timePtr:       "MTD",
			expectedStart: fmt.Sprintf("%d-%02d-%02d", refTime.Year(), refTime.Month(), 01),
			expectedEnd:   fmt.Sprintf("%d-%02d-%02d", refTime.Year(), refTime.Month(), refTime.Day()),
		},
		{
			name:          "YTD",
			timePtr:       "YTD",
			expectedStart: fmt.Sprintf("%d-%02d-%02d", refTime.Year(), 01, 01),
			expectedEnd:   fmt.Sprintf("%d-%02d-%02d", refTime.Year(), refTime.Month(), refTime.Day()),
		},
		{
			name:          "1Y",
			timePtr:       "1Y",
			expectedStart: refTime.AddDate(-1, 0, 0).Format("2006-01-02"),
			expectedEnd:   fmt.Sprintf("%d-%02d-%02d", refTime.Year(), refTime.Month(), refTime.Day()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end := getTimePeriod(&tc.timePtr)
			require.Equal(t, tc.expectedStart, start)
			require.Equal(t, tc.expectedEnd, end)
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
