package cluster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var BackplaneClusterAdmin = "backplane-cluster-admin"

// validatePullSecretOptions defines the struct for running validate-pull-secret command
type validatePullSecretOptions struct {
	clusterID string
	elevate   bool
	kubeCli   *k8s.LazyClient
	reason    string
}

func newCmdValidatePullSecret(kubeCli *k8s.LazyClient) *cobra.Command {
	ops := newValidatePullSecretOptions(kubeCli)
	validatePullSecretCmd := &cobra.Command{
		Use:   "validate-pull-secret [CLUSTER_ID]",
		Short: "Checks if the pull secret email matches the owner email",
		Long: `Checks if the pull secret email matches the owner email.

The owner's email to check will be determined by the cluster identifier passed to the command, while the pull secret checked will be determined by the cluster that the caller is currently logged in to.

By default, it will run a managed-script in the cluster to get the pull-secret's email in the cluster.

In case the managed-script fails, --elevate can be added to get the pull-secret in the cluster directly without a managed-script.
`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ops.clusterID = args[0]
			cmdutil.CheckErr(ops.run())
		},
	}
	validatePullSecretCmd.Flags().BoolVar(&ops.elevate, "elevate", false, "get pull-secret with backplane-cluster-admin without running a managed-script, mandatory when a reason is provided")
	validatePullSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command to be run (usualy an OHSS or PD ticket), mandatory when using elevate")
	validatePullSecretCmd.MarkFlagsRequiredTogether("elevate", "reason")
	return validatePullSecretCmd
}

func newValidatePullSecretOptions(client *k8s.LazyClient) *validatePullSecretOptions {
	return &validatePullSecretOptions{
		kubeCli: client,
	}
}

func (o *validatePullSecretOptions) run() error {
	// get the pull secret in OCM
	emailOCM, err, done := o.getPullSecretFromOCM()
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	// get the pull secret in cluster
	emailCluster, err, done := o.getPullSecretFromCluster()
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

func (o *validatePullSecretOptions) getPullSecretFromCluster() (email string, err error, sentSL bool) {
	if o.elevate {
		return getPullSecretElevated(o.clusterID, o.kubeCli, o.reason)
	} else {
		return getPullSecretManagedScript(o.clusterID)
	}
}

// getPullSecretManagedScript runs a managed-script to get the pull-secret
// email from cluster without backplane elevation
// it returns the email, error and sentSL
// sentSL=true means a SL has been send to the cluster
func getPullSecretManagedScript(clusterID string) (email string, err error, sentSL bool) {
	jobId, err := createManagedJob()
	if err != nil {
		return "", err, false
	}
	err, sentSL = waitManagedJob(jobId, clusterID)
	if sentSL || err != nil {
		return "", err, sentSL
	}
	email, err = getManagedJobResult(jobId)

	return email, err, false
}

// getPullSecretElevated gets the pull-secret in the cluster
// with backplane elevation.
func getPullSecretElevated(clusterID string, kubeCli *k8s.LazyClient, reason string) (email string, err error, sentSL bool) {
	fmt.Println("Getting the pull-secret in the cluster with elevated permissions")
	kubeCli.Impersonate(BackplaneClusterAdmin, reason, fmt.Sprintf("Elevation required to get pull secret email to check if it matches the owner email for %s cluster", clusterID))
	secret := &corev1.Secret{}
	if err := kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret); err != nil {
		return "", err, false
	}

	clusterPullSecretEmail, err, done := getPullSecretEmail(clusterID, secret, true)
	if done {
		return "", err, true
	}
	fmt.Printf("email from cluster: %s\n", clusterPullSecretEmail)

	return clusterPullSecretEmail, nil, false
}

// createManagedJob creates a managed job to get the pull-secret email inside the cluster
func createManagedJob() (jobId string, err error) {
	fmt.Println("Creating a managedjob to get pull-secret email in the cluster")
	createJobCmd := "ocm backplane managedjob create SREP/get-pull-secret-email"
	createJobOutput, err := exec.Command("bash", "-c", createJobCmd).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run managed script SREP/get-pull-secret-email:\n%s", strings.TrimSpace(string(createJobOutput)))
	}
	// Get the job id from output
	re := regexp.MustCompile(`openshift-job-[a-z0-9]+`)
	matches := re.FindStringSubmatch(string(createJobOutput))
	if len(matches) == 0 {
		return "", fmt.Errorf("failed to find job id after creating managedjob")
	}
	jobId = matches[0]
	fmt.Printf("managedjob id: %s\n", jobId)
	return jobId, nil
}

// waitManagedJob waits for the managedjob to finish
// if there's a timeout, it will check the reason for timeout and send SL
// if timeout reason unknown, it returns an error.
func waitManagedJob(jobId string, clusterID string) (err error, sentSL bool) {
	fmt.Println("Waiting for managedjob to finish, it usually takes 30s")
	getJobStatusCmd := fmt.Sprintf("ocm backplane managedjob get %s", jobId)
	re := regexp.MustCompile(`Succeeded`)
	matches := []string{}
	// wait 10x10 seconds to finish
	for i := 0; i < 10; i++ {
		getJobStatusOutput, err := exec.Command("bash", "-c", getJobStatusCmd).CombinedOutput()
		if err != nil {
			continue
		}
		matches = re.FindStringSubmatch(string(getJobStatusOutput))
		if len(matches) > 0 {
			break
		}
		time.Sleep(10 * time.Second)
	}

	// managedjob succeed
	if len(matches) > 0 {
		return nil, false
	}

	// managedjob timeout, check the error.
	// instead of the go native way to get events:
	// https://github.com/openshift/oc/blob/c7b582ed27cfb2890068d6cb29cb2f5b936654cd/vendor/k8s.io/kubectl/pkg/describe/describe.go#L4258
	// we can simply run the oc describe command and match the output.
	describeJobCmd := fmt.Sprintf("oc describe pod %s -n openshift-backplane-managed-scripts", jobId)
	describeJobOutput, err := exec.Command("bash", "-c", describeJobCmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute: %s: %w", describeJobCmd, err), false
	}
	re = regexp.MustCompile(`unauthorized`)
	matches = re.FindStringSubmatch(string(describeJobOutput))
	if len(matches) > 0 {
		fmt.Printf("managedjob failed due to failed authentication to pull image, run the below command for more detail:\n%s\n", describeJobCmd)
		fmt.Println("Sending service log")
		postCmd := servicelog.PostCmdOptions{
			Template:  "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_change_breaking_upgradesync.json",
			ClusterId: clusterID,
		}
		if err = postCmd.Run(); err != nil {
			return err, true
		}
		return nil, true
	}
	return fmt.Errorf("managedjob timeout, try --elevate to validate pull-secret without a managed job"), false
}

// getManagedJobResult return's the email address fetched by the managedjob
func getManagedJobResult(jobId string) (string, error) {
	// Get the output of the managed script
	getJobResultCmd := fmt.Sprintf("ocm backplane managedjob logs %s", jobId)
	getJobResultOutput, err := exec.Command("bash", "-c", getJobResultCmd).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get the output of %s:%s", jobId, strings.TrimSpace(string(getJobResultOutput)))
	}

	re := regexp.MustCompile(`@`)
	matches := re.FindStringSubmatch(string(getJobResultOutput))
	if len(matches) == 0 {
		return "", fmt.Errorf("not a valid email output from the managed-script, output: %s", string(getJobResultOutput))
	}
	email := strings.TrimSpace(string(getJobResultOutput))
	fmt.Printf("email from managedjob: %s\n", email)

	return string(email), nil
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
