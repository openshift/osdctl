package org

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	currentCmd = &cobra.Command{
		Use:           "current",
		Short:         "gets current organization",
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
		
			org, err := run(cmd, ocmClient)
			if err != nil {
				cmdutil.CheckErr(err)
			}
			printOrg(org) 
		},
	}
)

type Account struct {
	Organization Organization `json:"organization"`
}

func init() {
	flags := currentCmd.Flags()

	AddOutputFlag(flags)
}

func run(cmd *cobra.Command, ocmClient *sdk.Connection) (Organization, error) {
	response, err := getCurrentOrg(ocmClient)
	if err != nil {
		return Organization{}, fmt.Errorf("invalid input: %q", err)
	}

	acc := Account{}
	if err := json.Unmarshal(response.Bytes(), &acc); err != nil {
		return Organization{}, fmt.Errorf("failed to parse response: %w", err)
	}

	return acc.Organization, nil
}

func getCurrentOrg(ocmClient *sdk.Connection) (*sdk.Response, error) {
	return sendRequest(createGetCurrentOrgRequest(ocmClient))
}

func createGetCurrentOrgRequest(ocmClient *sdk.Connection) *sdk.Request {
	request := ocmClient.Get()
	err := arguments.ApplyPathArg(request, currentAccountApiPath)
	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", currentAccountApiPath, err)
	}
	return request
}
