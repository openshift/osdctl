package printer

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// printer use to output something on screen with table format.
type printer struct {
	w *tabwriter.Writer
}

// NewTablePrinter creates a printer instance, and uses to format output with table.
func NewTablePrinter(o io.Writer, minWidth, tabWidth, padding int, padChar byte) *printer {
	w := tabwriter.NewWriter(o, minWidth, tabWidth, padding, padChar, 0)
	return &printer{w}
}

// AddRow adds a row of data.
func (p *printer) AddRow(row []string) {
	fmt.Fprintln(p.w, strings.Join(row, "\t"))
}

// Flush outputs all rows on screen.
func (p *printer) Flush() error {
	return p.w.Flush()
}

// ClearScreen clears all output on screen.
func (p *printer) ClearScreen() {
	fmt.Fprint(os.Stdout, "\033[2J")
	fmt.Fprint(os.Stdout, "\033[H")
}
