package cluster

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
	bpocmcli "github.com/openshift/backplane-cli/pkg/ocm"

	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubernetes "k8s.io/client-go/kubernetes"
	k8srest "k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	//"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var BackplaneClusterAdmin = "backplane-cluster-admin"

const cloudAuthKey = "cloud.openshift.com"

// validatePullSecretOptions defines the struct for running validate-pull-secret command
type validatePullSecretOptions struct {
	//elevate   bool //This appears unused ?
	account           *v1.Account
	clusterID         string
	kubeCli           *k8s.LazyClient
	reason            string
	hiveOCMConfigPath string
	ocm               *sdk.Connection
}

func newCmdValidatePullSecret(kubeCli *k8s.LazyClient) *cobra.Command {
	ops := newValidatePullSecretOptions(kubeCli)
	validatePullSecretCmd := &cobra.Command{
		Use:   "validate-pull-secret [CLUSTER_ID]",
		Short: "Checks if the pull-secret data is synced with current OCM data",
		Long: `
	Attempts to validate if a cluster's pull-secret auth and email values are in sync with account, 
	registry_credential, and access token data stored in OCM. 
	This requires the caller to be logged into the cluster to be validated. 
	Note: This utility attempts to validate that the data stored in the pull-secret on the cluster
	is in sync with data stored in OCM. It does not attempt to validate that the cluster pull-secret is in sync with Hive, 
	and/or that Hive is in-sync with OCM. 
`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ops.clusterID = args[0]
			cmdutil.CheckErr(ops.run())
		},
	}
	validatePullSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command to be run (usualy an OHSS or PD ticket), mandatory when using elevate")
	validatePullSecretCmd.Flags().StringVar(&ops.hiveOCMConfigPath, "hive-config-path", "", "Path to OCM 'prod' config used to connect to hive when target cluster is using 'stage' or 'integration' envs ")
	_ = validatePullSecretCmd.MarkFlagRequired("reason")
	return validatePullSecretCmd
}

func newValidatePullSecretOptions(client *k8s.LazyClient) *validatePullSecretOptions {
	return &validatePullSecretOptions{
		kubeCli: client,
	}
}

func (o *validatePullSecretOptions) run() error {
	var err error
	// Create OCM connection...
	o.ocm, err = utils.CreateConnection()
	if err != nil {
		return err
	}
	// Defer closing OCM connection once run() completes...
	defer func() {
		if ocmCloseErr := o.ocm.Close(); ocmCloseErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Cannot close OCM connection (possible memory leak): %q", ocmCloseErr)
		}
	}()

	// Check that current kubecli connection is established, and to the correct cluster...
	//TODO: to avoid relying on external config/connection, this tool should establish the backplane connection?
	clusterInfo, err := utils.GetCluster(o.ocm, o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch cluster:'%s' info from OCM\n", o.clusterID)
		return err
	}
	// Get the internal cluster ID from OCM for comparing to active/current kubecli connection...
	clusterID := clusterInfo.ID()
	connectedClusterID, err := k8s.GetCurrentCluster()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get current kubecli connection's clusterID. Is there an established backplane config/connection?\n")
		return err
	}
	if clusterID != connectedClusterID {
		return fmt.Errorf("provided clusterID:'%s':(%s) does not match current kubecli connection's clusterID:'%s'", o.clusterID, clusterID, connectedClusterID)
	} else {
		fmt.Fprintf(os.Stderr, "Using internal clusterID:'%s' for provided clusterID:'%s'\n", clusterID, o.clusterID)
		// Use the internal cluster ID from here on...
		o.clusterID = clusterID
	}

	// get account info from OCM
	o.account, err = o.getOCMAccountInfo()
	if err != nil {
		return err
	}
	emailOCM := o.account.Email()
	fmt.Printf("Found email for cluster's OCM account: %s\n", emailOCM)

	// Get a portion of the pull secret from OCM registry_credentials
	// Note: this does not contain the token from: '/api/accounts_mgmt/v1/access_token'
	regCreds, err, done := o.getOCMRegistryCredentials(o.account.ID())
	if err != nil {
		return err
	}
	if done {
		//Indicates posting a service log was attempted
		return nil
	}

	// get the pull secret in cluster
	pullSecret := &corev1.Secret{}
	emailCluster, err, done := getPullSecretElevated(o.clusterID, o.kubeCli, o.reason, pullSecret)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	// This checks that the 'cloud.openshift.com' auth object stored in the cluster's pull_secret
	// Has the same email as the current account email.
	if emailOCM != emailCluster {
		_, _ = fmt.Fprintln(os.Stderr, "Pull secret email doesn't match OCM user email. Sending service log.")
		postCmd := servicelog.PostCmdOptions{
			Template:  "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_user_mismatch.json",
			ClusterId: o.clusterID,
		}
		return postCmd.Run()
	}
	fmt.Printf("-------------------------------------------\n")
	fmt.Printf("Cluster pull_secret.auth['%s'].email matches OCM account. PASSED\n", cloudAuthKey)
	fmt.Printf("-------------------------------------------\n")
	if len(regCreds) <= 0 {
		// this error should be caught above as well
		return fmt.Errorf("empty registry_credentials for pull_secret returned from OCM")
	}
	err = o.checkRegistryCredsAgainstPullSecret(regCreds, pullSecret, emailOCM)
	if err != nil {
		return err
	}
	fmt.Printf("Checking OCM AccessToken auth values against secret:%s:%s on cluster...\n", pullSecret.Namespace, pullSecret.Name)
	/* Compare OCM stored access token to cluster's pull secret...*/

	userName := o.account.Username()
	if len(userName) <= 0 {
		return fmt.Errorf("found empty 'username', needed for accessToken. See account:'%s'", o.account.HREF())
	}
	accessToken, err := o.getAccessTokenFromOCM(userName)
	if err != nil {
		return err
	}
	err = o.checkAccessTokenToPullSecret(accessToken, pullSecret)
	if err != nil {
		return err
	}
	//Create Hive connection...
	fmt.Fprintf(os.Stderr, "\nCreating Hive connection...\n")
	var hiveKubeCli k8sclient.Client
	var hiveClientSet *kubernetes.Clientset
	if len(o.hiveOCMConfigPath) > 0 {
		hiveKubeCli, hiveClientSet, err = o.connectHiveUsingOCMConfig(o.clusterID, o.hiveOCMConfigPath, o.reason)
		if err != nil {
			return err
		}
	} else {
		hiveKubeCli, hiveClientSet, err = o.connectHiveSameOCMEnv(o.clusterID)
		if err != nil {
			return err
		}
	}
	//Fetch ClusterDeployment on Hive...
	fmt.Fprintf(os.Stderr, "\nAttempting to fetch ClusterDeployment on hive for target cluster...\n")
	clusterDep, err := o.getClusterDeployment(hiveKubeCli, o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while fetching hive clusterDeployment for cluster:'%s'\n", o.clusterID)
		return err
	}
	cdName := clusterDep.Name
	if len(cdName) <= 0 {
		return fmt.Errorf("retrieved empty clusterDeployment name in NameSpace:'%s'", clusterDep.Namespace)
	}
	//Fetch pull secret for this cluster on Hive...
	secretName := "pull"
	hiveSecret, err := hiveClientSet.CoreV1().Secrets(clusterDep.Namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete secret %v in namespacd %v: %w", secretName, clusterDep.Namespace, err)
	}
	//Validate Hive Creds against what is stored in OCM...
	fmt.Printf("Checking RegistryCredentials against Hive secret...\n")
	err = o.checkRegistryCredsAgainstPullSecret(regCreds, hiveSecret, emailOCM)
	if err != nil {
		return err
	}
	fmt.Printf("Checking AccessToken against Hive secret...\n")
	err = o.checkAccessTokenToPullSecret(accessToken, hiveSecret)
	if err != nil {
		return err
	}

	return nil
}

func (o *validatePullSecretOptions) getClusterDeployment(hiveKubeCli k8sclient.Client, clusterID string) (*hiveapiv1.ClusterDeployment, error) {
	var cds hiveapiv1.ClusterDeploymentList
	if err := hiveKubeCli.List(context.TODO(), &cds, &client.ListOptions{}); err != nil {
		return nil, err
	}
	for _, cd := range cds.Items {
		if strings.Contains(cd.Namespace, clusterID) {
			return &cd, nil
		}
	}
	return nil, fmt.Errorf("ClusterDeployment not found for:'%s'", clusterID)
}

func (o *validatePullSecretOptions) checkAccessTokenToPullSecret(accessToken *v1.AccessToken, pullSecret *corev1.Secret) error {
	// There is likely more auth sections in the pull secret on cluster than in the OCM accessToken.
	// Iterate over Access Token auth sections and confirm these values match on the cluster...
	for akey, auth := range accessToken.Auths() {
		fmt.Fprintf(os.Stderr, "\nChecking OCM AccessToken values against secret:'%s':'%s'...\n", pullSecret.Namespace, pullSecret.Name)
		fmt.Printf("-------------------------------------------\n")
		// Find the matching auth entry for this registry name in the cluster pull_secret data...
		psTokenAuth, err := getPullSecretTokenAuth(akey, pullSecret)
		if err != nil {
			return fmt.Errorf("OCM accessToken.auth['%s'], failed to fetch from cluster pull-secret, err:'%s'", akey, err)
		}
		if auth.Auth() != psTokenAuth.Auth() {
			return fmt.Errorf("OCM accessToken.auth['%s'] authToken does not match token found in cluster pull-secret ", akey)
		} else {
			fmt.Printf("OCM accessToken.auth['%s']. OCM and cluster tokens match. PASS\n", akey)
		}
		if auth.Email() != psTokenAuth.Email() {
			return fmt.Errorf("OCM accessToken.auth['%s'] authToken does not match email found in cluster pull-secret ", akey)
		} else {
			fmt.Printf("OCM accessToken.auth['%s']. Email matches OCM account. PASS\n", akey)
		}
	}
	fmt.Printf("-------------------------------------------\n")
	fmt.Printf("Access Token checks PASSED\n")
	return nil
}

// Check the registry_credentials against each of the corresponding cluster pull_secret auth sections...
// There is likely more auth sections in the pull secret on cluster than in the OCM registry_credentials.
func (o *validatePullSecretOptions) checkRegistryCredsAgainstPullSecret(regCreds []*v1.RegistryCredential, pullSecret *corev1.Secret, emailOCM string) error {
	fmt.Fprintf(os.Stderr, "\nChecking OCM registry_credential values against secret:'%s':'%s'...\n", pullSecret.Namespace, pullSecret.Name)
	for _, regCred := range regCreds {
		fmt.Printf("-------------------------------------------\n")
		fmt.Printf("OCM registry_credential:'%s'\n", regCred.HREF())
		// registry_credential.token is stored plain text in OCM, no need to decode here...
		token := regCred.Token()
		username := regCred.Username()
		if len(token) <= 0 {
			return fmt.Errorf("empty token for registry_credential. See:'ocm get %s'", regCred.HREF())
		}
		if len(username) <= 0 {
			return fmt.Errorf("empty username for registry_credential. See:'ocm get %s'", regCred.HREF())
		}
		// Auth token in cluster's pull-secret data uses format: "username + ':' + token"
		regToken := fmt.Sprintf("%s:%s", username, token)
		//Get the exact registry name from the registry_credentials registry.id ...
		registryID := regCred.Registry().ID()
		registry, err := o.getRegistryFromOCM(registryID)
		if err != nil {
			fmt.Printf("Failed to fetch registry:'%s' from OCM. Err:'%s'\n", registryID, err)
			return err
		}
		//The registry name is used to find the correct section in the cluster pull-secret data.
		regName := registry.Name()
		if len(regName) <= 0 {
			return fmt.Errorf("empty name for registry_credential. See:'ocm get %s'", registry.HREF())
		}
		// Find the matching auth entry for this registry name in the cluster pull-secret data...
		secTokenAuth, err := getPullSecretTokenAuth(regName, pullSecret)
		if err != nil {
			return fmt.Errorf("OCM registry_credential['%s'] failed to fetch auth section from cluster pull secret, err:'%s'", regName, err)
		}
		// Check all auth sections matching registries found in the registry_credentials for matching emails...
		secEmail := secTokenAuth.Email()
		if emailOCM != secEmail {
			return fmt.Errorf("OCM registry_credential['%s'] email:'%s' does not match value found in cluster pull_secret:'%s'", regName, emailOCM, secEmail)
		} else {
			fmt.Printf("OCM registry_credential['%s']. OCM and cluster emails match. PASS\n", regName)
		}
		// Get the token from the cluster pull_secret...
		secToken := secTokenAuth.Auth()
		if len(secToken) <= 0 {
			return fmt.Errorf("empty token found in cluster pull-secret for auth section:'%s', err:'%s'", regName, err)
		}
		// This token is stored base64 encoded with a prefix added...
		secTokDecoded, err := b64.StdEncoding.DecodeString(secToken)
		if err != nil {
			return fmt.Errorf("error decoding token in cluster pull-secret for auth section:'%s', err:'%s'", regName, err)
		}
		//Compare OCM registry_credential token to cluster-config/pull_secret token...
		if regToken != string(secTokDecoded) {
			// This should point to the sop and/or ocm pull secret sync util(s) available...
			fmt.Fprintf(os.Stderr, "OCM registry_credential['%s'] token did NOT match value found in cluster pull_secret!\n"+
				"May need to sync ocm credentials to cluster pull secret.\n", regName)
			return fmt.Errorf("OCM registry_credential['%s'] token did NOT match value found in cluster pull_secret", regName)
		} else {
			fmt.Printf("OCM registry_credential['%s']. OCM and cluster tokens match. PASS\n", regName)
		}
	}
	fmt.Printf("-------------------------------------------\n")
	fmt.Printf("registry_credentials checks PASSED\n")
	fmt.Printf("-------------------------------------------\n")
	return nil
}

func (o *validatePullSecretOptions) connectHiveSameOCMEnv(clusterID string) (k8sclient.Client, *kubernetes.Clientset, error) {
	// This action requires elevation
	elevationReasons := []string{}
	elevationReasons = append(elevationReasons, o.reason)
	elevationReasons = append(elevationReasons, "Elevation required to validate pull-secret")
	// Build hive client connection config from discovered env vars...
	hiveCluster, err := utils.GetHiveCluster(clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching hive for cluster:'%s'.\n", err)
		return nil, nil, err
	}
	if hiveCluster == nil || hiveCluster.ID() == "" {
		return nil, nil, fmt.Errorf("failed to fetch hive cluster ID")
	}
	fmt.Fprintf(os.Stderr, "Using Hive Cluster: '%s'\n", hiveCluster.ID())
	// If some elevationReasons are provided, then the config will be elevated with user backplane-cluster-admin...
	fmt.Fprintf(os.Stderr, "Creating hive backplane client using OCM environment variables")
	hiveKubeCli, _, hivecClientset, err := common.GetKubeConfigAndClient(hiveCluster.ID(), elevationReasons...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Err fetching hive cluster client, GetKubeConfigAndClient() err:'%+v'\n", err)
		return nil, nil, err
	}
	return hiveKubeCli, hivecClientset, nil
}

// This is intended to allow a backplane connection to a hive cluster that uses a different OCM env than
// the target cluster's OCM env. As of know the existing connection utils and wrapper functions
// rely on a chain of functions which use env vars as a higher precedence than the provided
// config obj(s).
// TODO: refactor connection functions to use a passed config object.
// The config object can be built/populated however needed prior to the connection functions.
func (o *validatePullSecretOptions) connectHiveUsingOCMConfig(clusterID string, ocmConfigHivePath string, elevateReason string) (k8sclient.Client, *kubernetes.Clientset, error) {
	fmt.Fprintf(os.Stderr, "Attempting to create OCM config for hive connection...\n")
	debug := true

	//ocmConfigHivePath = "~/.config/ocm/config.prod.json"
	hiveOCMConfig, err := utils.LoadOCMConfigPath(ocmConfigHivePath)
	if err != nil {
		return nil, nil, err
	}
	hiveOCMConn, err := utils.CreateConnectionFromConfig(hiveOCMConfig)
	if err != nil {
		return nil, nil, err
	}
	hiveShard, err := o.getHiveShardForCluster(hiveOCMConn, o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching hive shard for cluster. If OCM env for hive is different than target cluster, see --hive-ocm-config arg\nerr:'%s'\n", err)
		return nil, nil, err
	}
	if hiveShard == nil {
		return nil, nil, fmt.Errorf("failed to fetch hive cluster ID")
	}
	// Get the backplane URL from the OCM env request/response...
	responseEnv, err := hiveOCMConn.ClustersMgmt().V1().Environment().Get().Send()
	if err != nil {
		// Check if the error indicates a forbidden status
		var isForbidden bool
		if responseEnv != nil {
			isForbidden = responseEnv.Status() == http.StatusForbidden || (responseEnv.Error() != nil && responseEnv.Error().Status() == http.StatusForbidden)
		}

		// Construct error message based on whether the error is related to permissions
		var errorMessage string
		if isForbidden {
			errorMessage = "user does not have enough permissions to fetch the OCM environment resource. Please ensure you have the necessary permissions or try exporting the BACKPLANE_URL environment variable."
		} else {
			errorMessage = "failed to fetch OCM cluster environment resource"
		}
		fmt.Fprintf(os.Stderr, "Failed Hive connection: %s: %v\n", errorMessage, err)
		return nil, nil, fmt.Errorf("%s: %w", errorMessage, err)
	}

	ocmEnvResponse := responseEnv.Body()
	bpUrl, ok := ocmEnvResponse.GetBackplaneURL()
	if !ok {
		return nil, nil, fmt.Errorf("failed to find a working backplane proxy url for the OCM environment: %v", ocmEnvResponse.Name())
	}

	// Do the backplane config portion...
	bpFilePath, err := bpconfig.GetConfigFilePath()
	fmt.Fprintf(os.Stderr, "Using backplane config:'%s'\n", bpFilePath)
	if err != nil {
		return nil, nil, err
	}
	_, err = os.Stat(bpFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stat ocm config:'%s', err:'%v'\n", bpFilePath, err)
		return nil, nil, fmt.Errorf("failed to stat ocm config:'%s', err:'%v'", bpFilePath, err)
	}
	viper.AutomaticEnv()
	viper.SetConfigFile(bpFilePath)
	viper.SetConfigType("json")
	if err := viper.ReadInConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Hive OCM config err:'%v'\n", err)
		return nil, nil, err
	}
	if viper.GetStringSlice("proxy-url") == nil && os.Getenv("HTTPS_PROXY") == "" {
		return nil, nil, fmt.Errorf("proxy-url must be set explicitly in either config file or via the environment HTTPS_PROXY")
	}
	proxyURLs := viper.GetStringSlice("proxy-url")

	// Provide warning as well as config+path if aws proxy not provided...
	awsProxyUrl := viper.GetString(awsprovider.ProxyConfigKey)
	if awsProxyUrl == "" {
		fmt.Fprintf(os.Stderr, "key:'%s' not found in config:'%s'\n", awsprovider.ProxyConfigKey, viper.GetViper().ConfigFileUsed())
	}

	// Now that we have the backplane URL from OCM, use it find the first working Proxy URL...
	proxyURL := k8s.GetFirstWorkingProxyURL(bpUrl, proxyURLs, debug)

	// Get the OCM access token...
	accessToken, err := bpocmcli.DefaultOCMInterface.GetOCMAccessTokenWithConn(hiveOCMConn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching OCM access token:'%v'\n", err)
		return nil, nil, err
	}
	// Attempt to login and get the backplane API URL for this Hive Cluster...
	bpAPIClusterURL, err := k8s.GetBPAPIClusterLoginProxyUri(bpUrl, hiveShard.ID(), *accessToken, proxyURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Using backplane URL:'%s', proxy:'%s'\n", bpUrl, proxyURL)
		fmt.Fprintf(os.Stderr, "Failed BP hive login: URL:'%s', Proxy:'%s'. Check VPN, accessToken, etc...?\n", bpUrl, proxyURL)
		return nil, nil, fmt.Errorf("failed to backplane login to hive: '%s': '%v'", hiveShard.ID(), err)
	}
	fmt.Fprintf(os.Stderr, "Using backplane CLUSTER API URL: '%s'\n", bpAPIClusterURL)
	// Build the KubeConfig to be used to build our Kubernetes client...
	var kubeconfig k8srest.Config
	if len(elevateReason) > 0 {
		// This action requires elevation
		elevationReasons := []string{}
		elevationReasons = append(elevationReasons, elevateReason)
		kubeconfig = k8srest.Config{
			Host:        bpAPIClusterURL,
			BearerToken: *accessToken,
			Impersonate: k8srest.ImpersonationConfig{
				UserName: "backplane-cluster-admin",
			},
		}
		// Add the provided elevation reasons for impersonating request as user 'backplane-cluster-admin'
		kubeconfig.Impersonate.Extra = map[string][]string{"reason": elevationReasons}
	} else {
		kubeconfig = k8srest.Config{
			Host:        bpAPIClusterURL,
			BearerToken: *accessToken,
		}
	}
	// If a working proxyURL was found earlier, set the kube config proxy func...
	if len(proxyURL) > 0 {
		kubeconfig.Proxy = func(*http.Request) (*url.URL, error) {
			return url.Parse(proxyURL)
		}
	}

	// Finally create the kubeclient for the hive cluster connection...
	kubeCli, err := k8sclient.New(&kubeconfig, k8sclient.Options{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating Hive client:'%v'\n", err)
		return nil, nil, err
	}
	if kubeCli == nil {
		return nil, nil, fmt.Errorf("client.new() returned nil. Failed to setup Hive client")
	}
	// create the clientset
	clientset, err := kubernetes.NewForConfig(&kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(os.Stderr, "Success creating Hive kube client using ocm config:'%s'\n", ocmConfigHivePath)
	return kubeCli, clientset, nil
}

// Attempts to find hive shard for provided clusterID. Creates an OCM connection using the default
// environment (which is expected to be set for the target cluter), then attempts to return the
// hive cluster obj using the provided sdkConn created for the hive cluster's OCM environment.
// At this time it's possible for the target cluster to exist in a separate environment then it's hive
// cluster. This will occur for integration and staging envs.
func (o *validatePullSecretOptions) getHiveShardForCluster(sdkConn *sdk.Connection, clusterID string) (*cmv1.Cluster, error) {
	// Create connection using target cluster ocm env vars to lookup target cluster.
	connection, err := utils.CreateConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create defaults ocm sdk connection, err:'%s'\n", err)
		return nil, err
	}
	defer connection.Close()
	if sdkConn == nil {
		sdkConn = connection
	}
	shardPath, err := connection.ClustersMgmt().V1().Clusters().
		Cluster(clusterID).
		ProvisionShard().
		Get().
		Send()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get provisionShard, err:'%s'\n", err)
		return nil, err
	}
	if shardPath == nil {
		fmt.Fprintf(os.Stderr, "Failed to get provisionShard, returned nil\n")
		return nil, fmt.Errorf("failed to get provisionShard, returned nil")
	}

	id, ok := shardPath.Body().GetID()
	if ok {
		fmt.Fprintf(os.Stderr, "Got provision shard ID:'%s'\n", id)
	}

	shard := shardPath.Body().HiveConfig().Server()
	fmt.Fprintf(os.Stderr, "Got provision shard:'%s'\n", shard)

	hiveApiUrl, ok := shardPath.Body().HiveConfig().GetServer()
	if !ok {
		return nil, fmt.Errorf("no provision shard url found for %s", clusterID)
	}
	fmt.Fprintf(os.Stderr, "Got hiveApiUrl:'%s'", hiveApiUrl)

	// Use passed sdk connection to fetch hive cluster in case this is conneciton is created with a cli provided config...
	resp, err := sdkConn.ClustersMgmt().V1().Clusters().List().
		Parameter("search", fmt.Sprintf("api.url='%s'", hiveApiUrl)).
		Send()
	if err != nil {
		return nil, err
	}

	if resp.Items().Empty() {
		return nil, fmt.Errorf("failed to find cluster with api.url=%s", hiveApiUrl)
	}

	return resp.Items().Get(0), nil
}

// Parse out AccessTokenAuth for provided registry ID from provided (pull) secret.
func getPullSecretTokenAuth(registryID string, secret *corev1.Secret) (*v1.AccessTokenAuth, error) {
	if len(registryID) <= 0 {
		return nil, fmt.Errorf("error: registryID empty in getPullSecretToken()")
	}
	dockerConfigJsonBytes, found := secret.Data[".dockerconfigjson"]
	if !found {
		return nil, fmt.Errorf("secret missing '.dockerconfigjson'? See servicelog: https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_change_breaking_upgradesync.json")
	}
	dockerConfigJson, err := v1.UnmarshalAccessToken(dockerConfigJsonBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse secret, err: %w", err)
	}
	registryAuth, found := dockerConfigJson.Auths()[registryID]
	if !found {
		return nil, fmt.Errorf("error: registry '%s not found in pull_secret.auths", registryID)
	}
	return registryAuth, nil
}

// getPullSecretElevated gets the pull-secret in the cluster
// with backplane elevation.
func getPullSecretElevated(clusterID string, kubeCli *k8s.LazyClient, reason string, secret *corev1.Secret) (email string, err error, sentSL bool) {
	fmt.Println("Getting the pull-secret in the cluster with elevated permissions")
	kubeCli.Impersonate(BackplaneClusterAdmin, reason, fmt.Sprintf("Elevation required to get pull secret email to check if it matches the owner email for %s cluster", clusterID))
	//secret := &corev1.Secret{}
	if err := kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch openshift-config/pull-secret from cluster\n")
		return "", err, false
	}

	clusterPullSecretEmail, err, done := getPullSecretEmail(clusterID, secret, true)
	if done {
		return "", err, true
	}
	fmt.Printf("Email from cluster pull-secret['%s]: %s\n", cloudAuthKey, clusterPullSecretEmail)

	return clusterPullSecretEmail, nil, false
}

func (o *validatePullSecretOptions) getCurrentOCMUserInfo() (*v1.Account, error) {
	// Fetch OCM current_user info...
	currentAccountResp, err := o.ocm.AccountsMgmt().V1().CurrentAccount().Get().Send()
	if err != nil {
		return nil, err
	}
	currentAccount := currentAccountResp.Body()
	return currentAccount, nil
}

func (o *validatePullSecretOptions) getAccessTokenFromOCM(impersonateUser string) (*v1.AccessToken, error) {
	fmt.Fprintf(os.Stderr, "Attempting to get accessToken for user:'%s'\n", impersonateUser)
	var err error
	var tokenResp *v1.AccessTokenPostResponse
	if len(impersonateUser) <= 0 {
		return nil, fmt.Errorf("err, getAccessTokenFromOCM() provided empty user string")
	}
	currentUserInfo, err := o.getCurrentOCMUserInfo()
	if err != nil || currentUserInfo.Username() != impersonateUser {
		// Impersonate requires elevated (region-lead) permissions.
		tokenResp, err = o.ocm.AccountsMgmt().V1().AccessToken().Post().Impersonate(impersonateUser).Send()
	} else {
		// No need to impersonate, this is the current user's own account.
		// This will allow some level of testing to be performed when acting on one's own account/clusters.
		fmt.Fprintf(os.Stderr, "Impersonate not needed, this account is owned by current OCM user:'%s'\n", currentUserInfo.Username())
		tokenResp, err = o.ocm.AccountsMgmt().V1().AccessToken().Post().Send()
	}

	//For test purposes use the caller's ocm user with no impersonation...
	//tokenResp, err := o.ocm.AccountsMgmt().V1().AccessToken().Post().Send()
	if err != nil {
		if tokenResp != nil {
			if tokenResp.Status() == 403 {
				fmt.Fprintf(os.Stderr,
					"AccessToken ops may require 'region lead' permissions to execute.\n"+
						"See CLI equiv: ocm post --body=/dev/null --header=\"Impersonate-User=%s\" /api/accounts_mgmt/v1/access_token\n", impersonateUser)
			}
		}
		return nil, err
	}
	accessToken, ok := tokenResp.GetBody()
	if !ok {
		return nil, fmt.Errorf("failed to get accessToken response body for impersonated User:'%s'", impersonateUser)
	}
	return accessToken, nil
}

// Fetch OCM Registry for the provided registryID
func (o *validatePullSecretOptions) getRegistryFromOCM(registryID string) (*v1.Registry, error) {
	//fmt.Fprintf(os.Stderr, "Getting registry for registryID:'%s'\n", registryID)
	regResp, err := o.ocm.AccountsMgmt().V1().Registries().Registry(registryID).Get().Send()
	if err != nil {
		return nil, err
	}
	registry, ok := regResp.GetBody()
	if !ok {
		return nil, fmt.Errorf("failed to get request body for registryID:'%s'", registryID)
	}
	return registry, nil
}

// Fetch OCM account info for the clusterID attribute of current validatePullSecretOptions parent obj
func (o *validatePullSecretOptions) getOCMAccountInfo() (*v1.Account, error) {
	subscription, err := utils.GetSubscription(o.ocm, o.clusterID)
	if err != nil {
		return nil, err
	}

	account, err := utils.GetAccount(o.ocm, subscription.Creator().ID())
	if err != nil {
		return nil, err
	}
	return account, nil
}

// getPullSecretFromOCM gets the cluster registry_credentials from OCM
// it returns the email, credentials, error and done
// done means a service log has been sent
func (o *validatePullSecretOptions) getOCMRegistryCredentials(accountID string) ([]*v1.RegistryCredential, error, bool) {
	fmt.Println("Getting registry_credentials from OCM")
	if len(accountID) <= 0 {
		return nil, fmt.Errorf("getPullSecretFromOCM() provided empty accountID"), false
	}

	registryCredentials, err := utils.GetRegistryCredentials(o.ocm, accountID)
	if err != nil {
		return nil, err, false
	}
	// validate the registryCredentials before return
	if len(registryCredentials) == 0 {
		_, _ = fmt.Fprintf(os.Stderr, "There is no pull secret in OCM.\n"+
			"See: /api/accounts_mgmt/v1/registry_credentials -p search=\"account_id='%s'\""+
			"\nSending service log.\n", accountID)
		postCmd := servicelog.PostCmdOptions{
			Template:       "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/update_pull_secret.json",
			TemplateParams: []string{"REGISTRY=registry.redhat.io"},
			ClusterId:      o.clusterID,
		}
		if err = postCmd.Run(); err != nil {
			return nil, err, false
		}
		return nil, nil, true
	}
	return registryCredentials, nil, false
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

	cloudOpenshiftAuth, found := dockerConfigJson.Auths()[cloudAuthKey]
	if !found {
		_, _ = fmt.Fprintf(os.Stderr, "Secret does not contain entry for %s\n", cloudAuthKey)
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
		_, _ = fmt.Fprintf(os.Stderr, "%v%s\n%v\n%v\n",
			"Couldn't extract email address from pull secret for:", cloudAuthKey,
			"This can mean the pull secret is misconfigured. Please verify the pull secret manually:",
			"  oc get secret -n openshift-config pull-secret -o json | jq -r '.data[\".dockerconfigjson\"]' | base64 -d")
		return "", nil, true
	}
	return clusterPullSecretEmail, nil, false
}
