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

var currentCmd = &cobra.Command{
	Use:           "current",
	Short:         "gets current organization",
	Args:          cobra.ArbitraryArgs,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(run(cmd))
	},
}

type Account struct {
	Organization Organization `json:"organization"`
}

func run(cmd *cobra.Command) error {
	response, err := GetCurrentOrg()
	if err != nil {
		return fmt.Errorf("invalid input: %q", err)
	}

	acc := Account{}
	json.Unmarshal(response.Bytes(), &acc)
	printOrg(acc.Organization)

	return nil
}

func GetCurrentOrg() (*sdk.Response, error) {
	// Create OCM client to talk
	ocmClient := utils.CreateConnection()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Now get the current org
	return sendRequest(CreateGetCurrentOrgRequest(ocmClient))
}

func CreateGetCurrentOrgRequest(ocmClient *sdk.Connection) *sdk.Request {
	// Create and populate the request:
	request := ocmClient.Get()
	err := arguments.ApplyPathArg(request, currentAccountApiPath)
	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", currentAccountApiPath, err)
	}

	return request
}
