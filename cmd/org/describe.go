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
	describeCmd = &cobra.Command{
		Use:           "describe",
		Short:         "describe organization",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(checkOrgId(args))
			cmdutil.CheckErr(describeOrg(cmd, args[0]))
		},
	}
)

func init() {
	flags := describeCmd.Flags()

	AddOutputFlag(flags)
}

func describeOrg(cmd *cobra.Command, orgID string) error {

	response, err := sendDescribeOrgRequest(orgID)
	if err != nil {
		return fmt.Errorf("invalid input: %q", err)
	}

	org := Organization{}
	json.Unmarshal(response.Bytes(), &org)

	printOrg(org)

	return nil
}

func sendDescribeOrgRequest(orgID string) (*sdk.Response, error) {
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
	return sendRequest(createDescribeRequest(ocmClient, orgID))
}

func createDescribeRequest(ocmClient *sdk.Connection, orgID string) *sdk.Request {
	// Create and populate the request:
	request := ocmClient.Get()
	apiPath := organizationsAPIPath + "/" + orgID

	err := arguments.ApplyPathArg(request, apiPath)

	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", apiPath, err)
	}

	return request
}
