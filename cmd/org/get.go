package org

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	USER_SEARCH = 1
	EBS_SEARCH  = 2
)

var (
	getCmd = &cobra.Command{
		Use:           "get",
		Short:         "get organization by users",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(searchOrgs(cmd))
		},
	}
	searchEBSaccountID string
	searchUser         string
	seachLikePrepend   string
	searchLikeAppend   string = "%"
	isPartMatch        bool   = false
)

type OrgItems struct {
	Orgs []Organization `json:"items"`
}

type AccountItems struct {
	AccountItems []AccountItem `json:"items"`
}

type AccountItem struct {
	Org Organization `json:"organization"`
}

func init() {
	// define flags
	flags := getCmd.Flags()

	flags.StringVarP(
		&searchUser,
		"user",
		"u",
		"",
		"search organization by user name ",
	)
	flags.StringVarP(
		&searchEBSaccountID,
		"ebs-id",
		"",
		"",
		"search organization by ebs account id ",
	)
	flags.BoolVarP(
		&isPartMatch,
		"part-match",
		"",
		false,
		"Part matching user name",
	)
	AddOutputFlag(flags)

	getCmd.MarkFlagsMutuallyExclusive("user", "ebs-id")

}

func searchOrgs(cmd *cobra.Command) error {
	if searchUser == "" && searchEBSaccountID == "" {
		return fmt.Errorf("invalid search params")
	}

	response, err := getOrgs()

	if err != nil {
		return fmt.Errorf("invalid input: %q", err)
	}

	var orgList []Organization
	switch getSearchType() {

	case USER_SEARCH:
		items := AccountItems{}
		json.Unmarshal(response.Bytes(), &items)
		for _, account := range items.AccountItems {
			orgList = append(orgList, account.Org)
		}

	case EBS_SEARCH:
		items := OrgItems{}
		json.Unmarshal(response.Bytes(), &items)
		orgList = items.Orgs
	}

	printOrgList(orgList)

	return nil
}

func getOrgs() (*sdk.Response, error) {
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
	request := ocmClient.Get()
	apiPath := ""
	switch getSearchType() {

	case USER_SEARCH:
		apiPath = accountsAPIPath

	case EBS_SEARCH:
		apiPath = organizationsAPIPath
	}
	if err := arguments.ApplyPathArg(request, apiPath); err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", apiPath, err)
	}

	arguments.ApplyParameterFlag(request, []string{getSearchQuery()})
	return sendRequest(request)
}

func getSearchQuery() string {
	searchQuery := ""

	switch getSearchType() {
	case USER_SEARCH:
		if isPartMatch {
			seachLikePrepend = "%"
		}

		searchQuery = fmt.Sprintf(
			`search=username like '%s%s%s'`,
			seachLikePrepend,
			searchUser,
			searchLikeAppend,
		)
	case EBS_SEARCH:
		searchQuery = fmt.Sprintf(
			`search=ebs_account_id='%s'`,
			searchEBSaccountID,
		)
	}
	return searchQuery
}

func printOrgList(orgs []Organization) {

	if IsJsonOutput() {
		items := OrgItems{
			Orgs: orgs,
		}
		PrintJson(items)
	} else {
		table := printer.NewTablePrinter(os.Stdout, 20, 1, 6, ' ')
		table.AddRow([]string{"ID", "Name", "External ID", "EBS ID"})

		for _, org := range orgs {
			table.AddRow([]string{
				org.ID,
				org.Name,
				org.ExternalID,
				org.EBSAccoundID,
			})
		}

		table.AddRow([]string{})
		table.Flush()
	}

}

func getSearchType() int {
	if searchUser != "" {
		return USER_SEARCH
	}

	if searchEBSaccountID != "" {
		return EBS_SEARCH
	}

	return 0
}
