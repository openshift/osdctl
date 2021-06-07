package account

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// assignCmd represents the assign command
func newCmdAccountAssign(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newAccountAssignOptions(streams, flags)
	accountAssignCmd := &cobra.Command{
		Use:               "assign",
		Short:             "Assign account to user",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	ops.printFlags.AddFlags(accountAssignCmd)
	accountAssignCmd.Flags().StringVarP(&ops.output, "output", "o", "", "Output format. One of: json|yaml|jsonpath=...|jsonpath-file=... see jsonpath template [http://kubernetes.io/docs/user-guide/jsonpath].")
	accountAssignCmd.Flags().StringVarP(&ops.payerAccount, "payer-account", "p", "", "Payer account type")

	return accountAssignCmd
}

type accountAssignOptions struct {
	username     string
	payerAccount string
	output       string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newAccountAssignOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *accountAssignOptions {
	return &accountAssignOptions{
		flags:      flags,
		printFlags: printer.NewPrintFlags(),
		IOStreams:  streams,
	}
}

func (o *accountAssignOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.username == "" {
		return cmdutil.UsageErrorf(cmd, "RedHat username was not provided")
	}

	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func (o *accountAssignOptions) run() error {

	var (
		accountAssignID string
	)

	if o.username != "" && o.payerAccount == "" {
		o.payerAccount = "osd-staging-2"
	}
	//Instantiating aws client
	AwsClient, err := awsprovider.NewAwsClient(o.payerAccount, "us-east-1", "")
	if err != nil {
		return err
	}
	//Listing accounts
	input := &organizations.ListAccountsInput{}
	accounts, err := AwsClient.ListAccounts(input)
	if err != nil {
		fmt.Println(err.Error())
	}

	//Lopping through the list to find an unassigned account
	for _, a := range accounts.Accounts {
		if *a.Status == "UNASSIGNED" {
			accountAssignID = *a.Id
			break
		}
		if err != nil {
			return err
		}
	}

	//Tagging the account with username
	inputTag := &organizations.TagResourceInput{
		ResourceId: aws.String(accountAssignID),
		Tags: []*organizations.Tag{
			{
				Key:   aws.String(accountAssignID),
				Value: aws.String(o.username),
			},
		},
	}

	_, err = AwsClient.TagResource(inputTag)
	if err != nil {
		return err
	}

	//Moving to developers OU
	inputMove1 := &organizations.MoveAccountInput{

		AccountId:           aws.String(accountAssignID),
		DestinationParentId: aws.String("ou-0wd6-z6tzkjek"),
		SourceParentId:      aws.String("r-0wd6"),
	}
	inputMove2 := &organizations.MoveAccountInput{
		AccountId:           aws.String(accountAssignID),
		DestinationParentId: aws.String("ou-rs3h-i0v69q47"),
		SourceParentId:      aws.String("r-0wd6"),
	}

	if o.payerAccount == "osd-staging-2" {
		_, err = AwsClient.MoveAccount(inputMove2)
		if err != nil {
			return err
		}
	} else if o.payerAccount == "osd-staging-1" {
		_, err = AwsClient.MoveAccount(inputMove1)
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	if o.output == "" {
		fmt.Fprintln(o.IOStreams.Out, accountAssignID)
	}

	return nil
}
