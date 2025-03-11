package org

import (
	"encoding/json"
	"fmt"

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
		Run: func(_ *cobra.Command, _ []string) {
			ocmClient, err := utils.CreateConnection()
			if err != nil {
				cmdutil.CheckErr(err)
			}
			defer func() {
				if err := ocmClient.Close(); err != nil {
					cmdutil.CheckErr(fmt.Errorf("cannot close the ocmClient (possible memory leak): %q", err))
				}
			}()
			req, err := getOrgRequest(ocmClient)
			if err != nil {
				cmdutil.CheckErr(err)
			}
			resp, err := sendRequest(req)
			if err != nil {
				cmdutil.CheckErr(fmt.Errorf("invalid input: %q", err))
			}
			orgs, err := getCurrentOrg(resp.Bytes())
			if err != nil {
				cmdutil.CheckErr(err)
			}
			printOrg(*orgs)
		},
	}
)

type account struct {
	Organization Organization `json:"organization"`
}

func init() {
	flags := currentCmd.Flags()
	AddOutputFlag(flags)
}

func getCurrentOrg(data []byte) (*Organization, error) {
	acc := account{}
	err := json.Unmarshal(data, &acc)
	if err != nil {
		return nil, err
	}
	return &acc.Organization, nil
}

func getOrgRequest(ocmClient *sdk.Connection) (*sdk.Request, error) {
	req := ocmClient.Get()
	err := arguments.ApplyPathArg(req, currentAccountApiPath)
	if err != nil {
		return nil, fmt.Errorf("can't parse API path '%s': %v\n", currentAccountApiPath, err)
	}
	return req, nil
}