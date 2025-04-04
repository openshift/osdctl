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
	describeCmd = &cobra.Command{
		Use:           "describe",
		Short:         "describe organization",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(_ *cobra.Command, args []string) {
			ocmClient, err := utils.CreateConnection()
			if err != nil {
				cmdutil.CheckErr(err)
			}
			defer func() {
				if err := ocmClient.Close(); err != nil {
					cmdutil.CheckErr(fmt.Errorf("cannot close the ocmClient (possible memory leak): %q", err))
				}
			}()

			if err := checkOrgId(args); err != nil {
				cmdutil.CheckErr(err)
			}

			req, err := getDescribeOrgRequest(ocmClient, args[0])
			if err != nil {
				cmdutil.CheckErr(err)
			}

			resp, err := sendRequest(req)
			if err != nil {
				cmdutil.CheckErr(fmt.Errorf("invalid input: %q", err))
			}

			org, err := describeOrg(resp.Bytes())
			if err != nil {
				cmdutil.CheckErr(err)
			}

			printOrg(*org)
		},
	}
)

func init() {
	flags := describeCmd.Flags()
	AddOutputFlag(flags)
}

func describeOrg(data []byte) (*Organization, error) {
	org := Organization{}
	err := json.Unmarshal(data, &org)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func getDescribeOrgRequest(ocmClient *sdk.Connection, orgID string) (*sdk.Request, error) {
	req := ocmClient.Get()
	apiPath := organizationsAPIPath + "/" + orgID
	err := arguments.ApplyPathArg(req, apiPath)
	if err != nil {
		return nil, fmt.Errorf("can't parse API path '%s': %v\n", apiPath, err)
	}
	return req, nil
}
