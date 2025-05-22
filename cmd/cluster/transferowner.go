package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/cmd/servicelog"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	CheckSyncMaxAttempts = 24

	SL_TRANSFER_INITIATED = "https://raw.githubusercontent.com/openshift/managed-notifications/refs/heads/master/osd/clustertransfer_starting.json"
	SL_TRANSFER_COMPLETE  = "https://raw.githubusercontent.com/openshift/managed-notifications/refs/heads/master/osd/clustertransfer_completed.json"
	SL_PULLSEC_ROTATED    = "https://raw.githubusercontent.com/openshift/managed-notifications/refs/heads/master/osd/pull_secret_rotated.json"
)

// transferOwnerOptions defines the struct for running transferOwner command
type transferOwnerOptions struct {
	clusterID        string
	newOwnerName     string
	reason           string
	dryrun           bool
	hypershift       bool
	doPullSecretOnly bool
	opDescription    string
	cluster          *cmv1.Cluster

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

var red *color.Color
var blue *color.Color
var green *color.Color

const transferOwnerCmdExample = `
  # Transfer ownership
  osdctl cluster transfer-owner --new-owner "new_OCM_userName" --cluster-id 1kfmyclusteristhebesteverp8m --reason "transfer ownership per jira-id"
`

func newCmdTransferOwner(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newTransferOwnerOptions(streams, globalOpts)
	transferOwnerCmd := &cobra.Command{
		Use:               "transfer-owner",
		Short:             "Transfer cluster ownership to a new user (to be done by Region Lead)",
		Args:              cobra.NoArgs,
		Example:           transferOwnerCmdExample,
		DisableAutoGenTag: true,
		PreRun:            func(cmd *cobra.Command, args []string) { cmdutil.CheckErr(ops.preRun()) },
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
	}
	// can we get cluster-id from some context maybe?
	transferOwnerCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The Internal Cluster ID/External Cluster ID/ Cluster Name")
	transferOwnerCmd.Flags().StringVar(&ops.newOwnerName, "new-owner", ops.newOwnerName, "The new owners username to transfer the cluster to")
	transferOwnerCmd.Flags().BoolVarP(&ops.dryrun, "dry-run", "d", false, "Dry-run - show all changes but do not apply them")
	transferOwnerCmd.Flags().BoolVar(&ops.doPullSecretOnly, "pull-secret-only", false, "Update cluster pull secret from current OCM AccessToken data without ownership transfer")
	transferOwnerCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)")

	_ = transferOwnerCmd.MarkFlagRequired("cluster-id")
	_ = transferOwnerCmd.MarkFlagRequired("reason")
	_ = transferOwnerCmd.Flags().MarkHidden("pull-secret-only")
	_ = transferOwnerCmd.MarkFlagRequired("new-owner")
	return transferOwnerCmd
}

func newTransferOwnerOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *transferOwnerOptions {
	return &transferOwnerOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

type serviceLogParameters struct {
	ClusterID             string
	OldOwnerName          string
	OldOwnerID            string
	NewOwnerName          string
	NewOwnerID            string
	IsExternalOrgTransfer bool
}

func (o *transferOwnerOptions) preRun() error {
	// Initialize the color formats...
	red = color.New(color.FgHiRed, color.BgBlack)
	green = color.New(color.FgHiGreen, color.BgBlack)
	blue = color.New(color.FgHiBlue, color.BgBlack)
	if o.doPullSecretOnly {
		o.opDescription = "update pull-secret"
	} else {
		o.opDescription = "transfer ownership"
	}
	return nil
}

func generateInternalServiceLog(params serviceLogParameters) servicelog.PostCmdOptions {
	return servicelog.PostCmdOptions{
		ClusterId: params.ClusterID,
		TemplateParams: []string{
			"MESSAGE=" + fmt.Sprintf("From user '%s' in Red Hat account %s => user '%s' in Red Hat account %s.", params.OldOwnerName, params.OldOwnerID, params.NewOwnerName, params.NewOwnerID),
		},
		InternalOnly: true,
	}
}

func generateServiceLog(params serviceLogParameters, template string) servicelog.PostCmdOptions {
	return servicelog.PostCmdOptions{
		Template:  template,
		ClusterId: params.ClusterID,
	}
}

func updatePullSecret(conn *sdk.Connection, kubeCli client.Client, clientset *kubernetes.Clientset, clusterID string, pullsecret []byte) error {
	currentEnv := utils.GetCurrentOCMEnv(conn)
	if currentEnv == "stage" {
		// stage hive cluster namespaces are prefixed 'uhc-staging' although the ocm url is currently using 'stage'
		// This may be better as a loop over all the namespaces looking for a clusterid match instead?
		currentEnv = "staging"
	}
	secretName := "pull"
	hiveNamespace := "uhc-" + currentEnv + "-" + clusterID

	clusterDeployments := &hiveapiv1.ClusterDeploymentList{}
	if err := kubeCli.List(context.TODO(), clusterDeployments, client.InNamespace(hiveNamespace)); err != nil {
		return fmt.Errorf("failed to list cluster deployments in namespace %v: %w", hiveNamespace, err)
	}

	if len(clusterDeployments.Items) == 0 {
		return fmt.Errorf("error, found '0' cluster deployments in hive namespace:'%s'", hiveNamespace)
	}
	cdName := clusterDeployments.Items[0].ObjectMeta.Name

	// Delete the secret
	err := clientset.CoreV1().Secrets(hiveNamespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete secret %v in namespacd %v: %w", secretName, hiveNamespace, err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: hiveNamespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			".dockerconfigjson": pullsecret,
		},
	}
	_, err = clientset.CoreV1().Secrets(hiveNamespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create new secret in namespace %v: %w", hiveNamespace, err)
	}

	err = awaitPullSecretSyncSet(hiveNamespace, cdName, kubeCli)
	if err != nil {
		return fmt.Errorf("failed to synchronize pull secret for Hive namespace '%s' and ClusterDeployment '%s': %w", hiveNamespace, cdName, err)
	}

	return nil

}

func awaitPullSecretSyncSet(hiveNamespace string, cdName string, kubeCli client.Client) error {
	ctx := context.TODO()

	syncSet := &hiveapiv1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pull-secret-replacement",
			Namespace: hiveNamespace,
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
							Name:      "pull",
							Namespace: hiveNamespace,
						},
						TargetRef: hiveapiv1.SecretReference{
							Name:      "pull-secret",
							Namespace: "openshift-config",
						},
					},
				},
			},
		},
	}

	err := kubeCli.Create(ctx, syncSet)
	if err != nil {
		return fmt.Errorf("failed to create SyncSet: %w", err)
	}

	fmt.Printf("SyncSet pull-secret-replacement in namespace %s has been created.\n", hiveNamespace)

	err = hiveinternalv1alpha1.AddToScheme(kubeCli.Scheme())
	if err != nil {
		return fmt.Errorf("failed to add scheme: %w", err)
	}

	searchStatus := &hiveinternalv1alpha1.ClusterSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cdName,
			Namespace: hiveNamespace,
		},
	}
	foundStatus := &hiveinternalv1alpha1.ClusterSync{}
	isSSSynced := false
	for i := 0; i < CheckSyncMaxAttempts; i++ {
		err = kubeCli.Get(ctx, client.ObjectKeyFromObject(searchStatus), foundStatus)
		if err != nil {
			return fmt.Errorf("failed to get status for resource %s: %w", searchStatus.GetName(), err)
		}

		for _, status := range foundStatus.Status.SyncSets {
			if status.Name == "pull-secret-replacement" {
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
		return fmt.Errorf("syncset failed to sync. Please verify syncset is still there and manually delete syncset ")
	}

	// Clean up the SS on hive
	err = kubeCli.Delete(ctx, syncSet)
	if err != nil {
		return fmt.Errorf("failed to delete SyncSet: %w", err)
	}

	return nil
}

func rolloutTelemeterClientPods(clientset *kubernetes.Clientset, namespace, selector string) error {
	// Delete pods with the specified label selector in the specified namespace.
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace '%s' with label selector '%s': %w", namespace, selector, err)
	}

	for _, pod := range pods.Items {
		err = clientset.CoreV1().Pods(namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete pod '%s' in namespace '%s': %w", pod.Name, namespace, err)
		}
		fmt.Printf("Pod %s in namespace %s has been deleted.\n", pod.Name, namespace)
	}

	fmt.Printf("Pods in namespace %s with label selector '%s' have been deleted.\n", namespace, selector)
	return nil
}

func comparePullSecretAuths(pullSecret *corev1.Secret, expectedAuths map[string]*amv1.AccessTokenAuth) error {
	var err error = nil
	var psTokenAuth *amv1.AccessTokenAuth = nil
	blue.Println("\nComparing pull-secret to expected auth sections...")
	for akey, auth := range expectedAuths {
		// Find the matching auth entry for this registry name in the cluster pull_secret data...
		psTokenAuth, err = getPullSecretTokenAuth(akey, pullSecret)
		if err != nil {
			err = errors.Join(err, fmt.Errorf("failed to fetch expected auth['%s'] from cluster pull-secret, err:'%s'", akey, err))
			continue
		}
		if psTokenAuth == nil {
			err = errors.Join(err, fmt.Errorf("failed to fetch expected auth['%s'] from cluster pull-secret, err: (nil authToken)", akey))
			continue
		}
		if auth.Auth() != psTokenAuth.Auth() {
			err = errors.Join(err, fmt.Errorf("expected auth['%s'] does not match authToken found in cluster pull-secret", akey))
		} else {
			green.Printf("Auth '%s' - tokens match\n", akey)
		}
		if auth.Email() != psTokenAuth.Email() {
			err = errors.Join(err, fmt.Errorf("expected auth['%s'] does not match email found in cluster pull-secret", akey))
		} else {
			green.Printf("Auth '%s' - emails match\n", akey)
		}
	}
	return err
}

func verifyClusterPullSecret(clientset *kubernetes.Clientset, expectedPullSecret string, expectedAuths map[string]*amv1.AccessTokenAuth) error {
	// Retrieve the pull secret from the "openshift-config" namespace
	pullSecret, err := clientset.CoreV1().Secrets("openshift-config").Get(context.TODO(), "pull-secret", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pull secret: %w", err)
	}
	pullSecretData, ok := pullSecret.Data[".dockerconfigjson"]
	if !ok {
		return fmt.Errorf("pull secret data not found in the secret")
	}
	err = comparePullSecretAuths(pullSecret, expectedAuths)
	if err != nil {
		red.Printf("\nFound mis-matching auth values during compare. Please review:\n%s", err)
		fmt.Print("Would you like to continue?")
		if !utils.ConfirmPrompt() {
			return fmt.Errorf("operation aborted by the user")
		}
	} else {
		green.Println("\nComparison shows subset of Auths from OCM AuthToken have matching tokens + emails in cluster pull-secret. PASS")
	}
	// This step was in the original utlity so leaving the option to print data to terminal here,
	// but making it optional and prompting the user instead.  The new programatic
	// comparisons per comparePullSecretAuths() may negate the need for a visual inspection in most cases...
	red.Print("\nWARNING: This will print sensitive data to the terminal!\n")
	fmt.Print("Would you like to print pull secret content to screen for additional visual comparison?\n")
	fmt.Print("Choose 'N' to skip, 'Y' to display secret. ")
	if utils.ConfirmPrompt() {
		// Print the actual pull secret data
		blue.Println("Actual Cluster Pull Secret:")
		fmt.Println(string(pullSecretData))

		// Print the expected pull secret
		blue.Println("\nExpected Auths from OCM AccessToken expected to be present in Pull Secret (note this can be a subset):")
		fmt.Println(expectedPullSecret)

		// TODO: Consider confirming that the email and token values of the 'subset' of Auths
		// contained in the OCM AccessToken actually matches email/token values in the cluster's
		// openshift-config/pull-secret. Provide any descrepencies to the user here before
		// prompting to visually evaluate.
		//
		// Ask the user to confirm if the actual pull secret matches their expectation
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nDoes the actual pull secret match your expectation? (yes/no): ")
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read user input: %w", err)
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "yes" {
			return fmt.Errorf("operation aborted by the user")
		}

		green.Println("Pull secret verification (by user) successful.")
	} else {
		fmt.Println("(Skipping display)")
	}
	return nil
}

func updateManifestWork(conn *sdk.Connection, kubeCli client.Client, clusterID, mgmtClusterName string, pullsecret []byte) error {
	fmt.Printf("updateManifestwork begin...\n")
	if err := workv1.AddToScheme(kubeCli.Scheme()); err != nil {
		return fmt.Errorf("failed to add scheme: %w", err)
	}

	manifestWorkName := clusterID
	manifestWorkNamespace := mgmtClusterName
	hostedCluster, err := utils.GetClusterAnyStatus(conn, clusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	// Use domain prefix here instead of hostedcluster.Name, since the pull secret will follow the domain prefix
	secretNamePrefix := hostedCluster.DomainPrefix() + "-pull"

	// Generate a random new secret name based on the existing pull secret name
	// A new secret 'name' is used here to trigger the update(?)
	randomSuffix := func(chars string, length int) string {
		rand.Seed(time.Now().UnixNano())
		result := make([]byte, length)
		for i := range result {
			result[i] = chars[rand.Intn(len(chars))]
		}
		return string(result)
	}
	newSecretName := secretNamePrefix + "-" + randomSuffix("0123456789abcdef", 6)

	fmt.Printf("get() Manifestwork...\n")
	manifestWork := &workv1.ManifestWork{}
	err = kubeCli.Get(context.TODO(), types.NamespacedName{Name: manifestWorkName, Namespace: manifestWorkNamespace}, manifestWork)
	if err != nil {
		return fmt.Errorf("failed to get the target manifestwork for given cluster %v: %w", clusterID, err)
	}

	for i, manifest := range manifestWork.Spec.Workload.Manifests {
		if manifest.Raw != nil {
			var manifestData map[string]interface{}
			err := json.Unmarshal(manifest.Raw, &manifestData)
			if err != nil {
				return err
			}

			jsonData, err := json.Marshal(manifestData)
			if err != nil {
				return err
			}

			if manifestData["kind"] == "Secret" {
				secret := &corev1.Secret{}
				err = json.Unmarshal(jsonData, secret)
				if err != nil {
					return err
				}
				if strings.Contains(secret.Name, secretNamePrefix) {
					// Firstly, get the ecr auth from the existing pull secret and append it to new pull secret
					oldPullSecret := secret.Data[".dockerconfigjson"]
					newPullSecret, err := buildNewSecret(oldPullSecret, pullsecret)
					if err != nil {
						return fmt.Errorf("cannot build the new pull secret: %w", err)
					}
					// Then, update the secret with new name and new value
					secret.Name = newSecretName
					secret.Data[".dockerconfigjson"] = newPullSecret
				}
				secretJson, err := json.Marshal(secret)
				if err != nil {
					return err
				}
				manifestWork.Spec.Workload.Manifests[i].Raw = secretJson
			}
			if manifestData["kind"] == "HostedCluster" {
				hc := &hypershiftv1beta1.HostedCluster{}
				err = json.Unmarshal(jsonData, hc)
				if err != nil {
					return err
				}
				// Update the hosted cluster reference secret name to the new name
				hc.Spec.PullSecret.Name = newSecretName
				hcJson, err := json.Marshal(hc)
				if err != nil {
					return err
				}
				manifestWork.Spec.Workload.Manifests[i].Raw = hcJson
			}
		}
	}

	fmt.Printf("update() Manifestwork...\n")
	err = kubeCli.Update(context.TODO(), manifestWork, &client.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("cannot update the pull-secret within manifestwork: %w", err)
	}

	// The secret will be synced to the management cluster and guest cluster in a few seconds, wait here
	fmt.Println("Manifest work updated. ")
	fmt.Println("Sleeping 60 seconds here to allow secret to be synced on guest cluster")
	time.Sleep(time.Second * 60)

	return nil
}

// buildNewSecret will build the pull secret with updating the old pullsecret from the give new pullsecret
func buildNewSecret(oldpullsecret, newpullsecret []byte) ([]byte, error) {
	type Auth struct {
		Auth  string `json:"auth"`
		Email string `json:"email"`
	}

	type Auths struct {
		Auths map[string]Auth `json:"auths"`
	}

	var oldAuths, newAuths Auths

	err := json.Unmarshal(oldpullsecret, &oldAuths)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(newpullsecret, &newAuths)
	if err != nil {
		return nil, err
	}

	for k, v := range newAuths.Auths {
		oldAuths.Auths[k] = v
	}

	auth, err := json.Marshal(oldAuths)
	if err != nil {
		return nil, err
	}

	return auth, nil
}

func (o *transferOwnerOptions) run() error {
	// Initiate Connections First

	// Create an OCM client to talk to the cluster API
	// the user has to be logged in (e.g. 'ocm login')
	var err error
	// To avoid warnings/backtrace, if k8s controller-runtime logger is not yet set, do it now...
	log.SetLogger(zap.New(zap.WriteTo(os.Stderr)))
	ocm, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("failed to create OCM client: %w", err)
	}
	defer func() {
		if ocmCloseErr := ocm.Close(); ocmCloseErr != nil {
			fmt.Printf("Cannot close the ocm (possible memory leak): %q", ocmCloseErr)
		}
	}()

	// Gather all required data
	cluster, err := utils.GetClusterAnyStatus(ocm, o.clusterID)
	o.cluster = cluster
	o.clusterID = cluster.ID()
	var userName string
	var subscription *amv1.Subscription = nil
	var oldOwnerAccount *amv1.Account = nil
	var userDetails *amv1.AccountGetResponse
	var ok bool
	if o.doPullSecretOnly {
		// This is updating the pull secret and not a ownwership transfer.
		// Use existing subscription, account, and userName value...
		subscription, err = utils.GetSubscription(ocm, o.clusterID)
		if err != nil {
			return fmt.Errorf("failed to get subscription info for cluster:'%s', err: '%v'", o.clusterID, err)
		}
		oldOwnerAccount, err = utils.GetAccount(ocm, subscription.Creator().ID())
		if err != nil {
			return fmt.Errorf("failed to get account info from subscription, err:'%v'", err)
		}
		userName = oldOwnerAccount.Username()
		fmt.Printf("Old username:'%s'\n", userName)
	} else {
		// This is an ownership transfer.
		// Lookup New Owner Name...
		userDetails, err = ocm.AccountsMgmt().V1().Accounts().Account(o.newOwnerName).Get().Send()
		if err != nil {
			return fmt.Errorf("failed to fetch Account info, err:'%v'", err)
		}
		ok := false
		userName, ok = userDetails.Body().GetUsername()
		if !ok {
			return fmt.Errorf("failed to get username from new user id")
		}
	}

	var mgmtCluster, svcCluster, hiveCluster, masterCluster *cmv1.Cluster

	o.hypershift, err = utils.IsHostedCluster(o.clusterID)
	if err != nil {
		return fmt.Errorf("failed to check if the given cluster is HCP: %w", err)
	}

	// Find and setup all resources that are needed
	if o.hypershift {
		fmt.Printf("Given cluster is HCP, start to proceed for an HCP '%s' \n", o.opDescription)
		mgmtCluster, err = utils.GetManagementCluster(o.clusterID)
		svcCluster, err = utils.GetServiceCluster(o.clusterID)
		if err != nil {
			return err
		}
		masterCluster = svcCluster
	} else {
		fmt.Printf("Given cluster is OSD/ROSA classic, start to proceed for a classic '%s'\n", o.opDescription)
		hiveCluster, err = utils.GetHiveCluster(o.clusterID)
		if err != nil {
			return err
		}
		masterCluster = hiveCluster
	}

	elevationReasons := []string{
		o.reason,
	}
	if o.doPullSecretOnly {
		elevationReasons = append(elevationReasons, "Updating pull secret using osdctl")
	} else {
		elevationReasons = append(elevationReasons, fmt.Sprintf("Updating pull secret using osdctl to tranfer owner to %s", o.newOwnerName))
	}
	// Gather all required information
	fmt.Printf("Gathering all required information for the cluster '%s'...\n", o.opDescription)
	cluster, err = utils.GetCluster(ocm, o.clusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster information for cluster with ID %s: %w", o.clusterID, err)
	}

	externalClusterID, ok := cluster.GetExternalID()
	if !ok {
		return fmt.Errorf("cluster has no external id")
	}

	if subscription == nil {
		subscription, err = utils.GetSubscription(ocm, o.clusterID)
		if err != nil {
			return fmt.Errorf("could not get subscription: %w", err)
		}
	}

	subscriptionID, ok := subscription.GetID()
	if !ok {
		return fmt.Errorf("Could not get subscription id")
	}
	if oldOwnerAccount == nil {
		oldOwnerAccount, ok = subscription.GetCreator()
		if !ok {
			return fmt.Errorf("cluster has no owner account")
		}
	}

	oldOrganizationId, ok := subscription.GetOrganizationID()
	if !ok {
		return fmt.Errorf("old organization has no ID")
	}

	// We have to get the organization from the ID because it's not nested
	// under the subscription.GetCreator
	oldOrganization, err := utils.GetOrganization(ocm, subscriptionID)
	if err != nil {
		fmt.Printf("Error: %s", err)
		return fmt.Errorf("could not get current owner organization")
	}
	var newAccount *amv1.Account = nil
	var newOrganization *amv1.Organization = nil
	var oldUsername string
	if o.doPullSecretOnly {
		// This is not an ownership transfer, just pull-secret update,
		// new info == old info.
		// TODO: Can likely skip most of the next set of checks, but why not?
		oldUsername, ok = oldOwnerAccount.GetUsername()
		if !ok {
			fmt.Printf("old username not found?\n")
		}
		fmt.Printf("Using old account values. OwnerAccount:'%s'\n", oldUsername)
		newAccount = oldOwnerAccount
		newOrganization = oldOrganization
	} else {
		newAccount, err = utils.GetAccount(ocm, o.newOwnerName)
		if err != nil {
			return fmt.Errorf("could not get new owners account: %w", err)
		}
		newOrganization, ok = newAccount.GetOrganization()
		if !ok {
			return fmt.Errorf("new account has no organization")
		}
	}

	newOrganizationId, ok := newOrganization.GetID()
	if !ok {
		return fmt.Errorf("new organization has no ID")
	}
	fmt.Printf("old orgID:'%s', new orgID:'%s'\n", newOrganizationId, oldOrganizationId)
	if o.doPullSecretOnly && newOrganizationId != oldOrganizationId {
		return fmt.Errorf("new org != old org. Ownership transfer not expected with pull-secret-only flag")
	}

	accountID, ok := newAccount.GetID()
	if !ok {
		return fmt.Errorf("account has no id")
	}

	clusterConsole, ok := cluster.GetConsole()
	if !ok {
		return fmt.Errorf("cluster has no console url")
	}

	clusterURL, ok := clusterConsole.GetURL()
	if !ok {
		return fmt.Errorf("cluster has no console url")
	}

	displayName, ok := subscription.GetDisplayName()
	if !ok {
		return fmt.Errorf("subscription has no displayName")
	}

	oldOwnerAccountID, ok := oldOwnerAccount.GetID()
	if !ok {
		return fmt.Errorf("cannot get old owner account id")
	}

	oldOwnerAccount, err = utils.GetAccount(ocm, oldOwnerAccountID)
	if err != nil {
		return fmt.Errorf("cannot get old owner account")
	}

	oldOwnerUsername, ok := oldOwnerAccount.GetUsername()
	if !ok {
		return fmt.Errorf("cannot get old owner username")
	}

	oldOrganizationEbsAccountID, ok := oldOrganization.GetEbsAccountID()
	if !ok {
		return fmt.Errorf("cannot get old org ebs id")
	}

	newOwnerUsername, ok := newAccount.GetUsername()
	if !ok {
		return fmt.Errorf("cannot get new owner username")
	}

	newOrganizationEbsAccountID, ok := newOrganization.GetEbsAccountID()
	if !ok {
		return fmt.Errorf("cannot get new org ebs id")
	}

	orgChanged := oldOrganizationId != newOrganizationId

	// build common SL parameters struct
	slParams := serviceLogParameters{
		ClusterID:             o.clusterID,
		OldOwnerName:          oldOwnerUsername,
		OldOwnerID:            oldOrganizationEbsAccountID,
		NewOwnerName:          newOwnerUsername,
		NewOwnerID:            newOrganizationEbsAccountID,
		IsExternalOrgTransfer: orgChanged,
	}

	var postCmd servicelog.PostCmdOptions
	//TODO: If only updating the pull-secret, and not transferring ownership
	//      should we send a SL both before and after the rotate operation?
	//      Currently only sending an after the pull-secret update is completed
	//      when not also transferring ownership.
	if !o.doPullSecretOnly {
		// Send a SL saying we're about to start ownership transfer
		fmt.Println("Notify the customer before ownership transfer commences. Sending service log.")
		postCmd = generateServiceLog(slParams, SL_TRANSFER_INITIATED)
		if err := postCmd.Run(); err != nil {
			fmt.Println("Failed to POST customer service log. Please manually send a service log to notify the customer before ownership transfer commences:")
			fmt.Printf("osdctl servicelog post %v -t %v -p %v\n",
				o.clusterID, SL_TRANSFER_INITIATED, strings.Join(postCmd.TemplateParams, " -p "))
		}
	}

	// Send internal SL to cluster with additional details in case we
	// need them later. This prevents leaking PII to customers.
	fmt.Print("\nPlease review the following'Internal' ServiceLog. (Choose 'Y' to send, or 'N' to skip sending this SL...)\n")
	if o.doPullSecretOnly {
		postCmd = servicelog.PostCmdOptions{
			ClusterId: slParams.ClusterID,
			TemplateParams: []string{
				"MESSAGE=" + fmt.Sprintf("Pull-secret update initiated. UserName:'%s', OwnerID:'%s'", slParams.OldOwnerID, slParams.OldOwnerName),
			},
			InternalOnly: true,
		}
		if err := postCmd.Run(); err != nil {
			fmt.Println("Failed to POST internal service log. Please manually send a service log to persist details of the customer transfer before proceeding:")
			fmt.Printf("osdctl servicelog post -i -p MESSAGE=\"Pull-secret update. UserName:'%s', OwnerID:'%s'.\" %s \n", slParams.OldOwnerID, slParams.OldOwnerName, slParams.ClusterID)
		}
	} else {
		postCmd = generateInternalServiceLog(slParams)
		if err := postCmd.Run(); err != nil {
			fmt.Println("Failed to POST internal service log. Please manually send a service log to persist details of the customer transfer before proceeding:")
			fmt.Printf("osdctl servicelog post -i -p MESSAGE=\"From user '%s' in Red Hat account %s => user '%s' in Red Hat account %s.\" %s \n", slParams.OldOwnerName, slParams.OldOwnerID, slParams.NewOwnerName, slParams.NewOwnerID, slParams.ClusterID)
		}
	}

	masterKubeCli, _, masterKubeClientSet, err := common.GetKubeConfigAndClient(masterCluster.ID(), elevationReasons...)
	if err != nil {
		return fmt.Errorf("failed to retrieve Kubernetes configuration and client for Hive cluster ID %s: %w", masterCluster.ID(), err)
	}

	// Get account running this command to compare against cluster's account for
	// impersonation purposes.
	currentAccountResp, err := ocm.AccountsMgmt().V1().CurrentAccount().Get().Send()
	var currentOCMAccount *amv1.Account = nil
	if err != nil {
		//Ignore this error and continue with an attempt to use 'impersonate' instead...
		fmt.Fprintf(os.Stderr, "Failed to fetch currentAccount info, err:'%v'\n", err)
		currentAccountResp = nil
	} else {
		currentOCMAccount = currentAccountResp.Body()
	}

	// Fetch the current Access Token for pull secret with the given new username from OCM
	var response *amv1.AccessTokenPostResponse = nil
	if currentOCMAccount == nil || currentOCMAccount.Username() != userName {
		// This account is not owned by the OCM account running this command, so impersonate...
		// Impersonate requires region-lead permissions at this time.
		response, err = ocm.AccountsMgmt().V1().AccessToken().Post().Impersonate(userName).Parameter("body", nil).Send()
	} else {
		// This account is owned by the OCM account running this command, no need to impersonate
		// This allows non-region leads to test this utility against their own test clusters.
		response, err = ocm.AccountsMgmt().V1().AccessToken().Post().Send()
	}
	if err != nil {
		return fmt.Errorf("failed to fetch OCM AccessToken: %w", err)
	}

	auths, ok := response.Body().GetAuths()
	if !ok {
		return fmt.Errorf("Error validating pull secret structure. This shouldn't happen, so you might need to contact SDB")
	}
	authsMap := map[string]map[string]string{}
	for k, auth := range auths {
		authsMap[k] = map[string]string{
			"auth":  auth.Auth(),
			"email": auth.Email(),
		}
	}

	pullSecret, err := json.Marshal(map[string]map[string]map[string]string{
		"auths": authsMap,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal pull secret data: %w", err)
	}

	// This step was in the original utlity so leaving the option to print data to terminal here,
	// but making it optional and prompting the user instead.  The new programatic
	// comparisons per comparePullSecretAuths() may negate the need for a visual inspection in most cases...
	red.Print("\nWARNING: This will print sensitive data to the terminal!\n")
	fmt.Print("Would you like to print pull secret content to screen for visual review?\nDisplay pullsecret data (choose 'N' to skip, 'Y' to display)? ")
	if utils.ConfirmPrompt() {
		//Attempt to pretty print the json for easier user initial review...
		prettySecret, err := json.MarshalIndent(map[string]map[string]map[string]string{
			"auths": authsMap,
		}, "", " ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error Marshalling data for pretty print. Err:'%v'", err)
		} else {
			blue.Println("Pull Secret data(Indented)...")
			blue.Printf("\n%s\n", prettySecret)
		}

		// Print the pull secret in it's actual form for user to confirm (ie no go, json, formatting errors, etc)
		green.Print("\nPlease review Pull Secret data to be used for update(after formatting):\n")
		fmt.Println(string(pullSecret))

		// Ask the user if they would like to continue
		var continueConfirmation string
		fmt.Print("\nDo you want to continue? (yes/no): ")
		_, err = fmt.Scanln(&continueConfirmation)
		if err != nil {
			return fmt.Errorf("failed to read user input: %w", err)
		}

		// Check the user's response
		if continueConfirmation != "yes" {
			return fmt.Errorf("operation aborted by the user")
		}
	} else {
		fmt.Println("(Skipping display)")
	}

	if o.hypershift {
		err = updateManifestWork(ocm, masterKubeCli, o.clusterID, mgmtCluster.Name(), pullSecret)
		if err != nil {
			return fmt.Errorf("failed to update pull secret for service cluster with ID %s: %w", o.clusterID, err)
		}
	} else {
		err = updatePullSecret(ocm, masterKubeCli, masterKubeClientSet, o.cluster.ID(), pullSecret)
		if err != nil {
			return fmt.Errorf("failed to update pull secret for Hive cluster with ID %s: %w", o.clusterID, err)
		}
	}

	fmt.Println("Create cluster kubecli...")
	_, _, targetClientSet, err := common.GetKubeConfigAndClient(o.clusterID, elevationReasons...)
	if err != nil {
		return fmt.Errorf("failed to retrieve Kubernetes configuration and client for cluster with ID %s: %w", o.clusterID, err)
	}
	fmt.Println("Cluster kubecli created")

	// Rollout the telemeterClient pod for non HCP clusters
	if !o.hypershift {
		err = rolloutTelemeterClientPods(targetClientSet, "openshift-monitoring", "app.kubernetes.io/name=telemeter-client")
		if err != nil {
			return fmt.Errorf("failed to roll out Telemeter Client pods in namespace 'openshift-monitoring' with label selector 'app.kubernetes.io/name=telemeter-client': %w", err)
		}
	}

	err = verifyClusterPullSecret(targetClientSet, string(pullSecret), auths)
	if err != nil {
		return fmt.Errorf("error verifying cluster pull secret: %w", err)
	}

	if o.doPullSecretOnly {
		// User has chosen to update pull secret w/o ownership transfer.
		// Send SL to notify customer this is completed, then return the command.
		fmt.Println("Notify the customer the pull-secret update is completed. Sending service log.")
		//postCmd = generateServiceLog(slParams, SL_PULL_SECRET_ROTATED)
		postCmd = servicelog.PostCmdOptions{
			Template:       SL_PULLSEC_ROTATED,
			ClusterId:      o.clusterID,
			TemplateParams: []string{fmt.Sprintf("ACCOUNT=%s", oldOwnerAccountID)},
		}

		if err := postCmd.Run(); err != nil {
			fmt.Println("Failed to POST service log. Please manually send a service log to notify the customer the pull-secrete update completed:")
			fmt.Printf("osdctl servicelog post %v -t %v -p %v\n",
				o.clusterID, SL_PULLSEC_ROTATED, strings.Join(postCmd.TemplateParams, " -p "))
		}

		fmt.Printf("Pull secret update complete, exiting successfully\n")
		return nil
	}

	// Transfer ownership specific operations...

	fmt.Printf("\nTransfer cluster: \t\t'%v' (%v)\n", externalClusterID, cluster.Name())
	fmt.Printf("from user \t\t\t'%v' to '%v'\n", oldOwnerAccount.ID(), accountID)
	fmt.Print("Is the above correct? Proceed with transfer? ")
	if !utils.ConfirmPrompt() {
		return nil
	}

	ok = validateOldOwner(oldOrganizationId, subscription, oldOwnerAccount)
	if !ok {
		fmt.Print("can't validate this is old owners cluster, this could be because of a previously failed run\n")
		if !utils.ConfirmPrompt() {
			return nil
		}
	}

	subscriptionOrgPatch, err := amv1.NewSubscription().OrganizationID(newOrganizationId).Build()

	if err != nil {
		return fmt.Errorf("can't create subscription organization patch: %w", err)
	}

	subscriptionCreatorPatchRequest, err := createSubscriptionCreatorPatchRequest(ocm, subscriptionID, accountID)

	if err != nil {
		return fmt.Errorf("can't create subscription creator patch: %w", err)
	}

	newRoleBinding, err := amv1.
		NewRoleBinding().
		AccountID(accountID).
		SubscriptionID(subscriptionID).
		Type("Subscription").
		RoleID("ClusterOwner").
		Build()

	if err != nil {
		return fmt.Errorf("can't create new owners rolebinding %w", err)
	}

	if orgChanged {
		fmt.Printf("with organization change from \t'%v' to '%v'\n", oldOrganizationId, newOrganizationId)
	}

	if o.dryrun {
		fmt.Print("This is a dry run, nothing changed.\n")
		return nil
	}

	// Validation done, now update everything

	// org has to be patched before creator
	if orgChanged {
		subscriptionClient := ocm.AccountsMgmt().V1().Subscriptions().Subscription(subscriptionID)
		response, err := subscriptionClient.Update().Body(subscriptionOrgPatch).Send()

		if err != nil || response.Status() != 200 {
			return fmt.Errorf("update Subscription request to patch org failed with status: %d,  err:'%w'", response.Status(), err)
		}
		fmt.Printf("Patched organization on subscription\n")
	}

	// patch creator on subscription
	patchRes, err := subscriptionCreatorPatchRequest.Send()

	if err != nil || patchRes.Status() != 200 {
		// err var is not always set to something meaningful here.
		// Instead the response body usually contains the err info...
		red.Fprintf(os.Stderr, "Error, Patch Request Response: '%s'\n", patchRes.String())
		var errString string
		if err != nil {
			errString = fmt.Sprintf("%v", err)
		} else {
			errString = patchRes.String()
		}
		return fmt.Errorf("subscription request to patch creator failed with status: %d, err: '%s'", patchRes.Status(), errString)
	}
	fmt.Printf("Patched creator on subscription\n")

	// delete old rolebinding but do not exit on fail could be a rerun
	err = deleteOldRoleBinding(ocm, subscriptionID)

	if err != nil {
		fmt.Printf("Warning, can't delete old rolebinding, err: %v \n", err)
	}

	// create new rolebinding
	newRoleBindingClient := ocm.AccountsMgmt().V1().RoleBindings()
	postRes, err := newRoleBindingClient.Add().Body(newRoleBinding).Send()

	// don't fail if the rolebinding already exists, could be rerun
	if err != nil {
		return fmt.Errorf("account new roleBinding request failed, err: '%w'", err)
	} else if postRes.Status() == 201 {
		fmt.Printf("Created new role binding.\n")
	} else if postRes.Status() == 409 {
		fmt.Printf("can't add new rolebinding, rolebinding already exists\n")
	} else {
		return fmt.Errorf("account new roleBinding request failed with status: %d, err: '%w'", postRes.Status(), err)
	}

	// If the organization id has changed, re-register the cluster with CS with the new organization id
	if orgChanged {

		request, err := createNewRegisterClusterRequest(ocm, externalClusterID, subscriptionID, newOrganizationId, clusterURL, displayName)
		if err != nil {
			return fmt.Errorf("can't create RegisterClusterRequest with CS, err:'%w'", err)
		}

		response, err := request.Send()
		if err != nil || (response.Status() != 200 && response.Status() != 201) {
			return fmt.Errorf("newRegisterClusterRequest failed with status: %d, err:'%w'", response.Status(), err)
		}
		fmt.Print("Re-registered cluster\n")
	}

	err = validateTransfer(ocm, subscription.ClusterID(), newOrganizationId)
	if err != nil {
		return fmt.Errorf("error while validating transfer. %w", err)
	}
	fmt.Print("Transfer complete\n")

	fmt.Println("Notify the customer the ownership transfer is completed. Sending service log.")
	postCmd = generateServiceLog(slParams, SL_TRANSFER_COMPLETE)
	if err := postCmd.Run(); err != nil {
		fmt.Println("Failed to POST service log. Please manually send a service log to notify the customer the ownership transfer is completed:")
		fmt.Printf("osdctl servicelog post %v -t %v -p %v\n",
			o.clusterID, SL_TRANSFER_COMPLETE, strings.Join(postCmd.TemplateParams, " -p "))
	}

	return nil
}

func getRoleBinding(ocm *sdk.Connection, subscriptionID string) (*amv1.RoleBinding, error) {
	roleBindingQuery := "subscription_id = '%s' and role_id = 'ClusterOwner'"
	searchString := fmt.Sprintf(roleBindingQuery, subscriptionID)
	response, err := ocm.AccountsMgmt().V1().RoleBindings().List().Parameter("search", searchString).
		Send()

	if err != nil {
		return nil, fmt.Errorf("RoleBindings list, can't send request: %v", err)
	}

	if response.Total() == 0 {
		return nil, fmt.Errorf("no rolebinding found")
	}

	_, ok := response.Items().Get(0).GetID()

	if !ok {
		return nil, fmt.Errorf("rolebinding has no ID")
	}

	return response.Items().Get(0), nil
}

// deletes old rolebinding by subscription id
func deleteOldRoleBinding(ocm *sdk.Connection, subscriptionID string) error {
	oldRoleBinding, err := getRoleBinding(ocm, subscriptionID)

	if err != nil {
		return fmt.Errorf("can't get old owners rolebinding: %w", err)
	}

	oldRoleBindingID, ok := oldRoleBinding.GetID()
	if !ok {
		return fmt.Errorf("old rolebinding has no id, err: %w", err)
	}
	oldRoleBindingClient := ocm.AccountsMgmt().V1().RoleBindings().RoleBinding(oldRoleBindingID)

	response, err := oldRoleBindingClient.Delete().Send()

	if err != nil {
		return fmt.Errorf("request failed, err: '%w'", err)
	}
	if response.Status() == 204 {
		fmt.Printf("Deleted old rolebinding: %v\n", oldRoleBindingID)
		return nil
	}
	if response.Status() == 404 {
		fmt.Printf("can't find old rolebinding: %v\n", oldRoleBindingID)
		return nil
	}
	fmt.Printf("delete RoleBindingrequest failed with status: %d\n", response.Status())
	return nil
}

type CreatorPatch struct {
	Creator_ID string `json:"creator_id"`
}

// creates subscription patch with new owner id
func createSubscriptionCreatorPatchRequest(ocm *sdk.Connection, subscriptionID string, accountID string) (*sdk.Request, error) {

	targetAPIPath := "/api/accounts_mgmt/v1/subscriptions/%s"

	request := ocm.Patch()
	err := arguments.ApplyPathArg(request, fmt.Sprintf(targetAPIPath, subscriptionID))
	if err != nil {
		return nil, fmt.Errorf("cannot parse API path '%s': %w", targetAPIPath, err)
	}

	body, err := json.Marshal(CreatorPatch{accountID})
	if err != nil {
		return nil, fmt.Errorf("cannot create body for request, err: '%w'", err)
	}

	request.Bytes(body)

	return request, nil
}

type RegisterCluster struct {
	External_id     string `json:"external_id"`
	Subscription_id string `json:"subscription_id"`
	Organization_id string `json:"organization_id"`
	Console_url     string `json:"console_url"`
	Display_name    string `json:"display_name"`
}

// api schema 'ClusterRegistration' accepts two more optional fields that are not documented
// look at https://gitlab.cee.redhat.com/service/uhc-clusters-service/-/blob/master/pkg/mappers/inbound/clusters.go#L30
// fix is tracked in https://issues.redhat.com/browse/SDA-6652
// after ocm-sdk is fixed this function can be cleaned up

// re-register the cluster with CS with the new organization id
func createNewRegisterClusterRequest(ocm *sdk.Connection, externalClusterID string, subscriptionID string, organizationID string, consoleURL string, displayName string) (*sdk.Request, error) {

	targetAPIPath := "/api/clusters_mgmt/v1/register_cluster"

	request := ocm.Post()
	err := arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		return nil, fmt.Errorf("cannot parse API path '%s': %w", targetAPIPath, err)
	}

	body, err := json.Marshal(RegisterCluster{externalClusterID, subscriptionID, organizationID, consoleURL, displayName})
	if err != nil {
		return nil, fmt.Errorf("cannot create body for request, err: '%w'", err)
	}

	request.Bytes(body)

	return request, nil
}

// Confirm the cluster record matches the subscription record regarding organization
func validateTransfer(ocm *sdk.Connection, clusterID string, newOrgID string) error {

	const query = "organization.id = '%s' and id = '%s'"
	searchString := fmt.Sprintf(query, newOrgID, clusterID)
	response, err := ocm.ClustersMgmt().V1().Clusters().List().Parameter("search", searchString).
		Send()

	if err != nil || response.Status() != 200 {
		return fmt.Errorf("list clusters request failed with status: %d, err: '%w'", response.Status(), err)
	}

	if response.Total() == 0 {
		return fmt.Errorf("organization id differs between cluster and subscription record")
	}

	return nil
}

// checks if old owner is on subscription
func validateOldOwner(oldOrganizationId string, subscription *amv1.Subscription, oldOwner *amv1.Account) bool {
	oldOrgSub, ok := subscription.GetOrganizationID()
	if !ok {
		fmt.Printf("subscription has no organization\n")
		return ok
	}
	oldOwnerSub, ok := subscription.GetCreator()
	if !ok {
		fmt.Printf("subscription has no creator\n")
		return ok
	}
	userIDSub, ok := oldOwnerSub.GetID()
	if !ok {
		fmt.Printf("old owner has no id\n")
		return ok
	}

	ok = true
	if oldOrgSub != oldOrganizationId {
		fmt.Printf("old owners organization on subscription differs from the specified one. ")
		fmt.Printf("Subscription has organization ID: %v (specified was: %v)\n", oldOrgSub, oldOrganizationId)
		ok = false
	}
	if userIDSub != oldOwner.ID() {
		fmt.Printf("old owners ID on subscription differs from the specified one. ")
		fmt.Printf("Subscription has owner: %v (specified was: %v)\n", userIDSub, oldOwner.ID())
		ok = false
	}
	return ok
}
