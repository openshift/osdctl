package org

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	awsAccountsCmd = &cobra.Command{
		Use:           "aws-accounts",
		Short:         "get organization AWS Accounts",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(searchChildAwsAccounts(cmd))
		},
	}
	ouID = ""
)

type AWSAccountItems struct {
	Accounts []types.Child `json:"items"`
}

func init() {
	// define flags
	flags := awsAccountsCmd.Flags()

	flags.StringVarP(
		&awsProfile,
		"aws-profile",
		"p",
		"",
		"specify AWS profile",
	)

	flags.StringVarP(
		&ouID,
		"ou-id",
		"",
		"",
		"specify orgnaization unit id",
	)

	AddOutputFlag(flags)
}

func searchChildAwsAccounts(cmd *cobra.Command) error {
	awsClient, err := initAWSClient(awsProfile)
	if err != nil {
		return fmt.Errorf("could not create AWS client: %q", err)
	}
	children, err := awsClient.ListChildren(&organizations.ListChildrenInput{
		ParentId:  &ouID,
		ChildType: "ACCOUNT",
	})
	if err != nil {
		return fmt.Errorf("cannot get organization children: %q", err)
	}
	printAccounts(children)

	return nil
}

func printAccounts(children *organizations.ListChildrenOutput) {
	if IsJsonOutput() {
		items := AWSAccountItems{
			Accounts: children.Children,
		}
		PrintJson(items)
	} else {
		table := printer.NewTablePrinter(os.Stdout, 20, 1, 2, ' ')
		table.AddRow([]string{"ID", "Type"})
		for _, item := range children.Children {
			table.AddRow([]string{
				*item.Id,
				string(item.Type),
			})
		}

		table.AddRow([]string{})
		table.Flush()
	}

}
