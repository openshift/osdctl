package getoutput

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v2"
)

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
