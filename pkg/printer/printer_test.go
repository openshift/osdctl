package printer

import (
	"bytes"
	"io"
	"testing"

	. "github.com/onsi/gomega"
)

func TestAddRow(t *testing.T) {
	g := NewGomegaWithT(t)

	testCases := []struct {
		title  string
		rows   [][]string
		output string
	}{
		{
			title:  "one row and one column",
			rows:   [][]string{{"foo"}},
			output: "foo\n",
		},
		{
			title:  "one row and three columns",
			rows:   [][]string{{"foo", "bar", "buz"}},
			output: "foo                 bar                 buz\n",
		},
		{
			title: "two rows and three columns",
			rows:  [][]string{{"foo", "bar", "buz"}, {"foo1", "foo2", "foo3"}},
			output: `foo                 bar                 buz
foo1                foo2                foo3
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			buf := &bytes.Buffer{}
			p := NewTablePrinter(buf, 20, 1, 3, ' ')
			for _, row := range tc.rows {
				p.AddRow(row)
			}
			err := p.Flush()
			g.Expect(err).ShouldNot(HaveOccurred())

			data, err := io.ReadAll(buf)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(string(data)).Should(Equal(tc.output))
		})
	}
}
