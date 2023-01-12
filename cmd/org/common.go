package org

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	organizationsAPIPath  = "/api/accounts_mgmt/v1/organizations"
	accountsAPIPath       = "/api/accounts_mgmt/v1/accounts"
	currentAccountApiPath = "/api/accounts_mgmt/v1/current_account"
)

var (
	awsProfile string = ""
	output     string = ""
)

type Organization struct {
	ID           string `json:"id"`
	ExternalID   string `json:"external_id"`
	Name         string `json:"name"`
	EBSAccoundID string `json:"ebs_account_id"`
	Created      string `json:"created_at"`
	Updated      string `json:"updated_at"`
}

func sendRequest(request *sdk.Request) (*sdk.Response, error) {
	response, err := request.Send()
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %q", err)
	}
	return response, nil
}

func checkOrgId(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("organization id was not provided. please provide a organization id")
	}
	if len(args) != 1 {
		return fmt.Errorf("too many arguments. expected 1 got %d", len(args))
	}

	return nil
}

func initAWSClient(awsProfile string) (awsprovider.Client, error) {
	return awsprovider.NewAwsClient(awsProfile, common.DefaultRegion, "")
}

func printOrg(org Organization) {
	// Print org details
	if IsJsonOutput() {
		PrintJson(org)
	} else {
		table := printer.NewTablePrinter(os.Stdout, 20, 1, 2, ' ')
		table.AddRow([]string{"ID:", org.ID})
		table.AddRow([]string{"Name:", org.Name})
		table.AddRow([]string{"External ID:", org.ExternalID})
		table.AddRow([]string{"EBS ID:", org.EBSAccoundID})
		table.AddRow([]string{"Created:", org.Created})
		table.AddRow([]string{"Updated:", org.Updated})

		table.AddRow([]string{})
		table.Flush()
	}
}

func AddOutputFlag(flags *pflag.FlagSet) {

	flags.StringVarP(
		&output,
		"output",
		"o",
		"",
		"valid output formats are ['', 'json']",
	)
}

func IsJsonOutput() bool {
	return output == "json"
}

func PrintJson(data interface{}) {
	marshalledStruct, _ := json.MarshalIndent(data, "", "  ")
	dump.Pretty(os.Stdout, marshalledStruct)
}
