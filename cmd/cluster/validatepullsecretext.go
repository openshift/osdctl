package cluster

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"text/tabwriter"

	"github.com/fatih/color"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var BackplaneClusterAdmin = "backplane-cluster-admin"

// Default auth section used to validate ocm account email againstsa
const cloudAuthKey = "cloud.openshift.com"

// Service log template URLs
const (
	ServiceLogMultipleSyncFailures = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_multiple_sync_failures.json"
	ServiceLogUpdatePullSecret     = "https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/update_pull_secret.json"
)

type Result int

// Const values for results table entries
const (
	Fail Result = iota
	Pass
	NotRun
)

// validatePullSecretExtOptions defines the struct for running validate-pull-secret command
type validatePullSecretExtOptions struct {
	account              *v1.Account         // Account which owns target cluster
	clusterID            string              // Target cluster containing pull-secret to be validated against OCM values
	reason               string              // Reason or justification for accessing sensitive data. (ie jira ticket)
	ocm                  *sdk.Connection     // openshift api client
	results              *tabwriter.Writer   // Used for printing tabled results
	log                  *logrus.Logger      // Simple stderr logger
	verboseLevel         string              // Logging level
	useAccessToken       bool                // Flag to use OCM access token values for validations
	useRegCreds          bool                // Flag to use OCM registry credentials values for validations
	skipServiceLogs      bool                // Flag to skip service logs
	failuresByServiceLog map[string][]string // Track failures by template
}

const VPSExample string = `
	# Compare OCM Access-Token, OCM Registry-Credentials, and OCM Account Email against cluster's pull secret
	osdctl cluster validate-pull-secret-ext ${CLUSTER_ID} --reason "OSD-XYZ"

	# Exclude Access-Token, and Registry-Credential checks
	osdctl cluster validate-pull-secret-ext ${CLUSTER_ID} --reason "OSD-XYZ" --skip-access-token --skip-registry-creds

	# Skip sending service logs (useful for testing)
	osdctl cluster validate-pull-secret-ext ${CLUSTER_ID} --reason "OSD-XYZ" --skip-service-logs
`

func newCmdValidatePullSecretExt() *cobra.Command {
	ops := newValidatePullSecretExtOptions()
	validatePullSecretCmd := &cobra.Command{
		Use:   "validate-pull-secret-ext [CLUSTER_ID]",
		Short: "Extended checks to confirm pull-secret data is synced with current OCM data",
		Long: `
	Attempts to validate if a cluster's pull-secret auth values are in sync with the account's email,
	registry_credential, and access token data stored in OCM.

	Service logs are automatically sent for detected issues. Multiple failures are aggregated into
	a single service log. Use --skip-service-logs to prevent sending service logs.

	If this is being executed against a cluster which is not owned by the current OCM account,
	Region Lead permissions are required to view and validate the OCM AccessToken.
`,
		Example:           VPSExample,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		PreRun:            func(cmd *cobra.Command, args []string) { cmdutil.CheckErr(ops.preRun(cmd, args)) },
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
	}
	validatePullSecretCmd.Flags().StringVar(&ops.reason, "reason", "", "Mandatory reason for this command to be run (usually includes an OHSS or PD ticket)")
	validatePullSecretCmd.Flags().StringVarP(&ops.verboseLevel, "log-level", "l", "info", "debug, info, warn, error. (default=info)")
	validatePullSecretCmd.Flags().Bool("skip-registry-creds", false, "Exclude OCM Registry Credentials checks against cluster secret")
	validatePullSecretCmd.Flags().Bool("skip-access-token", false, "Exclude OCM AccessToken checks against cluster secret")
	validatePullSecretCmd.Flags().BoolVar(&ops.skipServiceLogs, "skip-service-logs", false, "Skip sending service logs (useful for testing/automation)")

	_ = validatePullSecretCmd.MarkFlagRequired("reason")
	return validatePullSecretCmd
}

func newValidatePullSecretExtOptions() *validatePullSecretExtOptions {
	return &validatePullSecretExtOptions{}
}

func (o *validatePullSecretExtOptions) preRun(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Required 1 positional arg for 'Cluster ID'")
	}
	o.clusterID = args[0]
	o.useAccessToken = true
	o.useRegCreds = true

	// Setup logger
	log := logrus.New()
	log.ReportCaller = true
	level, err := logrus.ParseLevel(o.verboseLevel)
	if err != nil {
		return fmt.Errorf("log level error:'%v", err)
	}
	log.SetLevel(logrus.Level(level))
	log.Formatter = new(logrus.TextFormatter)
	log.Formatter.(*logrus.TextFormatter).DisableLevelTruncation = true
	log.Formatter.(*logrus.TextFormatter).PadLevelText = true
	log.Formatter.(*logrus.TextFormatter).DisableQuote = true
	log.Formatter.(*logrus.TextFormatter).CallerPrettyfier = func(f *runtime.Frame) (string, string) {
		return "", fmt.Sprintf("[%s:%d]", filepath.Base(f.File), f.Line)
	}
	o.log = log

	flags := cmd.Flags()
	noToken, err := flags.GetBool("skip-access-token")
	if err != nil {
		return err
	}
	if noToken {
		o.useAccessToken = false
	}
	noRegCreds, err := flags.GetBool("skip-registry-creds")
	if err != nil {
		return err
	}
	o.useRegCreds = !noRegCreds

	return nil
}

func addResultsTitles(resultsTable *tabwriter.Writer) {
	lines := []string{"----------", "----", "---------", "------", "----", "------"}
	titles := []string{"OCM_SOURCE", "AUTH", "NAMESPACE", "SECRET", "ATTR", "RESULT"}
	fmt.Fprintln(resultsTable, strings.Join(lines, "\t"))
	fmt.Fprintln(resultsTable, strings.Join(titles, "\t"))
	fmt.Fprintln(resultsTable, strings.Join(lines, "\t"))
}

func (o *validatePullSecretExtOptions) run() error {
	var err error
	pullSecret := &corev1.Secret{}
	var regCreds []*v1.RegistryCredential = nil
	var accessToken *v1.AccessToken = nil

	// Initialize failure tracking map
	o.failuresByServiceLog = make(map[string][]string)

	// Create OCM connection...
	o.ocm, err = utils.CreateConnection()
	if err != nil {
		return err
	}
	// Defer closing OCM connection once run() completes...
	defer func() {
		if ocmCloseErr := o.ocm.Close(); ocmCloseErr != nil {
			o.log.Warnf("Cannot close OCM connection (possible memory leak): %q", ocmCloseErr)
		}
	}()

	clusterInfo, err := utils.GetCluster(o.ocm, o.clusterID)
	if err != nil {
		o.log.Errorf("Failed to fetch cluster:'%s' info from OCM (url:'%s')\n", o.clusterID, o.ocm.URL())
		return err
	}
	// Get the internal cluster ID from OCM for comparing to active/current kubecli connection...
	clusterID := clusterInfo.ID()
	// Make sure we're using the internal cluster ID from here on...
	if o.clusterID != clusterID {
		o.log.Infof("Using internal clusterID:'%s' for provided clusterID:'%s'\n", clusterID, o.clusterID)
		o.clusterID = clusterID
	}

	// init results table...
	o.results = tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	addResultsTitles(o.results)

	// Defer printing whatever results are available when run() returns
	defer func() {
		fmt.Printf("\n\n")
		o.results.Flush()
	}()

	// Defer sending aggregated service logs after all validations complete
	defer func() {
		if err := o.sendAggregatedServiceLogs(); err != nil {
			o.log.Errorf("Failed to send aggregated service logs: %v", err)
		}
	}()

	// get account info from OCM
	o.account, err = o.getOCMAccountInfo()
	if err != nil {
		return err
	}
	// account email to be for auth comparison
	emailOCM := o.account.Email()
	o.log.Infof("Found email for cluster's OCM account: %s\n", emailOCM)

	// get the pull secret in cluster
	err = getClusterPullSecret(o.clusterID, o.reason, pullSecret)
	if err != nil {
		return err
	}
	// Validate auth email
	err = o.validateAuthEmail(pullSecret, emailOCM, cloudAuthKey)
	if err != nil {
		fmt.Printf("Error validating pull-secret auth['%s] email.\nErr:'%s'\nWould you like to continue with validations? ", cloudAuthKey, err)
		if !utils.ConfirmPrompt() {
			return err
		}
	}

	// If user chose to use OCM RegistryCredentials in validations...
	if o.useRegCreds {
		// Get a portion of the pull secret from OCM registry_credentials
		// Note: this does not contain the remaining auths from from: '/api/accounts_mgmt/v1/access_token'
		regCreds, err = o.getOCMRegistryCredentials(o.account.ID())
		if err != nil {
			regCreds = nil
			o.addResult("registry_credential", "-", "-", "-", "-", NotRun)
			fmt.Printf("Error fetching registry credentials:%s'.\nWould you like to continue with validations? ", err)
			if !utils.ConfirmPrompt() {
				return err
			}
		}
	}
	if regCreds != nil {
		// Iterate over registry credentials and compare against cluster's pull secret
		err = o.checkRegistryCredsAgainstPullSecret(regCreds, pullSecret, emailOCM)
		if err != nil {
			fmt.Printf("\nError validating registry credentials:%s'.\nWould you like to continue with validations? ", err)
			if !utils.ConfirmPrompt() {
				return err
			}
		}
	}

	// If user chose to use the OCM AccessToken in validations...
	if o.useAccessToken {
		userName := o.account.Username()
		if len(userName) <= 0 {
			o.log.Errorf("found empty 'username' for account:'%s', needed for accessToken", o.account.HREF())
			err = fmt.Errorf("found empty 'username' for account:'%s', needed for accessToken", o.account.HREF())
		} else {
			accessToken, err = o.getAccessTokenFromOCM(userName)
		}
		if err != nil {
			accessToken = nil
			o.addResult("access_token", "-", "-", "-", "-", NotRun)
			o.log.Errorf("getAccessTokenFromOCM() got error:'%v'\n", err)
			fmt.Printf("\nError fetching OCM AccessToken:\n\t%s.\nWould you like to continue with validations? ", err)
			if !utils.ConfirmPrompt() {
				return err
			}
		}
	}

	if accessToken != nil {
		/* Compare OCM stored access token to cluster's pull secret...*/
		o.log.Debugf("Checking OCM AccessToken auth values against secret:%s:%s on cluster...\n", pullSecret.Namespace, pullSecret.Name)
		// Iterate over access token auths and compare against cluster's pull secret
		err = o.checkAccessTokenToPullSecret(accessToken, pullSecret)
		if err != nil {
			fmt.Printf("\nError validating AccessToken:%s'.\nWould you like to continue with validations? ", err)
			if !utils.ConfirmPrompt() {
				return err
			}
		}
	}
	return nil
}

func (o *validatePullSecretExtOptions) validateAuthEmail(pullSecret *corev1.Secret, emailOCM string, authKey string) error {
	// Extract email from cluster pull-secret.
	emailCluster, err := getPullSecretAuthEmail(pullSecret, authKey)
	if err != nil {
		o.log.Errorf("Error fetching pull secret email:'%s'", err)
		var errAENF *ErrorAuthEmailNotFound
		if errors.As(err, &errAENF) {
			o.log.Errorf("Couldn't extract email address from pull secret for: '%s'"+
				"This can mean the pull secret is misconfigured. Please verify the pull secret manually:\n"+
				"	oc get secret -n openshift-config pull-secret -o json | jq -r '.data[\".dockerconfigjson\"]' | base64 -d", errAENF.auth)
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, errAENF.auth)
		}
		if errors.Is(err, ErrSecretMissingDockerConfigJson) {
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, "pull-secret (missing .dockerconfigjson)")
		}
		var errSANF *ErrorSecretAuthNotFound
		if errors.As(err, &errSANF) {
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, errSANF.auth)
		}
		//Todo: Should this prompt for a service log for other errors too (such as fail to unmarshall)?
		return err
	}
	o.log.Debugf("Email from cluster pull-secret auth['%s]: %s\n", authKey, emailCluster)
	// This checks that the 'cloud.openshift.com' auth object stored in the cluster's pull_secret
	// Has the same email as the current account email.
	if emailOCM != emailCluster {
		o.addResult("account.Email", authKey, pullSecret.Namespace, pullSecret.Name, "email", Fail)
		err = fmt.Errorf("pull-secret auth:'%s', email:'%s' doesn't match user email from OCM:'%s'", cloudAuthKey, emailCluster, emailOCM)
		o.log.Errorf("%s\n", err)
		o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, authKey)
		return err
	}
	o.addResult("account.Email", authKey, pullSecret.Namespace, pullSecret.Name, "email", Pass)
	o.log.Debugf("Cluster pull_secret.auth['%s'].email matches OCM account. PASSED\n", authKey)
	return nil
}

func (o *validatePullSecretExtOptions) addResult(ocmSource string, auth string, psNamespace string, psName string, attr string, result Result) {
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
	resStr := []string{ocmSource, auth, psNamespace, psName, attr, resultStr}
	fmt.Fprintln(o.results, strings.Join(resStr, "\t"))
}

// There is likely more auth sections in the pull secret on cluster than in the OCM accessToken.
// Iterate over Access Token auth sections and confirm these values match on the cluster...
func (o *validatePullSecretExtOptions) checkAccessTokenToPullSecret(accessToken *v1.AccessToken, pullSecret *corev1.Secret) error {
	var hasErrors bool = false
	for akey, auth := range accessToken.Auths() {
		o.log.Debugf("\nChecking OCM AccessToken values against secret:'%s':'%s'...\n", pullSecret.Namespace, pullSecret.Name)
		// Find the matching auth entry for this auth name in the cluster pull_secret data...
		psTokenAuth, err := getPullSecretTokenAuth(akey, pullSecret)
		if err != nil {
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "auth", Fail)
			o.log.Errorf("OCM accessToken.auth['%s'], failed to fetch this auth from cluster pull-secret, err:'%s'", akey, err)
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, akey)
			hasErrors = true
			// no matching auth present containing email + token
			continue
		}

		if auth.Auth() != psTokenAuth.Auth() {
			// Record token mismatch
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "token", Fail)
			o.log.Errorf("OCM accessToken.auth['%s'] does not match token found in cluster pull-secret ", akey)
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, akey)
			hasErrors = true
		} else {
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "token", Pass)
			o.log.Debugf("OCM accessToken.auth['%s']. OCM and cluster tokens match. PASS\n", akey)
		}

		if auth.Email() != psTokenAuth.Email() {
			// Record email mismatch
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "email", Fail)
			o.log.Errorf("auth['%s'], pull-secret email:'%s' does not match OCM accessToken.email:'%s'", akey, psTokenAuth.Email(), auth.Email())
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, akey)
			hasErrors = true
		} else {
			o.addResult("access_token", akey, pullSecret.Namespace, pullSecret.Name, "email", Pass)
			o.log.Debugf("OCM accessToken.auth['%s']. Email matches OCM account. PASS\n", akey)
		}
	}
	if hasErrors {
		return fmt.Errorf("OCM AccessToken auths did not match on cluster pull-secret. See logged output for more info")
	}
	o.log.Debugf("-------------------------------------------\n")
	o.log.Debugf("Access Token checks PASSED\n")
	o.log.Debugf("-------------------------------------------\n")
	return nil
}

// Check the registry_credentials against each of the corresponding cluster pull_secret auth sections...
// There is likely more auth sections in the pull secret on cluster than in the OCM registry_credentials.
func (o *validatePullSecretExtOptions) checkRegistryCredsAgainstPullSecret(regCreds []*v1.RegistryCredential, pullSecret *corev1.Secret, emailOCM string) error {
	o.log.Debugf("Checking OCM registry_credential values against secret:'%s':'%s'...", pullSecret.Namespace, pullSecret.Name)
	var hasErrors bool = false
	for _, regCred := range regCreds {
		var regErr bool = false // store error value for indiv reg cred iteration
		setErr := func() {
			regErr = true
			hasErrors = true
		}
		o.log.Debugf("OCM registry_credential:'%s'\n", regCred.HREF())
		// registry_credential.token is stored plain text in OCM, no need to decode here...
		token := regCred.Token()
		username := regCred.Username()
		if len(token) <= 0 {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			o.log.Errorf("empty token for OCM registry_credential. See:'ocm get %s'", regCred.HREF())
			setErr()
		}
		if len(username) <= 0 {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			o.log.Errorf("empty username for registry_credential. See:'ocm get %s'", regCred.HREF())
			setErr()
		}
		if regErr {
			continue
		}
		// Auth token in cluster's pull-secret data uses format: "username + ':' + token"
		regToken := fmt.Sprintf("%s:%s", username, token)
		//Get the exact registry name from the registry_credentials registry.id ...
		registryID := regCred.Registry().ID()
		registry, err := o.getRegistryFromOCM(registryID)
		if err != nil {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "registry", Fail)
			o.log.Errorf("Failed to fetch registry:'%s' from OCM. Err:'%s'\n", registryID, err)
			setErr()
			continue
		}
		//The registry name is used to find the correct section in the cluster pull-secret data.
		regName := registry.Name()
		if len(regName) <= 0 {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "registry", Fail)
			o.log.Errorf("empty name for registry_credential. See:'ocm get %s'", registry.HREF())
			setErr()
			continue
		}
		// Find the matching auth entry for this registry name in the cluster pull-secret data...
		secTokenAuth, err := getPullSecretTokenAuth(regName, pullSecret)
		if err != nil {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "auth", Fail)
			o.log.Errorf("OCM registry_credential['%s'] failed to fetch auth section from cluster pull secret, err:'%s'", regName, err)
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, regName)
			setErr()
			continue
		}
		// Check all auth sections matching registries found in the registry_credentials for matching emails...
		secEmail := secTokenAuth.Email()
		if emailOCM != secEmail {
			o.addResult("account.Email", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "email", Fail)
			o.log.Errorf("pull-secret auth['%s'].email:'%s' does not match OCM account.Email:'%s'.", regName, secEmail, emailOCM)
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, regName)
			setErr() // set the error but continue to check the token portion...
		} else {
			o.addResult("account.Email", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "email", Pass)
			o.log.Debugf("OCM registry_credential['%s']. OCM and cluster emails match. PASS\n", regName)
		}
		// Get the token from the cluster pull_secret...
		secToken := secTokenAuth.Auth()
		if len(secToken) <= 0 {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			o.log.Errorf("empty token found in cluster pull-secret for auth section:'%s', err:'%s'", regName, err)
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, regName)
			setErr()
			continue
		}
		// This token is stored base64 encoded with a prefix added...
		secTokDecoded, err := b64.StdEncoding.DecodeString(secToken)
		if err != nil {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			o.log.Errorf("error decoding token in cluster pull-secret for auth section:'%s', err:'%s'", regName, err)
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, regName)
			setErr()
			continue
		}
		//Compare OCM registry_credential token to cluster-config/pull_secret token...
		if regToken != string(secTokDecoded) {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Fail)
			o.log.Errorf("OCM registry_credential['%s'] token did NOT match value found in cluster pull_secret!\n"+
				"May need to sync ocm credentials to cluster pull secret.\n", regName)
			o.recordServiceLogFailure(ServiceLogMultipleSyncFailures, regName)
			setErr()
			continue
		} else {
			o.addResult("registry_credential", regCred.Registry().ID(), pullSecret.Namespace, pullSecret.Name, "token", Pass)
			o.log.Debugf("OCM registry_credential['%s']. OCM and cluster tokens match. PASS\n", regName)
		}
	}
	if hasErrors {
		return fmt.Errorf("OCM registryCredential values did not match values found in pull-secret on cluster. See logged output for more info ")
	}
	o.log.Debugf("-------------------------------------------\n")
	o.log.Debugf("registry_credentials checks PASSED\n")
	o.log.Debugf("-------------------------------------------\n")
	return nil
}

// Custom error types to help with unit tests...
var ErrSecretMissingDockerConfigJson = errors.New("secret missing '.dockerconfigjson'? See servicelog: https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/pull_secret_change_breaking_upgradesync.json")

type ErrorParseSecret struct {
	err error
}

func (e *ErrorParseSecret) Error() string {
	return fmt.Sprintf("failed to parse secret, err: %v", e.err)
}

type ErrorSecretAuthNotFound struct {
	auth string
}

func (e *ErrorSecretAuthNotFound) Error() string {
	return fmt.Sprintf("error: auth '%s' not found in secret.auths", e.auth)
}

type ErrorAuthEmailNotFound struct {
	auth string
}

func (e *ErrorAuthEmailNotFound) Error() string {
	return fmt.Sprintf("error, empty email for auth '%s'", e.auth)
}

// Parse out AccessTokenAuth for provided auth ID/key from provided (pull) secret.
func getPullSecretTokenAuth(authID string, secret *corev1.Secret) (*v1.AccessTokenAuth, error) {
	if len(authID) <= 0 {
		return nil, fmt.Errorf("error: provided an empty auth ID to getPullSecretTokenAuth()")
	}
	dockerConfigJsonBytes, found := secret.Data[".dockerconfigjson"]
	if !found {
		return nil, ErrSecretMissingDockerConfigJson
	}
	dockerConfigJson, err := v1.UnmarshalAccessToken(dockerConfigJsonBytes)
	if err != nil {
		return nil, &ErrorParseSecret{err: err}
	}
	secretAuth, found := dockerConfigJson.Auths()[authID]
	if !found {
		return nil, &ErrorSecretAuthNotFound{auth: authID}
	}
	return secretAuth, nil
}

// getPullSecret gets the pull-secret in the cluster
// with backplane elevation.
func getClusterPullSecret(clusterID string, reason string, secret *corev1.Secret) (err error) {
	kubeClient, err := k8s.NewAsBackplaneClusterAdmin(clusterID, client.Options{}, reason)
	if err != nil {
		return fmt.Errorf("failed to login to cluster as 'backplane-cluster-admin': %w", err)
	}
	if err := kubeClient.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret); err != nil {
		return err
	}
	return nil
}

func (o *validatePullSecretExtOptions) getCurrentOCMUserInfo() (*v1.Account, error) {
	// Fetch OCM current_user info...
	currentAccountResp, err := o.ocm.AccountsMgmt().V1().CurrentAccount().Get().Send()
	if err != nil {
		return nil, err
	}
	currentAccount := currentAccountResp.Body()
	return currentAccount, nil
}

func (o *validatePullSecretExtOptions) getAccessTokenFromOCM(impersonateUser string) (*v1.AccessToken, error) {
	o.log.Debugf("Attempting to get accessToken for user:'%s'\n", impersonateUser)
	var err error
	var tokenResp *v1.AccessTokenPostResponse
	if len(impersonateUser) <= 0 {
		return nil, fmt.Errorf("err, getAccessTokenFromOCM() provided empty user string")
	}
	currentUserInfo, err := o.getCurrentOCMUserInfo()
	if err != nil {
		// log this error, and attempt token request using impersonate regardless
		o.log.Errorf("Error fetching OCM user info for current osdctl user? err:'%v", err)
	}
	if err != nil || currentUserInfo.Username() != impersonateUser {
		// Impersonate requires elevated (region-lead) permissions.
		tokenResp, err = o.ocm.AccountsMgmt().V1().AccessToken().Post().Impersonate(impersonateUser).Send()
	} else {
		// No need to impersonate, this is the current user's own account.
		// This will allow some level of testing to be performed when acting on one's own account/clusters.
		o.log.Debugf("Impersonate not needed, this account is owned by current OCM user:'%s'\n", currentUserInfo.Username())
		tokenResp, err = o.ocm.AccountsMgmt().V1().AccessToken().Post().Send()
	}

	// Check error to see if user should be informed of Region Lead requirements...
	if err != nil {
		if tokenResp != nil {
			if tokenResp.Status() == 403 {
				o.log.Errorf("%v\n", err)
				o.log.Errorf(
					"AccessToken ops may require 'region lead' permissions to execute.\n"+
						"See CLI equiv: ocm post --body=/dev/null --header=\"Impersonate-User=%s\" /api/accounts_mgmt/v1/access_token\n", impersonateUser)
				err = fmt.Errorf("%v. AccessToken ops may require 'region lead' permissions to execute", err)
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
func (o *validatePullSecretExtOptions) getRegistryFromOCM(registryID string) (*v1.Registry, error) {
	o.log.Debugf("Getting registry for registryID:'%s'\n", registryID)
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
func (o *validatePullSecretExtOptions) getOCMAccountInfo() (*v1.Account, error) {
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
func (o *validatePullSecretExtOptions) getOCMRegistryCredentials(accountID string) ([]*v1.RegistryCredential, error) {
	o.log.Debugf("Getting registry_credentials from OCM\n")
	if len(accountID) <= 0 {
		return nil, fmt.Errorf("getPullSecretFromOCM() provided empty accountID")
	}

	registryCredentials, err := utils.GetRegistryCredentials(o.ocm, accountID)
	if err != nil {
		return nil, err
	}
	// validate the registryCredentials before return
	if len(registryCredentials) <= 0 {
		err := fmt.Errorf("registryCredentials not found for Account:'%s' in OCM", accountID)
		o.log.Errorf("%s\nSee: /api/accounts_mgmt/v1/registry_credentials -p search=\"account_id='%s'\"", err, accountID)
		postCmd := servicelog.PostCmdOptions{
			Template:       ServiceLogUpdatePullSecret,
			TemplateParams: []string{"REGISTRY=registry.redhat.io"},
			ClusterId:      o.clusterID,
		}
		sendServiceLog(postCmd, fmt.Sprintf("%s\n", err))
		return nil, err
	}
	return registryCredentials, nil
}

// Provide information, and prompt user to send a service log.
func sendServiceLog(postCmd servicelog.PostCmdOptions, message string) error {
	var err error = nil
	if len(postCmd.ClusterId) <= 0 {
		fmt.Fprintf(os.Stderr, "Empty clusterID provided to sendServiceLog()\n")
		return fmt.Errorf("empty clusterID provided to sendServiceLog function")
	}
	if len(postCmd.Template) <= 0 {
		fmt.Fprintf(os.Stderr, "Empty template url provided to sendServiceLog()\n")
		return fmt.Errorf("empty template URL provided to sendServiceLog function")
	}
	// Print provided message then prompt user whether or not to send a service log.
	if len(message) > 0 {
		fmt.Printf("%s\n", message)
	}
	fmt.Printf("Would you like to send a service log now using the following options: '%v'?", postCmd)
	if utils.ConfirmPrompt() {
		err = postCmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error incurred sending service log:'%s'\n", err)
			return err
		}
	}
	return err
}

// buildTemplateParameters creates the template parameter array for service log
func buildTemplateParameters(failures []string) []string {
	failureList := strings.Join(failures, ", ")
	return []string{fmt.Sprintf("FAILURE_LIST=%s", failureList)}
}

// formatFailureDisplay formats the visual display of failures for user output
func formatFailureDisplay(category string, failures []string) string {
	var output strings.Builder
	output.WriteString(fmt.Sprintf("\nPull Secret Validation Failures: %s\n", category))
	output.WriteString(fmt.Sprintf("Found %d failure(s):\n\n", len(failures)))

	for i, failure := range failures {
		output.WriteString(fmt.Sprintf("  %d. %s\n", i+1, failure))
	}
	output.WriteString("\n")

	return output.String()
}

// recordServiceLogFailure collects failures to be aggregated and sent at the end
func (o *validatePullSecretExtOptions) recordServiceLogFailure(template string, authSource string) {
	if o.skipServiceLogs {
		return // Don't collect if we're skipping service logs
	}

	if o.failuresByServiceLog == nil {
		o.failuresByServiceLog = make(map[string][]string)
	}

	// Add authSource to the list for this template
	o.failuresByServiceLog[template] = append(o.failuresByServiceLog[template], authSource)
	o.log.Debugf("Recorded service log failure for template %s: %s", template, authSource)
}

// sendAggregatedServiceLogs sends collected service logs after all validations complete
func (o *validatePullSecretExtOptions) sendAggregatedServiceLogs() error {
	if o.skipServiceLogs {
		o.log.Infof("Skipping service logs (--skip-service-logs flag set)")
		return nil
	}

	// Get all failures (we only use one template now)
	allFailures := o.failuresByServiceLog[ServiceLogMultipleSyncFailures]
	if len(allFailures) == 0 {
		o.log.Infof("No validation failures requiring service logs")
		return nil
	}

	// Display failures to user
	display := formatFailureDisplay("Pull Secret Issues", allFailures)
	fmt.Print(display)

	// Build template parameters
	templateParams := buildTemplateParameters(allFailures)

	// Use servicelog package's built-in prompting and validation
	postCmd := servicelog.PostCmdOptions{
		Template:       ServiceLogMultipleSyncFailures,
		TemplateParams: templateParams,
		ClusterId:      o.clusterID,
	}

	if err := postCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error sending service log: %s\n", err)
		return err
	}

	o.log.Infof("Service log sent successfully")
	return nil
}

// getPullSecretAuthEmail extract the email for a specific auth from the provided secret
func getPullSecretAuthEmail(secret *corev1.Secret, authKey string) (string, error) {
	dockerConfigJsonBytes, found := secret.Data[".dockerconfigjson"]
	if !found {
		return "", ErrSecretMissingDockerConfigJson
	}

	dockerConfigJson, err := v1.UnmarshalAccessToken(dockerConfigJsonBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal pull-secret dockerconfigjson\n")
		return "", err
	}

	auth, found := dockerConfigJson.Auths()[authKey]
	if !found {
		return "", &ErrorSecretAuthNotFound{authKey}
	}

	clusterPullSecretEmail := auth.Email()
	if clusterPullSecretEmail == "" {
		return "", &ErrorAuthEmailNotFound{auth: authKey}
	}
	return clusterPullSecretEmail, nil
}
