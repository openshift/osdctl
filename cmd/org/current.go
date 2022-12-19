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

const (
	targetAPIPath = "/api/accounts_mgmt/v1/current_account"
)

var currentCmd = &cobra.Command{
	Use:           "current",
	Short:         "gets current oraganization",
	Args:          cobra.ArbitraryArgs,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(run(cmd))
	},
}

type Account struct {
	Organization Organization `json:"organization"`
}

type Organization struct {
	ID         string `json:"id"`
	ExternalId string `json:"external_id"`
	Name       string `json:"name"`
}

func run(cmd *cobra.Command) error {
	response, err := GetCurrentOrg()
	if err != nil {
		// If the response has errored, likely the input was bad, so show usage
		err := cmd.Help()
		if err != nil {
			return err
		}
		return err
	}

	acc := Account{}
	json.Unmarshal(response.Bytes(), &acc)
	printCurrentOrg(acc)

	if err != nil {
		// If outputing the data errored, there's likely an internal error, so just return the error
		return err
	}
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
	err := arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", targetAPIPath, err)

	}

	return request
}

func printCurrentOrg(acc Account) (err error) {
	// Print current go details
	fmt.Println("Name: \t \t ", acc.Organization.Name)
	fmt.Println("ID:  \t \t ", acc.Organization.ID)
	fmt.Println("External ID:  \t ", acc.Organization.ExternalId)
	return nil

}
