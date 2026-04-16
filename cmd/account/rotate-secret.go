package account

import (
	"context"
	"fmt"
	"os"
	"strings"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	ccov1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/controller"
	"github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newCmdRotateSecret implements the rotate-secret command which rotate IAM User credentials
func newCmdRotateSecret(streams genericclioptions.IOStreams, client *k8s.LazyClient) *cobra.Command {
	ops := newRotateSecretOptions(streams, client)
	rotateSecretCmd := &cobra.Command{
		Use:               "rotate-secret <aws-account-cr-name>",
		Short:             "Rotate IAM credentials secret",
		Long:              "When logged into a hive shard, this rotates IAM credential secrets for a given `account` CR.",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	rotateSecretCmd.Flags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile")
	rotateSecretCmd.Flags().BoolVar(&ops.updateCcsCreds, "ccs", false, "Also rotates osdCcsAdmin credential. Use caution.")
	rotateSecretCmd.Flags().BoolVar(&ops.dryRun, "dry-run", false, "Only print what actions would be taken without performing any mutations (no AWS key creation/deletion, no k8s resource changes)")
	rotateSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)")
	rotateSecretCmd.Flags().StringVar(&ops.osdManagedAdminUsername, "admin-username", "", "The admin username to use for generating access keys. Must be in the format of `osdManagedAdmin*`. If not specified, this is inferred from the account CR.")
	rotateSecretCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "OCM internal/external cluster id or cluster name")
	rotateSecretCmd.Flags().StringVar(&ops.hiveOcmUrl, "hive-ocm-url", "", "(optional) OCM environment URL for Hive operations. Aliases: 'production', 'staging', 'integration'. This only changes how the Hive cluster is resolved; the target cluster still comes from the current/default OCM environment.")
	_ = rotateSecretCmd.MarkFlagRequired("reason")
	_ = rotateSecretCmd.MarkFlagRequired("cluster-id")

	return rotateSecretCmd
}

// rotateSecretOptions defines the struct for running rotate-iam command
type rotateSecretOptions struct {
	accountCRName           string
	profile                 string
	updateCcsCreds          bool
	dryRun                  bool
	awsAccountTimeout       *int32
	reason                  string
	osdManagedAdminUsername string
	clusterID               string
	hiveOcmUrl              string

	genericclioptions.IOStreams
	kubeCli *k8s.LazyClient
}

func newRotateSecretOptions(streams genericclioptions.IOStreams, client *k8s.LazyClient) *rotateSecretOptions {
	return &rotateSecretOptions{
		IOStreams: streams,
		kubeCli:   client,
	}
}

func getSessionNameFromUserId(userid string) string {
	return strings.Replace(userid, ":", "-", 1)
}

func (o *rotateSecretOptions) complete(cmd *cobra.Command, args []string) error {

	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Account CR argument is required")
	}

	o.accountCRName = args[0]

	// The aws account timeout. The min the API supports is 15mins.
	// 900 sec is 15min
	o.awsAccountTimeout = awsSdk.Int32(900)

	if o.osdManagedAdminUsername != "" && !strings.HasPrefix(o.osdManagedAdminUsername, common.OSDManagedAdminIAM) {
		return cmdutil.UsageErrorf(cmd, "admin-username must start with %v", common.OSDManagedAdminIAM)
	}

	return nil
}

func (o *rotateSecretOptions) run() error {

	ctx := context.TODO()
	var err error

	// Resolve the k8s client for hive operations.
	var kubeCli client.Client
	if o.hiveOcmUrl != "" {
		kubeCli, err = o.initHiveClient()
		if err != nil {
			return fmt.Errorf("failed to initialize hive client: %w", err)
		}
	} else {
		o.kubeCli.Impersonate("backplane-cluster-admin", o.reason, fmt.Sprintf("Elevation required to rotate secrets %s aws-account-cr-name", o.accountCRName))
		kubeCli = o.kubeCli
	}

	// Get the associated Account CR from the provided name
	account, err := k8s.GetAWSAccount(ctx, kubeCli, common.AWSAccountNamespace, o.accountCRName)
	if err != nil {
		return err
	}

	accountID := account.Spec.AwsAccountID

	// Get IAM user suffix from CR label (needed for role chaining ARN)
	accountIDSuffixLabel, ok := account.Labels["iamUserId"]
	if !ok {
		return fmt.Errorf("no label on Account CR for IAM User")
	}

	awsSetupClient, err := awsprovider.NewAwsClient(o.profile, "us-east-1", "")
	if err != nil {
		return err
	}

	callerIdentityOutput, err := awsSetupClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}
	roleSessionName := getSessionNameFromUserId(*callerIdentityOutput.UserId)

	// Build the final AWS client via role chaining
	awsClient, err := o.buildAssumedRoleClient(ctx, kubeCli, awsSetupClient, account, accountID, accountIDSuffixLabel, &roleSessionName)
	if err != nil {
		return err
	}

	// Create a k8s client for the managed cluster (uses the default/target OCM
	// environment, not the hive one) to delete CredentialRequests after sync.
	managedScheme := runtime.NewScheme()
	_ = ccov1.AddToScheme(managedScheme)
	managedClient, err := k8s.NewAsBackplaneClusterAdmin(
		o.clusterID,
		client.Options{Scheme: managedScheme},
		o.reason,
		fmt.Sprintf("Elevation required to rotate CredentialRequests for %s", o.accountCRName),
	)
	if err != nil {
		return fmt.Errorf("failed to create managed cluster client: %w", err)
	}

	return controller.RotateSecret(ctx, &controller.RotateSecretInput{
		AccountCRName:           o.accountCRName,
		Account:                 account,
		OsdManagedAdminUsername: o.osdManagedAdminUsername,
		UpdateCcsCreds:          o.updateCcsCreds,
		DryRun:                  o.dryRun,
		AwsClient:               awsClient,
		HiveKubeClient:          kubeCli,
		ManagedClusterClient:    managedClient,
		Out:                     os.Stdout,
	})
}

// buildAssumedRoleClient performs the BYOC or non-BYOC role chain to get an
// AWS client with permissions in the target account.
func (o *rotateSecretOptions) buildAssumedRoleClient(
	ctx context.Context,
	kubeCli client.Client,
	awsSetupClient awsprovider.Client,
	account *awsv1alpha1.Account,
	accountID string,
	accountIDSuffixLabel string,
	roleSessionName *string,
) (awsprovider.Client, error) {

	var credAccessKeyId, credSecretAccessKey, credSessionToken *string

	if account.Spec.BYOC {
		// Get the aws-account-operator configmap
		cm := &corev1.ConfigMap{}
		cmErr := kubeCli.Get(ctx, types.NamespacedName{Namespace: common.AWSAccountNamespace, Name: common.DefaultConfigMap}, cm)
		if cmErr != nil {
			return nil, fmt.Errorf("there was an error getting the ConfigMap to get the SRE Access Role %s", cmErr)
		}

		SREAccessARN := cm.Data["CCS-Access-Arn"]
		if SREAccessARN == "" {
			return nil, fmt.Errorf("SRE Access ARN is missing from configmap")
		}

		srepRoleCredentials, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, o.awsAccountTimeout, roleSessionName, &SREAccessARN)
		if err != nil {
			return nil, err
		}

		srepRoleClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
			AccessKeyID:     *srepRoleCredentials.AccessKeyId,
			SecretAccessKey: *srepRoleCredentials.SecretAccessKey,
			SessionToken:    *srepRoleCredentials.SessionToken,
			Region:          "us-east-1",
		})
		if err != nil {
			return nil, err
		}

		JumpARN := cm.Data["support-jump-role"]
		if JumpARN == "" {
			return nil, fmt.Errorf("jump Access ARN is missing from configmap")
		}

		jumpRoleCreds, err := awsprovider.GetAssumeRoleCredentials(srepRoleClient, o.awsAccountTimeout, roleSessionName, &JumpARN)
		if err != nil {
			return nil, err
		}

		jumpRoleClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
			AccessKeyID:     *jumpRoleCreds.AccessKeyId,
			SecretAccessKey: *jumpRoleCreds.SecretAccessKey,
			SessionToken:    *jumpRoleCreds.SessionToken,
			Region:          "us-east-1",
		})
		if err != nil {
			return nil, err
		}

		roleArn := awsSdk.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, "ManagedOpenShift-Support-"+accountIDSuffixLabel))
		credentials, err := awsprovider.GetAssumeRoleCredentials(jumpRoleClient, o.awsAccountTimeout, roleSessionName, roleArn)
		if err != nil {
			return nil, err
		}
		credAccessKeyId = credentials.AccessKeyId
		credSecretAccessKey = credentials.SecretAccessKey
		credSessionToken = credentials.SessionToken
	} else {
		roleArn := awsSdk.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, awsv1alpha1.AccountOperatorIAMRole))
		credentials, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, o.awsAccountTimeout, roleSessionName, roleArn)
		if err != nil {
			return nil, err
		}
		credAccessKeyId = credentials.AccessKeyId
		credSecretAccessKey = credentials.SecretAccessKey
		credSessionToken = credentials.SessionToken
	}

	return awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
		AccessKeyID:     *credAccessKeyId,
		SecretAccessKey: *credSecretAccessKey,
		SessionToken:    *credSessionToken,
		Region:          "us-east-1",
	})
}

// initHiveClient creates a k8s client connected to the hive cluster that manages the target cluster.
func (o *rotateSecretOptions) initHiveClient() (client.Client, error) {
	resolvedURL, err := utils.ValidateAndResolveOcmUrl(o.hiveOcmUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid --hive-ocm-url: %w", err)
	}

	targetOCM, err := utils.CreateConnection()
	if err != nil {
		return nil, fmt.Errorf("failed to create target cluster OCM connection: %w", err)
	}
	defer targetOCM.Close()

	hiveOCM, err := utils.CreateConnectionWithUrl(resolvedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create hive OCM connection with URL '%s': %w", resolvedURL, err)
	}
	defer hiveOCM.Close()

	cluster, err := utils.GetClusterAnyStatus(targetOCM, o.clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OCM cluster info for %s: %w", o.clusterID, err)
	}

	hive, err := utils.GetHiveClusterWithConn(cluster.ID(), targetOCM, hiveOCM)
	if err != nil {
		return nil, fmt.Errorf("failed to get hive cluster (OCM URL:'%s'): %w", resolvedURL, err)
	}

	fmt.Printf("Connecting to hive cluster %s via OCM URL: %s\n", hive.Name(), resolvedURL)

	elevationMsg := fmt.Sprintf("Elevation required to rotate secrets for %s", o.accountCRName)
	hiveClient, err := k8s.NewAsBackplaneClusterAdminWithConn(
		hive.ID(),
		client.Options{Scheme: scheme.Scheme},
		hiveOCM,
		o.reason,
		elevationMsg,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create hive k8s client (OCM URL:'%s'): %w", resolvedURL, err)
	}

	return hiveClient, nil
}
