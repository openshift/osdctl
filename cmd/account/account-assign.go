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

// Global variables
var (
	OSDStaging2RootID = "r-rs3h"
	OSDStaging2OuID   = "ou-rs3h-i0v69q47"
	OSDStaging1RootID = "r-0wd6"
	OSDStaging1OuID   = "ou-0wd6-z6tzkjek"
)

// assignCmd assigns an aws account to user under osd-staging-2 by default unless osd-staging-1 is specified
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
	accountAssignCmd.Flags().StringVarP(&ops.username, "username", "u", "", "LDAP username")

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
		return cmdutil.UsageErrorf(cmd, "LDAP username was not provided")
	}
	if o.payerAccount == "" {
		return cmdutil.UsageErrorf(cmd, "Payer account was not provided")
	}

	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	return nil
}

func (o *accountAssignOptions) run() error {

	var (
		accountAssignID string
		destinationOU   string
		rootID          string
	)

	if o.username != "" && o.payerAccount == "osd-staging-2" {
		rootID = OSDStaging2RootID
		destinationOU = OSDStaging2OuID
	} else if o.username != "" && o.payerAccount == "osd-staging-1" {
		rootID = OSDStaging1RootID
		destinationOU = OSDStaging1OuID
	}
	//Instantiate aws client
	awsClient, err := awsprovider.NewAwsClient(o.payerAccount, "us-east-1", "")
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	//List accounts that are not in any OU
	input := &organizations.ListAccountsInput{}
	accounts, err := awsClient.ListAccounts(input)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	if len(accounts.Accounts) == 0 {
		return fmt.Errorf("No accounts available to assign")
	}

	//Get one account and tag it
	a := accounts.Accounts[0]
	accountAssignID = *a.Id
	//Create input for tagging
	inputTag := &organizations.TagResourceInput{
		ResourceId: aws.String(accountAssignID),
		Tags: []*organizations.Tag{
			{
				Key:   aws.String("owner"),
				Value: aws.String(o.username),
			},
			{
				Key:   aws.String("claimed"),
				Value: aws.String("true"),
			},
		},
	}
	
	_, err = awsClient.TagResource(inputTag)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	//Move account to developers OU
	inputMove := &organizations.MoveAccountInput{
		AccountId:           aws.String(accountAssignID),
		DestinationParentId: aws.String(destinationOU),
		SourceParentId:      aws.String(rootID),
	}
	
	_, err = awsClient.MoveAccount(inputMove)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	
	if o.output == "" {
		fmt.Fprintln(o.IOStreams.Out, accountAssignID)
	}

	return nil
}
