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
	defaultBYOCEnv    = "ou-rs3h-i0v69q47"
	defaultNonBYOCEnv = "ou-0wd6-z6tzkjek"
	defaultRootId     = "r-0wd6"
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
		return cmdutil.UsageErrorf(cmd, "Red Hat username was not provided")
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
		destinationOu   string
		nonDefaultPayer string
		defaultPayer    = "osd-staging-2"
		claimTag        = "Claimed"
		claimTagValue   = "True"
	)

	o.payerAccount = defaultPayer
	if o.username != "" && o.payerAccount != "" {
		nonDefaultPayer = o.payerAccount
	}
	//Instantiate aws client
	awsClient, err := awsprovider.NewAwsClient(o.payerAccount, "us-east-1", "")
	if err != nil {
		return err
	}
	//List accounts that are not in any OU
	input := &organizations.ListAccountsForParentInput{
		ParentId: aws.String(defaultRootId),
	}
	accounts, err := awsClient.ListAccountsForParent(input)
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	//Create input for tagging
	inputTag := &organizations.TagResourceInput{
		ResourceId: aws.String(accountAssignID),
		Tags: []*organizations.Tag{
			{
				Key:   aws.String("Owner"),
				Value: aws.String(o.username),
			},
			{
				Key:   aws.String(claimTag),
				Value: aws.String(claimTagValue),
			},
		},
	}

	//Loop through the list of accounts and get ID
	for _, a := range accounts.Accounts {
		accountAssignID = *a.Id
		//Tag account
		_, err = awsClient.TagResource(inputTag)
		if err != nil {
			return err
		}
		break
	}

	//Move account to developers OU
	inputMove := &organizations.MoveAccountInput{
		AccountId:           aws.String(accountAssignID),
		DestinationParentId: aws.String(destinationOu),
		SourceParentId:      aws.String(defaultRootId),
	}

	if o.payerAccount == defaultPayer {
		destinationOu = defaultBYOCEnv
		_, err = awsClient.MoveAccount(inputMove)
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
	} else if o.payerAccount == nonDefaultPayer {
		destinationOu = defaultNonBYOCEnv
		_, err = awsClient.MoveAccount(inputMove)
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
	}

	if o.output == "" {
		fmt.Fprintln(o.IOStreams.Out, accountAssignID)
	}

	return nil
}
