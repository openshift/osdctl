package printer

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
)

type PrintFlags struct {
	JSONYamlFlags *genericclioptions.JSONYamlPrintFlags
	JSONPathFlags *genericclioptions.JSONPathPrintFlags
}

func NewPrintFlags() *PrintFlags {
	template := ""
	return &PrintFlags{
		JSONYamlFlags: genericclioptions.NewJSONYamlPrintFlags(),
		JSONPathFlags: &genericclioptions.JSONPathPrintFlags{TemplateArgument: &template},
	}
}

func (p *PrintFlags) AddFlags(c *cobra.Command) {
	p.JSONYamlFlags.AddFlags(c)
	p.JSONPathFlags.AddFlags(c)
}

func (p *PrintFlags) ToPrinter(output string) (printers.ResourcePrinter, error) {
	if p, err := p.JSONYamlFlags.ToPrinter(output); !genericclioptions.IsNoCompatiblePrinterError(err) {
		return p, err
	}

	if p, err := p.JSONPathFlags.ToPrinter(output); !genericclioptions.IsNoCompatiblePrinterError(err) {
		return p, err
	}

	return nil, genericclioptions.NoCompatiblePrinterError{OutputFormat: &output, AllowedFormats: p.AllowedFormats()}
}

// AllowedFormats is the list of formats in which data can be displayed
func (p *PrintFlags) AllowedFormats() []string {
	formats := p.JSONYamlFlags.AllowedFormats()
	formats = append(formats, p.JSONPathFlags.AllowedFormats()...)
	return formats
}
