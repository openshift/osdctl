package getoutput

import (
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func GetOutput(cmd *cobra.Command) (string, error) {

	out, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", err
	}
	if out != "" && out != "json" && out != "yaml" {
		return "", cmdutil.UsageErrorf(cmd, "Invalid output format: Valid formats are ['', 'json', 'yaml']")
	}
	return out, nil
}
