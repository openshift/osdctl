package k8s

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog"

	"github.com/openshift/osd-utils-cli/cmd/common"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// ClusterResourceFactoryOptions defines the struct for running list account command
type ClusterResourceFactoryOptions struct {
	AccountName      string
	AccountID        string
	AccountNamespace string
	ClusterID        string

	Awscloudfactory awsprovider.FactoryOptions

	Flags   *genericclioptions.ConfigFlags
	KubeCli client.Client
}

// AttachCobraCliFlags adds cobra cli flags to cobra command
func (factory *ClusterResourceFactoryOptions) AttachCobraCliFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&factory.AccountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	cmd.Flags().StringVarP(&factory.AccountName, "account-name", "a", "", "The AWS account CR we need to create a temporary AWS console URL for")
	cmd.Flags().StringVarP(&factory.AccountID, "account-id", "i", "", "The AWS account ID we need to create AWS credentials for -- This argument will not work for CCS accounts")
	cmd.Flags().StringVarP(&factory.ClusterID, "cluster-id", "C", "", "The Internal Cluster ID from Hive to create AWS console URL for")

	factory.Awscloudfactory.AttachCobraCliFlags(cmd)
}

func (factory *ClusterResourceFactoryOptions) countAccountIdentifiers() int {
	targets := []string{
		factory.AccountName,
		factory.AccountID,
		factory.ClusterID,
	}
	targetCount := 0
	for _, t := range targets {
		if t != "" {
			targetCount++
		}
	}
	return targetCount
}

func (factory *ClusterResourceFactoryOptions) hasOnlyOneTarget() bool {
	return factory.countAccountIdentifiers() == 1
}

// ValidateIdentifiers checks for presence and validity of account identifiers
func (factory *ClusterResourceFactoryOptions) ValidateIdentifiers() (bool, error) {
	if factory.countAccountIdentifiers() == 0 {
		// Not expecting to use this feature, not an error
		return false, nil
	}

	if !factory.hasOnlyOneTarget() {
		return false, errors.New("AWS account CR name, AWS account ID and Cluster ID cannot be combined, please use only one")
	}

	return true, nil
}

// GetCloudProvider placeholder
func (factory *ClusterResourceFactoryOptions) GetCloudProvider(verbose bool) (awsprovider.Client, error) {

	// only initialize kubernetes client when account name is set or cluster ID is set
	if factory.AccountName != "" || factory.ClusterID != "" {
		var err error
		factory.KubeCli, err = NewClient(factory.Flags)
		if err != nil {
			return nil, err
		}
	}

	var err error
	awsClient, err := factory.Awscloudfactory.NewAwsClient()
	if err != nil {
		return nil, err
	}

	ctx := context.TODO()
	var accountID string
	if factory.ClusterID != "" {
		accountClaim, err := GetAccountClaimFromClusterID(ctx, factory.KubeCli, factory.ClusterID)
		if err != nil {
			return nil, err
		}
		if accountClaim == nil {
			return nil, fmt.Errorf("Could not find any accountClaims for cluster with ID: %s", factory.ClusterID)
		}
		if accountClaim.Spec.AccountLink == "" {
			return nil, fmt.Errorf("An unexpected error occured: the AccountClaim has no Account")
		}
		factory.AccountName = accountClaim.Spec.AccountLink
	}
	var isBYOC bool
	var acctSuffix string
	if factory.AccountName != "" {
		account, err := GetAWSAccount(ctx, factory.KubeCli, factory.AccountNamespace, factory.AccountName)
		if err != nil {
			return nil, err
		}
		accountID = account.Spec.AwsAccountID
		isBYOC = account.Spec.BYOC
		acctSuffix = account.Labels["iamUserId"]
	} else {
		accountID = factory.AccountID
		isBYOC = false
	}

	callerIdentityOutput, err := awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		klog.Error("Fail to get caller identity. Could you please validate the credentials?")
		return nil, err
	}
	if verbose {
		fmt.Printf("%s\n", callerIdentityOutput)
	}
	splitArn := strings.Split(*callerIdentityOutput.Arn, "/")
	username := splitArn[1]
	sessionName := fmt.Sprintf("RH-SRE-%s", username)

	// If BYOC we need to role-chain to use the right creds.
	// Use the OrgAccess Role by default, override if BYOC
	roleName := awsv1alpha1.AccountOperatorIAMRole

	// TODO: Come back to this and do a lookup for the account CR if the account ID is the only one set so we can do this too.
	if isBYOC {
		cm := &corev1.ConfigMap{}
		err = factory.KubeCli.Get(ctx, types.NamespacedName{Namespace: awsv1alpha1.AccountCrNamespace, Name: awsv1alpha1.DefaultConfigMap}, cm)
		if err != nil {
			klog.Error("There was an error getting the configmap.")
			return nil, err
		}
		roleArn := cm.Data["CCS-Access-Arn"]

		if roleArn == "" {
			klog.Error("Empty SRE Jump Role in ConfigMap")
			return nil, fmt.Errorf("Empty ConfigMap Value")
		}

		// Build the role-name for Access:
		if acctSuffix == "" {
			klog.Error("Unexpected error parsing the account CR suffix")
			return nil, fmt.Errorf("Unexpected error parsing the account CR suffix")
		}
		roleName = fmt.Sprintf("BYOCAdminAccess-%s", acctSuffix)

		// Get STS Credentials
		if verbose {
			fmt.Printf("Elevating Access to SRE Jump Role for user %s\n", sessionName)
		}
		creds, err := awsprovider.GetAssumeRoleCredentials(awsClient, &factory.Awscloudfactory.ConsoleDuration, aws.String(sessionName), aws.String(roleArn))
		if err != nil {
			klog.Error("Failed to get jump-role creds for CCS")
			return nil, err
		}

		awsClientInput := &awsprovider.AwsClientInput{
			AccessKeyID:     *creds.AccessKeyId,
			SecretAccessKey: *creds.SecretAccessKey,
			SessionToken:    *creds.SessionToken,
			Region:          "us-east-1",
		}
		// New Client with STS Credentials
		awsClient, err = awsprovider.NewAwsClientWithInput(awsClientInput)
		if err != nil {
			klog.Error("Failed to assume jump-role for CCS")
			return nil, err
		}
	}

	factory.Awscloudfactory.CallerIdentity = callerIdentityOutput
	factory.Awscloudfactory.RoleName = roleName
	factory.Awscloudfactory.SessionName = sessionName
	factory.AccountID = accountID

	return awsClient, nil
}
