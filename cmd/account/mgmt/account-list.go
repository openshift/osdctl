package mgmt

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	outputflag "github.com/openshift/osdctl/cmd/getoutput"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type accountListOptions struct {
	awsClient    awsprovider.Client
	m            map[string][]string
	username     string
	payerAccount string
	accountID    string
	output       string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

type listResponse struct {
	Username string   `json:"username" yaml:"username"`
	Accounts []string `json:"accounts" yaml:"accounts"`
}

func (f listResponse) String() string {

	return fmt.Sprintf("  Username: %s\n  Accounts: %s\n", f.Username, f.Accounts)

}

func newAccountListOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *accountListOptions {
	return &accountListOptions{
		flags:         flags,
		printFlags:    printer.NewPrintFlags(),
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func newCmdAccountList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newAccountListOptions(streams, flags, globalOpts)
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
	accountListCmd.Flags().StringVarP(&ops.username, "user", "u", "", "LDAP username")
	accountListCmd.Flags().StringVarP(&ops.payerAccount, "payer-account", "p", "", "Payer account type")
	accountListCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "Account ID")

	return accountListCmd
}

func (o *accountListOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.payerAccount == "" {
		return cmdutil.UsageErrorf(cmd, "Payer account was not provided")
	}
	if o.username != "" && o.accountID != "" {
		return cmdutil.UsageErrorf(cmd, "Cannot provide both username and account ID")
	}

	o.output = o.GlobalOptions.Output

	return nil
}

func (o *accountListOptions) run() error {

	var (
		OuID string
	)
	// Instantiate Aws client
	awsClient, err := awsprovider.NewAwsClient(o.payerAccount, "us-east-1", "")
	if err != nil {
		return err
	}

	if o.payerAccount == "osd-staging-2" {
		OuID = "ou-rs3h-ry0hn2l9"
	} else if o.payerAccount == "osd-staging-1" {
		OuID = "ou-0wd6-z6tzkjek"
	}

	o.awsClient = awsClient
	if o.accountID != "" {
		owner, err := o.listUserName(o.accountID)
		if err != nil {
			return err
		}
		fmt.Println(owner)
		return nil
	}

	// Initialize hashmap

	o.m = make(map[string][]string)

	if o.username != "" {

		user := o.username
		o.m[user], err = o.listAccountsByUser(user)
		if err != nil {
			return err
		}
	}

	if o.username == "" && o.accountID == "" {

		o.m, err = o.listAllAccounts(OuID)
		if err != nil {
			return err
		}
	}

	for key, value := range o.m {
		resp := listResponse{
			Username: key,
			Accounts: value,
		}

		err := outputflag.PrintResponse(o.output, resp)
		if err != nil {
			fmt.Println("Error while printing response: ", err.Error())
			return err
		}

	}

	return nil
}

var ErrNoOwnerTag error = fmt.Errorf("No owner tag on aws account")
var ErrNoTagsOnAccount error = fmt.Errorf("No tags on aws account")

func (o *accountListOptions) listUserName(accountIdInput string) (string, error) {

	input := &organizations.ListTagsForResourceInput{
		ResourceId: &accountIdInput,
	}
	val, err := o.awsClient.ListTagsForResource(input)
	if err != nil {
		return "", err
	}

	if len(val.Tags) == 0 {
		return "", ErrNoTagsOnAccount
	}

	for _, t := range val.Tags {
		if *t.Key == "owner" {
			return *t.Value, nil
		}
	}
	return "", ErrNoOwnerTag
}

var ErrNoResources error = fmt.Errorf("No resources for AWS tag")

func (o *accountListOptions) listAccountsByUser(user string) ([]string, error) {
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
	accounts, err := o.awsClient.GetResources(inputFilterTag)
	if err != nil {
		return []string{}, err
	}

	if len(accounts.ResourceTagMappingList) == 0 {
		return []string{}, ErrNoResources
	}

	// Get last 12 digits of ResourceARN and append it to account list
	tempAccountIDs := []string{}
	for _, a := range accounts.ResourceTagMappingList {
		tempAccountIDs = append(tempAccountIDs, (*a.ResourceARN)[len(*a.ResourceARN)-12:])
	}

	return tempAccountIDs, nil
}

var ErrNoAccountsForParent error = fmt.Errorf("no accounts for OU")
var ErrAccountsWithNoOwner error = fmt.Errorf("aws accounts available but no owner tags present")

func (o *accountListOptions) listAllAccounts(OuIdInput string) (map[string][]string, error) {

	m := map[string][]string{}

	input := &organizations.ListAccountsForParentInput{
		ParentId: &OuIdInput,
	}
	accounts, err := o.awsClient.ListAccountsForParent(input)
	if err != nil {
		return m, err
	}

	if len(accounts.Accounts) == 0 {
		return map[string][]string{}, ErrNoAccountsForParent
	}

	// Loop through list of accounts and build hashmap of users and accounts
	var user string

	for _, a := range accounts.Accounts {

		inputListTags := &organizations.ListTagsForResourceInput{
			ResourceId: a.Id,
		}
		tagVal, err := o.awsClient.ListTagsForResource(inputListTags)
		if err != nil {
			return m, err
		}

		for _, t := range tagVal.Tags {
			if *t.Key == "owner" {
				user = *t.Value
				break
			}
		}
		// If account has no owner, don't print
		if user == "" {
			continue
		}
		// Check if user is already in the hashmap
		// If yes append to list of accounts
		// If no create list of accounts and add it as value to key
		_, ok := m[user]
		if ok {
			m[user] = append(m[user], *a.Id)
		} else {
			m[user] = []string{*a.Id}
		}
	}
	if len(m) == 0 && len(accounts.Accounts) != 0 {
		return map[string][]string{}, ErrAccountsWithNoOwner
	}
	return m, nil
}
