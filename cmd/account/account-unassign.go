package account

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
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
	accountUnassignCmd.Flags().StringVarP(&ops.output, "output", "o", "", "Output format. One of: json|yaml|jsonpath=...|jsonpath-file=... see jsonpath template [http://kubernetes.io/docs/user-guide/jsonpath].")
	accountUnassignCmd.Flags().StringVarP(&ops.payerAccount, "payer-account", "p", "", "Payer account type")
	accountUnassignCmd.Flags().StringVarP(&ops.username, "username", "u", "", "LDAP username")
	accountUnassignCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "Account ID")

	return accountUnassignCmd
}

type accountUnassignOptions struct {
	username     string
	payerAccount string
	output       string
	accountID    string

	flags      *genericclioptions.ConfigFlags
	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli client.Client
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
	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}
	return nil
}

func askForConfirmation(s string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [y/n]: ", s)

		response, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" {
			return true
		} else if response == "n" {
			return false
		}
	}
}

func (o *accountUnassignOptions) run() error {

	var (
		accountUsername string
		accountIdList   []string
		hiveShardList   []string
		destinationOU   string
		rootID          string
	)

	hiveShardList = []string{"hivei01ue1", "hive-stage-1", "hives02ue1", "hive-stage-v3", "hive-integration-v3",
		"hivep01ue1", "hivep02ue1", "hivep03uw1", "hivep04uw1", "hive-production-v3"}

	// Instantiate Aws client
	awsClient, err := awsprovider.NewAwsClient(o.payerAccount, "us-east-1", "")
	if err != nil {
		return err
	}

	if o.accountID != "" {
		accountIdList = append(accountIdList, o.accountID)

		// Go through tags to find the Username and verify it's not a non-ccs account
		inputListTags := &organizations.ListTagsForResourceInput{
			ResourceId: &o.accountID,
		}
		tagVal, err := awsClient.ListTagsForResource(inputListTags)
		if err != nil {
			return err
		}

		for _, t := range tagVal.Tags {
			if t.Key == aws.String("owner") {
				for _, i := range hiveShardList {
					if *t.Value == i {
						return fmt.Errorf("non-ccs account provided, only developers account accepted")
					}
				}
			} else {
				accountUsername = *t.Value
			}
		}
	}

	if o.username != "" {
		// Check that username is not a hive
		for _, j := range hiveShardList {
			if o.username == j {
				return fmt.Errorf("non-ccs account provided, only developers account accepted")
			}
		}

		accountUsername = o.username

		// Get all accounts from user
		inputFilterTag := &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters: []*resourcegroupstaggingapi.TagFilter{
				{
					Key: aws.String("owner"),
					Values: []*string{
						aws.String(accountUsername),
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
			accountIdList = append(accountIdList, (*a.ResourceARN)[len(*a.ResourceARN)-9:])
		}
		if err != nil {
			return err
		}
	}

	c := askForConfirmation("Are you sure you want to unassign the accounts? y/n")
	if c {
		// Delete login profile
		inputDeleteLogin := &iam.DeleteLoginProfileInput{
			UserName: &accountUsername,
		}
		_, err = awsClient.DeleteLoginProfile(inputDeleteLogin)
		if err != nil {
			return err
		}
		// Delete access keys
		inputListAccessKeys := &iam.ListAccessKeysInput{
			UserName: &accountUsername,
		}
		accessKeys, err := awsClient.ListAccessKeys(inputListAccessKeys)
		if err != nil {
			return err
		}

		for _, m := range accessKeys.AccessKeyMetadata {
			inputDelKey := &iam.DeleteAccessKeyInput{
				AccessKeyId: m.AccessKeyId,
				UserName:    &accountUsername,
			}
			_, err = awsClient.DeleteAccessKey(inputDelKey)
			if err != nil {
				return err
			}
		}
		// Delete signing certificates
		inputListCert := &iam.ListSigningCertificatesInput{
			UserName: &accountUsername,
		}
		cert, err := awsClient.ListSigningCertificates(inputListCert)
		if err != nil {
			return err
		}

		for _, c := range cert.Certificates {
			inputDelCert := &iam.DeleteSigningCertificateInput{
				CertificateId: c.CertificateId,
				UserName:      &accountUsername,
			}
			_, err = awsClient.DeleteSigningCertificate(inputDelCert)
			if err != nil {
				return err
			}
		}
		// Delete policies
		inputListPolicies := &iam.ListUserPoliciesInput{
			UserName: &accountUsername,
		}
		policies, err := awsClient.ListUserPolicies(inputListPolicies)
		if err != nil {
			return err
		}

		for _, p := range policies.PolicyNames {
			inputDelPolicies := &iam.DeleteUserPolicyInput{
				PolicyName: p,
				UserName:   &accountUsername,
			}
			_, err = awsClient.DeleteUserPolicy(inputDelPolicies)
			if err != nil {
				return err
			}
		}
		// Delete attached policies
		inputListAttachedPol := &iam.ListAttachedUserPoliciesInput{
			UserName: &accountUsername,
		}
		attachedPol, err := awsClient.ListAttachedUserPolicies(inputListAttachedPol)
		if err != nil {
			return err
		}

		for _, ap := range attachedPol.AttachedPolicies {
			inputDetachPol := &iam.DetachUserPolicyInput{
				PolicyArn: ap.PolicyArn,
				UserName:  &accountUsername,
			}
			_, err = awsClient.DetachUserPolicy(inputDetachPol)
			if err != nil {
				return err
			}
		}
		// Delete groups
		inputListGroups := &iam.ListGroupsForUserInput{
			UserName: &accountUsername,
		}
		groups, err := awsClient.ListGroupsForUser(inputListGroups)
		if err != nil {
			return err
		}

		for _, g := range groups.Groups {
			inputRemoveFromGroup := &iam.RemoveUserFromGroupInput{
				GroupName: g.GroupName,
				UserName:  &accountUsername,
			}
			_, err = awsClient.RemoveUserFromGroup(inputRemoveFromGroup)
			if err != nil {
				return err
			}
		}
		// Delete user
		inputDelUser := &iam.DeleteUserInput{
			UserName: &accountUsername,
		}
		_, err = awsClient.DeleteUser(inputDelUser)
		if err != nil {
			return err
		}
		// loop through accounts list and untag and move them back into root OU
		for _, id := range accountIdList {

			inputUntag := &organizations.UntagResourceInput{
				ResourceId: &id,
				TagKeys: []*string{
					aws.String("owner"),
					aws.String("claimed"),
				},
			}
			_, err = awsClient.UntagResource(inputUntag)
			if err != nil {
				return err
			}

			destinationOU = "r-rs3h-ry0hn2l9"
			rootID = "r-rs3h"
			if o.payerAccount == "osd-staging-1" {
				destinationOU = "ou-0wd6-z6tzkjek"
				rootID = "ou-0wd6"
			}

			inputMove := &organizations.MoveAccountInput{
				AccountId:           aws.String(id),
				DestinationParentId: aws.String(rootID),
				SourceParentId:      aws.String(destinationOU),
			}

			_, err = awsClient.MoveAccount(inputMove)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
