package org

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const (
	statusActive = "Active"
)

var (
	clustersCmd = &cobra.Command{
		Use:           "clusters",
		Short:         "get organization clusters",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(SearchClusters(cmd, args))
		},
	}
	onlyActive   bool   = false
	awsProfile   string = ""
	awsAccountID string = ""
)

type SubscriptionItems struct {
	Subscriptions []Subscription `json:"items"`
}

type Subscription struct {
	ClusterID   string `json:"cluster_id"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

func init() {
	// define flags
	flags := clustersCmd.Flags()

	flags.BoolVarP(
		&onlyActive,
		"active",
		"",
		false,
		"get organization active clusters",
	)

	flags.StringVarP(
		&awsProfile,
		"aws-profile",
		"p",
		"",
		"specify AWS profile",
	)

	flags.StringVarP(
		&awsAccountID,
		"aws-account-id",
		"a",
		"",
		"specify AWS Account Id",
	)
}

func SearchClusters(cmd *cobra.Command, args []string) error {

	var err error
	if !hasOrgId(args) && !isAWSProfileSearch() {
		err = fmt.Errorf("specify either org-id or --aws-profile,--aws-account-id arguments")
	}
	if hasOrgId(args) {
		err = searchclustersByOrg(cmd, args[0])
	}
	if isAWSProfileSearch() {
		err = searchClustersByAWSProfile(cmd)
	}

	if err != nil {
		return err
	}
	return nil
}

func initAWSClients() (awsprovider.Client, error) {
	return awsprovider.NewAwsClient(awsProfile, common.DefaultRegion, "")
}

func searchClustersByAWSProfile(cmd *cobra.Command) error {
	awsClient, err := initAWSClients()
	if err != nil {
		return fmt.Errorf("could not create AWS client: %q", err)
	}
	parent, err := awsClient.ListParents(&organizations.ListParentsInput{
		ChildId: aws.String(awsAccountID),
	})
	if err != nil {
		return fmt.Errorf("cannot get organization parents: %q", err)
	}
	parentId := *parent.Parents[0].Id

	result, err := awsClient.DescribeOrganizationalUnit(
		&organizations.DescribeOrganizationalUnitInput{
			OrganizationalUnitId: aws.String(parentId),
		})

	if err != nil {
		log.Fatalln("cannot get Organizational Unit:", err)
	}

	searchclustersByOrg(cmd, *result.OrganizationalUnit.Id)

	return nil
}

func searchclustersByOrg(cmd *cobra.Command, orgID string) error {

	response, err := getClusters(orgID)
	if err != nil {
		return fmt.Errorf("invalid input: %q", err)
	}

	items := SubscriptionItems{}
	json.Unmarshal(response.Bytes(), &items)

	printClusters(items.Subscriptions)
	if err != nil {
		// If outputing the data errored, there's likely an internal error, so just return the error
		return err
	}
	return nil
}

func getClusters(orgID string) (*sdk.Response, error) {
	// Create OCM client to talk
	ocmClient := utils.CreateConnection()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Now get the matching orgs
	return sendRequest(createGetClustersRequest(ocmClient, orgID))
}

func createGetClustersRequest(ocmClient *sdk.Connection, orgID string) *sdk.Request {
	// Create and populate the request:
	request := ocmClient.Get()
	subscriptionApiPath := "/api/accounts_mgmt/v1/subscriptions"

	err := arguments.ApplyPathArg(request, subscriptionApiPath)

	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", subscriptionApiPath, err)

	}

	formatMessage := fmt.Sprintf(
		`search=organization_id='%s'`,
		orgID,
	)
	arguments.ApplyParameterFlag(request, []string{formatMessage})

	return request
}

func printClusters(items []Subscription) {
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"DISPLAY NAME", "CLUSTER ID", "STATUS"})

	for _, subscription := range items {
		if subscription.Status != statusActive && onlyActive {
			// skip non active clusters when --active flag set
			continue
		}
		table.AddRow([]string{
			subscription.DisplayName,
			subscription.ClusterID,
			subscription.Status,
		})
	}

	table.AddRow([]string{})
	table.Flush()
}

func hasOrgId(args []string) bool {
	return len(args) == 1
}

func isAWSProfileSearch() bool {
	return awsProfile != "" && awsAccountID != ""
}
