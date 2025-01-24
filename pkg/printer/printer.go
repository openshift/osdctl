package printer

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
)

// Printer use to output something on screen with table format.
type Printer struct {
	w *tabwriter.Writer
}

// NewTablePrinter creates a printer instance, and uses to format output with table.
func NewTablePrinter(o io.Writer, minWidth, tabWidth, padding int, padChar byte) *Printer {
	w := tabwriter.NewWriter(o, minWidth, tabWidth, padding, padChar, 0)
	return &Printer{w}
}

// AddRow adds a row of data.
func (p *Printer) AddRow(row []string) {
	fmt.Fprintln(p.w, strings.Join(row, "\t"))
}

// Flush outputs all rows on screen.
func (p *Printer) Flush() error {
	return p.w.Flush()
}

// ClearScreen clears all output on screen.
func (p *Printer) ClearScreen() {
	fmt.Fprint(os.Stdout, "\033[2J")
	fmt.Fprint(os.Stdout, "\033[H")
}

var PrintfGreen func(format string, a ...interface{}) = color.New(color.FgGreen).PrintfFunc()

var PrintlnGreen func(a ...interface{}) = color.New(color.FgGreen).PrintlnFunc()
