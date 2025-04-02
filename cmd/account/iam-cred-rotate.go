package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	stsTypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/backplane-cli/pkg/backplaneapi"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
	bpocmcli "github.com/openshift/backplane-cli/pkg/ocm"
	"github.com/spf13/viper"
	"k8s.io/client-go/rest"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	credreqv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8json "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8slog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const cmdName string = "iam-secret-mgmt"

func newCmdRotateAWSCreds(streams genericclioptions.IOStreams) *cobra.Command {
	cmdPrefix := fmt.Sprintf(`%s $CLUSTER_ID --reason "$JIRA_TICKET" --aws-profile rhcontrol `, cmdName)
	examples := fmt.Sprintf(`
Basic usage involves selecting a set actions to run:
Choose 1 or more IAM users to rotate: --rotate-managed-admin, --rotate-ccs-admin
-or- 
Choose 1 or more describe actions: --describe-keys, --describe-secrets
----------------------------------------------------------------------

# Rotate credentials for IAM user "OsdManagedAdmin" (or user provided by --admin-username)
%s --rotate-managed-admin

# Rotate credentials for special IAM user "OsdCcsAdmin", verbose log level 4. 
%s --rotate-ccs-admin --verbose 4

# Rotate credentials for both users "OsdManagedAdmin" and then "OsdCcsAdmin"
%s --rotate-managed-admin --rotate-ccs-admin

# Describe credential-request secrets 
%s --describe-secrets -o yaml

# Describe AWS Access keys in use by users "OsdManagedAdmin" and "OsdCcsAdmin"
%s --describe-keys -o json | jq

# Non 'production' environments: 
# May require --ocm-config-hive to separate OCM config used
# to create backplane connections with Hive which differs from the OCM config needed to
# for the target cluster.
# Example, Hive resides in 'prod' and will use a config "config.prod.json",  while target 
cluster resides in 'staging' and will use the env var OCM_CONFIG to reference "config.staging.json":
> export OCM_CONFIG="$HOME/.config/ocm/config.staging.json
> %s $CLUSTER_ID --reason "$JIRA_TICKET" --aws-profile osd-staging-1 --ocm-config-hive ~/.config/ocm/config.prod.json 
# Note: when using --ocm-config-hive the target cluter will use the default environment 
# variables, and the hive connection will only reference config + tokens in the provided
# config file. 


# The --aws-profile will likely change to match the non-production env:
# example profile names typically used: rhcontrol for prod, osd-staging-* for staging, etc..`,
		cmdPrefix, cmdPrefix, cmdPrefix, cmdPrefix, cmdPrefix, cmdName)

	ops := newRotateAWSOptions(streams)
	rotateAWSCredsCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <clusterId> --reason <reason> <options>", cmdName),
		Short: "Rotate IAM credentials and secrets",
		Long: `
Util to rotate managed IAM credentials and related secrets for a given cluster account.
Please use with caution!
'Rotate': operations are intended to be somewhat interactive and will provide information 
followed by prompting the user to verify if and/or how to proceed. 
'Describe': operations 
are intended to provide info and status related to the artifacts to be rotated. 

These operations require the following:
	- A valid cluster-id (not an alias)
	- An elevation reason (hint: This will usually involve a Jira ID.)  
	- A local AWS profile for the cluster environment (ie: osd-staging, rhcontrol, etc), if not provided 'default' is used. 
	- A valid OCM config and authenticated connection.  
`,
		Example:           examples,
		DisableAutoGenTag: true,
		Args:              cobra.MatchAll(cobra.ExactArgs(1)),
		PreRun: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.preRunCliChecks(cmd, args))
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.preRunSetup())
			cmdutil.CheckErr(ops.run())
		},
	}

	rotateAWSCredsCmd.Flags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile from local config to use(default='default')")
	rotateAWSCredsCmd.Flags().StringVarP(&ops.reason, "reason", "r", "", "(Required) The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket).")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.updateMgmtCredsCli, "rotate-managed-admin", false, "Rotate osdManagedAdmin user credentials, and CR secrets. Interactive. Use caution.")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.updateCcsCredsCli, "rotate-ccs-admin", false, "Rotate osdCcsAdmin user credentials, and CR secrets. Interactive. Use caution!")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.rotateCRSecretsOnly, "rotate-cr-secrets", false, "Delete Openshift secrets related to AWSProvider Credential Requests w/o needing to rotate IAM keys")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.describeKeysCli, "describe-keys", false, "Print AWS AccessKey info for osdManagedAdmin and osdCcsAdmin relevant cred rotation, and exit")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.describeSecretsCli, "describe-secrets", false, "Print AWS CredentialRequests ref'd secrets info relevant to cred rotation, and exit")
	rotateAWSCredsCmd.Flags().StringVar(&ops.osdManagedAdminUsername, "admin-username", "", "The admin username to use for generating access keys. Must be in the format of `osdManagedAdmin*`. If not specified, this is inferred from the account CR.")
	rotateAWSCredsCmd.Flags().StringVarP(&ops.output, "output", "o", "", "Describe CMD valid formats are ['', 'json', 'yaml']")
	rotateAWSCredsCmd.Flags().StringVar(&ops.ocmConfigHivePath, "ocm-config-hive", "", "OCM config file path used to access Hive")
	rotateAWSCredsCmd.Flags().IntVarP(&ops.verboseLevel, "verbose", "v", 3, "debug=4, (default)info=3, warn=2, error=1")
	// Reason is required for elevated backplane-admin impersonated requests
	_ = rotateAWSCredsCmd.MarkFlagRequired("reason")
	rotateAWSCredsCmd.MarkFlagsOneRequired("describe-keys", "describe-secrets", "rotate-managed-admin", "rotate-ccs-admin", "rotate-cr-secrets")
	// Dont allow mixing describe and rotate options...
	rotateAWSCredsCmd.MarkFlagsMutuallyExclusive("describe-keys", "rotate-managed-admin")
	rotateAWSCredsCmd.MarkFlagsMutuallyExclusive("describe-keys", "rotate-ccs-admin")
	rotateAWSCredsCmd.MarkFlagsMutuallyExclusive("describe-keys", "rotate-cr-secrets")
	rotateAWSCredsCmd.MarkFlagsMutuallyExclusive("describe-secrets", "rotate-managed-admin")
	rotateAWSCredsCmd.MarkFlagsMutuallyExclusive("describe-secrets", "rotate-ccs-admin")
	rotateAWSCredsCmd.MarkFlagsMutuallyExclusive("describe-secrets", "rotate-cr-secrets")

	return rotateAWSCredsCmd
}

// rotateSecretOptions defines the struct for running rotate-iam command
type rotateCredOptions struct {
	// CLI provided params
	profile                 string // Local AWS profile used to run this script
	reason                  string // Reason used to justify elevate/impersonate ops
	updateCcsCredsCli       bool   // Bool flag to indicate whether or not to update special AWS user 'osdCcsAdmin' creds.
	updateMgmtCredsCli      bool   // Bool flag to indicate whether or not to update AWS user 'osdManagedAdmin' creds.
	rotateCRSecretsOnly     bool   // Bool flag to delete secrets related to AWSProvider Credential Requests w/o rotating IAM keys
	osdManagedAdminUsername string // Name of AWS Managed Admin user. Legacy default values are used if not provided.
	describeSecretsCli      bool   // Print Cred requests ref'd AWS secrets info and exit
	describeKeysCli         bool   // Print Access Key info and exit
	output                  string // Format used to printing describe cmd output (json, yaml, or "")
	clusterID               string // Cluster id, user cluster used by account/creds
	awsAccountTimeout       *int32 // Default timeout
	ocmConfigHivePath       string // Optional OCM config path to use for Hive connections

	// Runtime attrs
	verboseLevel int             //Logging level
	log          *logrus.Logger  // Used for logging runtime messages
	ctx          context.Context // Context to user for rotation ops
	genericclioptions.IOStreams

	// AWS runtime attrs
	account              *awsv1alpha1.Account // aws-account-operator account obj
	accountIDSuffixLabel string               // account suffix label
	accountID            string               // AWS account id
	claimName            string               // AWS account claim name custom resource
	secretName           string               // account AWS creds secret
	awsClient            awsprovider.Client   //AWS client used for final access and cred rotation

	// Openshift runtime attrs
	cluster           *cmv1.Cluster         // Cluster object representing user cluster of 'clusterID'
	clusterKubeClient client.Client         // Cluster BP kube client connection
	clusterClientset  *kubernetes.Clientset // Cluster BP Kube ClientSet
	isCCS             bool                  // Flag to indicate cluster is CCS
	hiveCluster       *cmv1.Cluster         // Hive cluster/shard managing this user cluster
	hiveKubeClient    client.Client         // Hive kube client connection
	cdName            string                // Cluster Deployments name

}

const (
	jsonFormat     = "json"
	yamlFormat     = "yaml"
	standardFormat = ""
	awsSyncSetName = "aws-sync"
)

// func newRotateAWSOptions(streams genericclioptions.IOStreams, client *k8s.LazyClient) *rotateCredOptions {
func newRotateAWSOptions(streams genericclioptions.IOStreams) *rotateCredOptions {
	return &rotateCredOptions{
		IOStreams: streams,
	}
}

/* Initial function used to validate user input */
func (o *rotateCredOptions) preRunCliChecks(cmd *cobra.Command, args []string) error {
	var err error = nil
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Required 1 positional arg for 'Cluster ID'")
	}
	o.clusterID = args[0]

	// The aws account timeout. The min the API supports is 15mins (900seconds)
	o.awsAccountTimeout = awsSdk.Int32(900)

	if o.osdManagedAdminUsername != "" && !strings.HasPrefix(o.osdManagedAdminUsername, common.OSDManagedAdminIAM) {
		return cmdutil.UsageErrorf(cmd, "admin-username must start with %s", common.OSDManagedAdminIAM)
	}

	if len(o.ocmConfigHivePath) > 0 {
		if _, err := os.Stat(o.ocmConfigHivePath); err != nil {
			return fmt.Errorf("config file:'%s', error: %v", o.ocmConfigHivePath, err)
		}
	}

	if o.profile == "" {
		o.profile = "default"
	}
	// Setup logger
	logger := logrus.New()
	logger.ReportCaller = true
	if o.verboseLevel > int(logrus.DebugLevel) {
		o.verboseLevel = int(logrus.DebugLevel)
	}
	logger.SetLevel(logrus.Level(o.verboseLevel))
	logger.Formatter = new(logrus.TextFormatter)
	logger.Formatter.(*logrus.TextFormatter).DisableLevelTruncation = true
	logger.Formatter.(*logrus.TextFormatter).PadLevelText = true
	logger.Formatter.(*logrus.TextFormatter).DisableQuote = true
	logger.Formatter.(*logrus.TextFormatter).CallerPrettyfier = func(f *runtime.Frame) (string, string) {
		return "", fmt.Sprintf("[%s:%d]", filepath.Base(f.File), f.Line)
	}
	o.log = logger

	// init context
	o.ctx = context.TODO()

	// This is also caught by the Cobra flags, but Cobra's error message is a bit cryptic so catching here first...
	if !o.updateCcsCredsCli && !o.updateMgmtCredsCli && !o.rotateCRSecretsOnly && !o.describeKeysCli && !o.describeSecretsCli {
		return cmdutil.UsageErrorf(cmd, "must provide one or more actions: ('--rotate-managed-admin', and/or '--rotate-ccs-admin', or '--rotate-cr-secrets), or '--describe-secrets', or '--describe-keys'")
	}
	if (o.updateCcsCredsCli || o.updateMgmtCredsCli || o.rotateCRSecretsOnly) && (o.describeKeysCli || o.describeSecretsCli) {
		return cmdutil.UsageErrorf(cmd, "can not combine 'describe*' with 'rotate*' commands")
	}

	// At this time json/yaml output is only relevant to describe commands
	if !(o.describeKeysCli || o.describeSecretsCli) {
		o.output = standardFormat
	}

	o.output = strings.ToLower(o.output)
	switch o.output {
	case jsonFormat, yamlFormat, standardFormat:
		break
	default:
		return fmt.Errorf("unsupported output type provided:'%s", o.output)
	}

	// Fail early if aws config is not correct
	_, err = config.LoadDefaultConfig(o.ctx, config.WithSharedConfigProfile(o.profile))
	if err != nil {
		o.log.Errorf("Failed to load AWS config:'%v'\n", err)
		return err
	}
	// To avoid warnings/backtrace, if k8s controller-runtime logger is not yet set, do it now...
	if !k8slog.Log.Enabled() {
		k8slog.SetLogger(zap.New(zap.WriteTo(io.Discard)))
	}
	return nil
}

/* Main function used to run this CLI utility */
func (o *rotateCredOptions) run() error {
	var err error = nil
	if o.log.GetLevel() >= logrus.DebugLevel && (o.describeKeysCli || o.describeSecretsCli) {
		err = o.printClusterInfo()
	}
	if err != nil {
		return err
	}

	if o.describeSecretsCli {
		// Make sure we have a backplane connection to the target cluster...
		if o.clusterKubeClient == nil {
			err := o.connectClusterClient()
			if err != nil {
				o.log.Errorf("Error connecting to cluster:'%v'", err)
				return err
			}
		}
		// Print info for secrets referenced by AWS provider credentialRequests then exit...
		err = o.printAWSCredRequestSecrets(nil)
		if err != nil {
			return err
		}
		if !o.describeKeysCli {
			return nil
		}
	}

	// Setup AWS/Hive related artifacts if requested op requires them...
	if o.updateMgmtCredsCli || o.updateCcsCredsCli || o.describeKeysCli {
		err = o.connectHiveClient()
		if err != nil {
			return err
		}
		err = o.fetchAwsClaimNameInfo()
		if err != nil {
			return err
		}

		err = o.fetchAWSAccountInfo()
		if err != nil {
			return err
		}
		err = o.setupAwsClient()
		if err != nil {
			return err
		}
	}

	if o.describeKeysCli {
		// Print AWS key info then exit
		err = o.cliPrintOsdManagedAdminCreds()
		if err != nil {
			return err
		}
		err = o.cliPrintOsdCcsAdminCreds()
		if err != nil {
			return err
		}
		// Return here to avoid mixing rotate and describe commands
		return nil
	}

	// Begin rotate specific operations...
	if !o.updateMgmtCredsCli && !o.updateCcsCredsCli && !o.rotateCRSecretsOnly {
		// user did not select a rotate operation, nothing more to do here.
		return nil
	}

	// Display cluster info, let user confirm they are on the correct cluster, etc...
	o.printClusterInfo()
	fmt.Printf("Confirm cluster info above is correct. Proceed?\n")
	if !utils.ConfirmPrompt() {
		fmt.Println("User quit.")
		return nil
	}

	// Rotate credentials for IAM user OsdManagedAdmin
	if o.updateMgmtCredsCli {
		err = o.doRotateManagedAdminAWSCreds()
		if err != nil {
			return err
		}
	}
	// Rotate credentials for IAM user OsdCcsAdmin
	if o.updateCcsCredsCli {
		o.log.Infof("ccs cli flag was set. Attempting to update osdCcsAdmin user creds...\n")
		err = o.doRotateCcsCreds()
		if err != nil {
			return err
		}
	}
	err = o.deleteAWSCredRequestSecrets()
	if err != nil {
		o.log.Warnf("fetchCredentialsRequests returned err:'%s'\n", err)
		return err
	}
	fmt.Printf("\nOptions --describe-keys, --describe-secrets can be used to provide additional info/status\n")
	fmt.Printf("Script Run Completed Successfully.\n")
	return nil
}

func (o *rotateCredOptions) printJsonYaml(obj any) error {
	if o.output == jsonFormat {
		pbuf, err := json.MarshalIndent(obj, "", "    ")
		if err != nil {
			o.log.Errorf("Failed to marshal json. Err:'%v'\n", err)
			return err
		}
		fmt.Printf("%s\n", string(pbuf))
		return nil
	}
	if o.output == yamlFormat {
		pbuf, err := yaml.Marshal(obj)
		if err != nil {
			o.log.Errorf("Failed to marshal yaml. Err:'%v'\n", err)
			return err
		}
		fmt.Printf("%s\n", string(pbuf))
		return nil
	}
	return fmt.Errorf("printJsonYaml function called while format set to:'%s'", o.output)
}

// Some very basic 'early' validation on the key input if provided.
// AWS docs, access key constraints: Minimum length of 16. Maximum length of 128.
func (o *rotateCredOptions) isValidAccessKeyId(keyId string) bool {
	if len(keyId) < 16 || len(keyId) > 128 {
		o.log.Errorf("Invalid Access key length:%d\n", len(keyId))
		return false
	}
	return true
}

func (o *rotateCredOptions) printClusterInfo() error {
	output := os.Stdout
	if o.output != standardFormat {
		output = os.Stderr
	}
	w := tabwriter.NewWriter(output, 1, 1, 1, ' ', 0)
	fmt.Fprintf(w, "\n----------------------------------------------------------------------\n")
	fmt.Fprintf(w, "Cluster ID:\t%s\n", o.clusterID)
	fmt.Fprintf(w, "Cluster External ID:\t%s\n", o.cluster.ExternalID())
	fmt.Fprintf(w, "Cluster Name:\t%s\n", o.cluster.Name())
	fmt.Fprintf(w, "Cluster Is CCS:\t%t\n", o.isCCS)
	fmt.Fprintf(w, "----------------------------------------------------------------------\n")
	w.Flush()
	return nil
}

// Get list of credentialsRequests with openshift namespaces for AWS provider type.
// The secrets referenced by these credReqs will need to be updated after cred rotation.
func (o *rotateCredOptions) getAWSCredentialsRequests(nameSpace string) ([]credreqv1.CredentialsRequest, error) {
	// Make sure context and cluster connections have been established first
	if o.ctx == nil {
		return nil, fmt.Errorf("invalid ctx, nil")
	}
	// Make sure we have a backplane connection to the target cluster...
	if o.clusterKubeClient == nil {
		err := o.connectClusterClient()
		if err != nil {
			return nil, err
		}
	}

	const AWSProviderSpecType string = "AWSProviderSpec"
	delCredReqList := []credreqv1.CredentialsRequest{}
	var credRequestList credreqv1.CredentialsRequestList
	var listOptions = client.ListOptions{}
	//NOTE:
	// Original aws-creds-rotate.sh script uses -A to query all namespaces.
	// CCO manages CRs outside the 'openshift-cloud-credential-operator' namespace
	// so for OSD/ROSA gathering CRs from all namespaces may be needed at this time.
	// Could possibly cross check with managed fields to confirm the CR is managed by
	// CCO.
	if len(nameSpace) > 0 {
		// ie: listOptions.Namespace = credreqv1.CloudCredOperatorNamespace
		listOptions.Namespace = nameSpace
	}
	err := o.clusterKubeClient.List(o.ctx, &credRequestList, &listOptions)
	if err != nil {
		o.log.Warnf("Error fetching CredentialsRequestList:'%s'\n", err)
		return nil, err
	}

	for _, cr := range credRequestList.Items {
		if strings.HasPrefix(cr.Namespace, "openshift") {
			kindStr, err := getProviderSpecKind(cr)
			if err != nil {
				o.log.Warnf("Skipping cr:'%s', err:'%s'\n", cr.Name, err)
				continue
			}
			if kindStr == AWSProviderSpecType {
				delCredReqList = append(delCredReqList, cr)
			}
		}
	}
	return delCredReqList, nil
}

// Currently 'ProviderSpec' needs to be unmarshal'd/parsed to fetch 'kind' attr(?)
func getProviderSpecKind(cr credreqv1.CredentialsRequest) (string, error) {
	var specJson map[string]interface{}
	if cr.Spec.ProviderSpec.Raw != nil {
		err := json.Unmarshal(cr.Spec.ProviderSpec.Raw, &specJson)
		if err != nil {
			return "", fmt.Errorf("unmarshal credRequest err:'%s'", err)
		}
	} else {
		return "", fmt.Errorf("credRequest '%s' cr.Spec.ProviderSpec.Raw is nil", cr.Name)
	}
	specKind, ok := specJson["kind"]
	if ok {
		kindStr, ok := specKind.(string)
		if !ok {
			return "", fmt.Errorf("cred request:'%s', ProviderSpec.kind attr is not a string", cr.Name)
		} else {
			return kindStr, nil
		}
	} else {
		return "", fmt.Errorf("cred request:'%s', no 'kind' attr in ProviderSpec", cr.Name)
	}
}

// Struct for displaying CR secret info
type credReqSecrets struct {
	SecretName                 string                       `json:"SecretName"`
	SecretNamespace            string                       `json:"SecretNamespace"`
	SecretCreated              string                       `json:"SecretCreated"`
	CredentialRequestName      string                       `json:"CredentialRequestName"`
	CredentialRequestNamespace string                       `json:"CredentialRequestNamespace"`
	CredentialRequestObj       credreqv1.CredentialsRequest `json:"CredentialRequestObj"`
}

// Print metadata for secrets referenced by AWS provider CredentialRequests resource(s)...
func (o *rotateCredOptions) printAWSCredRequestSecrets(awsCredReqs *[]credreqv1.CredentialsRequest) error {
	if awsCredReqs == nil {
		reqs, err := o.getAWSCredentialsRequests("")
		if err != nil {
			o.log.Warnf("Error fetching AWS provider credentialRequests, err:'%s'\n", err)
			return err
		}
		awsCredReqs = &reqs
	}
	if awsCredReqs == nil {
		o.log.Infof("No AWS provider Credential Requests to show")
		return nil
	}
	var w *tabwriter.Writer
	var secrets []*credReqSecrets
	if o.output == standardFormat {
		w = tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		fmt.Fprintf(w, "--------------------------------------------------------------------------\n")
		fmt.Fprintf(w, "AWS Provider CredentialsRequest referenced secrets:\n")
		fmt.Fprintf(w, "--------------------------------------------------------------------------\n")
	}
	for ind, cr := range *awsCredReqs {
		secret, err := o.clusterClientset.CoreV1().Secrets(cr.Spec.SecretRef.Namespace).Get(context.TODO(), cr.Spec.SecretRef.Name, metav1.GetOptions{})
		if err != nil {
			o.log.Warnf("Failed to get secret:'%s', err:'%s'", cr.Spec.SecretRef.Name, err)
			continue
		}
		if o.output == standardFormat {
			fmt.Fprintf(w, "(%d)\tNamespace:'%s'\tSecret:'%s'\tCreated:'%v'\n", ind, secret.Namespace, secret.Name, secret.CreationTimestamp)
		} else {
			secrets = append(secrets, &credReqSecrets{SecretName: secret.Name, SecretNamespace: secret.Namespace, SecretCreated: secret.CreationTimestamp.String(),
				CredentialRequestName: cr.Name, CredentialRequestNamespace: cr.Namespace, CredentialRequestObj: cr})

		}
	}
	if o.output == standardFormat {
		fmt.Fprintf(w, "--------------------------------------------------------------------------\n")
		w.Flush()
	} else {
		o.printJsonYaml(secrets)
	}
	return nil
}

// #1 Print secrets to be deleted and CredentialRequest objs which reference them.
// #2 Prompt user after printing, ask to continue.
// #3 Prompt User per secret before deleting.
func (o *rotateCredOptions) deleteAWSCredRequestSecrets() error {
	rotateAllWithoutPrompts := false
	awsCredReqs, err := o.getAWSCredentialsRequests("")
	if err != nil {
		o.log.Warnf("Error fetching AWS related credentialRequests, err:'%s'\n", err)
		return err
	}
	if awsCredReqs == nil {
		return fmt.Errorf("failed to find any AWS provider credential request secrets to delete")
	}
	fmt.Printf("\nAWS CredentialRequest referenced secrets to be deleted:\n")
	// Print secrets info for user to review before choosing to continue
	o.printAWSCredRequestSecrets(&awsCredReqs)
	fmt.Println("Please review above credentialRequest Secrets to be deleted.")
	fmt.Println("(DO NOT CONTINUE IF ANYTHING LOOKS AMISS)")
	var userInput string = ""
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
inputLoop:
	for {
		fmt.Fprintf(w, "'Y'\t- User will be prompted to choose per Secret for deletion.\n")
		fmt.Fprintf(w, "'N'\t- Do not continue\n")
		fmt.Fprintf(w, "'all'\t- Delete all without interactive prompts\n")
		fmt.Fprintf(w, "Continue? ('y','n','all'):")
		w.Flush()

		_, _ = fmt.Scanln(&userInput)
		switch strings.ToLower(userInput) {
		case "y", "yes":
			userInput = "y"
			break inputLoop
		case "n", "no":
			userInput = "n"
			o.log.Infof("User chose to not delete secrets...\n")
			return nil
		case "all":
			rotateAllWithoutPrompts = true
			break inputLoop
		default:
			fmt.Println("Invalid input. Expecting (y)es or (N)o or 'all'")
		}
	}
	var idx = 0
	// Print out each secret before prompting and/or deleting...
	for sidx, cr := range awsCredReqs {
		idx = sidx + 1
		secret, err := o.clusterClientset.CoreV1().Secrets(cr.Spec.SecretRef.Namespace).Get(context.TODO(), cr.Spec.SecretRef.Name, metav1.GetOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting secret:'%s', skipping\n", secret.Name)
			continue
		}
		fmt.Fprintf(w, "\nSecret (%d/%d):\n", idx, len(awsCredReqs))
		fmt.Fprintf(w, "\tCredentialRequest:\t'%s'\n", cr.Name)
		fmt.Fprintf(w, "\tCR Namespace:\t'%s'\n", cr.Namespace)
		fmt.Fprintf(w, "\tSecret Name:\t'%s'\n", secret.Name)
		fmt.Fprintf(w, "\tSecret Namespace:\t'%s'\n", secret.Namespace)
		fmt.Fprintf(w, "\tSecret Created:\t'%v'\n", secret.CreationTimestamp)
		w.Flush()
		// Prompt user to delete, unless they choose to skip prompts...
		if rotateAllWithoutPrompts || func() bool {
			fmt.Printf("Delete Secret ('%s')? ", secret.Name)
			return utils.ConfirmPrompt()
		}() {
			o.log.Debugf("Deleting secret(%d/%d):'%s'\n", idx, len(awsCredReqs), cr.Spec.SecretRef.Name)
			// Delete the referenced secret
			err := o.clusterClientset.CoreV1().Secrets(secret.Namespace).Delete(context.TODO(), secret.Name, metav1.DeleteOptions{})
			if err != nil {
				return fmt.Errorf("failed to delete secret '%v' in namespace:'%v', err '%w'", secret.Name, secret.Namespace, err)
			}
			o.log.Debugf("Deleted Secret: '%s'\n", secret.Name)
		} else {
			o.log.Debugf("Skipping secret:'%s'...\n", secret.Name)
			continue
		}
	}
	return nil
}

/* Create the initial connections, fetch the cluster + internal ID, etc
 * Create an OCM client to talk to the cluster API
 * the user has to be logged in (e.g. 'ocm login') to stage, prod, etc..
 * initialize the ocm connection...
 */
func (o *rotateCredOptions) preRunSetup() error {
	var err error
	o.log.Debugf("Creating OCM connection...\n")
	ocmConn, err := utils.CreateConnection()
	if err != nil {
		o.log.Errorf("Failed to create OCM connection: %v", err)
		return err
	}
	defer o.closeOcmConn(ocmConn)
	o.log.Debugf("Created OCM connection")

	// Fetch the cluster info, this will return an error if more than 1 cluster is matched
	cluster, err := utils.GetClusterAnyStatus(ocmConn, o.clusterID)
	if err != nil {
		o.log.Errorf("Failed to fetch cluster:'%s' from OCM using:'%s'", o.clusterID, ocmConn.URL())
		return fmt.Errorf("failed to fetch cluster:'%s' from OCM. err: %w", o.clusterID, err)
	}
	if cluster.Hypershift().Enabled() {
		o.log.Errorf("Cluster '%s' is hypershift enabled. This is currently not supported", cluster.ID())
		return fmt.Errorf("Cluster '%s' is hypershift enabled. This is currently not supported", cluster.ID())
	}
	// Store the cluster obj...
	o.cluster = cluster
	// Validate that the UUID and not an Alias was provided by the user per cli input...
	if !(o.clusterID == cluster.ID() || o.clusterID == cluster.ExternalID()) {
		return fmt.Errorf("cluster id:'%s', does not match internal:'%s' or external:'%s' IDs. No aliases allowed", o.clusterID, cluster.ID(), cluster.ExternalID())
	}

	// Make sure we're using the internal ID from here on...
	o.clusterID = cluster.ID()
	if o.clusterID == "" {
		return fmt.Errorf("OCM cluster.ID() returned empty value")
	}

	// currently using account account.spec.byoc value later for ccs sanity checks, are both values still needed?
	o.isCCS, err = utils.IsClusterCCS(ocmConn, o.clusterID)
	if err != nil {
		return err
	}
	o.log.Debugf("Is Cluster CCS?:%t\n", o.isCCS)

	return nil
}

/* OCM connection cleanup */
func (o *rotateCredOptions) closeOcmConn(ocmConn *sdk.Connection) {
	// Close ocm connection if created...
	if ocmConn != nil {
		ocmCloseErr := ocmConn.Close()
		if ocmCloseErr != nil {
			o.log.Errorf("Error during ocm.close() (possible memory leak): %q", ocmCloseErr)
		}
		ocmConn = nil
	}
}

/* populate aws account claim name info */
func (o *rotateCredOptions) fetchAwsClaimNameInfo() error {
	var err error = nil
	var claimName string = ""
	if o.hiveKubeClient == nil {
		o.log.Errorf("fetchAwsClaimNameInfo called with nil hive client")
		return fmt.Errorf("fetchAwsClaimNameInfo called with nil hive client")
	}
	if len(o.clusterID) <= 0 {
		o.log.Errorf("fetchAwsClaimNameInfo called with empty clusterID")
		return fmt.Errorf("fetchAwsClaimNameInfo called with empty clusterID")
	}
	accountClaim, err := k8s.GetAccountClaimFromClusterID(o.ctx, o.hiveKubeClient, o.clusterID)

	if err != nil {
		o.log.Warnf("k8s.GetAccountClaimFromClusterID err:'%s', trying resources instead...\n", err)
	} else {
		o.log.Debugf("Got AccountClaimFromClusterID, name:'%s', accountlink:'%s'\n", accountClaim.Name, accountClaim.Spec.AccountLink)
		if accountClaim.Spec.AccountLink != "" {
			o.claimName = accountClaim.Spec.AccountLink
			o.secretName = o.claimName + "-secret"
			return nil
		} else {
			o.log.Warnf("accountClaim.Spec.AccountLink contained empty string, trying resources instead...\n")
		}
	}
	claimName, err = o.getResourcesClaimName()
	if err != nil {
		return err
	}
	o.claimName = claimName
	o.secretName = o.claimName + "-secret"
	return nil
}

/* fetch aws account claim CR name from resources output
 * This should not be called in a normal flow and only used
 * here as a backup to newer 'k8s.GetAccountClaimFromClusterID()'
 */
func (o *rotateCredOptions) getResourcesClaimName() (string, error) {
	o.claimName = ""
	if len(o.clusterID) <= 0 {
		return "", fmt.Errorf("clusterID has not been populated prior to cluster resources request")
	}
	o.log.Debugf("Creating OCM connection to fetch claim info...\n")
	ocmConn, err := utils.CreateConnection()
	if err != nil {
		return "", fmt.Errorf("failed to create OCM client: %w", err)
	}
	defer o.closeOcmConn(ocmConn)
	//o.ocmConn.ClustersMgmt().V1().AWSInquiries().STSCredentialRequests().List().Send()
	liveResponse, err := ocmConn.ClustersMgmt().V1().Clusters().Cluster(o.clusterID).Resources().Live().Get().Send()
	if err != nil {
		return "", fmt.Errorf("error fetching cluster resources: %w", err)
	}
	respBody := liveResponse.Body().Resources()
	if awsAccountClaim, ok := respBody["aws_account_claim"]; ok {
		//Unpack into account claim struct...
		var claimObj awsv1alpha1.AccountClaim
		err = json.Unmarshal([]byte(awsAccountClaim), &claimObj)
		if err != nil {
			o.log.Warnf("Error getting account claim:'%s'", err)
			return "", err
		} else {
			o.log.Debugf("Got awsAccountClaim accountLink:'%s'", claimObj.Spec.AccountLink)
		}
		return claimObj.Spec.AccountLink, nil
	}
	return "", fmt.Errorf("failed to parse aws_account_claim from api 'clusterMgmt/v1/clusters/%s/resources/live/'", o.clusterID)
}

// Connect Backplane client to target cluster...
func (o *rotateCredOptions) connectClusterClient() error {
	o.log.Debugf("Connect BP client cluster:'%s'\n", o.clusterID)
	if len(o.reason) <= 0 {
		return fmt.Errorf("accessing cred secrets requires a reason for elevated command")
	}
	// This action requires elevation
	elevationReasons := []string{}
	elevationReasons = append(elevationReasons, o.reason)
	elevationReasons = append(elevationReasons, fmt.Sprintf("Elevation required to rotate secrets '%s' aws-account-cr-name", o.claimName))

	// If some elevationReasons are provided, then the config will be elevated with user backplane-cluster-admin...
	// Fetch the backplane kubeconfig, ignore the returned kubecli for now...
	tmpClient, kubeConfig, kubClientSet, err := common.GetKubeConfigAndClient(o.clusterID, elevationReasons...)
	if err != nil {
		o.log.Warnf("Err creating cluster:'%s' client, GetKubeConfigAndClient() err:'%+v'\n", o.clusterID, err)
		return err
	}
	if tmpClient != nil {
		tmpClient = nil
	}
	o.clusterClientset = kubClientSet
	// Create new kube client with the CredentialsRequest schema using the backplane config from above...
	scheme := k8sruntime.NewScheme()
	err = credreqv1.AddToScheme(scheme)
	if err != nil {
		o.log.Warnf("Error adding CredentialsRequests to schema:'%s'\n", err)
		return err
	}
	kubeCli, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		o.log.Warnf("Error creating new schema'd client for cluster:'%s', err:'%s'\n", o.clusterID, err)
		return err
	}
	o.clusterKubeClient = kubeCli
	return nil
}

/* fetch and connect to hive cluster for the provided user cluster */
func (o *rotateCredOptions) connectHiveClient() error {
	// This requires a valid ocm login token on 'prod' as at the moment hive only resides in prod.
	// Creates backplane client for hive and stores the resulting client in o.hiveKubeClient

	o.log.Debugf("Connect hive client for cluster:'%s'\n", o.clusterID)
	if len(o.reason) <= 0 {
		//This client requires a reason in order to impersonate backplane-cluster-admin...
		return fmt.Errorf("accessing AWS credentials requires a reason for elevated command")
	}
	// This action requires elevation
	elevationReasons := []string{}
	elevationReasons = append(elevationReasons, o.reason)
	elevationReasons = append(elevationReasons, fmt.Sprintf("Elevation required to rotate secrets '%s' aws-account-cr-name", o.claimName))
	if len(o.ocmConfigHivePath) <= 0 {
		// This is the expected prod path for hive connections using env vars which
		// are shared with the target cluster.
		hiveCluster, err := utils.GetHiveCluster(o.clusterID)
		if err != nil {
			o.log.Errorf("error fetching hive for cluster:'%s'.\n", err)
			o.log.Errorf("If target cluster is not using prod, see option'--ocm-config-hive'. Confirm token is not expired")
			return err
		}
		o.hiveCluster = hiveCluster
		if o.hiveCluster == nil || o.hiveCluster.ID() == "" {
			return fmt.Errorf("failed to fetch hive cluster ID")
		}
		o.log.Infof("Using Hive Cluster: '%s'\n", hiveCluster.ID())
		// If some elevationReasons are provided, then the config will be elevated with user backplane-cluster-admin...
		o.log.Infof("Creating hive backplane client using OCM environment variables")
		hiveKubeCli, _, _, err := common.GetKubeConfigAndClient(o.hiveCluster.ID(), elevationReasons...)
		if err != nil {
			o.log.Warnf("Err fetching hive cluster client, GetKubeConfigAndClient() err:'%+v'\n", err)
			return err
		}
		o.hiveKubeClient = hiveKubeCli
		return nil
	} else {
		// This is the expected non-prod path where the (int/staging) target cluster does not share
		// the same env/vars as the (prod) Hive connection. See OSD-28241.
		//TODO: - This path allows for testing outside of production environments.
		//      - Most of the code in this path can/should be implemented in backplane cli, and
		//        this code can removed from this util at that time. (see OSD-28241)
		//      - What else could be unique between OCM envs now or in the future?
		//
		// This portion builds a hive client using CLI provided OCM config path `--ocm-config-hive`
		// This is intended to allow use with clusters running outside of the same 'production' env hive runs in.
		// At the time of writing this, Hive can exist in 'prod' while the target cluster can reside in staging,
		// integration, or other environments. This seems to require different sets of OCM config per environment and
		// backplane cli libs does not appear to support this at this time.
		// This differs from typical(?) usage where OCM settings, and config paths are set + shared using environment vars,
		// expecting a single OCM environment.
		// The following attempts to use 'only' config found within the provided --ocm-config-hive file
		// path for the hive connection. The target cluster is expected to continue to use whatever is set in the
		// OCM environment variables.
		// A lot borrowed from backplane-cli here and repurposed to support an ocm config not sourced from the os's env.
		// If/when there are upstream exported functions allowing config objects to be passed between them, much of this
		// should go away.

		o.log.Debugf("Using ocm config for hive connection:'%s'", o.ocmConfigHivePath)

		// First fetch and read in the backplane config file content for the potential proxy-url list, and
		// then check each backplane proxy against the OCM url from the 'cli provided config' (not env) to find a working proxy.
		//
		// Note: backplane-cli.GetBackplaneConfiguration() fetches the OCM provided backplane URL using environment vars,
		// and then validates the list of proxies against this OCM URL, returning a single 'working' proxy or none.
		// This excludes the other proxy URIs from the original slice, which we may need to validate against our user provided config.
		// It's likely the same proxy is valid for the URL the user has provided here too, and this is overkill, but the intent of
		// the following is to allow this util to validate the provided proxy urls against the specific 'hive'
		// config and url provided by the user instead.

		bpFilePath, err := bpconfig.GetConfigFilePath()
		o.log.Debugf("Using backplane config:'%s'", bpFilePath)
		if err != nil {
			return err
		}
		_, err = os.Stat(bpFilePath)
		if err != nil {
			o.log.Errorf("Failed to stat ocm config:'%s', err:%v", o.ocmConfigHivePath, err)
			return fmt.Errorf("failed to stat ocm config:'%s', err:'%v'", o.ocmConfigHivePath, err)
		}
		viper.AutomaticEnv()
		viper.SetConfigFile(bpFilePath)
		viper.SetConfigType("json")
		if err := viper.ReadInConfig(); err != nil {
			o.log.Errorf("Hive OCM config err:'%v", err)
			return err
		}
		if viper.GetStringSlice("proxy-url") == nil && os.Getenv("HTTPS_PROXY") == "" {
			return fmt.Errorf("proxy-url must be set explicitly in either config file or via the environment HTTPS_PROXY")
		}
		proxyURLs := viper.GetStringSlice("proxy-url")

		// Provide warning as well as config+path if aws proxy not provided...
		awsProxyUrl := viper.GetString(awsprovider.ProxyConfigKey)
		if awsProxyUrl == "" {
			o.log.Warnf("key:'%s' not found in config:'%s'", awsprovider.ProxyConfigKey, viper.GetViper().ConfigFileUsed())
		}

		// Now get the backplane url and access token from OCM...
		ocmConfig, err := readOcmConfigFile(o.ocmConfigHivePath)
		if err != nil {
			o.log.Errorf("OCM config error:'%v'", err)
			return err
		}
		o.log.Debugf("Read ocm config: url:'%s', client:'%s'", ocmConfig.URL, ocmConfig.ClientID)
		// Can use the sdk.connection builder or alternatively omc cli's connection builder wrappers here.
		// Each returns an ocm-sdk connection builder.
		ocmSdkConnBuilder := sdk.NewConnectionBuilder()
		ocmSdkConnBuilder.URL(ocmConfig.URL)
		ocmSdkConnBuilder.Tokens(ocmConfig.AccessToken, ocmConfig.RefreshToken)
		ocmSdkConnBuilder.Client(ocmConfig.ClientID, ocmConfig.ClientSecret)
		ocmSdkConn, err := ocmSdkConnBuilder.Build()

		if err != nil {
			o.log.Errorf("OCM connection error. Config:'%s', err:'%v'", o.ocmConfigHivePath, err)
			return err
		}
		defer ocmSdkConn.Close()
		o.log.Debugf("Using ocm sdk connection, url:'%s' \n", ocmSdkConn.URL())
		hiveShard, err := o.getHiveShardCluster(ocmSdkConn, o.clusterID)
		if err != nil {
			// For debug purposes see if we can get the shard API url...
			// This might help indicate an ocm env mismatch to the end user(?).
			hiveShard, debugErr := utils.GetHiveShard(o.clusterID)
			if debugErr != nil {
				o.log.Warnf("error fetching hive shard from utils.GetHiveShard():'%s'.\n", err)
			} else {
				o.log.Infof("Hive shard API url:'%s'\n", hiveShard)
			}
			// Return the original error failing to fetch the hive cluster...
			o.log.Errorf("Failed to get hive cluster using OCM config:'%s', err:'%v'", o.ocmConfigHivePath, err)
			return err
		}

		if hiveShard == nil {
			return fmt.Errorf("failed to fetch hive cluster ID")
		}
		o.hiveCluster = hiveShard
		o.log.Infof("Using Hive Cluster: '%s'\n", o.hiveCluster.ID())

		// Get the backplane URL from the OCM env request/response...
		responseEnv, err := ocmSdkConn.ClustersMgmt().V1().Environment().Get().Send()
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
			o.log.Errorf("Failed Hive connection: %s: %v", errorMessage, err)
			return fmt.Errorf("%s: %w", errorMessage, err)
		}

		ocmEnvResponse := responseEnv.Body()
		bpUrl, ok := ocmEnvResponse.GetBackplaneURL()

		// Now that we have the backplane URL from OCM, use it find the first working Proxy URL...
		proxyURL := getFirstWorkingProxyURL(bpUrl, proxyURLs, o.log, o.ctx)
		if !ok {
			return fmt.Errorf("failed to find a working backplane proxy url for the OCM environment: %v", ocmEnvResponse.Name())
		}
		// Get the OCM access token...
		accessToken, err := bpocmcli.DefaultOCMInterface.GetOCMAccessTokenWithConn(ocmSdkConn)
		if err != nil {
			o.log.Errorf("error fetching OCM access token:'%v'", err)
			return err
		}

		// Attempt to login and get the backplane API URL for this Hive Cluster...
		bpAPIClusterURL, err := doLogin(bpUrl, o.hiveCluster.ID(), *accessToken, proxyURL)
		if err != nil {
			o.log.Infof("Using backplane URL:'%s', proxy:'%s'", bpUrl, proxyURL)
			o.log.Errorf("Failed BP hive login: URL:'%s', Proxy:'%s'. Check VPN, accessToken, etc...?", bpUrl, proxyURL)
			return fmt.Errorf("failed to backplane login to hive: '%s': '%v'", o.hiveCluster.ID(), err)
		}
		o.log.Debugf("Using backplane CLUSTER API URL: '%s'", bpAPIClusterURL)
		// Build the KubeConfig to be used to build our Kubernetes client...
		kubeconfig := rest.Config{
			Host:        bpAPIClusterURL,
			BearerToken: *accessToken,
			Impersonate: rest.ImpersonationConfig{
				UserName: "backplane-cluster-admin",
			},
		}
		// Add the provided elevation reasons for impersonating request as user 'backplane-cluster-admin'
		kubeconfig.Impersonate.Extra = map[string][]string{"reason": elevationReasons}

		// If a working proxyURL was found earlier, set the kube config proxy func...
		if len(proxyURL) > 0 {
			kubeconfig.Proxy = func(*http.Request) (*url.URL, error) {
				return url.Parse(proxyURL)
			}
		}

		// Finally create the kubeclient for the hive cluster connection...
		kubeCli, err := client.New(&kubeconfig, client.Options{})
		if err != nil {
			o.log.Errorf("Error creating Hive client:'%v'", err)
			return err
		}
		if kubeCli == nil {
			return fmt.Errorf("client.new() returned nil. Failed to setup Hive client")
		}
		o.log.Debugf("Success creating Hive kube client using ocm config:'%s'", o.ocmConfigHivePath)
		o.hiveKubeClient = kubeCli
		return nil
	}
}

func (o *rotateCredOptions) getHiveShardCluster(sdkConn *sdk.Connection, clusterID string) (*cmv1.Cluster, error) {
	connection, err := utils.CreateConnection()
	if err != nil {
		o.log.Errorf("Failed to create default ocm sdk connection, err:'%s'", err)
		return nil, err
	}
	defer connection.Close()

	shardPath, err := connection.ClustersMgmt().V1().Clusters().
		Cluster(clusterID).
		ProvisionShard().
		Get().
		Send()
	if err != nil {
		o.log.Errorf("Failed to get provisionShard, err:'%s'", err)
		return nil, err
	}
	if shardPath == nil {
		o.log.Errorf("Failed to get provisionShard, returned nil")
		return nil, fmt.Errorf("failed to get provisionShard, returned nil")
	}

	id, ok := shardPath.Body().GetID()
	if ok {
		o.log.Debugf("Got provision shard ID:'%s'", id)
	}

	shard := shardPath.Body().HiveConfig().Server()
	o.log.Debugf("Got provision shard:'%s'", shard)

	hiveApiUrl, ok := shardPath.Body().HiveConfig().GetServer()
	if !ok {
		return nil, fmt.Errorf("no provision shard url found for %s", clusterID)
	}
	o.log.Debugf("Got hiveApiUrl:'%s'", hiveApiUrl)

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

// doLogin returns the proxy url for the target cluster.
// borrowed from backplane-cli/ocm-backplane/login and repurposed here to
// support providing token and proxy instead of pulling these from the env vars in the chain of vendored functions.
func doLogin(api, clusterID, accessToken string, proxyURL string) (string, error) {
	// This ends up using 'ocm.DefaultOCMInterface.GetOCMEnvironment()' to get the backplane url from OCM
	//client, err := backplaneapi.DefaultClientUtils.MakeRawBackplaneAPIClientWithAccessToken(api, accessToken)
	var proxyArg *string
	if len(proxyURL) > 0 {
		proxyArg = &proxyURL
	}
	client, err := backplaneapi.DefaultClientUtils.GetBackplaneClient(api, accessToken, proxyArg)
	if err != nil {
		return "", fmt.Errorf("unable to create backplane api client")
	}

	resp, err := client.LoginCluster(context.TODO(), clusterID)
	// Print the whole response if we can't parse it. Eg. 5xx error from http server.
	if err != nil {
		// trying to determine the error
		errBody := err.Error()
		if strings.Contains(errBody, "dial tcp") && strings.Contains(errBody, "i/o timeout") {
			return "", fmt.Errorf("unable to connect to backplane api")
		}
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		strErr, err := tryParseBackplaneAPIErrorMsg(resp)
		if err != nil {
			return "", fmt.Errorf("failed to parse response error. Parse err:'%v'", err)
		}
		return "", fmt.Errorf("loginCluster() response error:'%s'", strErr)
	}
	//respProxyUri, err := bpclient.ParseLoginClusterResponse(resp)
	respProxyUri, err := getClusterResponseProxyUri(resp)
	if err != nil {
		return "", fmt.Errorf("unable to parse response body from backplane: \n Status Code: %d", resp.StatusCode)
	}

	//return api + *loginResp.JSON200.ProxyUri, nil
	return api + respProxyUri, nil
}

// Borrowed from backplaneApi Error
// Error defines model for Error.
type bpError struct {
	// Message Error Message
	Message *string `json:"message,omitempty"`

	// StatusCode HTTP status code
	StatusCode *int `json:"statusCode,omitempty"`
}

func tryParseBackplaneAPIErrorMsg(rsp *http.Response) (string, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return "", err
	}
	var dest bpError
	if err := json.Unmarshal(bodyBytes, &dest); err != nil {
		return "", err
	}
	if dest.Message != nil && dest.StatusCode != nil {
		return fmt.Sprintf("error from backplane: \n Status Code: %d\n Message: %s", *dest.StatusCode, *dest.Message), nil
	} else {
		return fmt.Sprintf("error from backplane: \n Status Code: %d\n Message: %s", rsp.StatusCode, rsp.Status), nil
	}

}

// Borrowed from backplaneApi LoginResponse
// LoginResponse Login status response
type loginResponse struct {
	// Message message
	Message *string `json:"message,omitempty"`

	// ProxyUri KubeAPI proxy URI
	ProxyUri *string `json:"proxy_uri,omitempty"`

	// StatusCode status code
	StatusCode *int `json:"statusCode,omitempty"`
}

// Intended to parse ProxyUri from login response avoiding
// avoids vendoring 'github.com/openshift/backplane-api/pkg/client'
func getClusterResponseProxyUri(rsp *http.Response) (string, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return "", err
	}
	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest loginResponse
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return "", err
		}
		return *dest.ProxyUri, nil

	}
	// Calling function should check status code , but log here just in case.
	if rsp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Did not parse ProxyUri from cluster login response. resp code:'%d'", rsp.StatusCode)
	}
	return "", nil
}

// Borrowed from backplane-cli/config, repurposed here to allow the backplane url to be provided as an arg instead of
// discovering the url from the OCM environment vars at the time executed.
// Test proxy urls, return first proxy-url that results in a successful request to the <backplaneBaseUrl>/healthz endpoint
func getFirstWorkingProxyURL(bpBaseURL string, proxyURLs []string, logger *logrus.Logger, ctx context.Context) string {
	bpHealthzURL := bpBaseURL + "/healthz"

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	for _, testProxy := range proxyURLs {
		proxyURL, err := url.ParseRequestURI(testProxy)
		if err != nil {
			// Warn user against proxy aliases such as 'prod', 'stage', etc. in config
			// so they can resolve to proper URLs (ie https://...openshift.com)
			logger.Warn(ctx, "proxy-url: '%v' could not be parsed as URI. Proxy Aliases not yet supported", testProxy)
			continue
		}

		client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		req, _ := http.NewRequest("GET", bpHealthzURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			logger.Info(ctx, "Proxy: %s returned an error: %s", proxyURL, err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return testProxy
		}
		logger.Info(ctx, "proxy: %s did not pass healthcheck, expected response code 200, got %d, discarding", testProxy, resp.StatusCode)
	}
	logger.Info(ctx, "Failed to find a working proxy-url for backplane request path:'%s'", bpHealthzURL)
	if len(proxyURLs) > 0 {
		logger.Info(ctx, "falling back to first proxy-url after all proxies failed health checks: %s", proxyURLs[0])
		return proxyURLs[0]
	}
	return ""
}

// utils has a local 'copy' of the config struct
// rather than vendor from "github.com/openshift-online/ocm-cli/pkg/config"
func readOcmConfigFile(file string) (*utils.Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		err = fmt.Errorf("can't read config file '%s': %v", file, err)
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("empty config file:'%s'", file)
	}
	cfg := &utils.Config{}
	err = json.Unmarshal(data, cfg)
	if err != nil {
		err = fmt.Errorf("can't parse config file '%s': %v", file, err)
		return nil, err
	}
	return cfg, nil
}

func (o *rotateCredOptions) fetchAWSAccountInfo() error {
	if len(o.claimName) <= 0 {
		return fmt.Errorf("fetchAWSAccountInfo() empty claim name field found. Can not fetch AWS info without it")
	}

	o.log.Debugf("----------------------------------------------------------------------\n")
	o.log.Debugf("Fetching account info per aws-account-operator\n")
	o.log.Debugf("----------------------------------------------------------------------\n")
	if o.hiveKubeClient == nil {
		err := o.connectHiveClient()
		if err != nil {
			return err
		}
	}
	// Get the associated Account CR from the provided name
	o.log.Debugf("Fetching aws-account-operator account object for '%s.%s' ...", common.AWSAccountNamespace, o.claimName)
	accountObj, err := k8s.GetAWSAccount(o.ctx, o.hiveKubeClient, common.AWSAccountNamespace, o.claimName)
	if err != nil {
		o.log.Warnf("k8s.GetAWSAccount() err:'%s'\n", err)
		return err
	}

	if accountObj.Spec.ManualSTSMode {
		return fmt.Errorf("account %s is manual STS mode - No IAM User Credentials to Rotate", o.claimName)
	}
	// Are both byoc and ccs value checks needed?
	if !accountObj.Spec.BYOC && !o.isCCS && o.updateCcsCredsCli {
		// Check for specifics early? ...or should this just be ignored and a no-op later?
		o.log.Warnf("arg '--rotate-ccs-admin' provided to rotate osdCcsAdmin creds on non-CCS cluster id:'%s', name:'%s'\n. No changes were made to this cluster\n", o.cluster.Name(), o.clusterID)
		return fmt.Errorf("arg '--rotate-ccs-admin' provided to rotate osdCcsAdmin creds on non-CCS cluster")
	}
	o.account = accountObj

	// Set the account ID
	o.accountID = o.account.Spec.AwsAccountID
	if o.accountID == "" {
		return fmt.Errorf("empty AwsAccountID string found in account spec")
	}
	// Get IAM user suffix from CR label
	iamUserID, ok := o.account.Labels["iamUserId"]
	if !ok {
		return fmt.Errorf("no iamUserId label on Account CR")
	}
	o.accountIDSuffixLabel = iamUserID
	// Should this error out here if the suffix is an empty string?
	if o.accountIDSuffixLabel == "" {
		return fmt.Errorf("account label 'iamUserId' is empty string")
	}
	o.log.Debugf("AccountID:'%s', iamUserId label:'%s' \n", o.accountID, o.accountIDSuffixLabel)
	return nil
}

/* - Fetch creds needed for this script/client's AWS access, create AWS client */
func (o *rotateCredOptions) setupAwsClient() error {
	var err error

	if len(o.accountIDSuffixLabel) <= 0 {
		return fmt.Errorf("doAwsCredRotate() empty required accountIDSuffixLabel")
	}
	o.log.Debugf("----------------------------------------------------------------------\n")
	o.log.Debugf(" AWS setup access, local config:'%s'\n", o.profile)
	o.log.Debugf("----------------------------------------------------------------------\n")

	o.log.Debugf("Creating 'AWS-Initial-setup' client using local aws profile: '%s'...\n", o.profile)
	// Since this is only using "IAM", hard coding to us-east-1 region should be ok...
	awsSetupClient, err := awsprovider.NewAwsClient(o.profile, "us-east-1", "")
	if err != nil {
		o.log.Errorf("Failed to create initial AWS client using AWS profile:'%s'. Err:'%v'\n", o.profile, err)
		return err
	}

	// Ensure AWS calls are successful with client
	callerIdentityOutput, err := awsSetupClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}
	o.log.Debugf("STS calleridentity Account:'%s', userid:'%s' \n", *callerIdentityOutput.Account, *callerIdentityOutput.UserId)

	var credentials *stsTypes.Credentials
	// Need to role chain if the cluster is CCS
	if o.isCCS {
		o.log.Debugf("----------------------------------------------------------------------\n")
		o.log.Debugf("Cluster is CCS. Begin AWS role chaining...\n")
		o.log.Debugf("----------------------------------------------------------------------\n")
		// Get the aws-account-operator configmap
		cm := &corev1.ConfigMap{}
		o.log.Debugf("Fetching SRE CCS Access ARN, and Support Jump ARN from config map: '%s.%s' \n", common.AWSAccountNamespace, common.DefaultConfigMap)
		//cmErr := o.kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: common.AWSAccountNamespace, Name: common.DefaultConfigMap}, cm)
		cmErr := o.hiveKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: common.AWSAccountNamespace, Name: common.DefaultConfigMap}, cm)
		if cmErr != nil {
			o.log.Warnf("error getting ConfigMap:'%s'.'%s' needed for SRE Access Role. err: %s", common.AWSAccountNamespace, common.DefaultConfigMap, cmErr)
			return fmt.Errorf("error getting ConfigMap:'%s'.'%s' needed for SRE Access Role. err: %s", common.AWSAccountNamespace, common.DefaultConfigMap, cmErr)
		}
		// Get the ARN value
		SREAccessARN := cm.Data["CCS-Access-Arn"]
		if SREAccessARN == "" {
			return fmt.Errorf("SRE Access ARN is missing from '%s' configmap:'%s'", common.AWSAccountNamespace, common.DefaultConfigMap)
		}
		// Get the Jump ARN value
		JumpARN := cm.Data["support-jump-role"]
		if JumpARN == "" {
			return fmt.Errorf("support jump role ARN is missing from '%s' configmap:'%s'", common.AWSAccountNamespace, common.DefaultConfigMap)
		}
		o.log.Debugf("Using 'AWS-Initial-setup' client to fetch assume role creds for 'SRE Access ARN'...\n")
		// Fetch assumed SRE Access role creds
		srepRoleCredentials, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, o.awsAccountTimeout, callerIdentityOutput.UserId, &SREAccessARN)
		if err != nil {
			return err
		}
		o.log.Debugf("Creating new 'AWS-SRE-Access' client using assumed SRE access role creds...\n")
		// Create client with the SREP role
		srepRoleClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
			AccessKeyID:     *srepRoleCredentials.AccessKeyId,
			SecretAccessKey: *srepRoleCredentials.SecretAccessKey,
			SessionToken:    *srepRoleCredentials.SessionToken,
			Region:          "us-east-1",
		})
		if err != nil {
			return err
		}

		// Fetch assumed support jump role creds
		o.log.Debugf("Using 'AWS-SRE-Access' client to fetch assume role creds for 'Support Jump Role ARN'...\n")
		jumpRoleCreds, err := awsprovider.GetAssumeRoleCredentials(srepRoleClient, o.awsAccountTimeout, callerIdentityOutput.UserId, &JumpARN)
		if err != nil {
			return err
		}
		o.log.Debugf("Creating new 'AWS-Support-Jump-Role' client using assumed Support Jump role creds...\n")
		// Create client with the Jump role
		jumpRoleClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
			AccessKeyID:     *jumpRoleCreds.AccessKeyId,
			SecretAccessKey: *jumpRoleCreds.SecretAccessKey,
			SessionToken:    *jumpRoleCreds.SessionToken,
			Region:          "us-east-1",
		})
		if err != nil {
			return err
		}
		o.log.Debugf("Using 'AWS-Support-Jump-Role' client to fetch creds using ManagedOpenShift-support role ARN...\n")
		// Role chain to assume ManagedOpenShift-Support-{uid}
		roleArn := awsSdk.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", o.accountID, "ManagedOpenShift-Support-"+o.accountIDSuffixLabel))
		credentials, err = awsprovider.GetAssumeRoleCredentials(jumpRoleClient, o.awsAccountTimeout,
			callerIdentityOutput.UserId, roleArn)
		if err != nil {
			return err
		}

	} else {
		o.log.Debugf("----------------------------------------------------------------------\n")
		o.log.Debugf("Cluster is 'NOT' CCS. Begin AWS role chaining...\n")
		o.log.Debugf("Using 'AWS Initial setup' client to fetch creds using '%s' ARN...\n", awsv1alpha1.AccountOperatorIAMRole)
		o.log.Debugf("----------------------------------------------------------------------\n")

		// Assume the OrganizationAdminAccess role
		roleArn := awsSdk.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", o.accountID, awsv1alpha1.AccountOperatorIAMRole))
		credentials, err = awsprovider.GetAssumeRoleCredentials(awsSetupClient, o.awsAccountTimeout,
			callerIdentityOutput.UserId, roleArn)
		if err != nil {
			return err
		}
	}

	// Build a new client with the assumed role credentials...
	o.log.Debugf("----------------------------------------------------------------------\n")
	o.log.Debugf("Creating final AWS client for cred rotation with assumed role...\n")
	o.log.Debugf("----------------------------------------------------------------------\n")
	awsClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		Region:          "us-east-1",
	})
	if err != nil {
		return err
	}
	o.log.Debugf("Created final AWS client\n")
	o.awsClient = awsClient
	return nil
}

func (o *rotateCredOptions) getKeyInfo(iamUser string, nameSpace string, secretName string) (*iam.ListAccessKeysOutput, *awsprovider.ClientInput, error) {
	currKeys, err := o.awsClient.ListAccessKeys(&iam.ListAccessKeysInput{UserName: awsSdk.String(iamUser)})
	if err != nil {
		return nil, nil, err
	}
	clientInput, err := k8s.GetAWSAccountCredentials(o.ctx, o.hiveKubeClient, nameSpace, secretName)
	if err != nil {
		o.log.Warnf("Failed to fetch current aws account creds from secret, err:'%s'\n", err)
		o.log.Warnf("May require manual check of secret to determine access key in use?\n")
	}
	return currKeys, clientInput, nil
}

type accessKeyUserInfo struct {
	IAMUser        string     `json:"IAMUser"`
	KeyCount       int        `json:"KeyCount"`
	AccountCRKeyID string     `json:"AccountCRKeyID"`
	AccessKeys     []*keyInfo `json:"Keys"`
}

type keyInfo struct {
	ID         string `json:"ID"`
	InUse      bool   `json:"InUse"`
	CreateDate string `json:"CreateDate"`
	Status     string `json:"Status"`
	IAMUser    string `json:"IAMUser"`
}

/* Display AWS key info to user running this util. Intended to help the user identify if
 * a key rotation is needed before/after running this script, etc..
 * Displays the following info:
 * - AccessKey ID,
 * - (CR-IN-USE) Bool, whether this key is referenced by the IAM Credentials created with AWS Account CR ,
 * - Date Created
 * - Status
 * - IAM user/ower
 */
func (o *rotateCredOptions) printKeyInfo(currKeys *iam.ListAccessKeysOutput, clientInput *awsprovider.ClientInput, iamUser string) error {
	if currKeys == nil || clientInput == nil {
		return fmt.Errorf("nil input passed to printKeyInfo()")
	}
	var inUseStr string = "unknown?"
	userKeyInfo := accessKeyUserInfo{IAMUser: iamUser, KeyCount: len(currKeys.AccessKeyMetadata), AccountCRKeyID: clientInput.AccessKeyID}
	p := printer.NewTablePrinter(o.IOStreams.Out, 3, 1, 3, ' ')
	headers := []string{"#", "ACCESS-KEY-ID", "CR-IN-USE", "CREATED", "STATUS", "IAM-USER"}
	p.AddRow(headers)
	if o.output == standardFormat {
		fmt.Printf("\n---------------------------------------------------------------------------------------------------------------\n")
		fmt.Printf("Access Key Info, IAM user:'%s', num of keys:'%d' of max 2\n", iamUser, len(currKeys.AccessKeyMetadata))
		fmt.Printf("Account CR in-use accessKeyID:'%s'\n", clientInput.AccessKeyID)
		fmt.Printf("---------------------------------------------------------------------------------------------------------------\n")
	}
	for Ind, Akey := range currKeys.AccessKeyMetadata {
		if Akey.AccessKeyId == nil {
			return fmt.Errorf("accessKey *id nil in List AccessKeysOutput provided to printKeyInfo()")
		}
		if len(clientInput.AccessKeyID) > 0 {
			inUseStr = fmt.Sprintf("%t", clientInput.AccessKeyID == *Akey.AccessKeyId)
		} else {
			inUseStr = "unknown?"
		}
		if o.output == standardFormat {
			row := []string{fmt.Sprintf("%d", Ind), *Akey.AccessKeyId, inUseStr, Akey.CreateDate.String(), string(Akey.Status), *Akey.UserName}
			p.AddRow(row)
		} else {
			userKeyInfo.AccessKeys = append(userKeyInfo.AccessKeys, &keyInfo{ID: *Akey.AccessKeyId,
				InUse: clientInput.AccessKeyID == *Akey.AccessKeyId, CreateDate: Akey.CreateDate.String(),
				Status: string(Akey.Status), IAMUser: *Akey.UserName})
		}
	}
	if o.output == standardFormat {
		err := p.Flush()
		fmt.Printf("---------------------------------------------------------------------------------------------------------------\n")
		return err
	} else {
		err := o.printJsonYaml(userKeyInfo)
		return err
	}
}

func (o *rotateCredOptions) hasAccessKey(accessKey string, keys *iam.ListAccessKeysOutput) bool {
	for _, Akey := range keys.AccessKeyMetadata {
		if *Akey.AccessKeyId == accessKey {
			return true
		}
	}
	return false
}

/* Check Access key count for AWS documented max (2) keys for the provided user.
 * if iam user has max keys, this will prompt (stdout/stdin) the user to interactively delete a key or exit
 * Alternatively, the user can provide the cli arg 'delete-key-id' to delete a key non-interactively.
 * keys will only be deleted if >1 key exists for the provided iam user.
 */
func (o *rotateCredOptions) checkAccessKeysMaxDelete(iamUser string, nameSpace string, secretName string, deleteKeyID string) error {
	o.log.Debugf("----------------------------------------------------------------------\n")
	o.log.Debugf("Checking user:'%s' existing access-keys...\n", iamUser)
	o.log.Debugf("----------------------------------------------------------------------\n")
	currKeys, clientInput, err := o.getKeyInfo(iamUser, nameSpace, secretName)
	if err != nil {
		return err
	}
	// Print key metdata info for user to review, choosing a key to delete, etc...
	err = o.printKeyInfo(currKeys, clientInput, iamUser)
	if err != nil {
		return err
	}
	// Allow user to interactively delete keys if user has the max.
	// Todo: Should this be presented when a user has less than max too?
	if len(currKeys.AccessKeyMetadata) >= 2 {
		// TODO: Should this script provide an option to delete the oldest key for the user? Too risky?
		deleteSuccess := false
		o.log.Warnf("user:'%s' already has max number of access keys:'%d'\n", iamUser, len(currKeys.AccessKeyMetadata))

		if len(deleteKeyID) <= 0 {
			fmt.Printf("\nIAM User:'%s' already has max number of access keys:'%d'\n", iamUser, len(currKeys.AccessKeyMetadata))
			fmt.Println("Would you like to specify an AccessKey to delete now?")
			deleteKey := utils.ConfirmPrompt()
			if deleteKey {
				fmt.Printf("Cluster account currently in-use accessKeyID:'%s' for user:'%s'\n", clientInput.AccessKeyID, iamUser)
				fmt.Println("\tUse debug output, and/or aws console info to choose which key to delete.")
				fmt.Println("\tConsider which key is currently in-use, and/or possibly older key, etc..")
				max_attempts := 3
				for attempt := 1; attempt <= max_attempts; attempt++ {
					fmt.Print("Enter Access Key ID to delete: ")
					var keyInput string = ""
					_, _ = fmt.Scanln(&keyInput)
					// Basic input validation...?
					if o.isValidAccessKeyId(keyInput) {
						if o.hasAccessKey(keyInput, currKeys) {
							deleteKeyID = keyInput
							break
						} else {
							fmt.Printf("(attempt %d/%d) Key:'%s' not found in iam.ListAccessKeysOutput\n", attempt, max_attempts, keyInput)
						}
					} else {
						o.log.Errorf("(attempt %d/%d) Invalid access key ID entered:'%s'", attempt, max_attempts, keyInput)
					}
				}
				if len(deleteKeyID) <= 0 {
					return fmt.Errorf("failed to find and delete access key for IAM user:'%s' with user provided input", iamUser)
				}
			}
		}
		if len(deleteKeyID) > 0 {
			// Iterate over key list to confirm this key exists for this user
			if o.hasAccessKey(deleteKeyID, currKeys) {
				o.log.Infof("Attempting to delete AccessKey:%s\n", deleteKeyID)
				_, err = o.awsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{UserName: awsSdk.String(iamUser), AccessKeyId: awsSdk.String(deleteKeyID)})
				if err != nil {
					o.log.Errorf("Error attempting to delete IAM user:'%s' accesskey:'%s'. Err:'%s'\n", iamUser, deleteKeyID, err)
					return err
				} else {
					o.log.Infof("Delete AccessKey:'%s', Success\n", deleteKeyID)
					deleteSuccess = true
				}
			} else {
				o.log.Warnf("IAM user:'%s', AccessKey:'%s', not found in iam.ListAccessKeysOutput\n", iamUser, deleteKeyID)
				return fmt.Errorf("failed to find provided AccessKey:'%s' in iam.ListAccessKeysOutput", deleteKeyID)
			}
		}
		// User has either not chosen to delete a key, or key deletion failed...
		if !deleteSuccess {
			return fmt.Errorf("user:'%s' already has max number of access keys:'%d'. Please manually delete a key from the console (likely the oldest and/or not in-use), or provide 'delete-key-id' kwarg before running this script again", iamUser, len(currKeys.AccessKeyMetadata))
		}
	}
	return nil
}

/* Check whether user input for osdManagedUsername is valid.
 * Attempts to validate whether IAM user exists, will attempt (legacy?) non-suffix'd name if not found.
 * returns iam user name if found
 */
func (o *rotateCredOptions) checkOsdManagedUsername() (string, error) {
	if o.awsClient == nil {
		return "", fmt.Errorf("AWS client has not been created yet for doRotateAWSCreds()")
	}
	// Update osdManagedAdmin secrets
	osdManagedAdminUsername := o.osdManagedAdminUsername
	if osdManagedAdminUsername == "" {
		osdManagedAdminUsername = common.OSDManagedAdminIAM + "-" + o.accountIDSuffixLabel
	}
	o.log.Debugf("Check if AWS user '%s' exists\n", osdManagedAdminUsername)
	checkUser, err := o.awsClient.GetUser(&iam.GetUserInput{UserName: awsSdk.String(osdManagedAdminUsername)})
	var nse *iamTypes.NoSuchEntityException
	if (err != nil && errors.As(err, &nse)) || checkUser == nil {
		o.log.Infof("User Not Found: '%s', trying user:'%s' instead...\n", osdManagedAdminUsername, common.OSDManagedAdminIAM)
		osdManagedAdminUsername = common.OSDManagedAdminIAM
		checkUser, err := o.awsClient.GetUser(&iam.GetUserInput{UserName: awsSdk.String(osdManagedAdminUsername)})
		if err != nil {
			return "", err
		}
		if checkUser == nil {
			return "", fmt.Errorf("user Not Found:'%s'", o.osdManagedAdminUsername)
		}
	}
	if err != nil {
		return "", err
	}
	return osdManagedAdminUsername, nil
}

// Cli helper to print aws access key info and return
func (o *rotateCredOptions) cliPrintOsdManagedAdminCreds() error {
	osdManagedAdminUsername, err := o.checkOsdManagedUsername()
	if err != nil {
		return err
	}
	currKeys, clientInput, err := o.getKeyInfo(osdManagedAdminUsername, common.AWSAccountNamespace, o.secretName)
	if err != nil {
		return err
	}
	err = o.printKeyInfo(currKeys, clientInput, osdManagedAdminUsername)
	if err != nil {
		return err
	}
	return nil
}

// Intended to save a failed syncSet in yaml format so a user can review and/or apply it from the CLI
func (o *rotateCredOptions) saveSyncSetYaml(syncSet *hiveapiv1.SyncSet) error {
	o.log.Warnf("Attempting to save syncSet yaml to file, ns:'%s', name:'%s'\n", syncSet.Namespace, syncSet.Name)
	scheme := k8sruntime.NewScheme()
	err := hiveapiv1.AddToScheme(scheme)
	if err != nil {
		o.log.Warnf("Error adding hiveapiv1 to schema:'%s'\n", err)
		return err
	}
	s := k8json.NewYAMLSerializer(k8json.DefaultMetaFactory, scheme, scheme)
	if s == nil {
		err := fmt.Errorf("err saving syncSet to yaml. Failed to create k8 serializer")
		o.log.Warnf("%v", err)
		return err
	}
	saveFile, err := os.CreateTemp(os.TempDir(), string(awsSyncSetName))
	if err != nil {
		o.log.Warnf("Error creating tmp file to save '%s' syncSet", syncSet.Name)
		return err
	}
	if saveFile == nil {
		err := fmt.Errorf("got nil tmp file attempting to save syncset: '%s'", syncSet.Name)
		o.log.Warnf("%v", err)
		return err
	}
	defer saveFile.Close()
	//Write to tmp file, return path
	err = s.Encode(syncSet, saveFile)
	if err != nil {
		o.log.Warnf("Error saving syncSet:'%s'. Err:'%v'", syncSet.Name, err)
		return err
	}
	fmt.Printf("!! Saved syncSet yaml to: '%s'. Please review, and apply manually if needed. Delete this file after use!\n", saveFile.Name())
	return nil
}

func (o *rotateCredOptions) doRotateManagedAdminAWSCreds() error {
	if o.awsClient == nil {
		return fmt.Errorf("AWS client has not been created yet for doRotateManagedAdminAWSCreds()")
	}
	// Update osdManagedAdmin secrets
	osdManagedAdminUsername, err := o.checkOsdManagedUsername()
	if err != nil {
		return err
	}
	o.log.Infof("Using osdManagedAdminUsername: '%s' \n", osdManagedAdminUsername)
	// Check for max keys before attempting to create, use user provided input to delete a key if needed...
	err = o.checkAccessKeysMaxDelete(osdManagedAdminUsername, common.AWSAccountNamespace, o.secretName, "")
	if err != nil {
		return err
	}

	o.log.Debugf("----------------------------------------------------------------------\n")
	o.log.Debugf("Creating new access key for '%s'\n", osdManagedAdminUsername)
	o.log.Debugf("----------------------------------------------------------------------\n")

	o.log.Infof("Creating new access key...\n")
	createAccessKeyOutput, err := o.awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{UserName: awsSdk.String(osdManagedAdminUsername)})
	if err != nil {
		o.log.Warnf("Failed to create access key for user:'%s'\n", osdManagedAdminUsername)
		return err
	}

	fmt.Printf("New key created:\nuser:'%s'\nAccessKey:'%s'\n", *createAccessKeyOutput.AccessKey.UserName, *createAccessKeyOutput.AccessKey.AccessKeyId)
	// Place new credentials into body for secret
	newOsdManagedAdminSecretData := map[string][]byte{
		"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
		"aws_access_key_id":     []byte(*createAccessKeyOutput.AccessKey.AccessKeyId),
		"aws_secret_access_key": []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey),
	}

	// Update existing osdManagedAdmin secret
	// UpdateSecret() includes a check for existing secret
	o.log.Infof("Updating osdManagedAdmin secret:'%s.%s' with new AWS cred values...\n", common.AWSAccountNamespace, o.secretName)
	err = common.UpdateSecret(o.hiveKubeClient, o.secretName, common.AWSAccountNamespace, newOsdManagedAdminSecretData)
	if err != nil {
		o.log.Warnf("Error updating '%s.%s' secret with new creds. Err:'%s\n", common.AWSAccountNamespace, o.secretName, err)
		return err
	}

	// Update secret in ClusterDeployment's namespace
	o.log.Infof("Updating ClusterDeployment secret:'%s.aws' with new AWS cred values...\n", o.account.Spec.ClaimLinkNamespace)
	err = common.UpdateSecret(o.hiveKubeClient, "aws", o.account.Spec.ClaimLinkNamespace, newOsdManagedAdminSecretData)
	if err != nil {
		o.log.Warnf("Error updating '%s.%s' secret with new creds. Err:'%s\n", o.account.Spec.ClaimLinkNamespace, "aws", err)
		return err
	}

	o.log.Debugf("---------------------------------------------------------\n")
	o.log.Infof("AWS creds updated on hive.\n")
	o.log.Debugf("---------------------------------------------------------\n")

	o.log.Infof("Begin syncset ops to sync to cluster....\n")
	err = o.doSyncSetInteractive()
	if err != nil {
		return err
	}
	o.log.Infof("Successfully rotated secrets for user:'%s'\n", osdManagedAdminUsername)
	return nil
}

func (o *rotateCredOptions) doSyncSetInteractive() error {
	// Previous SOP mentions potential collisions with SyncSets already in progress.
	// This retry loop is intended to allow the user to remedy an err'd situation externally
	// and then retrying the AWS syncset operations. IF the user choses to exit, the
	// util will attempt to save the syncset yaml obj to a tmp location and alert the
	// user it is available to review or apply.
	max_retries := 5
	var syncErr error = nil
	var syncSet *hiveapiv1.SyncSet = nil
	for retry_cnt := 1; retry_cnt <= max_retries; retry_cnt++ {
		syncSet, syncErr = o.doAWSSyncSetRequest()
		if syncErr == nil {
			break
		} else {
			o.log.Errorf("Error during SyncSet '%s', err: '%s'", awsSyncSetName, syncErr)
			doRetry := false
			if retry_cnt < max_retries {
				fmt.Printf("Error applying Syncset:'%s'. Hive cluserID:'%s'\n", awsSyncSetName, o.hiveCluster.ID())
				fmt.Printf("Would you like to retry the syncset operation?\n")
				doRetry = utils.ConfirmPrompt()
			}
			if !doRetry {
				fmt.Println("Exiting without syncset retry")
				if syncSet != nil {
					o.log.Debugf("Attempting to save syncset to tmp file...")
					o.saveSyncSetYaml(syncSet)
				}
				return syncErr
			}
			fmt.Printf("Retry '%s' syncset. Attempt '%d' of '%d'", syncSet.Name, retry_cnt+1, max_retries)
		}
	}
	if syncErr != nil {
		o.log.Errorf("Exceeded max syncset retry attempts:%d", max_retries)
	}
	return syncErr
}

func (o *rotateCredOptions) getClusterDeploymentName() (string, error) {
	if len(o.cdName) > 0 {
		return o.cdName, nil
	}
	if o.hiveKubeClient == nil {
		err := o.connectHiveClient()
		if err != nil {
			return "", err
		}
	}
	if o.account == nil {
		err := o.fetchAWSAccountInfo()
		if err != nil {
			return "", err
		}
	}
	clusterDeployments := &hiveapiv1.ClusterDeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(o.account.Spec.ClaimLinkNamespace),
	}
	err := o.hiveKubeClient.List(o.ctx, clusterDeployments, listOpts...)
	if err != nil {
		o.log.Errorf("Error fetching clusterDeployments:'%v", err)
		return "", err
	}
	if len(clusterDeployments.Items) <= 0 {
		o.log.Errorf("empty clusterDeployments list in response")
		return "", fmt.Errorf("failed to retreive cluster deployments")
	}
	cdName := clusterDeployments.Items[0].ObjectMeta.Name
	o.log.Debugf("Got cluster deployment name:'%s'\n", cdName)
	if len(cdName) <= 0 {
		return "", fmt.Errorf("failed to retrieve cluster deployment name for syncset")
	}
	o.cdName = cdName
	return cdName, nil
}

func (o *rotateCredOptions) getAWSSyncSetRequestObj() (*hiveapiv1.SyncSet, error) {

	o.log.Debugf("Fetching cluster deployments list from hive...\n")
	cdName, err := o.getClusterDeploymentName()
	if err != nil {
		return nil, err
	}
	syncSet := &hiveapiv1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      awsSyncSetName,
			Namespace: o.account.Spec.ClaimLinkNamespace,
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
							Name: "aws",
						},
						TargetRef: hiveapiv1.SecretReference{
							Name:      "aws-creds",
							Namespace: "kube-system",
						},
					},
				},
			},
		},
	}
	return syncSet, nil
}

func (o *rotateCredOptions) doAWSSyncSetRequest() (*hiveapiv1.SyncSet, error) {
	o.log.Infof("Creating syncset:'%s.%s', to deploy the updated creds to the cluster for CCO...\n", o.account.Spec.ClaimLinkNamespace, awsSyncSetName)
	o.log.Debugf("Syncing AWS creds down to cluster.")
	syncSet, err := o.getAWSSyncSetRequestObj()
	if err != nil {
		return nil, err
	}
	cdName, err := o.getClusterDeploymentName()
	if err != nil {
		return nil, err
	}
	if o.hiveKubeClient == nil {
		err := o.connectHiveClient()
		if err != nil {
			return nil, err
		}
	}
	if o.account == nil {
		err := o.fetchAWSAccountInfo()
		if err != nil {
			return nil, err
		}
	}
	err = o.hiveKubeClient.Create(o.ctx, syncSet)
	if err != nil {
		return syncSet, err
	}

	fmt.Printf("Watching Cluster Sync Status for deployment...\n")
	hiveinternalv1alpha1.AddToScheme(o.hiveKubeClient.Scheme())
	searchStatus := &hiveinternalv1alpha1.ClusterSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cdName,
			Namespace: o.account.Spec.ClaimLinkNamespace,
		},
	}
	foundStatus := &hiveinternalv1alpha1.ClusterSync{}
	isSSSynced := false
	start := time.Now()
	for i := 0; i < 6; i++ {
		err = o.hiveKubeClient.Get(o.ctx, client.ObjectKeyFromObject(searchStatus), foundStatus)
		if err != nil {
			return syncSet, err
		}
		elapsed := time.Since(start)
		for _, status := range foundStatus.Status.SyncSets {
			if status.Name == awsSyncSetName {
				if status.FirstSuccessTime != nil {
					o.log.Debugf("Syncset '%s' first success time set, elapsed:'%s'\n", awsSyncSetName, elapsed)
					isSSSynced = true
					break
				}
				if status.FailureMessage != "" {
					o.log.Infof("Found SyncSet:'%s' current status failureMessage:'%s'\n", awsSyncSetName, status.FailureMessage)
				}
			}
		}

		if isSSSynced {
			o.log.Infof("\nSync '%s' completed. Elapsed:'%s'\n", awsSyncSetName, time.Since(start))
			break
		}

		fmt.Printf(".")
		time.Sleep(time.Second * 5)
	}
	if !isSSSynced {
		elapsed := time.Since(start)
		return syncSet, fmt.Errorf("syncset failed to sync after elapsed:'%s'. Please verify manually", elapsed)
	}

	o.log.Debugf("Clean up the SS on hive...\n")
	err = o.hiveKubeClient.Delete(o.ctx, syncSet)
	if err != nil {
		o.log.Warnf("Error deleting syncset:'%s.%s' on hive: '%s'\n", syncSet.Namespace, syncSet.Name, err)
		return syncSet, err
	}
	return syncSet, nil
}

// Cli helper to print aws access key info and return
func (o *rotateCredOptions) cliPrintOsdCcsAdminCreds() error {
	const userName string = "osdCcsAdmin"
	const secretName string = "byoc"
	currKeys, clientInput, err := o.getKeyInfo(userName, o.account.Spec.ClaimLinkNamespace, secretName)
	if err != nil {
		return err
	}
	err = o.printKeyInfo(currKeys, clientInput, userName)
	if err != nil {
		return err
	}
	return nil
}

// Update OsdCcsAdmin IAM user with new credentials...
func (o *rotateCredOptions) doRotateCcsCreds() error {
	// Only update osdCcsAdmin credential if specified
	const userName string = "osdCcsAdmin"
	const secretName string = "byoc"

	o.log.Debugf("----------------------------------------------------------------------------\n")
	o.log.Infof("ccs cli flag was set. Attempting to update '%s' user creds...\n", userName)
	o.log.Debugf("----------------------------------------------------------------------------\n")
	// Only update if the Account CR is actually CCS
	if o.isCCS && o.account.Spec.BYOC {

		err := o.checkAccessKeysMaxDelete(userName, o.account.Spec.ClaimLinkNamespace, secretName, "")
		if err != nil {
			return err
		}
		// Rotate osdCcsAdmin creds
		createAccessKeyOutputCCS, err := o.awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{
			UserName: awsSdk.String(userName),
		})
		if err != nil {
			return err
		}
		o.log.Infof("Created new AccessKey for user:'%s'", userName)
		newOsdCcsAdminSecretData := map[string][]byte{
			"aws_user_name":         []byte(*createAccessKeyOutputCCS.AccessKey.UserName),
			"aws_access_key_id":     []byte(*createAccessKeyOutputCCS.AccessKey.AccessKeyId),
			"aws_secret_access_key": []byte(*createAccessKeyOutputCCS.AccessKey.SecretAccessKey),
		}
		o.log.Debugf("Created '%s' creds: UserName:'%s', AccessKey:'%s'\n", userName, *createAccessKeyOutputCCS.AccessKey.UserName, *createAccessKeyOutputCCS.AccessKey.AccessKeyId)
		o.log.Debugf("Updating '%s.%s' secret with new creds...\n", o.account.Spec.ClaimLinkNamespace, secretName)
		err = common.UpdateSecret(o.hiveKubeClient, secretName, o.account.Spec.ClaimLinkNamespace, newOsdCcsAdminSecretData)
		if err != nil {
			o.log.Warnf("Error updating '%s.%s' secret with new creds. Err:'%s\n", o.account.Spec.ClaimLinkNamespace, secretName, err)
			return err
		}
		o.log.Debugf("Successfully updated secrets for '%s'\n", userName)
	} else {
		// This is a secondary check, and should have also been performed early on.
		// before any intrusive ops are attempted to avoid confusing the end user as
		// to what ops will-be/have-been completed
		o.log.Warnf("account:'%s' is not BYOC/CCS, skipping osdCcsAdmin credential rotation", o.accountID)
		return nil
	}
	return nil
}
