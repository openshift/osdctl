package cluster

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

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
)

const CheckSyncMaxAttempts = 24

// transferOwnerOptions defines the struct for running transferOwner command
type transferOwnerOptions struct {
	output       string
	clusterID    string
	newOwnerName string
	reason       string
	dryrun       bool
	hypershift   bool
	cluster      *cmv1.Cluster

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

func newCmdTransferOwner(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newTransferOwnerOptions(streams, globalOpts)
	transferOwnerCmd := &cobra.Command{
		Use:               "transfer-owner",
		Short:             "Transfer cluster ownership to a new user (to be done by Region Lead)",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
	}
	// can we get cluster-id from some context maybe?
	transferOwnerCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The Internal Cluster ID/External Cluster ID/ Cluster Name")
	transferOwnerCmd.Flags().StringVar(&ops.newOwnerName, "new-owner", ops.newOwnerName, "The new owners username to transfer the cluster to")
	transferOwnerCmd.Flags().BoolVarP(&ops.dryrun, "dry-run", "d", false, "Dry-run - show all changes but do not apply them")
	transferOwnerCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)")

	_ = transferOwnerCmd.MarkFlagRequired("cluster-id")
	_ = transferOwnerCmd.MarkFlagRequired("new-owner")
	_ = transferOwnerCmd.MarkFlagRequired("reason")

	return transferOwnerCmd
}

func newTransferOwnerOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *transferOwnerOptions {
	return &transferOwnerOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func generateServiceLog(clusterId string, template string) servicelog.PostCmdOptions {
	return servicelog.PostCmdOptions{
		Template:       template,
		ClusterId:      clusterId,
		TemplateParams: []string{fmt.Sprintf("DATE=%v", time.Now().UTC().Format(time.RFC3339))},
	}
}

func updatePullSecret(conn *sdk.Connection, kubeCli client.Client, clientset *kubernetes.Clientset, clusterID string, pullsecret []byte) error {
	currentEnv := utils.GetCurrentOCMEnv(conn)
	secretName := "pull"
	hiveNamespace := "uhc-" + currentEnv + "-" + clusterID

	clusterDeployments := &hiveapiv1.ClusterDeploymentList{}
	if err := kubeCli.List(context.TODO(), clusterDeployments, client.InNamespace(hiveNamespace)); err != nil {
		return fmt.Errorf("failed to list cluster deployments in namespace %v: %w", hiveNamespace, err)
	}

	if len(clusterDeployments.Items) == 0 {
		return fmt.Errorf("failed to retreive cluster deployments")
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

func verifyClusterPullSecret(clientset *kubernetes.Clientset, expectedPullSecret string) error {
	// Retrieve the pull secret from the "openshift-config" namespace
	pullSecret, err := clientset.CoreV1().Secrets("openshift-config").Get(context.TODO(), "pull-secret", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pull secret: %w", err)
	}

	// Print the actual pull secret data
	pullSecretData, ok := pullSecret.Data[".dockerconfigjson"]
	if !ok {
		return fmt.Errorf("pull secret data not found in the secret")
	}

	fmt.Println("Actual Cluster Pull Secret:")
	fmt.Println(string(pullSecretData))

	// Print the expected pull secret
	fmt.Println("\nExpected Cluster Pull Secret:")
	fmt.Println(expectedPullSecret)

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

	fmt.Println("Pull secret verification successful.")

	return nil
}

func updateManifestWork(conn *sdk.Connection, kubeCli client.Client, clusterID, mgmtClusterName string, pullsecret []byte) error {

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
	randomSuffix := func(chars string, length int) string {
		rand.Seed(time.Now().UnixNano())
		result := make([]byte, length)
		for i := range result {
			result[i] = chars[rand.Intn(len(chars))]
		}
		return string(result)
	}
	newSecretName := secretNamePrefix + "-" + randomSuffix("0123456789abcdef", 6)

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

	err = kubeCli.Update(context.TODO(), manifestWork, &client.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("cannot update the pull-secret within manifestwork: %w", err)
	}

	// The secret will be synced to the management cluster and guest cluster in a few seconds, wait here
	fmt.Println("sleep 60 seconds here to make sure secret gets synced on guest cluster")
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
	fmt.Println("Notify the customer before ownership transfer commences. Sending service log.")
	postCmd := generateServiceLog(o.clusterID, "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/maintenance_starting.json")
	if err := postCmd.Run(); err != nil {
		fmt.Println("Failed to generate service log. Please manually send a service log to Notify the customer before ownership transfer commences:")
		fmt.Printf("osdctl servicelog post %v -t %v -p %v\n",
			o.clusterID, "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/maintenance_starting.json", strings.Join(postCmd.TemplateParams, " -p "))
	}

	// Create an OCM client to talk to the cluster API
	// the user has to be logged in (e.g. 'ocm login')
	ocm, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("failed to create OCM client: %w", err)
	}
	defer func() {
		if ocmCloseErr := ocm.Close(); ocmCloseErr != nil {
			fmt.Printf("Cannot close the ocm (possible memory leak): %q", ocmCloseErr)
		}
	}()
	cluster, err := utils.GetClusterAnyStatus(ocm, o.clusterID)
	o.cluster = cluster
	o.clusterID = cluster.ID()

	userDetails, err := ocm.AccountsMgmt().V1().Accounts().Account(o.newOwnerName).Get().Send()
	userName, ok := userDetails.Body().GetUsername()
	if !ok {
		return fmt.Errorf("Failed to get username from new user id")
	}

	var mgmtCluster, svcCluster, hiveCluster, masterCluster *cmv1.Cluster

	o.hypershift, err = utils.IsHostedCluster(o.clusterID)
	if err != nil {
		return fmt.Errorf("failed to check if the given cluster is HCP: %w", err)
	}

	// Find and setup all resources that are needed
	if o.hypershift {
		fmt.Println("Given cluster is HCP, start to proceed the HCP owner transfer")
		mgmtCluster, err = utils.GetManagementCluster(o.clusterID)
		svcCluster, err = utils.GetServiceCluster(o.clusterID)
		if err != nil {
			return err
		}
		masterCluster = svcCluster
	} else {
		fmt.Println("Given cluster is OSD/ROSA classic, start to proceed the classic owner transfer")
		hiveCluster, err = utils.GetHiveCluster(o.clusterID)
		if err != nil {
			return err
		}
		masterCluster = hiveCluster
	}

	elevationReasons := []string{
		o.reason,
		fmt.Sprintf("Updating pull secret using osdctl to tranfert owner to %s", o.newOwnerName),
	}

	masterKubeCli, _, masterKubeClientSet, err := common.GetKubeConfigAndClient(masterCluster.ID(), elevationReasons...)
	if err != nil {
		return fmt.Errorf("failed to retrieve Kubernetes configuration and client for Hive cluster ID %s: %w", masterCluster.ID(), err)
	}

	// Fetch the pull secret with the given new username
	response, err := ocm.AccountsMgmt().V1().AccessToken().Post().Impersonate(userName).Parameter("body", nil).Send()
	if err != nil {
		return fmt.Errorf("Can't send request: %w", err)
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

	// Print the pull secret
	fmt.Println("Pull Secret:")
	fmt.Println(string(pullSecret))

	// Ask the user if they would like to continue
	var continueConfirmation string
	fmt.Print("Do you want to continue? (yes/no): ")
	_, err = fmt.Scanln(&continueConfirmation)
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	// Check the user's response
	if continueConfirmation != "yes" {
		return fmt.Errorf("operation aborted by the user")
	}

	if o.hypershift {
		err = updateManifestWork(ocm, masterKubeCli, o.clusterID, mgmtCluster.Name(), pullSecret)
		if err != nil {
			return fmt.Errorf("failed to update pull secret for service cluster with ID %s: %w", o.clusterID, err)
		}
	} else {
		err = updatePullSecret(ocm, masterKubeCli, masterKubeClientSet, o.clusterID, pullSecret)
		if err != nil {
			return fmt.Errorf("failed to update pull secret for Hive cluster with ID %s: %w", o.clusterID, err)
		}
	}

	_, _, targetClientSet, err := common.GetKubeConfigAndClient(o.clusterID, elevationReasons...)
	if err != nil {
		return fmt.Errorf("failed to retrieve Kubernetes configuration and client for cluster with ID %s: %w", o.clusterID, err)
	}

	// Rollout the telemeterClient pod for non HCP clusters
	if !o.hypershift {
		err = rolloutTelemeterClientPods(targetClientSet, "openshift-monitoring", "app.kubernetes.io/name=telemeter-client")
		if err != nil {
			return fmt.Errorf("failed to roll out Telemeter Client pods in namespace 'openshift-monitoring' with label selector 'app.kubernetes.io/name=telemeter-client': %w", err)
		}
	}

	err = verifyClusterPullSecret(targetClientSet, string(pullSecret))
	if err != nil {
		return fmt.Errorf("error verifying cluster pull secret: %w", err)
	}

	cluster, err = utils.GetCluster(ocm, o.clusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster information for cluster with ID %s: %w", o.clusterID, err)
	}

	externalClusterID, ok := cluster.GetExternalID()
	if !ok {
		return fmt.Errorf("cluster has no external id")
	}

	subscription, err := utils.GetSubscription(ocm, o.clusterID)
	if err != nil {
		return fmt.Errorf("could not get subscription: %w", err)
	}

	oldOwnerAccount, ok := subscription.GetCreator()
	if !ok {
		return fmt.Errorf("cluster has no owner account")
	}

	oldOrganizationId, ok := subscription.GetOrganizationID()
	if !ok {
		return fmt.Errorf("old organization has no ID")
	}

	newAccount, err := utils.GetAccount(ocm, o.newOwnerName)
	if err != nil {
		return fmt.Errorf("could not get new owners account: %w", err)
	}

	newOrganization, ok := newAccount.GetOrganization()
	if !ok {
		return fmt.Errorf("new account has no organization")
	}

	newOrganizationId, ok := newOrganization.GetID()
	if !ok {
		return fmt.Errorf("new organization has no ID")
	}

	accountID, ok := newAccount.GetID()
	if !ok {
		return fmt.Errorf("account has no id")
	}

	subscriptionID, ok := subscription.GetID()
	if !ok {
		return fmt.Errorf("subscription has no id")
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

	fmt.Printf("Transfer cluster: \t\t'%v' (%v)\n", externalClusterID, cluster.Name())
	fmt.Printf("from user \t\t\t'%v' to '%v'\n", oldOwnerAccount.ID(), accountID)
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

	orgChanged := oldOrganizationId != newOrganizationId

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
			return fmt.Errorf("request failed with status: %d, '%w'", response.Status(), err)
		}
		fmt.Printf("Patched organization on subscription\n")
	}

	// patch creator on subscription
	patchRes, err := subscriptionCreatorPatchRequest.Send()

	if err != nil || patchRes.Status() != 200 {
		return fmt.Errorf("request failed with status: %d, '%w'", patchRes.Status(), err)
	}
	fmt.Printf("Patched creator on subscription\n")

	// delete old rolebinding but do not exit on fail could be a rerun
	err = deleteOldRoleBinding(ocm, subscriptionID)

	if err != nil {
		fmt.Printf("can't delete old rolebinding %v \n", err)
	}

	// create new rolebinding
	newRoleBindingClient := ocm.AccountsMgmt().V1().RoleBindings()
	postRes, err := newRoleBindingClient.Add().Body(newRoleBinding).Send()

	// don't fail if the rolebinding already exists, could be rerun
	if err != nil {
		return fmt.Errorf("request failed '%w'", err)
	} else if postRes.Status() == 201 {
		fmt.Printf("Created new role binding.\n")
	} else if postRes.Status() == 409 {
		fmt.Printf("can't add new rolebinding, rolebinding already exists\n")
	} else {
		return fmt.Errorf("request failed with status: %d, '%w'", postRes.Status(), err)
	}

	// If the organization id has changed, re-register the cluster with CS with the new organization id
	if orgChanged {

		request, err := createNewRegisterClusterRequest(ocm, externalClusterID, subscriptionID, newOrganizationId, clusterURL, displayName)
		if err != nil {
			return fmt.Errorf("can't create RegisterClusterRequest with CS, '%w'", err)
		}

		response, err := request.Send()
		if err != nil || (response.Status() != 200 && response.Status() != 201) {
			return fmt.Errorf("request failed with status: %d, '%w'", response.Status(), err)
		}
		fmt.Print("Re-registered cluster\n")
	}

	err = validateTransfer(ocm, subscription.ClusterID(), newOrganizationId)
	if err != nil {
		return fmt.Errorf("error while validating transfer %w", err)
	}
	fmt.Print("Transfer complete\n")

	fmt.Println("Notify the customer the ownership transfer is completed. Sending service log.")
	postCmd = generateServiceLog(o.clusterID, "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/maintenance_completed.json")
	if err := postCmd.Run(); err != nil {
		fmt.Println("Failed to generate service log. Please manually send a service log to Notify the customer  the ownership transfer is completed:")
		fmt.Printf("osdctl servicelog post %v -t %v -p %v\n",
			o.clusterID, "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/maintenance_completed.json", strings.Join(postCmd.TemplateParams, " -p "))
	}

	return nil
}

func getRoleBinding(ocm *sdk.Connection, subscriptionID string) (*amv1.RoleBinding, error) {
	roleBindingQuery := "subscription_id = '%s' and role_id = 'ClusterOwner'"
	searchString := fmt.Sprintf(roleBindingQuery, subscriptionID)
	response, err := ocm.AccountsMgmt().V1().RoleBindings().List().Parameter("search", searchString).
		Send()

	if err != nil {
		return nil, fmt.Errorf("can't send request: %v", err)
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
		return fmt.Errorf("can't get old owners rolebinding %w", err)
	}

	oldRoleBindingID, ok := oldRoleBinding.GetID()
	if !ok {
		return fmt.Errorf("old rolebinding has no id %w", err)
	}
	oldRoleBindingClient := ocm.AccountsMgmt().V1().RoleBindings().RoleBinding(oldRoleBindingID)

	response, err := oldRoleBindingClient.Delete().Send()

	if err != nil {
		return fmt.Errorf("request failed '%w'", err)
	}
	if response.Status() == 204 {
		fmt.Printf("Deleted old rolebinding: %v\n", oldRoleBindingID)
		return nil
	}
	if response.Status() == 404 {
		fmt.Printf("can't find old rolebinding: %v\n", oldRoleBindingID)
		return nil
	}
	fmt.Printf("request failed with status: %d\n", response.Status())
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
		return nil, fmt.Errorf("cannot create body for request '%w'", err)
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
		return nil, fmt.Errorf("cannot create body for request '%w'", err)
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
		return fmt.Errorf("request failed with status: %d, '%w'", response.Status(), err)
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
