package getoutput

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func GetOutput(cmd *cobra.Command) (string, error) {

	out, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", err
	}
	if out != "" && out != "json" && out != "yaml" && out != "env" {
		return "", cmdutil.UsageErrorf(cmd, "Invalid output format: Valid formats are ['', 'json', 'yaml', 'env']")
	}
	return out, nil
}

type CmdResponse interface {
	String() string
}

func PrintResponse(output string, resp CmdResponse) error {
	if output == "json" {

		accountsToJson, err := json.MarshalIndent(resp, "", "    ")
		if err != nil {
			return err
		}

		fmt.Println(string(accountsToJson))

	} else if output == "yaml" {

		accountIdToYaml, err := yaml.Marshal(resp)
		if err != nil {
			return err
		}

		fmt.Println(string(accountIdToYaml))

	} else {
		fmt.Println(resp)
	}
	return nil
}
