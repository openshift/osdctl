package cluster

import (
	"context"
	"fmt"
	"os"

	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// validatePullSecretOptions defines the struct for running validate-pull-secret command
type validatePullSecretOptions struct {
	clusterID            string
	elevate              bool
	reason               string
	pullSecretFetcher    PullSecretFetcher
	clusterSecretFetcher ClusterPullSecretFetcher
	oCMClientInterface   OCMClientInterface
}

func newCmdValidatePullSecret() *cobra.Command {
	ops := newValidatePullSecretOptions()
	validatePullSecretCmd := &cobra.Command{
		Use:   "validate-pull-secret [CLUSTER_ID]",
		Short: "Checks if the pull secret email matches the owner email",
		Long: `Checks if the pull secret email matches the owner email.

This command will automatically login to the cluster to check the current pull-secret defined in 'openshift-config/pull-secret'
`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ops.clusterID = args[0]
			cmdutil.CheckErr(ops.run())
		},
	}
	validatePullSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command to be run (usualy an OHSS or PD ticket), mandatory when using elevate")
	_ = validatePullSecretCmd.MarkFlagRequired("reason")
	return validatePullSecretCmd
}

func newValidatePullSecretOptions() *validatePullSecretOptions {
	return &validatePullSecretOptions{
		clusterSecretFetcher: &clusterPullSecretFetcher{},
		oCMClientInterface:   &OCMClientImpl{},
	}
}

type PullSecretFetcher interface {
	getPullSecretFromOCM() (string, error, bool)
}

// Define an interface for getPullSecretElevated
type ClusterPullSecretFetcher interface {
	getPullSecretElevated(clusterID string, reason string) (string, error, bool)
}

type clusterPullSecretFetcher struct{}

type OCMClientInterface interface {
	CreateConnection() (*sdk.Connection, error)
	GetSubscription(connection *sdk.Connection, key string) (*v1.Subscription, error)
	GetAccount(connection *sdk.Connection, key string) (*v1.Account, error)
	GetRegistryCredentials(connection *sdk.Connection, key string) ([]*v1.RegistryCredential, error)
	Close() error
}
type OCMClientImpl struct {
	ocm *sdk.Connection
}

func (o *OCMClientImpl) CreateConnection() (*sdk.Connection, error) {
	obj, err := utils.CreateConnection()
	o.ocm = obj
	return obj, err
}

func (o *OCMClientImpl) GetSubscription(connection *sdk.Connection, key string) (*v1.Subscription, error) {
	return utils.GetSubscription(connection, key)
}

func (o *OCMClientImpl) GetAccount(connection *sdk.Connection, key string) (*v1.Account, error) {
	return utils.GetAccount(connection, key)
}

func (o *OCMClientImpl) GetRegistryCredentials(connection *sdk.Connection, key string) ([]*v1.RegistryCredential, error) {
	return utils.GetRegistryCredentials(connection, key)
}

func (o *OCMClientImpl) Close() error {
	return o.ocm.Close()
}

func (o *validatePullSecretOptions) run() error {
	// get the pull secret in OCM
	emailOCM, err, done := o.pullSecretFetcher.getPullSecretFromOCM()
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	// get the pull secret in cluster
	emailCluster, err, done := o.clusterSecretFetcher.getPullSecretElevated(o.clusterID, o.reason)
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	if emailOCM != emailCluster {
		_, _ = fmt.Fprintln(os.Stderr, "Pull secret email doesn't match OCM user email. Sending service log.")
		postCmd := servicelog.PostCmdOptions{
			Template:  "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_user_mismatch.json",
			ClusterId: o.clusterID,
		}
		return postCmd.Run()
	}

	fmt.Println("Email addresses match.")
	return nil
}

// getPullSecretElevated gets the pull-secret in the cluster with backplane elevation.
func (c clusterPullSecretFetcher) getPullSecretElevated(clusterID string, reason string) (email string, err error, sentSL bool) {
	kubeClient, err := k8s.NewAsBackplaneClusterAdmin(clusterID, client.Options{}, reason)
	if err != nil {
		return "", fmt.Errorf("failed to login to cluster as 'backplane-cluster-admin': %w", err), false
	}

	secret := &corev1.Secret{}
	if err := kubeClient.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret); err != nil {
		return "", err, false
	}

	clusterPullSecretEmail, err, done := getPullSecretEmail(clusterID, secret, true)
	if done {
		return "", err, true
	}
	fmt.Printf("email from cluster: %s\n", clusterPullSecretEmail)

	return clusterPullSecretEmail, nil, false
}

// getPullSecretFromOCM gets the cluster owner email from OCM
// it returns the email, error and done
// done means a service log has been sent
func (o *validatePullSecretOptions) getPullSecretFromOCM() (string, error, bool) {
	fmt.Println("Getting email from OCM")
	ocm, err := o.oCMClientInterface.CreateConnection()
	if err != nil {
		return "", err, false
	}
	defer func() {
		if ocmCloseErr := o.oCMClientInterface.Close(); ocmCloseErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Cannot close the ocm (possible memory leak): %q", ocmCloseErr)
		}
	}()

	subscription, err := o.oCMClientInterface.GetSubscription(ocm, o.clusterID)
	if err != nil {
		fmt.Println("error is", err)
		return "", err, false
	}

	account, err := o.oCMClientInterface.GetAccount(ocm, subscription.Creator().ID())
	if err != nil {
		return "", err, false
	}

	// validate the registryCredentials before return
	registryCredentials, err := o.oCMClientInterface.GetRegistryCredentials(ocm, account.ID())
	if err != nil {
		return "", err, false
	}
	if len(registryCredentials) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "There is no pull secret in OCM. Sending service log.")
		postCmd := servicelog.PostCmdOptions{
			Template:       "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/update_pull_secret.json",
			TemplateParams: []string{"REGISTRY=registry.redhat.io"},
			ClusterId:      o.clusterID,
		}
		if err = postCmd.Run(); err != nil {
			return "", err, false
		}
		return "", nil, true
	}

	fmt.Printf("email from OCM: %s\n", account.Email())
	return account.Email(), nil, false
}

// getPullSecretEmail extract the email from the pull-secret secret in cluster
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
