package k8s

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/openshift/osdctl/cmd/common"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterResourceFactoryOptions defines the struct for running list account command
type ClusterResourceFactoryOptions struct {
	AccountName      string
	AccountID        string
	AccountNamespace string
	ClusterID        string
	SupportRoleARN   string

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
	cmd.Flags().StringVarP(&factory.ClusterID, "cluster-id", "C", "", "The Internal Cluster ID/External Cluster ID/ Cluster Name from Hive to create AWS console URL for")

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
		factory.KubeCli = NewClient(factory.Flags)
		factory.Awscloudfactory.Region = endpoints.UsEast1RegionID
	}

	supportRoleDefined := false

	ctx := context.TODO()
	var accountClaim *awsv1alpha1.AccountClaim
	if factory.ClusterID != "" {
		// Create an OCM client to talk to the cluster API
		// the user has to be logged in (e.g. 'ocm login')
		ocmClient := utils.CreateConnection()
		defer func() {
			if err := ocmClient.Close(); err != nil {
				fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
			}
		}()

		clusters := utils.GetClusters(ocmClient, []string{factory.ClusterID})
		if len(clusters) != 1 {
			return nil, fmt.Errorf("unexpected number of clusters matched input. Expected 1 got %d", len(clusters))

		}
		factory.ClusterID = clusters[0].ID()
		var err error
		accountClaim, err = GetAccountClaimFromClusterID(ctx, factory.KubeCli, factory.ClusterID)
		if err != nil {
			return nil, err
		}
		if accountClaim == nil {
			return nil, fmt.Errorf("Could not find any accountClaims for cluster with ID: %s", factory.ClusterID)
		}
		if accountClaim.Spec.AccountLink == "" {
			return nil, fmt.Errorf("an unexpected error occurred: the AccountClaim has no Account")
		}
		factory.AccountName = accountClaim.Spec.AccountLink
		if accountClaim.Spec.SupportRoleARN != "" {
			supportRoleDefined = true
		}
		factory.Awscloudfactory.Region = accountClaim.Spec.Aws.Regions[0].Name
	}

	var err error
	awsClient, err := factory.Awscloudfactory.NewAwsClient()
	if err != nil {
		return nil, err
	}

	var isBYOC bool
	var acctSuffix string
	if factory.AccountName != "" {
		account, err := GetAWSAccount(ctx, factory.KubeCli, factory.AccountNamespace, factory.AccountName)
		if err != nil {
			return nil, err
		}
		factory.AccountID = account.Spec.AwsAccountID
		isBYOC = account.Spec.BYOC
		acctSuffix = account.Labels["iamUserId"]
	} else {
		isBYOC = false
	}

	if factory.AccountID == "" {
		klog.Error("No account ID provided or in Account Claim. Please use -i or ensure ID is in Account Claim referenced in -C or the Account referenced in -A")
		return nil, fmt.Errorf("no account ID provided")
	}

	callerIdentityOutput, err := awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		klog.Error("Fail to get caller identity. Could you please validate the credentials?")
		return nil, err
	}
	factory.Awscloudfactory.CallerIdentity = callerIdentityOutput
	roleArn, err := arn.Parse(aws.StringValue(callerIdentityOutput.Arn))
	if err != nil {
		return nil, err
	}

	splitArn := strings.Split(roleArn.Resource, "/")
	username := splitArn[1]
	factory.Awscloudfactory.SessionName = fmt.Sprintf("RH-SRE-%s", username)

	// If BYOC we need to role-chain to use the right creds.
	// Use the OrgAccess Role by default, override if BYOC
	factory.Awscloudfactory.RoleName = awsv1alpha1.AccountOperatorIAMRole

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
			return nil, fmt.Errorf("Empty ConfigMap Value - CCS-Access-Arn")
		}

		// Build the role-name for Access:
		if acctSuffix == "" {
			klog.Error("Unexpected error parsing the account CR suffix")
			return nil, fmt.Errorf("Unexpected error parsing the account CR suffix")
		}
		factory.Awscloudfactory.RoleName = fmt.Sprintf("ManagedOpenShift-Support-%s", acctSuffix)

		// Get STS Credentials
		if verbose {
			klog.Infof("Elevating Access to SRE Jump Role for user %s\n", factory.Awscloudfactory.SessionName)
		}
		factory.Awscloudfactory.Credentials, err = awsprovider.GetAssumeRoleCredentials(awsClient,
			&factory.Awscloudfactory.ConsoleDuration, aws.String(factory.Awscloudfactory.SessionName), aws.String(roleArn))
		if err != nil {
			klog.Error("Failed to get jump-role creds for CCS")
			return nil, err
		}

		if supportRoleDefined {
			byocClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
				AccessKeyID:     *factory.Awscloudfactory.Credentials.AccessKeyId,
				SecretAccessKey: *factory.Awscloudfactory.Credentials.SecretAccessKey,
				SessionToken:    *factory.Awscloudfactory.Credentials.SessionToken,
				Region:          factory.Awscloudfactory.Region,
			})
			if err != nil {
				return nil, err
			}

			roleArn := cm.Data["support-jump-role"]
			if roleArn == "" {
				klog.Error("Empty Support Jump Role in AwsAccountOperator Configmap")
				return nil, fmt.Errorf("Empty Configmap Value - support-jump-role")
			}

			factory.Awscloudfactory.Credentials, err = awsprovider.GetAssumeRoleCredentials(byocClient,
				&factory.Awscloudfactory.ConsoleDuration, aws.String(factory.Awscloudfactory.SessionName), aws.String(roleArn))
			if err != nil {
				klog.Error("Failed to set support jump-role creds")
				return nil, err
			}
			customerRoleNameSlice := strings.Split(accountClaim.Spec.SupportRoleARN, "/")
			if len(customerRoleNameSlice) != 2 {
				klog.Errorf("Error splitting customer role name from provided ARN: %s", accountClaim.Spec.SupportRoleARN)
				return nil, err
			}
			factory.Awscloudfactory.RoleName = customerRoleNameSlice[1]
		}
	} else {
		factory.Awscloudfactory.Credentials, err = awsprovider.GetAssumeRoleCredentials(awsClient, &factory.Awscloudfactory.ConsoleDuration,
			factory.Awscloudfactory.CallerIdentity.UserId,
			aws.String(fmt.Sprintf("arn:%s:iam::%s:role/%s",
				roleArn.Partition,
				factory.AccountID,
				factory.Awscloudfactory.RoleName)))
		if err != nil {
			return nil, err
		}
	}
	awsClient, err = awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
		AccessKeyID:     *factory.Awscloudfactory.Credentials.AccessKeyId,
		SecretAccessKey: *factory.Awscloudfactory.Credentials.SecretAccessKey,
		SessionToken:    *factory.Awscloudfactory.Credentials.SessionToken,
		Region:          factory.Awscloudfactory.Region,
	})
	if err != nil {
		return nil, err
	}

	callerIdentityOutput, err = awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		klog.Error("failed to get caller identity. Could you please validate the credentials?")
		return nil, err
	}
	factory.Awscloudfactory.CallerIdentity = callerIdentityOutput

	return awsClient, nil
}
