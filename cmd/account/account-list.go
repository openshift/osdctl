package account

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func newCmdAccountList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newAccountListOptions(streams, flags)
	accountListCmd := &cobra.Command{
		Use:               "list",
		Short:             "List out accounts for username",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	ops.printFlags.AddFlags(accountListCmd)
	accountListCmd.Flags().StringVarP(&ops.output, "output", "o", "", "Output format. One of: json|yaml|jsonpath=...|jsonpath-file=... see jsonpath template [http://kubernetes.io/docs/user-guide/jsonpath].")
	accountListCmd.Flags().StringVarP(&ops.username, "user", "u", "", "RH username")
	accountListCmd.Flags().StringVarP(&ops.payerAccount, "payer-account", "p", "", "Payer account type")
	accountListCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "Account ID")

	return accountListCmd
}

type accountListOptions struct {
	username     string
	payerAccount string
	accountID    string
	output       string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newAccountListOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *accountListOptions {
	return &accountListOptions{
		flags:      flags,
		printFlags: printer.NewPrintFlags(),
		IOStreams:  streams,
	}
}

func (o *accountListOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.payerAccount == "" {
		return cmdutil.UsageErrorf(cmd, "Payer account was not provided")
	}
	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}
	return nil
}

func (o *accountListOptions) run() error {

	var (
		tempAccountIDs []string
		user           string
		OuID           string
	)
	// Instantiate Aws client
	awsClient, err := awsprovider.NewAwsClient(o.payerAccount, "us-east-1", "")
	if err != nil {
		return err
	}
	if o.payerAccount == "osd-staging-2" {
		OuID = OSDStaging2OuID
	} else if o.payerAccount == "osd-staging-1" {
		OuID = OSDStaging1OuID
	}
	input := &organizations.ListAccountsForParentInput{
		ParentId: &OuID,
	}
	accounts, err := awsClient.ListAccountsForParent(input)
	if err != nil {
		return err
	}
	// Initialize hashmap
	m := make(map[string][]string)
	if o.username == "" {
		// Loop through list of accounts and build hashmap of users and accounts
		for _, a := range accounts.Accounts {
			inputListTags := &organizations.ListTagsForResourceInput{
				ResourceId: a.Id,
			}
			tagVal, err := awsClient.ListTagsForResource(inputListTags)
			if err != nil {
				return err
			}
			for _, t := range tagVal.Tags {
				if t.Key == aws.String("owner") {
					user = *t.Value
				}
			}
			// Check if user is already in the hashmap
			// If yes append to list of accounts
			// If no create list of accounts and add it as value to key
			_, ok := m[user]
			if ok {
				_ = append(m[user], *a.Id)
			} else {
				tempAccountIDs = append(tempAccountIDs, *a.Id)
				m[user] = tempAccountIDs
			}
		}
	} else if o.username != "" {
		user = o.username
		// Create input to list the accounts from a specific user
		inputFilterTag := &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				{
					Key: aws.String("owner"),
					Values: []*string{
						aws.String(user),
					},
				},
			},
		}
		accounts, err := awsClient.GetResources(inputFilterTag)
		if err != nil {
			return err
		}
		// Get last 9 digits of ResourceARN and append it to account list
		for _, a := range accounts.ResourceTagMappingList {
			tempAccountIDs = append(tempAccountIDs, (*a.ResourceARN)[len(*a.ResourceARN)-9:])
		}
		if err != nil {
			return err
		}
		m[user] = tempAccountIDs
	}
	if o.accountID != "" {
		input := &organizations.ListTagsForResourceInput{
			ResourceId: &o.accountID,
		}
		val, err := awsClient.ListTagsForResource(input)
		if err != nil {
			return err
		}
		for _, t := range val.Tags {
			if t.Key == aws.String("owner") {
				fmt.Fprintln(o.IOStreams.Out, *t.Value)
			}
		}
	}

	if o.output == "" {
		for key, value := range m {
			fmt.Fprintln(o.IOStreams.Out, key, value)
		}
	}
	return nil
}
