package org

import (
	"fmt"
	"os"

	sdk "github.com/openshift-online/ocm-sdk-go"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	customersCmd = &cobra.Command{
		Use:           "customers",
		Short:         "get paying/non-paying organizations",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(_ *cobra.Command, _ []string) {
			ocmClient, err := utils.CreateConnection()
			if err != nil {
				cmdutil.CheckErr(err)
			}
			defer func() {
				if err := ocmClient.Close(); err != nil {
					fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
				}
			}()

			customers, err := getCustomers(ocmClient)
			if err != nil {
				cmdutil.CheckErr(err)
			}
			printCustomers(customers)
		},
	}
	paying   bool   = true
	subsType string = "Subscription"
)

type CustomerItems struct {
	Customers []Customer `json:"items"`
}

type Customer struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization-id"`
	SKU            string `json:"sku"`
}

func init() {
	// define flags
	flags := customersCmd.Flags()

	flags.BoolVarP(
		&paying,
		"paying",
		"",
		true,
		"get organization based on paying status",
	)

	AddOutputFlag(flags)
}

func getCustomers(ocmClient *sdk.Connection) ([]Customer, error) {
	pageSize := 1000
	pageIndex := 1

	if !paying {
		subsType = "Config"
	}

	searchQuery := fmt.Sprintf("type='%s'", subsType)
	var customerList []Customer

	for {
		response, err := ocmClient.AccountsMgmt().V1().ResourceQuota().List().
			Size(pageSize).
			Page(pageIndex).
			Parameter("search", searchQuery).
			Send()
		if err != nil {
			return nil, fmt.Errorf("can't retrieve accounts: %v", err)
		}

		response.Items().Each(func(resourseQuota *amv1.ResourceQuota) bool {
			customer := Customer{
				ID:             resourseQuota.ID(),
				OrganizationID: resourseQuota.OrganizationID(),
				SKU:            resourseQuota.SKU(),
			}
			customerList = append(customerList, customer)
			return true
		})

		if response.Size() < pageSize {
			break
		}
		pageIndex++
	}

	return customerList, nil
}

func printCustomers(items []Customer) {
	if IsJsonOutput() {
		customers := CustomerItems{
			Customers: items,
		}
		PrintJson(customers)
	} else {
		table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
		table.AddRow([]string{"ID", "OrganizationID", "SKU"})

		for _, customer := range items {
			table.AddRow([]string{
				customer.ID,
				customer.OrganizationID,
				customer.SKU,
			})
		}

		table.AddRow([]string{})
		table.Flush()
	}
}
