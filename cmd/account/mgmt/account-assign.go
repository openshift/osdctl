package mgmt

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/organizations"
	organizationTypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	outputflag "github.com/openshift/osdctl/cmd/getoutput"
	"github.com/openshift/osdctl/internal/utils/globalflags"
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
	accountID    string
	output       string
	iamUser      bool

	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

type assignResponse struct {
	Username string `json:"username" yaml:"username"`
	Id       string `json:"id" yaml:"id"`
}

func (f assignResponse) String() string {
	return fmt.Sprintf("  Username: %s\n  Account: %s\n", f.Username, f.Id)
}

func newAccountAssignOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *accountAssignOptions {
	return &accountAssignOptions{
		printFlags:    printer.NewPrintFlags(),
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

// assignCmd assigns an aws account to user under osd-staging-2 by default unless osd-staging-1 is specified
func newCmdAccountAssign(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newAccountAssignOptions(streams, globalOpts)
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
	accountAssignCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "(optional) Specific AWS account ID to assign")
	accountAssignCmd.Flags().BoolVarP(&ops.iamUser, "iam-user", "I", false, "(optional) Create an AWS IAM user and Access Key")

	return accountAssignCmd
}

func (o *accountAssignOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.username == "" {
		return cmdutil.UsageErrorf(cmd, "LDAP username was not provided")
	}

	o.output = o.GlobalOptions.Output

	return nil
}

func (o *accountAssignOptions) run() error {

	var (
		accountAssignID string
		destinationOU   string
		rootID          string
	)

	if o.payerAccount == osdStaging1 || os.Getenv(envKeyAWSAccountName) == osdStaging1 {
		rootID = OSDStaging1RootID
		destinationOU = OSDStaging1OuID
	} else if o.payerAccount == osdStaging2 || os.Getenv(envKeyAWSAccountName) == osdStaging2 {
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
	// We support passing in an aws account ID to be assigned, or retrieving one for the user.
	if o.accountID != "" {
		accountAssignID = o.accountID
		// ensure that the account we're assigning is not already owned
		isOwned, err := isOwned(accountAssignID, &o.awsClient)
		if err != nil {
			return err
		}
		if isOwned {
			return fmt.Errorf("the account you are attempting to assign is already owned, please use the 'unassign' command to unassign the account, or use 'assign' without a specific aws account id to be assigned one at random")
		}

		isSuspended, err := isSuspended(accountAssignID, o.awsClient)
		if err != nil {
			return err
		}
		if isSuspended {
			return fmt.Errorf("the account you are attempting to assign is suspended, please use another account, or use 'assign' without a specific aws account id to be assigned one at random")
		}

	} else {
		accountAssignID, err = o.findUntaggedAccount(rootID)
	}

	if err != nil {
		// If the error returned is not because of a lack of accounts, return the error
		if err != ErrNoUntaggedAccounts {
			return err
		}
		// otherwise, create a new account
		seed := time.Now().UnixNano()
		accountAssignID, err = o.buildAccount(seed)

		if err != nil {
			return err
		}
	}

	err = o.tagAccount(accountAssignID)
	if err != nil {
		return err
	}

	err = o.moveAccount(accountAssignID, destinationOU, rootID)
	if err != nil {
		return err
	}

	resp := assignResponse{
		Username: o.username,
		Id:       accountAssignID,
	}

	err = outputflag.PrintResponse(o.output, resp)
	if err != nil {
		fmt.Println("Error while calling PrintResponse(): ", err.Error())
	}

	// Create an AWS IAM user if iamUser is true
	if o.iamUser {
		fmt.Printf("Creating AWS IAM user for account %s...\n\n", accountAssignID)
		iamOptions := iamOptions{
			awsAccountID: accountAssignID,
			awsProfile:   o.payerAccount,
			awsRegion:    "us-east-1",
			kerberosUser: o.username,
			rotate:       false,
		}

		err = iamOptions.run()
		if err != nil {
			return fmt.Errorf("error while creating AWS IAM user: %s", err)
		}
	}

	return nil
}

var ErrNoUntaggedAccounts = fmt.Errorf("no untagged accounts available")

func (o *accountAssignOptions) findUntaggedAccount(rootOu string) (string, error) {

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
		return "", ErrNoUntaggedAccounts
	}

	identity, err := o.awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}

	// Loop through accounts and check that it's untagged and assign ID to user
	for _, a := range accounts.Accounts {
		if *a.Id == *identity.Account {
			// Don't allow the payer account to be assigned to an individual user
			continue
		}

		isOwned, err := isOwned(*a.Id, &o.awsClient)
		if err != nil {
			return "", err
		}

		if !isOwned {
			isSuspended, err := isSuspended(*a.Id, o.awsClient)
			if err != nil {
				return "", err
			}
			if !isSuspended {
				accountAssignID = *a.Id
				break
			}
		}
	}

	if accountAssignID == "" {
		return "", ErrNoUntaggedAccounts
	}
	return accountAssignID, nil
}

func isOwned(accountID string, awsClient *awsprovider.Client) (bool, error) {
	inputListTags := &organizations.ListTagsForResourceInput{
		ResourceId: &accountID,
	}
	tags, err := (*awsClient).ListTagsForResource(inputListTags)
	if err != nil {
		return false, err
	}

	for _, t := range tags.Tags {
		if *t.Key == "owner" || *t.Key == "claimed" {
			return true, nil
		}
	}

	return false, nil
}

func isSuspended(accountIdInput string, awsClient awsprovider.Client) (bool, error) {
	accountInfo, err := awsClient.DescribeAccount(
		&organizations.DescribeAccountInput{
			AccountId: &accountIdInput,
		},
	)

	if err != nil {
		return false, err
	}

	if accountInfo.Account.Status == "SUSPENDED" {
		return true, nil
	}

	return false, nil
}

func (o *accountAssignOptions) tagAccount(accountId string) error {
	ownerKey := "owner"
	claimedKey := "claimed"
	claimedValue := "true"
	inputTag := &organizations.TagResourceInput{
		ResourceId: &accountId,
		Tags: []organizationTypes.Tag{
			{
				Key:   &ownerKey,
				Value: &o.username,
			},
			{
				Key:   &claimedKey,
				Value: &claimedValue,
			},
		},
	}
	_, err := o.awsClient.TagResource(inputTag)
	if err != nil {
		return err
	}
	return nil
}

func (o *accountAssignOptions) buildAccount(seedVal int64) (string, error) {

	fmt.Println("Creating account")
	var newAccountId string

	orgOutput, orgErr := o.createAccount(seedVal)
	if orgErr != nil {

		// If email already exists retry until a new email is generated
		for orgErr == ErrEmailAlreadyExist {
			seedVal = time.Now().UnixNano()
			orgOutput, orgErr = o.createAccount(seedVal)
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

var (
	ErrAwsAccountLimitExceeded = fmt.Errorf("ErrAwsAccountLimitExceeded")
	ErrEmailAlreadyExist       = fmt.Errorf("ErrEmailAlreadyExist")
	ErrAwsInternalFailure      = fmt.Errorf("ErrAwsInternalFailure")
	ErrAwsTooManyRequests      = fmt.Errorf("ErrAwsTooManyRequests")
	ErrAwsFailedCreateAccount  = fmt.Errorf("ErrAwsFailedCreateAccount")
)

func (o *accountAssignOptions) createAccount(seedVal int64) (*organizations.DescribeCreateAccountStatusOutput, error) {
	randStr := RandomString(rand.New(rand.NewSource(seedVal)), 6)
	accountName := "osd-creds-mgmt+" + randStr
	email := accountName + "@redhat.com"

	createInput := &organizations.CreateAccountInput{
		AccountName: &accountName,
		Email:       &email,
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
		createStatus := status.CreateAccountStatus.State

		if createStatus == "FAILED" {
			var returnErr error
			switch status.CreateAccountStatus.FailureReason {
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

func RandomString(r *rand.Rand, length int) string {
	var letters = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

	s := make([]byte, length)
	for i := range s {
		s[i] = letters[r.Intn(len(letters))] //#nosec G404 -- math/rand is not used for a secret here, hence it's okay
	}
	return string(s)
}

func (o *accountAssignOptions) moveAccount(accountIdInput string, destOuInput string, rootIdInput string) error {

	inputMove := &organizations.MoveAccountInput{
		AccountId:           &accountIdInput,
		DestinationParentId: &destOuInput,
		SourceParentId:      &rootIdInput,
	}

	_, err := o.awsClient.MoveAccount(inputMove)
	if err != nil {
		return err
	}
	return nil
}
