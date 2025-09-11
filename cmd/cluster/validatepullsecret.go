package cluster

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/AlecAivazis/survey/v2"
	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	bpapi "github.com/openshift/backplane-cli/pkg/backplaneapi"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
	bputils "github.com/openshift/backplane-cli/pkg/utils"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/backplane"
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
	clusterID     string
	managedScript bool
	reason        string
}

var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

func newCmdValidatePullSecret() *cobra.Command {
	ops := newValidatePullSecretOptions()
	validatePullSecretCmd := &cobra.Command{
		Use:   "validate-pull-secret --cluster-id <cluster-identifier>",
		Short: "Checks if the pull secret email matches the owner email",
		Long: `Checks if the pull secret email matches the owner email.

The command will by default attempt to create a managedjob on the cluster to complete the task.
However if this fails (e.g. pod fails to run on the cluster), the fallback option of elevating
with backplane (requires reason for elevation) can be run. You can also directly use backplane
elevation by setting --managed-script=false.
`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
	}

	validatePullSecretCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The internal ID of the cluster to check (only required if elevating, and ID is not found within context.)")
	validatePullSecretCmd.Flags().BoolVar(&ops.managedScript, "managed-script", true, "Use managed job approach to get pull secret (default true). Set to false to use backplane elevation directly")
	validatePullSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command to be run (usually an OHSS or PD ticket)")

	return validatePullSecretCmd
}

func newValidatePullSecretOptions() *validatePullSecretOptions {
	return &validatePullSecretOptions{}
}

func (o *validatePullSecretOptions) run() error {
	if o.clusterID == "" {
		bpCluster, err := bputils.DefaultClusterUtils.GetBackplaneCluster()
		if err != nil {
			return fmt.Errorf("no cluster-id provided and failed to get cluster from current context: %w. Please provide --cluster-id or ensure you're logged into a cluster", err)
		}
		o.clusterID = bpCluster.ClusterID
		fmt.Printf("Using cluster from current context: %s\n", o.clusterID)
	}

	// get the pull secret in OCM
	emailOCM, err, done := o.getPullSecretFromOCM()
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	var emailCluster string
	var clusterErr error

	if o.managedScript {
		fmt.Println("Creating managedjob in-cluster to get pull-secret email")
		emailCluster, clusterErr = o.getPullSecretWithManagedJob()
		if clusterErr != nil {
			fmt.Printf("Managed job failed: %v\n", clusterErr)
			fmt.Println("Falling back to elevated access...")
		}
	}

	if !o.managedScript || clusterErr != nil {
		if o.reason == "" {
			var err error
			o.reason, err = o.promptForReason()
			if err != nil {
				return fmt.Errorf("failed to get reason for elevation: %w", err)
			}
		}
		emailCluster, clusterErr, done = getPullSecretElevated(o.clusterID, o.reason)
		if clusterErr != nil {
			return clusterErr
		}
		if done {
			return nil
		}
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

func (o *validatePullSecretOptions) getPullSecretWithManagedJob() (email string, err error) {
	bp, err := bpconfig.GetBackplaneConfiguration()
	if err != nil {
		return "", fmt.Errorf("failed to load backplane configuration: %w", err)
	}

	bpclient, err := bpapi.DefaultClientUtils.MakeRawBackplaneAPIClient(bp.URL)
	if err != nil {
		return "", fmt.Errorf("failed to create backplane API client: %w", err)
	}

	client := backplane.NewClient(bpclient, o.clusterID)
	canonicalName := "security/get-pull-secret-email"
	parameters := map[string]string{}

	result, err := client.RunManagedJobWithClient(canonicalName, parameters, 60)
	if err != nil {
		return "", err
	}

	fmt.Printf("email from managedjob (cluster): %s\n", result.Output)

	email = emailRegex.FindString(result.Output)
	if email == "" {
		return "", fmt.Errorf("failed to extract email from job output: %s", result.Output)
	}

	return email, nil
}

func (o *validatePullSecretOptions) promptForReason() (string, error) {
	prompt := &survey.Input{
		Message: "Enter reason for elevation (usually an OHSS or PD ticket):",
	}
	var reason string
	err := survey.AskOne(prompt, &reason, survey.WithValidator(survey.Required))
	if err != nil {
		return "", err
	}

	return reason, nil
}

// getPullSecretElevated gets the pull-secret in the cluster with backplane elevation.
func getPullSecretElevated(clusterID string, reason string) (email string, err error, sentSL bool) {
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
	ocm, err := utils.CreateConnection()
	if err != nil {
		return "", err, false
	}
	defer func() {
		if ocmCloseErr := ocm.Close(); ocmCloseErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Cannot close the ocm (possible memory leak): %q", ocmCloseErr)
		}
	}()

	subscription, err := utils.GetSubscription(ocm, o.clusterID)
	if err != nil {
		return "", err, false
	}

	account, err := utils.GetAccount(ocm, subscription.Creator().ID())
	if err != nil {
		return "", err, false
	}

	// validate the registryCredentials before return
	registryCredentials, err := utils.GetRegistryCredentials(ocm, account.ID())
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
