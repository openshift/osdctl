package cluster

import (
	"context"
	"fmt"
	"os"

	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// transferOwnerOptions defines the struct for running transferOwner command
type validatePullSecret struct {
	clusterID string
	reason    string
	kubeCli   *k8s.LazyClient
}

func newCmdValidatePullSecret(kubeCli *k8s.LazyClient) *cobra.Command {
	ops := newValidatePullSecret(kubeCli)
	validatePullSecretCmd := &cobra.Command{
		Use:   "validate-pull-secret [CLUSTER_ID]",
		Short: "Checks if the pull secret email matches the owner email",
		Long: `Checks if the pull secret email matches the owner email.

The owner's email to check will be determined by the cluster identifier passed to the command, while the pull secret checked will be determined by the cluster that the caller is currently logged in to.`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ops.clusterID = args[0]
			cmdutil.CheckErr(ops.ValidatePullSecret())
		},
	}
	validatePullSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)")
	_ = validatePullSecretCmd.MarkFlagRequired("reason")
	return validatePullSecretCmd
}

func newValidatePullSecret(client *k8s.LazyClient) *validatePullSecret {
	return &validatePullSecret{
		kubeCli: client,
	}
}

func (o *validatePullSecret) ValidatePullSecret() error {
	ocm, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		if ocmCloseErr := ocm.Close(); ocmCloseErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Cannot close the ocm (possible memory leak): %q", ocmCloseErr)
		}
	}()

	subscription, err := utils.GetSubscription(ocm, o.clusterID)
	if err != nil {
		return err
	}

	account, err := utils.GetAccount(ocm, subscription.Creator().ID())
	if err != nil {
		return err
	}

	registryCredentials, err := utils.GetRegistryCredentials(ocm, account.ID())
	if err != nil {
		return err
	}
	if len(registryCredentials) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "There is no pull secret in OCM. Sending service log.")
		postCmd := servicelog.PostCmdOptions{
			Template:       "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/update_pull_secret.json",
			TemplateParams: []string{"REGISTRY=registry.redhat.io"},
			ClusterId:      o.clusterID,
		}
		if err = postCmd.Run(); err != nil {
			return err
		}
		return nil
	}

	// We need to specify we will need elevation before using the client for the first time
	o.kubeCli.Impersonate("backplane-cluster-admin", o.reason, fmt.Sprintf("Elevation required to get pull secret email to check if it matches the owner email for %s cluster", o.clusterID))
	secret := &corev1.Secret{}
	if err := o.kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret); err != nil {
		return err
	}

	clusterPullSecretEmail, err, done := getPullSecretEmail(o.clusterID, secret, true)
	if done {
		return err
	}

	if account.Email() != clusterPullSecretEmail {
		_, _ = fmt.Fprintln(os.Stderr, "Pull secret email doesn't match OCM user email. Sending service log.")
		postCmd := servicelog.PostCmdOptions{
			Template:  "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_user_mismatch.json",
			ClusterId: o.clusterID,
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
		_, _ = fmt.Fprintln(os.Stderr, "Secret does not contain expected key '.dockerconfigjson'. Sending service log.")
		if sendServiceLog {
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
		_, _ = fmt.Fprintln(os.Stderr, "Secret does not contain entry for cloud.openshift.com")
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
		_, _ = fmt.Fprintf(os.Stderr, "%v\n%v\n%v\n",
			"Couldn't extract email address from pull secret for cloud.openshift.com",
			"This can mean the pull secret is misconfigured. Please verify the pull secret manually:",
			"  oc get secret -n openshift-config pull-secret -o json | jq -r '.data[\".dockerconfigjson\"]' | base64 -d")
		return "", nil, true
	}
	return clusterPullSecretEmail, nil, false
}
