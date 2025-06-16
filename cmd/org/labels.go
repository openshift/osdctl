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

var (
	labelsCmd = &cobra.Command{
		Use:           "labels",
		Short:         "get organization labels",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			ocmClient, err := utils.CreateConnection()
			if err != nil {
				cmdutil.CheckErr(err)
			}
			defer func() {
				if err := ocmClient.Close(); err != nil {
					fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
				}
			}()
			cmdutil.CheckErr(checkOrgId(args))
			cmdutil.CheckErr(searchLabelsByOrg(cmd, args[0], ocmClient))
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

func init() {
	flags := labelsCmd.Flags()

	AddOutputFlag(flags)
}

func searchLabelsByOrg(cmd *cobra.Command, orgID string, ocmClient *sdk.Connection) error {

	response, err := sendRequest(createGetLabelsRequest(ocmClient, orgID))
	if err != nil {
		return fmt.Errorf("invalid input: %q", err)
	}

	items := LabelItems{}
	json.Unmarshal(response.Bytes(), &items)

	printLabels(items.Labels)

	return nil
}

func getLabels(orgID string) (*sdk.Response, error) {
	// Create OCM client to talk
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Now get the matching orgs
	return sendRequest(createGetLabelsRequest(ocmClient, orgID))
}

func createGetLabelsRequest(ocmClient *sdk.Connection, orgID string) *sdk.Request {
	// Create and populate the request:
	request := ocmClient.Get()
	labelsApiPath := organizationsAPIPath + "/" + orgID + "/labels"

	err := arguments.ApplyPathArg(request, labelsApiPath)

	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", labelsApiPath, err)

	}

	return request
}

func printLabels(items []Label) {
	if IsJsonOutput() {
		lables := LabelItems{
			Labels: items,
		}
		PrintJson(lables)
	} else {
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
	}

}
