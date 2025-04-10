package cost

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// Overrideable time function for mocking time.Now
var mockNowFunc = time.Now

func TestGetTimePeriod(t *testing.T) {
	refTime := time.Date(2025, 4, 10, 0, 0, 0, 0, time.UTC)
	mockNowFunc = func() time.Time { return refTime }

	tests := []struct {
		name          string
		timePtr       string
		expectedStart string
		expectedEnd   string
	}{
		{"Default", "", "2024-04-01", "2025-04-10"},
		{"LM", "LM", "2025-03-01", "2025-04-01"},
		{"MTD", "MTD", "2025-04-01", "2025-04-10"},
		{"YTD", "YTD", "2025-01-01", "2025-04-10"},
		{"3M", "3M", "2025-01-10", "2025-04-10"},
		{"6M", "6M", "2024-10-10", "2025-04-10"},
		{"1Y", "1Y", "2024-04-10", "2025-04-10"},
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

	// CASE 1: CSV Output
	t.Run("CSV output", func(t *testing.T) {
		r, w, _ := os.Pipe()
		originalStdout := os.Stdout
		os.Stdout = w

		opts := &getOptions{csv: true}
		o := &getOptions{}

		err := o.printCostGet(cost, unit, opts, ou)
		g.Expect(err).ToNot(gomega.HaveOccurred())

		w.Close()
		os.Stdout = originalStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)

		expected := fmt.Sprintf("\n%s,%s,%s\n\n", *ou.Name, cost.StringFixed(2), unit)
		g.Expect(buf.String()).To(gomega.Equal(expected))
	})

	// CASE 2: Recursive output
	t.Run("Recursive output", func(t *testing.T) {
		r, w, _ := os.Pipe()
		originalStdout := os.Stdout
		os.Stdout = w

		opts := &getOptions{recursive: true}
		o := &getOptions{}

		err := o.printCostGet(cost, unit, opts, ou)
		g.Expect(err).ToNot(gomega.HaveOccurred())

		w.Close()
		os.Stdout = originalStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)

		output := buf.String()
		g.Expect(output).To(gomega.ContainSubstring("Cost of all accounts under OU:"))
		g.Expect(output).To(gomega.ContainSubstring("OuId: ou-1234"))
		g.Expect(output).To(gomega.ContainSubstring("OuName: Dev-Ou"))
		g.Expect(output).To(gomega.ContainSubstring("Cost: 123.45"))
	})

	// CASE 3: JSON Output
	t.Run("JSON output", func(t *testing.T) {
		r, w, _ := os.Pipe()
		originalStdout := os.Stdout
		os.Stdout = w

		opts := &getOptions{output: "json"}
		o := &getOptions{output: "json"}

		err := o.printCostGet(cost, unit, opts, ou)
		g.Expect(err).ToNot(gomega.HaveOccurred())

		w.Close()
		os.Stdout = originalStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)

		output := buf.String()
		// Validate JSON output matches exact lowercase struct tags
		g.Expect(output).To(gomega.ContainSubstring(`"ouid": "ou-1234"`))
		g.Expect(output).To(gomega.ContainSubstring(`"ouname": "Dev-Ou"`))
		g.Expect(output).To(gomega.ContainSubstring(`"costUSD": "123.45"`))
	})

	// CASE 4: YAML Output
	t.Run("YAML output", func(t *testing.T) {
		r, w, _ := os.Pipe()
		originalStdout := os.Stdout
		os.Stdout = w

		opts := &getOptions{output: "yaml"}
		o := &getOptions{output: "yaml"}

		err := o.printCostGet(cost, unit, opts, ou)
		g.Expect(err).ToNot(gomega.HaveOccurred())

		w.Close()
		os.Stdout = originalStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)

		output := buf.String()
		g.Expect(output).To(gomega.ContainSubstring("ouid: ou-1234"))
		g.Expect(output).To(gomega.ContainSubstring("ouname: Dev-Ou"))
		g.Expect(output).To(gomega.ContainSubstring("costUSD: \"123.45\""))
	})

}
