package account

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/spf13/cobra"

	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

// newCmdRotateSecret implements the rotate-secret command which rotate IAM User credentials
func newCmdRotateSecret(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newRotateSecretOptions(streams, flags, client)
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

	return rotateSecretCmd
}

// rotateSecretOptions defines the struct for running rotate-iam command
type rotateSecretOptions struct {
	accountCRName     string
	profile           string
	updateCcsCreds    bool
	awsAccountTimeout *int64

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newRotateSecretOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *rotateSecretOptions {
	return &rotateSecretOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *rotateSecretOptions) complete(cmd *cobra.Command, args []string) error {

	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Account CR argument is required")
	}

	o.accountCRName = args[0]

	if o.profile == "" {
		o.profile = "default"
	}

	// The aws account timeout. The min the API supports is 15mins.
	// 900 sec is 15min
	o.awsAccountTimeout = aws.Int64(900)

	return nil
}

func (o *rotateSecretOptions) run() error {

	ctx := context.TODO()
	var err error

	// Get the associated Account CR from the provided name
	var accountID string
	account, err := k8s.GetAWSAccount(ctx, o.kubeCli, common.AWSAccountNamespace, o.accountCRName)
	if err != nil {
		return err
	}
	if account.Spec.ManualSTSMode {
		return fmt.Errorf("Account %s is STS - No IAM User Credentials to Rotate", o.accountCRName)
	}

	// Set the account ID
	accountID = account.Spec.AwsAccountID

	// Get IAM user suffix from CR label

	accountIDSuffixLabel, ok := account.Labels["iamUserId"]
	if !ok {
		return fmt.Errorf("no label on Account CR for IAM User")
	}

	// Use provided profile
	awsSetupClient, err := awsprovider.NewAwsClient(o.profile, "", "")
	if err != nil {
		return err
	}

	// Ensure AWS calls are successful with client
	callerIdentityOutput, err := awsSetupClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}

	var credentials *sts.Credentials
	// Need to role chain if the cluster is CCS
	if account.Spec.BYOC {
		// Get the aws-account-operator configmap
		cm := &corev1.ConfigMap{}
		cmErr := o.kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: common.AWSAccountNamespace, Name: common.DefaultConfigMap}, cm)
		if cmErr != nil {
			return fmt.Errorf("there was an error getting the ConfigMap to get the SRE Access Role %s", cmErr)
		}
		// Get the ARN value
		SREAccessARN := cm.Data["CCS-Access-Arn"]
		if SREAccessARN == "" {
			return fmt.Errorf("SRE Access ARN is missing from configmap")
		}

		// Assume the ARN
		srepRoleCredentials, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, o.awsAccountTimeout, callerIdentityOutput.UserId, &SREAccessARN)
		if err != nil {
			return err
		}

		// Create client with the SREP role
		srepRoleClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
			AccessKeyID:     *srepRoleCredentials.AccessKeyId,
			SecretAccessKey: *srepRoleCredentials.SecretAccessKey,
			SessionToken:    *srepRoleCredentials.SessionToken,
			Region:          "us-east-1",
		})
		if err != nil {
			return err
		}

		// Get the Jump ARN value
		JumpARN := cm.Data["support-jump-role"]
		if JumpARN == "" {
			return fmt.Errorf("jump Access ARN is missing from configmap")
		}
		// Assume the ARN
		jumpRoleCreds, err := awsprovider.GetAssumeRoleCredentials(srepRoleClient, o.awsAccountTimeout, callerIdentityOutput.UserId, &JumpARN)
		if err != nil {
			return err
		}
		// Create client with the Jump role
		jumpRoleClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
			AccessKeyID:     *jumpRoleCreds.AccessKeyId,
			SecretAccessKey: *jumpRoleCreds.SecretAccessKey,
			SessionToken:    *jumpRoleCreds.SessionToken,
			Region:          "us-east-1",
		})
		if err != nil {
			return err
		}
		// Role chain to assume ManagedOpenShift-Support-{uid}
		roleArn := aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, "ManagedOpenShift-Support-"+accountIDSuffixLabel))
		credentials, err = awsprovider.GetAssumeRoleCredentials(jumpRoleClient, o.awsAccountTimeout,
			callerIdentityOutput.UserId, roleArn)
		if err != nil {
			return err
		}

	} else {
		// Assume the OrganizationAdminAccess role
		roleArn := aws.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, awsv1alpha1.AccountOperatorIAMRole))
		credentials, err = awsprovider.GetAssumeRoleCredentials(awsSetupClient, o.awsAccountTimeout,
			callerIdentityOutput.UserId, roleArn)
		if err != nil {
			return err
		}
	}

	// Build a new client with the assumed role
	awsClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.AwsClientInput{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		Region:          "us-east-1",
	})
	if err != nil {
		return err
	}

	// Update osdManagedAdmin secrets
	// Username is osdManagedAdmin-aaabbb
	osdManagedAdminUsername := common.OSDManagedAdminIAM + "-" + accountIDSuffixLabel

	// Create new access key
	createAccessKeyOutput, err := awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
		UserName: aws.String(osdManagedAdminUsername),
	})
	if err != nil {
		return err
	}

	// Place new credentials into body for secret
	newOsdManagedAdminSecretData := map[string][]byte{
		"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
		"aws_access_key_id":     []byte(*createAccessKeyOutput.AccessKey.AccessKeyId),
		"aws_secret_access_key": []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey),
	}

	// Update existing osdManagedAdmin secret
	err = common.UpdateSecret(o.kubeCli, o.accountCRName+"-secret", common.AWSAccountNamespace, newOsdManagedAdminSecretData)
	if err != nil {
		return err
	}

	// Update secret in ClusterDeployment's namespace
	err = common.UpdateSecret(o.kubeCli, "aws", account.Spec.ClaimLinkNamespace, newOsdManagedAdminSecretData)
	if err != nil {
		return err
	}

	fmt.Println("AWS creds updated on hive.")

	clusterDeployments := &hiveapiv1.ClusterDeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(account.Spec.ClaimLinkNamespace),
	}

	err = o.kubeCli.List(ctx, clusterDeployments, listOpts...)
	if err != nil {
		return err
	}

	if len(clusterDeployments.Items) == 0 {
		return fmt.Errorf("failed to retreive cluster deployments")
	}
	cdName := clusterDeployments.Items[0].ObjectMeta.Name

	// Create syncset to deploy the updated creds to the cluster for CCO
	syncSetName := "aws-sync"
	syncSet := &hiveapiv1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      syncSetName,
			Namespace: account.Spec.ClaimLinkNamespace,
		},
		Spec: hiveapiv1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: cdName,
				},
			},
			SyncSetCommonSpec: hiveapiv1.SyncSetCommonSpec{
				ResourceApplyMode: "Upsert",
				Secrets: []hiveapiv1.SecretMapping{
					{
						SourceRef: hiveapiv1.SecretReference{
							Name: "aws",
						},
						TargetRef: hiveapiv1.SecretReference{
							Name:      "aws-creds",
							Namespace: "kube-system",
						},
					},
				},
			},
		},
	}
	fmt.Println("Syncing AWS creds down to cluster.")
	err = o.kubeCli.Create(ctx, syncSet)
	if err != nil {
		return err
	}

	fmt.Printf("Watching Cluster Sync Status for deployment...")
	hiveinternalv1alpha1.AddToScheme(o.kubeCli.Scheme())
	searchStatus := &hiveinternalv1alpha1.ClusterSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cdName,
			Namespace: account.Spec.ClaimLinkNamespace,
		},
	}
	foundStatus := &hiveinternalv1alpha1.ClusterSync{}
	isSSSynced := false
	for i := 0; i < 6; i++ {
		err = o.kubeCli.Get(ctx, client.ObjectKeyFromObject(searchStatus), foundStatus)
		if err != nil {
			return err
		}

		for _, status := range foundStatus.Status.SyncSets {
			if status.Name == syncSetName {
				if status.FirstSuccessTime != nil {
					isSSSynced = true
					break
				}
			}
		}

		if isSSSynced {
			fmt.Printf("\nSync completed...\n")
			break
		}

		fmt.Printf(".")
		time.Sleep(time.Second * 5)
	}
	if !isSSSynced {
		return fmt.Errorf("syncset failed to sync. Please verify")
	}

	// Clean up the SS on hive
	err = o.kubeCli.Delete(ctx, syncSet)
	if err != nil {
		return err
	}

	fmt.Printf("Successfully rotated secrets for %s\n", osdManagedAdminUsername)

	// Only update osdCcsAdmin credential if specified
	if o.updateCcsCreds {
		// Only update if the Account CR is actually CCS
		if account.Spec.BYOC {
			// Rotate osdCcsAdmin creds
			createAccessKeyOutputCCS, err := awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
				UserName: aws.String("osdCcsAdmin"),
			})
			if err != nil {
				return err
			}

			newOsdCcsAdminSecretData := map[string][]byte{
				"aws_user_name":         []byte(*createAccessKeyOutputCCS.AccessKey.UserName),
				"aws_access_key_id":     []byte(*createAccessKeyOutputCCS.AccessKey.AccessKeyId),
				"aws_secret_access_key": []byte(*createAccessKeyOutputCCS.AccessKey.SecretAccessKey),
			}

			// Update byoc secret with new creds
			err = common.UpdateSecret(o.kubeCli, "byoc", account.Spec.ClaimLinkNamespace, newOsdCcsAdminSecretData)
			if err != nil {
				return err
			}

			fmt.Println("Successfully rotated secrets for osdCcsAdmin")
		} else {
			// Check yo self
			fmt.Println("Account is not CCS, skipping osdCcsAdmin credential rotation")
		}
	}

	return nil
}
