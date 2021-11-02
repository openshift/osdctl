package getoutput

import "github.com/spf13/cobra"

func GetOutput(cmd *cobra.Command) (string, error) {

	out, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", err
	}

	return out, nil
}
