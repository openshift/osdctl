package mgmt

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func newCmdAccountUnassign(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newAccountUnassignOptions(streams, flags)
	accountUnassignCmd := &cobra.Command{
		Use:               "unassign",
		Short:             "Unassign account to user",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	ops.printFlags.AddFlags(accountUnassignCmd)
	accountUnassignCmd.Flags().StringVarP(&ops.payerAccount, "payer-account", "p", "", "Payer account type")
	accountUnassignCmd.Flags().StringVarP(&ops.username, "username", "u", "", "LDAP username")
	accountUnassignCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "Account ID")

	return accountUnassignCmd
}

type accountUnassignOptions struct {
	awsClient    awsprovider.Client
	username     string
	payerAccount string
	accountID    string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
}

func newAccountUnassignOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *accountUnassignOptions {
	return &accountUnassignOptions{
		flags:      flags,
		printFlags: printer.NewPrintFlags(),
		IOStreams:  streams,
	}
}

func (o *accountUnassignOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.payerAccount == "" {
		return cmdutil.UsageErrorf(cmd, "Payer account was not provided")
	}
	if o.username == "" && o.accountID == "" {
		return cmdutil.UsageErrorf(cmd, "Please provide either an username or account ID")
	}
	return nil
}

func (o *accountUnassignOptions) run() error {

	var (
		accountUsername string
		accountIdList   []string
		destinationOU   string
		rootID          string
	)

	// Instantiate Aws client
	awsClient, err := awsprovider.NewAwsClient(o.payerAccount, "us-east-1", "")
	if err != nil {
		return err
	}

	if o.payerAccount == "osd-staging-1" {
		rootID = OSDStaging1RootID
		destinationOU = OSDStaging1OuID
	} else if o.payerAccount == "osd-staging-2" {
		rootID = OSDStaging2RootID
		destinationOU = OSDStaging2OuID
	} else {
		return fmt.Errorf("invalid payer account provided")
	}

	o.awsClient = awsClient

	if o.accountID != "" {

		err := o.checkForHiveNameTag(o.accountID)
		if err != nil {
			return err
		}
		accountIdList = append(accountIdList, o.accountID)

	}

	if o.username != "" {
		// Check that username is not a hive
		if strings.HasPrefix(o.username, "hive") {
			return ErrHiveNameProvided
		}

		accountUsername = o.username
		accountIdList, err = o.listAccountsFromUser(accountUsername)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Are you sure you want to unassign the accounts? [y/n] ")
	reader := bufio.NewReader(os.Stdin)

	response, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	if response == "y" || response == "Y" {

		// loop through accounts list and untag and move them back into root OU
		for _, id := range accountIdList {

			err = o.untagAccount(id)
			if err != nil {
				return err
			}

			err = o.moveAccount(id, rootID, destinationOU)
			if err != nil {
				return err
			}
		}

		if accountUsername != "" {
			// Delete login profile
			err := o.deleteLoginProfile(accountUsername)
			if err != nil {
				return err
			}
			// Delete access keys
			err = o.deleteAccessKeys(accountUsername)
			if err != nil {
				return err
			}
			// Delete signing certificates
			err = o.deleteSigningCert(accountUsername)
			if err != nil {
				return err
			}
			// Delete policies
			err = o.deletePolicies(accountUsername)
			if err != nil {
				return err
			}
			// Delete attached policies
			err = o.deleteAttachedPolicies(accountUsername)
			if err != nil {
				return err
			}
			// Delete groups
			err = o.deleteGroups(accountUsername)
			if err != nil {
				return err
			}
			// Delete user
			err = o.deleteUser(accountUsername)
			if err != nil {
				return err
			}
		}

	}
	return nil
}

var ErrHiveNameProvided error = fmt.Errorf("non-ccs account provided, only developers account accepted")
var ErrAccountPartiallyTagged error = fmt.Errorf("account is only partially tagged")

func (o *accountUnassignOptions) checkForHiveNameTag(id string) error {

	inputListTags := &organizations.ListTagsForResourceInput{
		ResourceId: &id,
	}
	tags, err := o.awsClient.ListTagsForResource(inputListTags)
	if err != nil {
		return err
	}

	if len(tags.Tags) == 0 {
		return ErrNoTagsOnAccount
	}

	for _, t := range tags.Tags {
		if *t.Key == "owner" && strings.HasPrefix(*t.Value, "hive") {
			return ErrHiveNameProvided
		}
	}
	return nil
}

func (o *accountUnassignOptions) untagAccount(id string) error {
	inputUntag := &organizations.UntagResourceInput{
		ResourceId: &id,
		TagKeys: []*string{
			aws.String("owner"),
			aws.String("claimed"),
		},
	}
	_, err := o.awsClient.UntagResource(inputUntag)
	if err != nil {
		return err
	}
	return nil
}

func (o *accountUnassignOptions) moveAccount(id string, rootID string, destinationOU string) error {
	inputMove := &organizations.MoveAccountInput{
		AccountId:           aws.String(id),
		DestinationParentId: aws.String(rootID),
		SourceParentId:      aws.String(destinationOU),
	}

	_, err := o.awsClient.MoveAccount(inputMove)
	if err != nil {
		return err
	}
	return nil
}

var ErrNoAccountsForUser error = fmt.Errorf("user has no aws accounts")

func (o *accountUnassignOptions) listAccountsFromUser(user string) ([]string, error) {

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
		return []string{}, ErrNoAccountsForUser
	}

	var accountIdList []string
	// Get last 12 digits of ResourceARN and append it to account list
	for _, a := range accounts.ResourceTagMappingList {
		accountIdList = append(accountIdList, (*a.ResourceARN)[len(*a.ResourceARN)-12:])
	}

	return accountIdList, nil
}

func (o *accountUnassignOptions) deleteLoginProfile(user string) error {

	inputDeleteLogin := &iam.DeleteLoginProfileInput{
		UserName: &user,
	}
	_, err := o.awsClient.DeleteLoginProfile(inputDeleteLogin)
	if err != nil {
		return err
	}
	return nil
}

func (o *accountUnassignOptions) deleteAccessKeys(user string) error {

	inputListAccessKeys := &iam.ListAccessKeysInput{
		UserName: &user,
	}
	accessKeys, err := o.awsClient.ListAccessKeys(inputListAccessKeys)
	if err != nil {
		return err
	}

	for _, m := range accessKeys.AccessKeyMetadata {

		inputDelKey := &iam.DeleteAccessKeyInput{
			AccessKeyId: m.AccessKeyId,
			UserName:    &user,
		}
		_, err = o.awsClient.DeleteAccessKey(inputDelKey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *accountUnassignOptions) deleteSigningCert(user string) error {

	inputListCert := &iam.ListSigningCertificatesInput{
		UserName: &user,
	}
	cert, err := o.awsClient.ListSigningCertificates(inputListCert)
	if err != nil {
		return err
	}

	for _, c := range cert.Certificates {
		inputDelCert := &iam.DeleteSigningCertificateInput{
			CertificateId: c.CertificateId,
			UserName:      &user,
		}
		_, err = o.awsClient.DeleteSigningCertificate(inputDelCert)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *accountUnassignOptions) deletePolicies(user string) error {
	inputListPolicies := &iam.ListUserPoliciesInput{
		UserName: &user,
	}
	policies, err := o.awsClient.ListUserPolicies(inputListPolicies)
	if err != nil {
		return err
	}

	for _, p := range policies.PolicyNames {
		inputDelPolicies := &iam.DeleteUserPolicyInput{
			PolicyName: p,
			UserName:   &user,
		}
		_, err = o.awsClient.DeleteUserPolicy(inputDelPolicies)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *accountUnassignOptions) deleteAttachedPolicies(user string) error {
	inputListAttachedPol := &iam.ListAttachedUserPoliciesInput{
		UserName: &user,
	}
	attachedPol, err := o.awsClient.ListAttachedUserPolicies(inputListAttachedPol)
	if err != nil {
		return err
	}

	for _, ap := range attachedPol.AttachedPolicies {
		inputDetachPol := &iam.DetachUserPolicyInput{
			PolicyArn: ap.PolicyArn,
			UserName:  &user,
		}
		_, err = o.awsClient.DetachUserPolicy(inputDetachPol)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *accountUnassignOptions) deleteGroups(user string) error {

	inputListGroups := &iam.ListGroupsForUserInput{
		UserName: &user,
	}
	groups, err := o.awsClient.ListGroupsForUser(inputListGroups)
	if err != nil {
		return err
	}

	for _, g := range groups.Groups {
		inputRemoveFromGroup := &iam.RemoveUserFromGroupInput{
			GroupName: g.GroupName,
			UserName:  &user,
		}
		_, err = o.awsClient.RemoveUserFromGroup(inputRemoveFromGroup)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *accountUnassignOptions) deleteUser(user string) error {
	inputDelUser := &iam.DeleteUserInput{
		UserName: &user,
	}
	_, err := o.awsClient.DeleteUser(inputDelUser)
	if err != nil {
		return err
	}
	return nil
}
