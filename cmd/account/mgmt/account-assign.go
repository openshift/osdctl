package mgmt

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/organizations"

	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// Global variables
var (
	OSDStaging2RootID = "r-rs3h"
	OSDStaging2OuID   = "ou-rs3h-ry0hn2l9"
	OSDStaging1RootID = "r-0wd6"
	OSDStaging1OuID   = "ou-0wd6-z6tzkjek"
)

type accountAssignOptions struct {
	awsClient    awsprovider.Client
	username     string
	payerAccount string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
}

type AccountCreate func(seedVal int64) (*organizations.DescribeCreateAccountStatusOutput, error)
type AccountBuild func(seedVal int64, accountCreateFunc AccountCreate) (string, error)

func newAccountAssignOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *accountAssignOptions {
	return &accountAssignOptions{
		flags:      flags,
		printFlags: printer.NewPrintFlags(),
		IOStreams:  streams,
	}
}

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
	accountAssignCmd.Flags().StringVarP(&ops.payerAccount, "payer-account", "p", "", "Payer account type")
	accountAssignCmd.Flags().StringVarP(&ops.username, "username", "u", "", "LDAP username")

	return accountAssignCmd
}

func (o *accountAssignOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.username == "" {
		return cmdutil.UsageErrorf(cmd, "LDAP username was not provided")
	}
	if o.payerAccount == "" {
		return cmdutil.UsageErrorf(cmd, "Payer account was not provided")
	}

	return nil
}

func (o *accountAssignOptions) run() error {

	var (
		accountAssignID string
		destinationOU   string
		rootID          string
	)

	if o.payerAccount == "osd-staging-1" {
		rootID = OSDStaging1RootID
		destinationOU = OSDStaging1OuID
	} else if o.payerAccount == "osd-staging-2" {
		rootID = OSDStaging2RootID
		destinationOU = OSDStaging2OuID
	} else {
		return fmt.Errorf("invalid payer account provided")
	}
	//Instantiate aws client
	awsClient, err := awsprovider.NewAwsClient(o.payerAccount, "us-east-1", "")
	if err != nil {
		return err
	}

	o.awsClient = awsClient
	seed := time.Now().UnixNano()
	accountAssignID, err = o.findUntaggedAccount(seed, rootID, buildAccount)
	if err != nil {
		return err
	}

	err = o.tagAccount(accountAssignID)
	if err != nil {
		return err
	}

	err = o.moveAccount(accountAssignID, destinationOU, rootID)
	if err != nil {
		return err
	}

	fmt.Fprintln(o.IOStreams.Out, accountAssignID)

	return nil
}

var ErrNoUntaggedAccounts = fmt.Errorf("no untagged accounts available")

func (o *accountAssignOptions) findUntaggedAccount(seedVal int64, rootOu string, buildAccountFunc AccountBuild) (string, error) {

	var accountAssignID string

	//List accounts that are not in any OU
	input := &organizations.ListAccountsForParentInput{
		ParentId: &rootOu,
	}
	accounts, err := o.awsClient.ListAccountsForParent(input)
	if err != nil {
		return "", err
	}

	if len(accounts.Accounts) == 0 {
		fmt.Println("No accounts available to assign")

		accountAssignID, err := buildAccountFunc(seedVal, o.createAccount)
		if err != nil {
			return "", err
		}
		return accountAssignID, nil
	}

	// Loop through accounts and check that it's untagged and assign ID to user
	for _, a := range accounts.Accounts {

		inputListTags := &organizations.ListTagsForResourceInput{
			ResourceId: a.Id,
		}
		tags, err := o.awsClient.ListTagsForResource(inputListTags)
		if err != nil {
			return "", err
		}

		hasNoOwnerClaimedTag := true

		for _, t := range tags.Tags {
			if *t.Key == "owner" || *t.Key == "claimed" {
				hasNoOwnerClaimedTag = false
				break
			}
		}

		if hasNoOwnerClaimedTag {
			accountAssignID = *a.Id
			break
		}
	}

	if accountAssignID == "" {
		return "", ErrNoUntaggedAccounts
	}
	return accountAssignID, nil
}

func (o *accountAssignOptions) tagAccount(accountIdInput string) error {

	inputTag := &organizations.TagResourceInput{
		ResourceId: aws.String(accountIdInput),
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
	_, err := o.awsClient.TagResource(inputTag)
	if err != nil {
		return err
	}
	return nil
}

func buildAccount(seedVal int64, accountCreateFunc AccountCreate) (string, error) {

	fmt.Println("Creating account")
	var newAccountId string

	orgOutput, orgErr := accountCreateFunc(seedVal)
	if orgErr != nil {

		// If email already exists retry until a new email is generated
		for orgErr == ErrEmailAlreadyExist {
			orgOutput, orgErr = accountCreateFunc(seedVal)
			if orgErr == nil {
				newAccountId = *orgOutput.CreateAccountStatus.AccountId
				return newAccountId, nil
			}
		}
		return "", orgErr
	}

	newAccountId = *orgOutput.CreateAccountStatus.AccountId
	return newAccountId, nil
}

var ErrAwsAccountLimitExceeded error = fmt.Errorf("ErrAwsAccountLimitExceeded")
var ErrEmailAlreadyExist error = fmt.Errorf("ErrEmailAlreadyExist")
var ErrAwsInternalFailure error = fmt.Errorf("ErrAwsInternalFailure")
var ErrAwsTooManyRequests error = fmt.Errorf("ErrAwsTooManyRequests")
var ErrAwsFailedCreateAccount error = fmt.Errorf("ErrAwsFailedCreateAccount")

func (o *accountAssignOptions) createAccount(seedVal int64) (*organizations.DescribeCreateAccountStatusOutput, error) {

	rand.Seed(seedVal)
	randStr := RandomString(6)
	accountName := "osd-creds-mgmt+" + randStr
	email := accountName + "@redhat.com"

	createInput := &organizations.CreateAccountInput{
		AccountName: aws.String(accountName),
		Email:       aws.String(email),
	}

	createOutput, err := o.awsClient.CreateAccount(createInput)
	if err != nil {
		return &organizations.DescribeCreateAccountStatusOutput{}, err
	}

	describeStatusInput := &organizations.DescribeCreateAccountStatusInput{
		CreateAccountRequestId: createOutput.CreateAccountStatus.Id,
	}

	var accountStatus *organizations.DescribeCreateAccountStatusOutput
	for {
		status, err := o.awsClient.DescribeCreateAccountStatus(describeStatusInput)
		if err != nil {
			return &organizations.DescribeCreateAccountStatusOutput{}, err
		}

		accountStatus = status
		createStatus := *status.CreateAccountStatus.State

		if createStatus == "FAILED" {
			var returnErr error
			switch *status.CreateAccountStatus.FailureReason {
			case "ACCOUNT_LIMIT_EXCEEDED":
				returnErr = ErrAwsAccountLimitExceeded
			case "EMAIL_ALREADY_EXISTS":
				returnErr = ErrEmailAlreadyExist
			case "INTERNAL_FAILURE":
				returnErr = ErrAwsInternalFailure
			default:
				returnErr = ErrAwsFailedCreateAccount
			}

			return &organizations.DescribeCreateAccountStatusOutput{}, returnErr
		}

		if createStatus != "IN_PROGRESS" {
			break
		}
	}

	return accountStatus, nil
}

func RandomString(n int) string {
	var letters = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

	s := make([]byte, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func (o *accountAssignOptions) moveAccount(accountIdInput string, destOuInput string, rootIdInput string) error {

	inputMove := &organizations.MoveAccountInput{
		AccountId:           aws.String(accountIdInput),
		DestinationParentId: aws.String(destOuInput),
		SourceParentId:      aws.String(rootIdInput),
	}

	_, err := o.awsClient.MoveAccount(inputMove)
	if err != nil {
		return err
	}
	return nil
}
