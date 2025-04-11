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
		{
			name:          "Default",
			timePtr:       "",
			expectedStart: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			expectedEnd:   refTime.AddDate(0, 0, 1).Format("2006-01-02"),
		},
		{
			name:          "LM",
			timePtr:       "LM",
			expectedStart: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			expectedEnd:   time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
		},
		{
			name:          "MTD",
			timePtr:       "MTD",
			expectedStart: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			expectedEnd:   refTime.AddDate(0, 0, 1).Format("2006-01-02"),
		},
		{
			name:          "YTD",
			timePtr:       "YTD",
			expectedStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			expectedEnd:   refTime.AddDate(0, 0, 1).Format("2006-01-02"),
		},
		{
			name:          "3M",
			timePtr:       "3M",
			expectedStart: refTime.AddDate(0, -3, 1).Format("2006-01-02"),
			expectedEnd:   refTime.AddDate(0, 0, 1).Format("2006-01-02"),
		},
		{
			name:          "6M",
			timePtr:       "6M",
			expectedStart: refTime.AddDate(0, -6, 1).Format("2006-01-02"),
			expectedEnd:   refTime.AddDate(0, 0, 1).Format("2006-01-02"),
		},
		{
			name:          "1Y",
			timePtr:       "1Y",
			expectedStart: refTime.AddDate(-1, 0, 1).Format("2006-01-02"),
			expectedEnd:   refTime.AddDate(0, 0, 1).Format("2006-01-02"),
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
		g.Expect(output).To(gomega.ContainSubstring(`"ouid": "ou-1234"`))
		g.Expect(output).To(gomega.ContainSubstring(`"ouname": "Dev-Ou"`))
		g.Expect(output).To(gomega.ContainSubstring(`"costUSD": "123.45"`))
	})

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
