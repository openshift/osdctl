package org

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/organizations"
	accountsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	allClustersFlag = false
	awsAccountID    = ""
	clustersCmd     = &cobra.Command{
		Use:   "clusters",
		Short: "get all active organization clusters",
		Long: `By default, returns all active clusters for a given organization. The organization can either be specified with an argument
passed in, or by providing both the --aws-profile and --aws-account-id flags. You can request all clusters regardless of status by providing the --all flag.`,
		Example: `Retrieving all active clusters for a given organizational unit:
osdctl org clusters 123456789AbcDEfGHiJklMnopQR

Retrieving all active clusters for a given organizational unit in JSON format:
osdctl org clusters 123456789AbcDEfGHiJklMnopQR -o json

Retrieving all clusters for a given organizational unit regardless of status:
osdctl org clusters 123456789AbcDEfGHiJklMnopQR --all

Retrieving all active clusters for a given AWS profile:
osdctl org clusters --aws-profile my-aws-profile --aws-account-id 123456789`,
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			orgId := ""
			if len(args) > 0 {
				orgId = args[0]
			}

			status := ""
			if !allClustersFlag {
				status = StatusActive
			}

			clusters, err := SearchSubscriptions(orgId, status)
			cmdutil.CheckErr(err)

			out, err := formatClustersOutput(clusters)
			cmdutil.CheckErr(err)
			os.Stdout.Write(out)
		},
	}
)

func init() {
	// define flags
	flags := clustersCmd.Flags()

	flags.BoolVarP(
		&allClustersFlag,
		"all",
		"A",
		false,
		"get all clusters regardless of status",
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

	AddOutputFlag(flags)
}

func SearchSubscriptions(orgId string, status string) ([]*accountsv1.Subscription, error) {
	if orgId == "" && !isAWSProfileSearch() {
		return nil, errors.New("specify either org-id or --aws-profile,--aws-account-id arguments")
	}
	if orgId != "" && isAWSProfileSearch() {
		return nil, errors.New("specify either an org id argument or --aws-profile, --aws-account-id arguments")
	}
	if isAWSProfileSearch() {
		orgIdFromAws, err := getOrganizationIdFromAWSProfile()
		if err != nil {
			return nil, fmt.Errorf("failed to get org ID from AWS profile: %w", err)
		}
		orgId = *orgIdFromAws
	}
	clusterSubscriptions, err := SearchAllSubscriptionsByOrg(orgId, status, false)
	if err != nil {
		return nil, err
	}

	return clusterSubscriptions, nil
}

func getOrganizationIdFromAWSProfile() (*string, error) {
	awsClient, err := initAWSClient(awsProfile)
	if err != nil {
		return nil, fmt.Errorf("could not create AWS client: %q", err)
	}
	parent, err := awsClient.ListParents(&organizations.ListParentsInput{
		ChildId: &awsAccountID,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot get organization parents: %q", err)
	}
	parentId := *parent.Parents[0].Id

	result, err := awsClient.DescribeOrganizationalUnit(
		&organizations.DescribeOrganizationalUnitInput{
			OrganizationalUnitId: &parentId,
		})
	if err != nil {
		return nil, fmt.Errorf("cannot get Organizational Unit: %w", err)
	}

	return result.OrganizationalUnit.Id, nil
}

func formatClustersOutput(items []*accountsv1.Subscription) ([]byte, error) {
	if IsJsonOutput() {
		subs := make([]map[string]string, 0, len(items))
		for _, item := range items {
			subs = append(subs, map[string]string{
				"cluster_id":   item.ClusterID(),
				"external_id":  item.ExternalClusterID(),
				"display_name": item.DisplayName(),
				"status":       item.Status(),
			})
		}
		return json.MarshalIndent(subs, "", "  ")
	} else {
		var buf bytes.Buffer
		table := printer.NewTablePrinter(&buf, 20, 1, 3, ' ')
		table.AddRow([]string{"DISPLAY NAME", "INTERNAL CLUSTER ID", "EXTERNAL CLUSTER ID", "STATUS"})
		for _, s := range items {
			table.AddRow([]string{s.DisplayName(), s.ClusterID(), s.ExternalClusterID(), s.Status()})
		}
		table.AddRow([]string{})
		table.Flush()
		return buf.Bytes(), nil
	}
}

func printClusters(items []*accountsv1.Subscription) {
	out, err := formatClustersOutput(items)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error formatting clusters output: %v\n", err)
		return
	}
	os.Stdout.Write(out)
}

// isAWSProfileSearch indicates if AWS profile flags are set.
func isAWSProfileSearch() bool {
	return awsProfile != "" && awsAccountID != ""
}
