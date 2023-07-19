package cluster

import (
	"context"
	"fmt"
	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var BackplaneClusterAdmin = "backplane-cluster-admin"

func newCmdValidatePullSecret(kubeCli client.Client, flags *genericclioptions.ConfigFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "validate-pull-secret [CLUSTER_ID]",
		Short: "Checks if the pull secret email matches the owner email",
		Long: `Checks if the pull secret email matches the owner email.

The owner's email to check will be determined by the cluster identifier passed to the command, while the pull secret checked will be determined by the cluster that the caller is currently logged in to.`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ValidatePullSecret(args[0], kubeCli, flags))
		},
	}
}

func ValidatePullSecret(clusterID string, kubeCli client.Client, flags *genericclioptions.ConfigFlags) error {
	ocm, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		if ocmCloseErr := ocm.Close(); ocmCloseErr != nil {
			fmt.Printf("Cannot close the ocm (possible memory leak): %q", ocmCloseErr)
		}
	}()

	subscription, err := utils.GetSubscription(ocm, clusterID)
	if err != nil {
		return err
	}

	fmt.Println("Checking if OCM for registry credential list")
	org, err := ocm.AccountsMgmt().V1().Organizations().Organization(subscription.OrganizationID()).Get().Send()
	if err != nil {
		return err
	}
	ebsAccountId := org.Body().EbsAccountID()
	registryCredentials, err := ocm.AccountsMgmt().V1().RegistryCredentials().List().Search(fmt.Sprintf("account_id = '%s'", ebsAccountId)).Send()
	if err != nil {
		return err
	}
	if registryCredentials.Size() == 0 {
		fmt.Println("There is no pull secret in OCM. Sending service log.")
		postCmd := servicelog.PostCmdOptions{
			Template:       "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/update_pull_secret.json",
			TemplateParams: []string{"REGISTRY=registry.redhat.io"},
			ClusterId:      clusterID,
		}
		if err = postCmd.Run(); err != nil {
			return err
		}
		return nil
	}

	fmt.Println("Checking if pull secret email matches user email")

	account, err := utils.GetAccount(ocm, subscription.Creator().ID())
	if err != nil {
		return err
	}

	// This is the flagset for the kubeCli object provided from the root command. Set here to retroactively impersonate backplane-cluster-admin
	flags.Impersonate = &BackplaneClusterAdmin
	secret := &corev1.Secret{}
	if err := kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret); err != nil {
		return err
	}

	clusterPullSecretEmail, err, done := getPullSecretEmail(clusterID, secret, true)
	if done {
		return err
	}

	if account.Email() != clusterPullSecretEmail {
		fmt.Println("Pull secret email doesn't match OCM user email. Sending service log.")
		postCmd := servicelog.PostCmdOptions{
			Template:  "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_user_mismatch.json",
			ClusterId: clusterID,
		}
		if err = postCmd.Run(); err != nil {
			return err
		}
		return nil
	}

	fmt.Println("Email addresses match.")
	return nil
}

func getPullSecretEmail(clusterID string, secret *corev1.Secret, sendServiceLog bool) (string, error, bool) {
	dockerConfigJsonBytes, found := secret.Data[".dockerconfigjson"]
	if !found {
		// Indicates issue w/ pull-secret, so we can stop evaluating and specify a more direct course of action
		fmt.Println("Secret does not contain expected key '.dockerconfigjson'.")
		if sendServiceLog {
			fmt.Println("Sending service log.")
			postCmd := servicelog.PostCmdOptions{
				Template:  "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_change_breaking_upgradesync.json",
				ClusterId: clusterID,
			}
			if err := postCmd.Run(); err != nil {
				return "", err, true
			}
		}

		return "", nil, true
	}

	dockerConfigJson, err := v1.UnmarshalAccessToken(dockerConfigJsonBytes)
	if err != nil {
		return "", err, true
	}

	cloudOpenshiftAuth, found := dockerConfigJson.Auths()["cloud.openshift.com"]
	if !found {
		fmt.Println("Secret does not contain entry for cloud.openshift.com")
		if sendServiceLog {
			fmt.Println("Sending service log")
			postCmd := servicelog.PostCmdOptions{
				Template:  "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_change_breaking_upgradesync.json",
				ClusterId: clusterID,
			}
			if err = postCmd.Run(); err != nil {
				return "", err, true
			}
		}
		return "", nil, true
	}

	clusterPullSecretEmail := cloudOpenshiftAuth.Email()
	if clusterPullSecretEmail == "" {
		fmt.Printf("%v\n%v\n%v\n",
			"Couldn't extract email address from pull secret for cloud.openshift.com",
			"This can mean the pull secret is misconfigured. Please verify the pull secret manually:",
			"  oc get secret -n openshift-config pull-secret -o json | jq -r '.data[\".dockerconfigjson\"]' | base64 -d")
		return "", nil, true
	}
	return clusterPullSecretEmail, nil, false
}
