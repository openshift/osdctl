package printer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v2"

	"k8s.io/apimachinery/pkg/runtime"
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

func FormatOutput(w io.Writer, format string, obj runtime.Object) error {
	var (
		bytes []byte
		err   error
	)
	switch format {
	case "yaml":
		bytes, err = yaml.Marshal(obj)
	case "json":
		bytes, err = json.Marshal(obj)
	default:
		return fmt.Errorf("Unsupported format %s\n", format)
	}

	if err != nil {
		return err
	}

	fmt.Fprintf(w, string(bytes))
	return nil
}
