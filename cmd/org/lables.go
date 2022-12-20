package org

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	orgAPIPath = "/api/accounts_mgmt/v1/organizations/"
)

var (
	lablesCmd = &cobra.Command{
		Use:           "lables",
		Short:         "get oraganization lables",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(checkLablesOrgId(cmd, args))
			cmdutil.CheckErr(SearchLabelsByOrg(cmd, args[0]))
		},
	}
)

type LabelItems struct {
	Labels []Label `json:"items"`
}

type Label struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func checkLablesOrgId(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		err := cmd.Help()
		if err != nil {
			return fmt.Errorf("error calling cmd.Help(): %w", err)

		}
		return fmt.Errorf("oraganization id was not provided. please provide a oraganization id")
	}

	return nil
}

func SearchLabelsByOrg(cmd *cobra.Command, orgID string) error {

	response, err := GetLabels(orgID)
	if err != nil {
		// If the response has errored, likely the input was bad, so show usage
		err := cmd.Help()
		if err != nil {
			return err
		}
		return err
	}

	items := LabelItems{}
	json.Unmarshal(response.Bytes(), &items)

	err = printLables(items.Labels)

	if err != nil {
		// If outputing the data errored, there's likely an internal error, so just return the error
		return err
	}

	return nil
}

func GetLabels(orgID string) (*sdk.Response, error) {
	// Create OCM client to talk
	ocmClient := utils.CreateConnection()
	ocmClient.AccountsMgmt().V1()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Now get the matching orgs
	return sendRequest(CreateGetLabelsRequest(ocmClient, orgID))
}

func CreateGetLabelsRequest(ocmClient *sdk.Connection, orgID string) *sdk.Request {
	// Create and populate the request:
	request := ocmClient.Get()
	labelsApiPath := orgAPIPath + orgID + "/labels"

	err := arguments.ApplyPathArg(request, labelsApiPath)

	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", getAPIPath, err)

	}

	return request
}

func printLables(items []Label) (err error) {
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"ID", "KEY", "VALUE"})

	for _, label := range items {
		table.AddRow([]string{
			label.ID,
			label.Key,
			label.Value,
		})
	}

	table.AddRow([]string{})
	table.Flush()
	return nil

}
