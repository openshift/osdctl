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
	"github.com/openshift-online/ocm-sdk-go/logging"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
	bpocmcli "github.com/openshift/backplane-cli/pkg/ocm"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
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

	"github.com/fatih/color"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var BackplaneClusterAdmin = "backplane-cluster-admin"

// Default auth section used to validate ocm account email againstsa
const cloudAuthKey = "cloud.openshift.com"

type Result int

// Const values for results table entries
const (
	Fail Result = iota
	Pass
	NotRun
)

// validatePullSecretOptions defines the struct for running validate-pull-secret command
type validatePullSecretOptions struct {
	account           *v1.Account      // Account which owns target cluster
	clusterID         string           // Target cluster containing pull-secret to be validated against OCM values
	kubeCli           *k8s.LazyClient  // Kubecli/api client for target cluster
	reason            string           // Reason or justification for accessing sensitive data. (ie jira ticket)
	hiveOCMConfigPath string           // Optional path to OCM config used to access hive cluster
	ocm               *sdk.Connection  // openshift api client
	results           *printer.Printer // Used for printing tabled results
	log               logging.Logger   // Simple stderr logger
	verboseLevel      int              // Logging level
	ctx               context.Context  // Context to user for rotation ops
	checkHive         bool             // Flag to validate hive secret data against ocm values
	useAccessToken    bool             // Flag to use OCM access token values for validations
	useRegCreds       bool             // Flag to use OCM registry credentials values for validations
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
`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		PreRun:            func(cmd *cobra.Command, args []string) { cmdutil.CheckErr(ops.preRun(cmd, args)) },
		Run: func(cmd *cobra.Command, args []string) {
			//ops.clusterID = args[0]
			cmdutil.CheckErr(ops.run())
		},
	}
	validatePullSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command to be run (usually an OHSS or PD ticket), mandatory when using elevate")
	validatePullSecretCmd.Flags().StringVar(&ops.hiveOCMConfigPath, "hive-config-path", "", "Path to OCM 'prod' config used to connect to hive when target cluster is using 'stage' or 'integration' envs ")
	validatePullSecretCmd.Flags().IntVarP(&ops.verboseLevel, "verbose", "v", 3, "debug=4, (default)info=3, warn=2, error=1")
	validatePullSecretCmd.Flags().BoolVar(&ops.checkHive, "check-hive", false, "Check values on Hive against OCM for this target cluster")
	validatePullSecretCmd.Flags().BoolVar(&ops.useAccessToken, "check-token", false, "Check OCM Access Token Auth values against cluster")
	validatePullSecretCmd.Flags().BoolVar(&ops.useRegCreds, "check-regcreds", false, "Check OCM Registry Credentials against cluster")
	validatePullSecretCmd.Flags().Bool("check-all", true, "Run all checks: Access Token, Registry Credentials, Hive.")

	_ = validatePullSecretCmd.MarkFlagRequired("reason")
	return validatePullSecretCmd
}

func newValidatePullSecretOptions(client *k8s.LazyClient) *validatePullSecretOptions {
	return &validatePullSecretOptions{
		kubeCli: client,
	}
}

func (o *validatePullSecretOptions) preRun(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Required 1 positional arg for 'Cluster ID'")
	}
	o.clusterID = args[0]

	// init context
	o.ctx = context.TODO()

	// Setup logger
	builder := logging.NewGoLoggerBuilder()
	builder.Debug(bool(o.verboseLevel >= 4))
	builder.Info(bool(o.verboseLevel >= 3))
	builder.Warn(bool(o.verboseLevel >= 2))
	builder.Error(bool(o.verboseLevel >= 1))
	logger, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to build logger: %s", err)
	}
	o.log = logger

	// Check that current kubecli connection is established, and to the correct cluster...
	//TODO: to avoid relying on external config/connection, this tool should establish the backplane connection?
	clusterInfo, err := utils.GetCluster(o.ocm, o.clusterID)
	if err != nil {
		o.log.Error(o.ctx, "Failed to fetch cluster:'%s' info from OCM\n", o.clusterID)
		return err
	}
	// Get the internal cluster ID from OCM for comparing to active/current kubecli connection...
	clusterID := clusterInfo.ID()
	connectedClusterID, err := k8s.GetCurrentCluster()
	if err != nil {
		o.log.Error(o.ctx, "failed to get current kubecli connection's clusterID. Is there an established backplane config/connection?\n")
		return err
	}
	if clusterID != connectedClusterID {
		return fmt.Errorf("provided clusterID:'%s':(%s) does not match current kubecli connection's clusterID:'%s'", o.clusterID, clusterID, connectedClusterID)
	} else {
		if o.clusterID != clusterID {
			o.log.Info(o.ctx, "Using internal clusterID:'%s' for provided clusterID:'%s'\n", clusterID, o.clusterID)
			// Use the internal cluster ID from here on...
			o.clusterID = clusterID
		}
	}
	// init results table...
	o.results = printer.NewTablePrinter(os.Stdout, 1, 1, 1, ' ')
	o.results.AddRow([]string{"----------", "----", "---------", "------", "____", "______"})
	o.results.AddRow([]string{"OCM_SOURCE", "AUTH", "NAMESPACE", "SECRET", "ATTR", "RESULT"})
	o.results.AddRow([]string{"----------", "----", "---------", "------", "____", "______"})

	return nil
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
			o.log.Warn(o.ctx, "Cannot close OCM connection (possible memory leak): %q", ocmCloseErr)
		}
	}()

	// Defer printing whatever results are available when run() returns
	defer func() {
		fmt.Printf("\n\n")
		o.results.Flush()
	}()

	// get account info from OCM
	o.account, err = o.getOCMAccountInfo()
	if err != nil {
		return err
	}
	// account email to be for auth comparison
	emailOCM := o.account.Email()
	o.log.Info(o.ctx, "Found email for cluster's OCM account: %s\n", emailOCM)

	// Get a portion of the pull secret from OCM registry_credentials
	// Note: this does not contain the remaining auths from from: '/api/accounts_mgmt/v1/access_token'
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
		o.addResult("Account", cloudAuthKey, pullSecret.Namespace, pullSecret.Name, "email", Fail)
		o.log.Error(o.ctx, "Pull secret email doesn't match OCM user email. Sending service log.")
		postCmd := servicelog.PostCmdOptions{
			Template:  "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_user_mismatch.json",
			ClusterId: o.clusterID,
		}
		return postCmd.Run()
	}
	o.addResult("Account", cloudAuthKey, pullSecret.Namespace, pullSecret.Name, "email", Pass)
	o.log.Info(o.ctx, "Cluster pull_secret.auth['%s'].email matches OCM account. PASSED\n", cloudAuthKey)
	if len(regCreds) <= 0 {
		// this error should be caught above as well
		return fmt.Errorf("empty registry_credentials for pull_secret returned from OCM")
	}

	// Iterate over registry credentials and compare against cluster's pull secret
	err = o.checkRegistryCredsAgainstPullSecret(regCreds, pullSecret, emailOCM)
	if err != nil {
		return err
	}
	/* Compare OCM stored access token to cluster's pull secret...*/
	o.log.Debug(o.ctx, "Checking OCM AccessToken auth values against secret:%s:%s on cluster...\n", pullSecret.Namespace, pullSecret.Name)
	userName := o.account.Username()
	if len(userName) <= 0 {
		return fmt.Errorf("found empty 'username', needed for accessToken. See account:'%s'", o.account.HREF())
	}
	accessToken, err := o.getAccessTokenFromOCM(userName)
	if err != nil {
		return err
	}
	// Iterate over access token auths and compare against cluster's pull secret
	err = o.checkAccessTokenToPullSecret(accessToken, pullSecret)
	if err != nil {
		return err
	}

	/* Checks against hive to confirm hive is sync'd with OCM */
	//Create Hive connection...
	o.log.Debug(o.ctx, "\nCreating Hive connection...\n")
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
	o.log.Debug(o.ctx, "Attempting to fetch ClusterDeployment on hive for target cluster...")
	clusterDep, err := o.getClusterDeployment(hiveKubeCli, o.clusterID)
	if err != nil {
		o.log.Error(o.ctx, "Error while fetching hive clusterDeployment for cluster:'%s'\n", o.clusterID)
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
	// Iterate over registry credentials and compare against hive's pull secretCM...
	o.log.Debug(o.ctx, "Checking RegistryCredentials against Hive secret...\n")
	err = o.checkRegistryCredsAgainstPullSecret(regCreds, hiveSecret, emailOCM)
	if err != nil {
		return err
	}
	// Iterate over access token auths and compare against hive's pull secret
	o.log.Debug(o.ctx, "Checking AccessToken against Hive secret...\n")
	err = o.checkAccessTokenToPullSecret(accessToken, hiveSecret)
	if err != nil {
		return err
	}

	return nil
}

func (o *validatePullSecretOptions) addResult(ocmSource string, auth string, psNamespace string, psName string, attr string, result Result) {
	var resultStr string
	switch result {
	case Pass:
		resultStr = color.GreenString("PASS")
	case Fail:
		resultStr = color.RedString("FAIL")
	case NotRun:
		resultStr = "Not_Run"
	default:
		resultStr = color.CyanString("Unknown(%d)", int(result))
	}
	o.results.AddRow([]string{ocmSource, auth, psNamespace, psName, attr, resultStr})
}

// Iterate over cluster deployments on this hive cluster, return CD matching the target cluster id...
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

// There is likely more auth sections in the pull secret on cluster than in the OCM accessToken.
// Iterate over Access Token auth sections and confirm these values match on the cluster...
func (o *validatePullSecretOptions) checkAccessTokenToPullSecret(accessToken *v1.AccessToken, pullSecret *corev1.Secret) error {
	for akey, auth := range accessToken.Auths() {
		o.log.Debug(o.ctx, "\nChecking OCM AccessToken values against secret:'%s':'%s'...\n", pullSecret.Namespace, pullSecret.Name)
		// Find the matching auth entry for this registry name in the cluster pull_secret data...
		psTokenAuth, err := getPullSecretTokenAuth(akey, pullSecret)
		if err != nil {
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "auth", Fail)
			return fmt.Errorf("OCM accessToken.auth['%s'], failed to fetch from cluster pull-secret, err:'%s'", akey, err)
		}
		if auth.Auth() != psTokenAuth.Auth() {
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "token", Fail)
			return fmt.Errorf("OCM accessToken.auth['%s'] authToken does not match token found in cluster pull-secret ", akey)
		} else {
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "token", Pass)
			o.log.Info(o.ctx, "OCM accessToken.auth['%s']. OCM and cluster tokens match. PASS\n", akey)
		}
		if auth.Email() != psTokenAuth.Email() {
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "email", Fail)
			return fmt.Errorf("OCM accessToken.auth['%s'] authToken does not match email found in cluster pull-secret ", akey)
		} else {
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "email", Pass)
			o.log.Info(o.ctx, "OCM accessToken.auth['%s']. Email matches OCM account. PASS\n", akey)
		}
	}
	o.log.Info(o.ctx, "-------------------------------------------\n")
	o.log.Info(o.ctx, "Access Token checks PASSED\n")
	o.log.Info(o.ctx, "-------------------------------------------\n")
	return nil
}

// Check the registry_credentials against each of the corresponding cluster pull_secret auth sections...
// There is likely more auth sections in the pull secret on cluster than in the OCM registry_credentials.
func (o *validatePullSecretOptions) checkRegistryCredsAgainstPullSecret(regCreds []*v1.RegistryCredential, pullSecret *corev1.Secret, emailOCM string) error {
	o.log.Debug(o.ctx, "Checking OCM registry_credential values against secret:'%s':'%s'...", pullSecret.Namespace, pullSecret.Name)
	for _, regCred := range regCreds {
		o.log.Debug(o.ctx, "OCM registry_credential:'%s'\n", regCred.HREF())
		// registry_credential.token is stored plain text in OCM, no need to decode here...
		token := regCred.Token()
		username := regCred.Username()
		if len(token) <= 0 {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			return fmt.Errorf("empty token for registry_credential. See:'ocm get %s'", regCred.HREF())
		}
		if len(username) <= 0 {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			return fmt.Errorf("empty username for registry_credential. See:'ocm get %s'", regCred.HREF())
		}
		// Auth token in cluster's pull-secret data uses format: "username + ':' + token"
		regToken := fmt.Sprintf("%s:%s", username, token)
		//Get the exact registry name from the registry_credentials registry.id ...
		registryID := regCred.Registry().ID()
		registry, err := o.getRegistryFromOCM(registryID)
		if err != nil {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "registry", NotRun)
			o.log.Error(o.ctx, "Failed to fetch registry:'%s' from OCM. Err:'%s'\n", registryID, err)
			return err
		}
		//The registry name is used to find the correct section in the cluster pull-secret data.
		regName := registry.Name()
		if len(regName) <= 0 {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "registry", NotRun)
			return fmt.Errorf("empty name for registry_credential. See:'ocm get %s'", registry.HREF())
		}
		// Find the matching auth entry for this registry name in the cluster pull-secret data...
		secTokenAuth, err := getPullSecretTokenAuth(regName, pullSecret)
		if err != nil {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "auth", Fail)
			return fmt.Errorf("OCM registry_credential['%s'] failed to fetch auth section from cluster pull secret, err:'%s'", regName, err)
		}
		// Check all auth sections matching registries found in the registry_credentials for matching emails...
		secEmail := secTokenAuth.Email()
		if emailOCM != secEmail {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "email", NotRun)
			return fmt.Errorf("OCM registry_credential['%s'] email:'%s' does not match value found in cluster pull_secret:'%s'", regName, emailOCM, secEmail)
		} else {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "email", Pass)
			o.log.Info(o.ctx, "OCM registry_credential['%s']. OCM and cluster emails match. PASS\n", regName)
		}
		// Get the token from the cluster pull_secret...
		secToken := secTokenAuth.Auth()
		if len(secToken) <= 0 {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			return fmt.Errorf("empty token found in cluster pull-secret for auth section:'%s', err:'%s'", regName, err)
		}
		// This token is stored base64 encoded with a prefix added...
		secTokDecoded, err := b64.StdEncoding.DecodeString(secToken)
		if err != nil {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			return fmt.Errorf("error decoding token in cluster pull-secret for auth section:'%s', err:'%s'", regName, err)
		}
		//Compare OCM registry_credential token to cluster-config/pull_secret token...
		if regToken != string(secTokDecoded) {
			// This should point to the sop and/or ocm pull secret sync util(s) available...
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			o.log.Error(o.ctx, "OCM registry_credential['%s'] token did NOT match value found in cluster pull_secret!\n"+
				"May need to sync ocm credentials to cluster pull secret.\n", regName)
			return fmt.Errorf("OCM registry_credential['%s'] token did NOT match value found in cluster pull_secret", regName)
		} else {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Pass)
			o.log.Info(o.ctx, "OCM registry_credential['%s']. OCM and cluster tokens match. PASS\n", regName)
		}
	}
	o.log.Info(o.ctx, "-------------------------------------------\n")
	o.log.Info(o.ctx, "registry_credentials checks PASSED\n")
	o.log.Info(o.ctx, "-------------------------------------------\n")
	return nil
}

// Connect to a hive cluster using the same OCM environment/config as the target cluster...
func (o *validatePullSecretOptions) connectHiveSameOCMEnv(clusterID string) (k8sclient.Client, *kubernetes.Clientset, error) {
	// This action requires elevation
	elevationReasons := []string{}
	elevationReasons = append(elevationReasons, o.reason)
	elevationReasons = append(elevationReasons, "Elevation required to validate pull-secret")
	// Build hive client connection config from discovered env vars...
	hiveCluster, err := utils.GetHiveCluster(clusterID)
	if err != nil {
		o.log.Error(o.ctx, "error fetching hive for cluster:'%s'.\n", err)
		return nil, nil, err
	}
	if hiveCluster == nil || hiveCluster.ID() == "" {
		return nil, nil, fmt.Errorf("failed to fetch hive cluster ID")
	}
	o.log.Debug(o.ctx, "Using Hive Cluster: '%s'\n", hiveCluster.ID())
	// If some elevationReasons are provided, then the config will be elevated with user backplane-cluster-admin...
	o.log.Debug(o.ctx, "Creating hive backplane client using OCM environment variables")
	hiveKubeCli, _, hivecClientset, err := common.GetKubeConfigAndClient(hiveCluster.ID(), elevationReasons...)
	if err != nil {
		o.log.Error(o.ctx, "Err fetching hive cluster client, GetKubeConfigAndClient() err:'%+v'\n", err)
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
	o.log.Debug(o.ctx, "Attempting to create OCM config for hive connection...\n")
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
		o.log.Error(o.ctx, "Error fetching hive shard for cluster. If OCM env for hive is different than target cluster, see --hive-ocm-config arg\nerr:'%s'\n", err)
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
		o.log.Error(o.ctx, "Failed Hive connection: %s: %v\n", errorMessage, err)
		return nil, nil, fmt.Errorf("%s: %w", errorMessage, err)
	}

	ocmEnvResponse := responseEnv.Body()
	bpUrl, ok := ocmEnvResponse.GetBackplaneURL()
	if !ok {
		return nil, nil, fmt.Errorf("failed to find a working backplane proxy url for the OCM environment: %v", ocmEnvResponse.Name())
	}

	// Do the backplane config portion...
	bpFilePath, err := bpconfig.GetConfigFilePath()
	o.log.Debug(o.ctx, "Using backplane config:'%s'\n", bpFilePath)
	if err != nil {
		return nil, nil, err
	}
	_, err = os.Stat(bpFilePath)
	if err != nil {
		o.log.Error(o.ctx, "Failed to stat ocm config:'%s', err:'%v'\n", bpFilePath, err)
		return nil, nil, fmt.Errorf("failed to stat ocm config:'%s', err:'%v'", bpFilePath, err)
	}
	viper.AutomaticEnv()
	viper.SetConfigFile(bpFilePath)
	viper.SetConfigType("json")
	if err := viper.ReadInConfig(); err != nil {
		o.log.Error(o.ctx, "Hive OCM config err:'%v'\n", err)
		return nil, nil, err
	}
	if viper.GetStringSlice("proxy-url") == nil && os.Getenv("HTTPS_PROXY") == "" {
		return nil, nil, fmt.Errorf("proxy-url must be set explicitly in either config file or via the environment HTTPS_PROXY")
	}
	proxyURLs := viper.GetStringSlice("proxy-url")

	// Provide warning as well as config+path if aws proxy not provided...
	awsProxyUrl := viper.GetString(awsprovider.ProxyConfigKey)
	if awsProxyUrl == "" {
		o.log.Error(o.ctx, "key:'%s' not found in config:'%s'\n", awsprovider.ProxyConfigKey, viper.GetViper().ConfigFileUsed())
	}

	// Now that we have the backplane URL from OCM, use it find the first working Proxy URL...
	proxyURL := k8s.GetFirstWorkingProxyURL(bpUrl, proxyURLs, debug)

	// Get the OCM access token...
	accessToken, err := bpocmcli.DefaultOCMInterface.GetOCMAccessTokenWithConn(hiveOCMConn)
	if err != nil {
		o.log.Error(o.ctx, "error fetching OCM access token:'%v'\n", err)
		return nil, nil, err
	}
	// Attempt to login and get the backplane API URL for this Hive Cluster...
	bpAPIClusterURL, err := k8s.GetBPAPIClusterLoginProxyUri(bpUrl, hiveShard.ID(), *accessToken, proxyURL)
	if err != nil {
		o.log.Error(o.ctx, "Using backplane URL:'%s', proxy:'%s'\n", bpUrl, proxyURL)
		o.log.Error(o.ctx, "Failed BP hive login: URL:'%s', Proxy:'%s'. Check VPN, accessToken, etc...?\n", bpUrl, proxyURL)
		return nil, nil, fmt.Errorf("failed to backplane login to hive: '%s': '%v'", hiveShard.ID(), err)
	}
	o.log.Debug(o.ctx, "Using backplane CLUSTER API URL: '%s'\n", bpAPIClusterURL)
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
		o.log.Error(o.ctx, "Error creating Hive client:'%v'\n", err)
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
	o.log.Debug(o.ctx, "Success creating Hive kube client using ocm config:'%s'\n", ocmConfigHivePath)
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
		o.log.Error(o.ctx, "Failed to create defaults ocm sdk connection, err:'%s'\n", err)
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
		o.log.Error(o.ctx, "Failed to get provisionShard, err:'%s'\n", err)
		return nil, err
	}
	if shardPath == nil {
		o.log.Error(o.ctx, "Failed to get provisionShard, returned nil\n")
		return nil, fmt.Errorf("failed to get provisionShard, returned nil")
	}

	id, ok := shardPath.Body().GetID()
	if ok {
		o.log.Debug(o.ctx, "Got provision shard ID:'%s'\n", id)
	}

	shard := shardPath.Body().HiveConfig().Server()
	o.log.Debug(o.ctx, "Got provision shard:'%s'\n", shard)

	hiveApiUrl, ok := shardPath.Body().HiveConfig().GetServer()
	if !ok {
		return nil, fmt.Errorf("no provision shard url found for %s", clusterID)
	}
	o.log.Debug(o.ctx, "Got hiveApiUrl:'%s'", hiveApiUrl)

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
	fmt.Fprintf(os.Stderr, "Getting the pull-secret in the cluster with elevated permissions\n")
	kubeCli.Impersonate(BackplaneClusterAdmin, reason, fmt.Sprintf("Elevation required to get pull secret email to check if it matches the owner email for %s cluster", clusterID))
	//secret := &corev1.Secret{}
	if err := kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret); err != nil {
		fmt.Printf("Failed to fetch openshift-config/pull-secret from cluster\n")
		return "", err, false
	}

	clusterPullSecretEmail, err, done := getPullSecretEmail(clusterID, secret, true)
	if done {
		return "", err, true
	}
	fmt.Fprintf(os.Stderr, "Email from cluster pull-secret auth['%s]: %s\n", cloudAuthKey, clusterPullSecretEmail)

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
	o.log.Debug(o.ctx, "Attempting to get accessToken for user:'%s'\n", impersonateUser)
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
		o.log.Debug(o.ctx, "Impersonate not needed, this account is owned by current OCM user:'%s'\n", currentUserInfo.Username())
		tokenResp, err = o.ocm.AccountsMgmt().V1().AccessToken().Post().Send()
	}

	//For test purposes use the caller's ocm user with no impersonation...
	//tokenResp, err := o.ocm.AccountsMgmt().V1().AccessToken().Post().Send()
	if err != nil {
		if tokenResp != nil {
			if tokenResp.Status() == 403 {
				o.log.Error(o.ctx,
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
	o.log.Debug(o.ctx, "Getting registry for registryID:'%s'\n", registryID)
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
	fmt.Fprintf(os.Stderr, "Getting registry_credentials from OCM\n")
	if len(accountID) <= 0 {
		return nil, fmt.Errorf("getPullSecretFromOCM() provided empty accountID"), false
	}

	registryCredentials, err := utils.GetRegistryCredentials(o.ocm, accountID)
	if err != nil {
		return nil, err, false
	}
	// validate the registryCredentials before return
	if len(registryCredentials) == 0 {
		o.log.Error(o.ctx, "There is no pull secret in OCM.\n"+
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
		fmt.Fprintf(os.Stderr, "Secret does not contain expected key '.dockerconfigjson'. Sending service log.")
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
		fmt.Fprintf(os.Stderr, "Secret does not contain entry for %s\n", cloudAuthKey)
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
		fmt.Fprintf(os.Stderr, "%v%s\n%v\n%v\n",
			"Couldn't extract email address from pull secret for:", cloudAuthKey,
			"This can mean the pull secret is misconfigured. Please verify the pull secret manually:",
			"  oc get secret -n openshift-config pull-secret -o json | jq -r '.data[\".dockerconfigjson\"]' | base64 -d")
		return "", nil, true
	}
	return clusterPullSecretEmail, nil, false
}
