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
	"github.com/aws/aws-sdk-go/service/sts"
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
	flags        *genericclioptions.ConfigFlags
	printFlags   *printer.PrintFlags
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
		accountUsername      string
		accountIdList        []string
		destinationOU        string
		rootID               string
		assumedRoleAwsClient awsprovider.Client
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
	var allUsers []string

	if o.accountID != "" {
		// Check aws tag to see if it's a ccs acct, if it's not return name of owner
		accountUsername, err = o.checkForHiveNameTag(o.accountID)
		if err != nil {
			return err
		}

		accountIdList = append(accountIdList, o.accountID)
	}

	if o.username != "" {
		// Check that username doesn't belong to a ccs acct
		if strings.HasPrefix(o.username, "hive") {
			return ErrHiveNameProvided
		}

		accountUsername = o.username

		accountIdList, err = o.listAccountsFromUser(accountUsername)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Are you sure you want to unassign account(s) [%v] from %s? [y/n] ", accountIdList, accountUsername)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	response = strings.ToLower(response[0:1])
	if response != "y" {
		os.Exit(0)
	}

	// loop through accounts list and untag and move them back into root OU
	for _, id := range accountIdList {

		// untag account
		err = o.untagAccount(id)
		if err != nil {
			return err
		}

		// move account
		err = o.moveAccount(id, rootID, destinationOU)
		if err != nil {
			return err
		}
		// instantiate new client with AssumeRole
		assumedRoleAwsClient, err = o.assumeRoleForAccount(id)
		if err != nil {
			return err
		}
		// delete roles
		err = deleteRoles(assumedRoleAwsClient)
		if err != nil {
			fmt.Println(err)
		}
		// delete account policies
		err = deleteAccountPolicies(assumedRoleAwsClient)
		if err != nil {
			fmt.Println(err)
		}
		// list iam users created by each account and append to slice
		users, err := listUsersFromAccount(assumedRoleAwsClient, id)
		if err != nil {
			fmt.Println(err)
		}

		allUsers = append(allUsers, users...)

		o.awsClient = assumedRoleAwsClient
	}

	for _, userName := range allUsers {
		// Delete login profile
		err = o.deleteLoginProfile(userName)
		if err != nil {
			fmt.Println(err)
		}
		// Delete access keys
		err = o.deleteAccessKeys(userName)
		if err != nil {
			fmt.Println(err)
		}
		// Delete signing certificates
		err = o.deleteSigningCert(userName)
		if err != nil {
			fmt.Println(err)
		}
		// Delete user policies
		err = o.deleteUserPolicies(userName)
		if err != nil {
			fmt.Println(err)
		}
		// Delete attached policies
		err = o.deleteAttachedPolicies(userName)
		if err != nil {
			fmt.Println(err)
		}
		// Delete groups
		err = o.deleteGroups(userName)
		if err != nil {
			fmt.Println(err)
		}
		// Delete user
		err = o.deleteUser(userName)
		if err != nil {
			fmt.Println(err)
		}
	}

	return nil
}

func (o *accountUnassignOptions) assumeRoleForAccount(account_id string) (awsprovider.Client, error) {

	roleArn := fmt.Sprintf("arn:aws:iam::%s:role/OrganizationAccountAccessRole", account_id)

	input := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String("osdctl-account-unassignment"),
	}

	result, err := o.awsClient.AssumeRole(input)
	if err != nil {
		return nil, err
	}

	newAwsClientInput := &awsprovider.AwsClientInput{
		AccessKeyID:     *result.Credentials.AccessKeyId,
		SecretAccessKey: *result.Credentials.SecretAccessKey,
		SessionToken:    *result.Credentials.SessionToken,
		Region:          "us-east-1",
	}

	newAWSClient, err := awsprovider.NewAwsClientWithInput(newAwsClientInput)
	if err != nil {
		return nil, err
	}

	return newAWSClient, nil
}

func listUsersFromAccount(newAWSClient awsprovider.Client, account_id string) ([]string, error) {

	listInput := &iam.ListUsersInput{}

	var users []*iam.User
	err := newAWSClient.ListUsersPages(listInput, func(page *iam.ListUsersOutput, lastPage bool) bool {
		users = append(users, page.Users...)
		return *page.IsTruncated
	})
	if err != nil {
		return []string{}, err
	}

	var userList []string

	for _, u := range users {
		userList = append(userList, *u.UserName)
	}

	return userList, nil
}

var ErrHiveNameProvided error = fmt.Errorf("hive-managed account provided, only developers account accepted")
var ErrAccountPartiallyTagged error = fmt.Errorf("account is only partially tagged")

func (o *accountUnassignOptions) checkForHiveNameTag(id string) (string, error) {

	inputListTags := &organizations.ListTagsForResourceInput{
		ResourceId: &id,
	}
	var tags []*organizations.Tag
	err := o.awsClient.ListTagsForResourcePages(inputListTags, func(page *organizations.ListTagsForResourceOutput, lastPage bool) bool {
		tags = append(tags, page.Tags...)
		return page.NextToken != nil
	})
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", ErrNoTagsOnAccount
	}

	for _, t := range tags {
		if *t.Key == "owner" && strings.HasPrefix(*t.Value, "hive") {
			return "", ErrHiveNameProvided
		}
		if *t.Key == "owner" && !strings.HasPrefix(*t.Value, "hive") {
			return *t.Value, nil
		}
	}
	return "", ErrNoOwnerTag
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

func (o *accountUnassignOptions) deleteUserPolicies(user string) error {

	inputListPolicies := &iam.ListUserPoliciesInput{
		UserName: &user,
	}
	var policyNames []*string
	err := o.awsClient.ListUserPoliciesPages(inputListPolicies, func(page *iam.ListUserPoliciesOutput, lastPage bool) bool {
		policyNames = append(policyNames, page.PolicyNames...)
		return *page.IsTruncated
	})
	if err != nil {
		return err
	}
	for _, p := range policyNames {
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

func detachRolePolicies(rolename *string, newAWSClient awsprovider.Client) error {

	listRolePolInput := &iam.ListAttachedRolePoliciesInput{
		RoleName: rolename,
	}
	rolePol, err := newAWSClient.ListAttachedRolePolicies(listRolePolInput)
	if err != nil {
		return err
	}

	for _, rp := range rolePol.AttachedPolicies {
		_, err := newAWSClient.DetachRolePolicy(&iam.DetachRolePolicyInput{
			PolicyArn: rp.PolicyArn,
			RoleName:  rolename,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteAccountPolicies(newAWSClient awsprovider.Client) error {

	listAcctPoliciesInput := &iam.ListPoliciesInput{
		Scope: aws.String("Local"),
	}

	var policies []*iam.Policy
	err := newAWSClient.ListPoliciesPages(listAcctPoliciesInput, func(
		page *iam.ListPoliciesOutput, lastPage bool) bool {
		policies = append(policies, page.Policies...)
		return *page.IsTruncated
	})
	if err != nil {
		return err
	}

	for _, pol := range policies {
		_, err := newAWSClient.DeletePolicy(&iam.DeletePolicyInput{
			PolicyArn: pol.Arn,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func deleteRoles(newAWSClient awsprovider.Client) error {

	listRoleInput := &iam.ListRolesInput{}
	var roles []*iam.Role
	err := newAWSClient.ListRolesPages(listRoleInput, func(page *iam.ListRolesOutput, lastPage bool) bool {
		roles = append(roles, page.Roles...)
		return *page.IsTruncated
	})
	if err != nil {
		return err
	}
	// delete all roles except OrganizationAccountAccessRole
	for _, rolename := range roles {

		if *rolename.RoleName == "OrganizationAccountAccessRole" || strings.Contains(*rolename.RoleName, "AWSServiceRole") {
			continue
		}
		inputDeleteRole := rolename.RoleName
		err := detachRolePolicies(inputDeleteRole, newAWSClient)
		if err != nil {
			return nil
		}

		_, err = newAWSClient.DeleteRole(&iam.DeleteRoleInput{RoleName: inputDeleteRole})
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
	fmt.Printf("user %s successfully deleted\n", user)
	return nil
}
