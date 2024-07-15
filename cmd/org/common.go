package org

import (
	"encoding/json"
	"fmt"
	"os"

	accountsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/pkg/utils"

	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/pflag"
)

const (
	organizationsAPIPath  = "/api/accounts_mgmt/v1/organizations"
	accountsAPIPath       = "/api/accounts_mgmt/v1/accounts"
	currentAccountApiPath = "/api/accounts_mgmt/v1/current_account"

	StatusActive = "Active"
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

func checkOrgId(args []string) error {
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

func SearchAllSubscriptionsByOrg(orgID string, status string, managedOnly bool) ([]*accountsv1.Subscription, error) {
	var clusterSubscriptions []*accountsv1.Subscription
	requestPageSize := 100
	morePages := true
	for page := 1; morePages; page++ {
		clustersData, err := getSubscriptions(orgID, status, managedOnly, page, requestPageSize)
		if err != nil {
			return nil, fmt.Errorf("encountered an error fetching subscriptions for page %v: %w", page, err)
		}

		clustersDataItems := clustersData.Items().Slice()
		clusterSubscriptions = append(clusterSubscriptions, clustersDataItems...)

		if clustersData.Size() < requestPageSize {
			morePages = false
		}
	}

	return clusterSubscriptions, nil
}

func getSubscriptions(orgID string, status string, managedOnly bool, page int, size int) (*accountsv1.SubscriptionsListResponse, error) {
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
	response, err := createGetSubscriptionsRequest(ocmClient, orgID, status, managedOnly, page, size).Send()
	if err != nil {
		return nil, fmt.Errorf("failed to get clusters: %w", err)
	}

	return response, nil
}

func createGetSubscriptionsRequest(ocmClient *sdk.Connection, orgID string, status string, managedOnly bool, page int, size int) *accountsv1.SubscriptionsListRequest {
	// Create and populate the request:
	request := ocmClient.AccountsMgmt().V1().Subscriptions().List().Page(page).Size(size)

	searchMessage := fmt.Sprintf(`organization_id='%s'`, orgID)
	if status != "" {
		searchMessage += fmt.Sprintf(` and status='%s'`, status)
	}
	if managedOnly {
		searchMessage += fmt.Sprintf(` and managed=%v`, managedOnly)
	}
	request = request.Search(searchMessage)

	return request
}
