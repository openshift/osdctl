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

var customersCmd = &cobra.Command{
	Use:           "customers",
	Short:         "Lists customers of the current organization",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		ocmClient, err := utils.CreateConnection()
		if err != nil {
			cmdutil.CheckErr(err)
		}
		defer func() {
			if err := ocmClient.Close(); err != nil {
				cmdutil.CheckErr(fmt.Errorf("cannot close ocmClient: %v", err))
			}
		}()

		orgReq, err := getOrgRequest(ocmClient)
		if err != nil {
			cmdutil.CheckErr(err)
		}
		orgResp, err := sendRequest(orgReq)
		if err != nil {
			cmdutil.CheckErr(fmt.Errorf("failed to fetch current organization: %v", err))
		}
		org, err := getCurrentOrg(orgResp.Bytes())
		if err != nil {
			cmdutil.CheckErr(err)
		}

		customersReq, err := getCustomersRequest(ocmClient, org.ID)
		if err != nil {
			cmdutil.CheckErr(err)
		}
		customersResp, err := sendRequest(customersReq)
		if err != nil {
			cmdutil.CheckErr(fmt.Errorf("failed to fetch customers: %v", err))
		}
		customers, err := parseCustomers(customersResp.Bytes())
		if err != nil {
			cmdutil.CheckErr(err)
		}

		printCustomers(customers)
	},
}

type Customer struct {
	ID           string `json:"id"`
	ClusterCount int    `json:"cluster_count"`
}

type CustomersList struct {
	Items []Customer `json:"items"`
}

func init() {
	flags := customersCmd.Flags()
	AddOutputFlag(flags)
}

func getCustomersRequest(ocmClient *sdk.Connection, orgID string) (*sdk.Request, error) {
	req := ocmClient.Get()
	path := fmt.Sprintf("/api/accounts_mgmt/v1/organizations/%s/customers", orgID)
	err := arguments.ApplyPathArg(req, path)
	if err != nil {
		return nil, fmt.Errorf("cannot apply path '%s': %v", path, err)
	}
	return req, nil
}

func parseCustomers(data []byte) ([]Customer, error) {
	var customersList CustomersList
	err := json.Unmarshal(data, &customersList)
	if err != nil {
		return nil, fmt.Errorf("cannot parse customers JSON: %v", err)
	}
	return customersList.Items, nil
}

func printCustomers(customers []Customer) {
	for _, customer := range customers {
		fmt.Printf("Customer ID: %s, Cluster Count: %d\n", customer.ID, customer.ClusterCount)
	}
}
