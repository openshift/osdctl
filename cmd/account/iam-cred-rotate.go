package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

	"github.com/openshift-online/ocm-sdk-go/logging"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	credreqv1 "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const cmdName string = "iam-secret-mgmt"
const saveFileCcs = "OsdCcsAdmin.txt"
const saveFileManaged = "OsdManagedAdmin.txt"

func newCmdRotateAWSCreds(streams genericclioptions.IOStreams) *cobra.Command {
	cmdPrefix := fmt.Sprintf(`%s $CLUSTER_ID --reason "$JIRA_TICKET" --aws-profile rhcontrol `, cmdName)
	examples := fmt.Sprintf(`
Basic usage involves selecting a set actions to run:
Choose 1 or more IAM users to rotate: --rotate-managed-admin, --rotate-ccs-admin
-or- 
Choose 1 or more describe actions: --describe-keys, --describe-secrets

# Rotate credentials for IAM user "OsdManagedAdmin" (or user provided by --admin-username)
%s --rotate-managed-admin

# Rotate credentials for special IAM user "OsdCcsAdmin", print secret access key contents to stdout. 
%s --rotate-ccs-admin --output-keys --verbose 4

# Rotate credentials for both users "OsdManagedAdmin" and then "OsdCcsAdmin"
%s --rotate-managed-admin --rotate-ccs-admin --output-keys --verbose 4

# Describe credential-request secrets 
%s --describe-secrets

# Describe AWS Access keys in use by users "OsdManagedAdmin" and "OsdCcsAdmin"
%s --describe-keys 

Describe credential-request secrets and AWS Access keys in use by users "OsdManagedAdmin" and "OsdCcsAdmin"
%s --describe-secrets --describe-keys `, cmdPrefix, cmdPrefix, cmdPrefix, cmdPrefix, cmdPrefix, cmdPrefix)

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
	- An elevation reason (hint: This will usually involve a Jira ticket ID.)  
	- A local AWS profile for the cluster environment (ie: osd-staging, rhcontrol, etc), if not provided 'default' is used. 
	- A valid OCM token and OCM_CONFIG.  
`,
		Example:           examples,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.preRunCliChecks(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	ops.perArgs = rotateAWSCredsCmd.PersistentFlags()
	ops.parentCmd = rotateAWSCredsCmd.Parent()
	rotateAWSCredsCmd.Flags().StringVarP(&ops.profile, "aws-profile", "p", "", "specify AWS profile from local config to use(default='default')")
	rotateAWSCredsCmd.Flags().StringVarP(&ops.reason, "reason", "r", "", "(Required) The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket).")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.updateMgmtCredsCli, "rotate-managed-admin", false, "Rotate osdManagedAdmin user credentials. Interactive. Use caution.")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.updateCcsCredsCli, "rotate-ccs-admin", false, "Rotate osdCcsAdmin user credentials. Interactive. Use caution!")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.describeKeysCli, "describe-keys", false, "Print AWS AccessKey info for osdManagedAdmin and osdCcsAdmin relevant cred rotation, and exit")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.describeSecretsCli, "describe-secrets", false, "Print AWS CredentialRequests ref'd secrets info relecant to cred rotation, and exit")
	rotateAWSCredsCmd.Flags().BoolVar(&ops.saveSecretKeyToFile, "save-keys", false, "Save 'newly created' secret access key contents stdout output during execution")
	rotateAWSCredsCmd.Flags().StringVar(&ops.osdManagedAdminUsername, "admin-username", "", "The admin username to use for generating access keys. Must be in the format of `osdManagedAdmin*`. If not specified, this is inferred from the account CR.")
	rotateAWSCredsCmd.Flags().IntVarP(&ops.verboseLevel, "verbose", "v", 3, "debug=4, (default)info=3, warn=2, error=1")
	// Reason is required for elevated bacplane-admin impersonated requests
	_ = rotateAWSCredsCmd.MarkFlagRequired("reason")

	return rotateAWSCredsCmd
}

// rotateSecretOptions defines the struct for running rotate-iam command
type rotateCredOptions struct {
	// CLI provided params
	perArgs                 *pflag.FlagSet // Parent/global args
	parentCmd               *cobra.Command
	profile                 string // Local AWS profile used to run this script
	reason                  string // Reason used to justify elevate/impersonate ops
	updateCcsCredsCli       bool   // Bool flag to inidcate whether or not to update special AWS user 'osdCcsAdmin' creds.
	updateMgmtCredsCli      bool   // Bool flag to indicate whether or not to update AWS user 'osdManagedAdmin' creds.
	osdManagedAdminUsername string // Name of AWS Managed Admin user. Legacy default values are used if not provided.
	saveSecretKeyToFile     bool   // Allow printing access key secret to debug output
	describeSecretsCli      bool   // Print Cred requests ref'd AWS secrets info and exit
	describeKeysCli         bool   // Print Access Key info and exit
	clusterID               string // Cluster id, user cluster used by account/creds
	awsAccountTimeout       *int32 // Default timeout

	//Runtime attrs
	verboseLevel int             //Logging level
	log          logging.Logger  // Used for logging runtime messages (default stdout)
	ctx          context.Context // Context to user for rotation ops
	genericclioptions.IOStreams

	//AWS runtime attrs
	account              *awsv1alpha1.Account // aws-account-operator account obj
	accountIDSuffixLabel string               // account suffix label
	accountID            string               // AWS account id
	claimName            string               // AWS account claim name custom resource
	secretName           string               // account AWS creds secret
	awsClient            awsprovider.Client   //AWS client used for final access and cred rotation

	//Openshift runtime attrs
	cluster           *cmv1.Cluster         // Cluster object representing user cluster of 'clusterID'
	clusterKubeClient client.Client         // Cluster BP kube client connection
	clusterClientset  *kubernetes.Clientset // Cluster BP Kube ClientSet
	ocmConn           *sdk.Connection       // ocm connection object
	hiveCluster       *cmv1.Cluster         // Hive cluster/shard managing this user cluster
	hiveKubeClient    client.Client         // Hive kube client conneciton

}

// func newRotateAWSOptions(streams genericclioptions.IOStreams, client *k8s.LazyClient) *rotateCredOptions {
func newRotateAWSOptions(streams genericclioptions.IOStreams) *rotateCredOptions {
	return &rotateCredOptions{
		IOStreams: streams,
	}
}

/* Initial function used to validate user input */
func (o *rotateCredOptions) preRunCliChecks(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Required 1 positional arg for 'Cluster ID'")
	}
	o.clusterID = args[0]

	// The aws account timeout. The min the API supports is 15mins (900seconds)
	o.awsAccountTimeout = awsSdk.Int32(900)

	if o.osdManagedAdminUsername != "" && !strings.HasPrefix(o.osdManagedAdminUsername, common.OSDManagedAdminIAM) {
		return cmdutil.UsageErrorf(cmd, fmt.Sprintf("admin-username must start with %v", common.OSDManagedAdminIAM))
	}

	if o.profile == "" {
		o.profile = "default"
	}
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

	if !o.updateCcsCredsCli && !o.updateMgmtCredsCli && !o.describeKeysCli && !o.describeSecretsCli {
		return cmdutil.UsageErrorf(cmd, "must provide one or more actions: ('--rotate-managed-admin' and/or '--rotate-ccs-admin'), or '--describe-secrets', or '--describe-keys'")
	}
	if (o.updateCcsCredsCli || o.updateMgmtCredsCli) && (o.describeKeysCli || o.describeSecretsCli) {
		return cmdutil.UsageErrorf(cmd, "can not combine 'describe*' with 'rotate*' commands")
	}

	if o.saveSecretKeyToFile {
		if o.updateCcsCredsCli && fileExists(saveFileCcs) {
			return fmt.Errorf("--save-keys: file '%s' already present, please move/remove before running", saveFileCcs)
		}
		if o.updateMgmtCredsCli && fileExists(saveFileManaged) {
			return fmt.Errorf("--save-keys: file '%s' already present, please move/remove before running", saveFileManaged)
		}
	}
	// Fail early if aws config is not correct
	_, err = config.LoadDefaultConfig(o.ctx, config.WithSharedConfigProfile(o.profile))
	if err != nil {
		o.log.Error(o.ctx, "Failed to load AWS config:'%v'\n", err)
		return err
	}

	return nil
}

/* Main function used to run this CLI utility */
func (o *rotateCredOptions) run() error {
	var err error = nil
	err = o.preRunSetup()
	if err != nil {
		return err
	}
	// defer the command cleanup...
	defer o.postRunCleanup()

	err = o.printClusterInfo()
	if err != nil {
		return err
	}

	if o.describeSecretsCli {
		// Print info for secrets referenced by AWS provider credentialRequests then exit...
		err = o.printAWSCredRequestSecrets(nil)
		if err != nil {
			return err
		}
		if !o.describeKeysCli {
			return nil
		}
	}

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
	if !o.updateMgmtCredsCli && !o.updateCcsCredsCli {
		// user did not select a rotate operation, this should be caught in pre-run checks
		return nil
	}
	// Display cluster info, let user confirm they are on the correct cluster, etc...
	o.printClusterInfo()
	fmt.Println("Proceed with Managed Admin AWS credentials rotation on this cluster?")
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
		o.log.Info(o.ctx, "ccs cli flag was set. Attempting to update osdCcsAdmin user creds...\n")
		err = o.doRotateCcsCreds()
		if err != nil {
			return err
		}
	}
	//
	err = o.deleteAWSCredRequestSecrets()
	if err != nil {
		o.log.Warn(o.ctx, "fetchCredentialsRequests returned err:'%s'\n", err)
		return err
	}
	fmt.Printf("\nOptions --describe-keys, --describe-secrets can be used to provide additional info/status\n")
	fmt.Printf("Script Run Completed Successfully.\n")
	return nil
}

func fileExists(fpath string) bool {
	_, err := os.Stat(fpath)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// Some very basic 'early' validation on the key input if provided.
// AWS docs, access key constraints: Minimum length of 16. Maximum length of 128.
func (o *rotateCredOptions) isValidAccessKeyId(keyId string) bool {
	if len(keyId) < 16 || len(keyId) > 128 {
		o.log.Error(o.ctx, "Invalid Access key length:%d\n", len(keyId))
		return false
	}
	return true
}

func (o *rotateCredOptions) printClusterInfo() error {
	// currently using account byoc value for ccs sanity checks, is this needed?
	isCCS, err := utils.IsClusterCCS(o.ocmConn, o.clusterID)
	if err != nil {
		return err
	}
	o.log.Debug(o.ctx, "Is Cluster CCS?:%t\n", isCCS)
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintf(w, "\n----------------------------------------------------------------------\n")
	fmt.Fprintf(w, "Cluster ID:\t%s\n", o.clusterID)
	fmt.Fprintf(w, "Cluster External ID:\t%s\n", o.cluster.ExternalID())
	fmt.Fprintf(w, "Cluster Name:\t%s\n", o.cluster.Name())
	fmt.Fprintf(w, "Cluster Is CCS:\t%t\n", isCCS)
	fmt.Fprintf(w, "----------------------------------------------------------------------\n")
	w.Flush()
	return nil
}

// Get list of credentialsRequests with openshift namespaces for AWS provider type.
// The secrets referenced by these credReqs will need to be updated after cred rotation.
func (o *rotateCredOptions) getAWSCredentialsRequests(nameSpace string) ([]credreqv1.CredentialsRequest, error) {
	// Make sure we have a backplane connection to the target cluster...
	if o.clusterKubeClient == nil {
		err := o.connectClusterClient()
		if err != nil {
			return nil, err
		}
	}
	if o.ctx == nil {
		return nil, fmt.Errorf("invalid ctx, nil")
	}
	const AWSProviderSpecType string = "AWSProviderSpec"
	delCredReqList := []credreqv1.CredentialsRequest{}
	var credRequestList credreqv1.CredentialsRequestList
	var listOptions = client.ListOptions{}
	// TODO:!! If 'nameSpace' is not provided query is agaist all namespaces.
	// Limiting this to the 'cloud-credential' op namespace does exclude some relevant secrets.
	// Should this provide a namespace here? ie namespace="openshift-cloud-credential-operator"
	// Original aws-creds-rotate.sh script does provide the -n $namespace,
	// 'but' also provides -A which would negate the provided namespace(?)
	if len(nameSpace) > 0 {
		// ie: listOptions.Namespace = credreqv1.CloudCredOperatorNamespace
		listOptions.Namespace = nameSpace
	}
	err := o.clusterKubeClient.List(o.ctx, &credRequestList, &listOptions)
	if err != nil {
		o.log.Warn(o.ctx, "Error fetching CredentialsRequestList:'%s'\n", err)
		return nil, err
	}

	for _, cr := range credRequestList.Items {
		if strings.HasPrefix(cr.Namespace, "openshift") {
			kindStr, err := getProviderSpecKind(cr)
			if err != nil {
				o.log.Warn(o.ctx, "Skipping cr:'%s', err:'%s'\n", cr.Name, err)
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

// Print metadata for secrets referenced by AWS provider CredentialRequests resource(s)...
func (o *rotateCredOptions) printAWSCredRequestSecrets(awsCredReqs *[]credreqv1.CredentialsRequest) error {
	if awsCredReqs == nil {
		reqs, err := o.getAWSCredentialsRequests("")
		if err != nil {
			o.log.Warn(o.ctx, "Error fetching AWS related credentialRequests to delete secrets, err:'%s'\n", err)
			return err
		}
		awsCredReqs = &reqs
	}
	if awsCredReqs == nil {
		fmt.Println("No AWS provider Credential Requests to show")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintf(w, "--------------------------------------------------------------------------\n")
	fmt.Fprintf(w, "AWS CredentialsRequest referenced secrets:\n")
	fmt.Fprintf(w, "--------------------------------------------------------------------------\n")
	for ind, cr := range *awsCredReqs {
		secret, err := o.clusterClientset.CoreV1().Secrets(cr.Spec.SecretRef.Namespace).Get(context.TODO(), cr.Spec.SecretRef.Name, metav1.GetOptions{})
		if err != nil {
			o.log.Warn(o.ctx, "Failed to get secret:'%s', err:'%s'", cr.Spec.SecretRef.Name, err)
			continue
		}
		fmt.Fprintf(w, "(%d)\tCredReq:'%s'\tNS:'%s'\tSecret:'%s'\tCreated:'%v'\n", ind, cr.Name, secret.Namespace, secret.Name, secret.CreationTimestamp)
	}
	fmt.Fprintf(w, "--------------------------------------------------------------------------\n")
	w.Flush()
	return nil
}

// #1 Print secrets to be deleted and CredentialRequest objs which reference them.
// #2 Prompt user after printing, ask to continue.
// #3 Prompt User per secret before deleting.
func (o *rotateCredOptions) deleteAWSCredRequestSecrets() error {
	awsCredReqs, err := o.getAWSCredentialsRequests("")
	if err != nil {
		o.log.Warn(o.ctx, "Error fetching AWS related credentialRequests to delete secrets, err:'%s'\n", err)
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
	fmt.Println("Are you sure you want to continue?")
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
			o.log.Info(o.ctx, "User chose to not delete secrets...\n")
			return nil
		case "all":
			break inputLoop
		default:
			fmt.Println("Invalid input. Expecting (y)es or (N)o or 'all'")
		}
	}
	if userInput == "y" || userInput == "all" {
		var idx = 0
		for sidx, cr := range awsCredReqs {
			idx = sidx + 1
			secret, err := o.clusterClientset.CoreV1().Secrets(cr.Spec.SecretRef.Namespace).Get(context.TODO(), cr.Spec.SecretRef.Name, metav1.GetOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting secret:'%s', skipping\n", secret.Name)
				continue
			}
			fmt.Fprintf(w, "\nSecret (%d/%d):\n", idx, len(awsCredReqs))
			fmt.Fprintf(w, "\tCredentialRequest:\t'%s'\n", cr.Name)
			fmt.Fprintf(w, "\tSecret:\t'%s'\n", secret.Name)
			fmt.Fprintf(w, "\tSecret Namespace:\t'%s'\n", secret.Namespace)
			fmt.Fprintf(w, "\tCreate:\t'%v'\n", secret.CreationTimestamp)
			w.Flush()
			fmt.Printf("Delete Secret ('%s'), ", secret.Name)
			if userInput == "all" || utils.ConfirmPrompt() {
				o.log.Debug(o.ctx, "Deleting secret(%d/%d):'%s'\n", cr.Spec.SecretRef.Name, idx, len(awsCredReqs))
				// Delete the referenced secret
				err := o.clusterClientset.CoreV1().Secrets(secret.Namespace).Delete(context.TODO(), secret.Name, metav1.DeleteOptions{})
				if err != nil {
					return fmt.Errorf("failed to delete secret '%v' in namespace:'%v', err '%w'", secret.Name, secret.Namespace, err)
				}
				o.log.Debug(o.ctx, "Deleted Secret: '%s'\n", secret.Name)
			} else {
				o.log.Debug(o.ctx, "Skipping secret:'%s'...\n", secret.Name)
				continue
			}
		}
	} else {
		o.log.Info(o.ctx, "User chose to not delete secrets...\n")
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
	o.log.Debug(o.ctx, "Creating OCM connection...\n")
	if o.ocmConn == nil {
		o.ocmConn, err = utils.CreateConnection()
		if err != nil {
			return fmt.Errorf("failed to create OCM client: %w", err)
		}
	}
	// Fetch the cluster info, this will return an error if more than 1 cluster is matched
	cluster, err := utils.GetClusterAnyStatus(o.ocmConn, o.clusterID)
	if err != nil {
		return fmt.Errorf("failed to fetch cluster:'%s' from OCM: %w", o.clusterID, err)
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
	return nil
}

/* Command cleanup */
func (o *rotateCredOptions) postRunCleanup() error {
	// Close ocm connection if created...
	if o.ocmConn != nil {
		ocmCloseErr := o.ocmConn.Close()
		if ocmCloseErr != nil {
			return fmt.Errorf("error during ocm.close() (possible memory leak): %q", ocmCloseErr)
		}
		o.ocmConn = nil
	}
	return nil
}

/* populate aws account claim name info */
func (o *rotateCredOptions) fetchAwsClaimNameInfo() error {
	var err error = nil
	var claimName string = ""
	accountClaim, err := k8s.GetAccountClaimFromClusterID(o.ctx, o.hiveKubeClient, o.clusterID)
	if err != nil {
		o.log.Warn(o.ctx, "k8s.GetAccountClaimFromClusterID err:'%s', trying resources instead...\n", err)
	} else {
		o.log.Debug(o.ctx, "Got AccountClaimFromClusterID, name:'%s', accountlink:'%s'\n", accountClaim.Name, accountClaim.Spec.AccountLink)
		if accountClaim.Spec.AccountLink != "" {
			o.claimName = accountClaim.Spec.AccountLink
			o.secretName = o.claimName + "-secret"
			return nil
		} else {
			o.log.Warn(o.ctx, "accountClaim.Spec.AccountLink contained empty string, trying resources instead...\n")
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
 * used as a backup to 'k8s.GetAccountClaimFromClusterID()'
 */
func (o *rotateCredOptions) getResourcesClaimName() (string, error) {
	o.claimName = ""
	if o.ocmConn == nil {
		err := o.preRunSetup()
		if err != nil {
			return "", err
		}
	}
	o.ocmConn.ClustersMgmt().V1().AWSInquiries().STSCredentialRequests().List().Send()
	liveResponse, err := o.ocmConn.ClustersMgmt().V1().Clusters().Cluster(o.clusterID).Resources().Live().Get().Send()
	if err != nil {
		return "", fmt.Errorf("error fetching cluster resources: %w", err)
	}
	respBody := liveResponse.Body().Resources()
	if awsAccountClaim, ok := respBody["aws_account_claim"]; ok {
		var claimJson map[string]interface{}
		err := json.Unmarshal([]byte(awsAccountClaim), &claimJson)
		if err != nil {
			return "", fmt.Errorf("err parsing account claim: %w", err)
		}
		if metaData, ok := claimJson["spec"]; ok {
			if rawClaimName, ok := metaData.(map[string]interface{})["accountLink"]; ok {
				claimName, ok := rawClaimName.(string)
				if !ok {
					return "", fmt.Errorf("parsed account claim metadata name is not a string")
				}
				o.claimName = claimName
				o.log.Info(o.ctx, "Got Cluster AWS Claim:%s\n", o.claimName)
				return claimName, nil
			}
		}
		return "", fmt.Errorf("unable to get account claim name from JSON")
	}
	return "", fmt.Errorf("cluster does not have AccountClaim")
}

func (o *rotateCredOptions) connectClusterClient() error {
	o.log.Info(o.ctx, "Connect BP client cluster:'%s'\n", o.clusterID)
	if len(o.reason) <= 0 {
		return fmt.Errorf("accessing cred secrets requires a reason for elevated command")
	}
	// This action requires elevation
	elevationReasons := []string{}
	elevationReasons = append(elevationReasons, o.reason)
	elevationReasons = append(elevationReasons, fmt.Sprintf("Elevation required to rotate secrets '%s' aws-account-cr-name", o.claimName))

	// If some elevationReasons are provided, then the config will be elevated with user backplane-cluster-admin...
	// Fetch the backplane kubeconfig, ignore the returned kubecli for now...
	_, kubeConfig, kubClientSet, err := common.GetKubeConfigAndClient(o.clusterID, elevationReasons...)
	if err != nil {
		o.log.Warn(o.ctx, "Err creating cluster:'%s' client, GetKubeConfigAndClient() err:'%+v'\n", o.clusterID, err)
		return err
	}
	o.clusterClientset = kubClientSet
	// Create new kube client with the CredentialsRequest schema using the backplane config from above...
	scheme := runtime.NewScheme()
	err = credreqv1.AddToScheme(scheme)
	if err != nil {
		o.log.Warn(o.ctx, "Error adding CredentialsRequests to schema:'%s'\n", err)
		return err
	}
	kubeCli, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		o.log.Warn(o.ctx, "Error creating new schema'd client for cluster:'%s', err:'%s'\n", o.clusterID, err)
		return err
	}
	o.clusterKubeClient = kubeCli
	return nil
}

/* fetch and connect to hive cluster for the provided user cluster */
func (o *rotateCredOptions) connectHiveClient() error {
	// This requires a valid ocm login token on 'prod'.
	// TODO: Can this differ from the login token needed earlier if the cluster
	// lives in another env (ie integration or staging) (?) If so this needs to allow a 2 step process with
	// multiple ocm login tokens(?), 1 for each env.
	// Logging into Hive Shard here (equiv of 'ocm backplane login $HIVESHARD')
	// Is specifially creating the hive connection here a better/worse option than connecting via 'ocm backplane'
	// externally, and using the LazyClient (ie o.kubeCli) instead?
	// When using the lazyClient, before running this osdctl command, the following was needed:
	//  # HIVESHARD=$(ocm get /api/clusters_mgmt/v1/clusters/$INTERNAL_ID/provision_shard | jq -r '.hive_config.server' | sed 's/.*api\.//;s/\..*//')
	//  (ie HIVESHARD="hives02ue1")
	//  # ocm backplane login $HIVESHARD
	//  # osdctl account $cred_rotate_cmd $options....
	//
	o.log.Info(o.ctx, "Connect hive client for cluster:'%s'\n", o.clusterID)
	if len(o.reason) <= 0 {
		return fmt.Errorf("accessing AWS credentials requires a reason for elevated command")
	}
	hiveCluster, err := utils.GetHiveCluster(o.clusterID)
	if err != nil {
		o.log.Warn(o.ctx, "error fetching hive for cluster:'%s'.\n", err)
		return err
	}
	o.hiveCluster = hiveCluster
	if o.hiveCluster == nil || o.hiveCluster.ID() == "" {
		return fmt.Errorf("failed to fetch hive cluster ID")
	}
	o.log.Info(o.ctx, "Using Hive Cluster: '%s'\n", hiveCluster.ID())
	// This action requires elevation
	elevationReasons := []string{}
	elevationReasons = append(elevationReasons, o.reason)
	elevationReasons = append(elevationReasons, fmt.Sprintf("Elevation required to rotate secrets '%s' aws-account-cr-name", o.claimName))
	// If some elevationReasons are provided, then the config will be elevated with user backplane-cluster-admin...
	hiveKubeCli, _, _, err := common.GetKubeConfigAndClient(o.hiveCluster.ID(), elevationReasons...)
	if err != nil {
		o.log.Warn(o.ctx, "Err fetching hive cluster client, GetKubeConfigAndClient() err:'%+v'\n", err)
		return err
	}
	o.hiveKubeClient = hiveKubeCli
	return nil
}

func (o *rotateCredOptions) fetchAWSAccountInfo() error {
	if len(o.claimName) <= 0 {
		return fmt.Errorf("fetchAWSAccountInfo() empty claim name field found. Can not fetch AWS info without it")
	}

	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
	o.log.Debug(o.ctx, "Fetching account info per aws-account-operator\n")
	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")

	// Get the associated Account CR from the provided name
	o.log.Debug(o.ctx, "Fetching aws-account-operator account object for '%s.%s' ...", common.AWSAccountNamespace, o.claimName)
	accountObj, err := k8s.GetAWSAccount(o.ctx, o.hiveKubeClient, common.AWSAccountNamespace, o.claimName)
	if err != nil {
		o.log.Warn(o.ctx, "k8s.GetAWSAccount() err:'%s'\n", err)
		return err
	}

	if accountObj.Spec.ManualSTSMode {
		return fmt.Errorf("account %s is manual STS mode - No IAM User Credentials to Rotate", o.claimName)
	}
	if !accountObj.Spec.BYOC && o.updateCcsCredsCli {
		// Check for specifics early? ...or should this just be ignored and a no-op later?
		o.log.Warn(o.ctx, "arg '--rotate-ccs-admin' provided to rotate osdCcsAdmin creds on non-CCS cluster id:'%s', name:'%s'\n. No changes were made to this cluster\n", o.cluster.Name(), o.clusterID)
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
	o.log.Debug(o.ctx, "AccountID:'%s', iamUserId label:'%s' \n", o.accountID, o.accountIDSuffixLabel)
	return nil
}

/* Do the actual aws and hive operations needed to rotate the aws credentials...
 * - Fetch creds needed for AWS access, create AWS client
 * - Validate AWS user and number of existing keys
 * - Create new keys, rotate hive secret contents.
 */
func (o *rotateCredOptions) setupAwsClient() error {
	var err error

	if len(o.accountIDSuffixLabel) <= 0 {
		return fmt.Errorf("doAwsCredRoteate() empty required accountIDSuffixLabel")
	}
	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
	o.log.Debug(o.ctx, " AWS setup access, local config:'%s'\n", o.profile)
	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")

	o.log.Debug(o.ctx, "Creating 'AWS-Initial-setup' client using local aws profile: '%s'...\n", o.profile)
	// Since this is only using "IAM", hard coding to us-east-1 region should be ok...
	awsSetupClient, err := awsprovider.NewAwsClient(o.profile, "us-east-1", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create initial AWS client. Check local AWS config/profile:'%s', connection, etc?\n", o.profile)
		return err
	}

	// Ensure AWS calls are successful with client
	callerIdentityOutput, err := awsSetupClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}
	o.log.Debug(o.ctx, "STS calleridentity Account:'%s', userid:'%s' \n", *callerIdentityOutput.Account, *callerIdentityOutput.UserId)

	var credentials *stsTypes.Credentials
	// Need to role chain if the cluster is CCS
	if o.account.Spec.BYOC {
		o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
		o.log.Debug(o.ctx, "Cluster is BYOC, CCS. Begin AWS role chaining...\n")
		o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
		// Get the aws-account-operator configmap
		cm := &corev1.ConfigMap{}
		o.log.Debug(o.ctx, "Fetching SRE CCS Access ARN, and Support Jump ARN from config map: '%s.%s' \n", common.AWSAccountNamespace, common.DefaultConfigMap)
		//cmErr := o.kubeCli.Get(context.TODO(), types.NamespacedName{Namespace: common.AWSAccountNamespace, Name: common.DefaultConfigMap}, cm)
		cmErr := o.hiveKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: common.AWSAccountNamespace, Name: common.DefaultConfigMap}, cm)
		if cmErr != nil {
			o.log.Warn(o.ctx, "error getting ConfigMap:'%s'.'%s' needed for SRE Access Role. err: %s", common.AWSAccountNamespace, common.DefaultConfigMap, cmErr)
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
		o.log.Debug(o.ctx, "Using 'AWS-Initial-setup' client to fetch assume role creds for 'SRE Access ARN'...\n")
		// Fetch assumed SRE Access role creds
		srepRoleCredentials, err := awsprovider.GetAssumeRoleCredentials(awsSetupClient, o.awsAccountTimeout, callerIdentityOutput.UserId, &SREAccessARN)
		if err != nil {
			return err
		}
		o.log.Debug(o.ctx, "Creating new 'AWS-SRE-Access' client using assumed SRE access role creds...\n")
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
		o.log.Debug(o.ctx, "Using 'AWS-SRE-Access' client to fetch assume role creds for 'Support Jump Role ARN'...\n")
		jumpRoleCreds, err := awsprovider.GetAssumeRoleCredentials(srepRoleClient, o.awsAccountTimeout, callerIdentityOutput.UserId, &JumpARN)
		if err != nil {
			return err
		}
		o.log.Debug(o.ctx, "Creating new 'AWS-Support-Jump-Role' client using assumed Support Jump role creds...\n")
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
		o.log.Debug(o.ctx, "Using 'AWS-Support-Jump-Role' client to fetch creds using ManagedOpenShift-support role ARN...\n")
		// Role chain to assume ManagedOpenShift-Support-{uid}
		roleArn := awsSdk.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", o.accountID, "ManagedOpenShift-Support-"+o.accountIDSuffixLabel))
		credentials, err = awsprovider.GetAssumeRoleCredentials(jumpRoleClient, o.awsAccountTimeout,
			callerIdentityOutput.UserId, roleArn)
		if err != nil {
			return err
		}

	} else {
		o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
		o.log.Debug(o.ctx, "Cluster is 'NOT' BYOC, CCS. Begin AWS role chaining...\n")
		o.log.Debug(o.ctx, "Using 'AWS Initial setup' client to fetch creds using '%s' ARN...\n", awsv1alpha1.AccountOperatorIAMRole)
		o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")

		// Assume the OrganizationAdminAccess role
		roleArn := awsSdk.String(fmt.Sprintf("arn:aws:iam::%s:role/%s", o.accountID, awsv1alpha1.AccountOperatorIAMRole))
		credentials, err = awsprovider.GetAssumeRoleCredentials(awsSetupClient, o.awsAccountTimeout,
			callerIdentityOutput.UserId, roleArn)
		if err != nil {
			return err
		}
	}

	// Build a new client with the assumed role credentials...
	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
	o.log.Debug(o.ctx, "Creating final AWS client for cred rotation with assumed role...\n")
	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
	awsClient, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		Region:          "us-east-1",
	})
	if err != nil {
		return err
	}
	o.log.Debug(o.ctx, "Created final AWS client\n")
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
		o.log.Warn(o.ctx, "Failed to fetch current aws account creds from secret, err:'%s'\n", err)
		o.log.Warn(o.ctx, "May require manual check of secret to determine access key in use?\n")
	}
	return currKeys, clientInput, nil
}

func (o *rotateCredOptions) printKeyInfo(currKeys *iam.ListAccessKeysOutput, clientInput *awsprovider.ClientInput, iamUser string) error {
	if currKeys == nil || clientInput == nil {
		return fmt.Errorf("nil input passed to printKeyInfo()")
	}
	var inUseStr string = "unknown?"
	p := printer.NewTablePrinter(o.IOStreams.Out, 3, 1, 3, ' ')
	headers := []string{"#", "ACCESS-KEY-ID", "CR-IN-USE", "CREATED", "STATUS", "IAM-USER"}
	p.AddRow(headers)

	fmt.Printf("\n---------------------------------------------------------------------------------------------------------------\n")
	fmt.Printf("Access Key Info, IAM user:'%s', num of keys:'%d' of max 2\n", iamUser, len(currKeys.AccessKeyMetadata))
	fmt.Printf("Account CR in-use accessKeyID:'%s'\n", clientInput.AccessKeyID)
	fmt.Printf("---------------------------------------------------------------------------------------------------------------\n")
	for Ind, Akey := range currKeys.AccessKeyMetadata {
		if Akey.AccessKeyId == nil {
			return fmt.Errorf("accessKey *id nil in List AccessKeysOutput provided to printKeyInfo()")
		}
		if len(clientInput.AccessKeyID) > 0 {
			inUseStr = fmt.Sprintf("%t", clientInput.AccessKeyID == *Akey.AccessKeyId)
		} else {
			inUseStr = "unknown?"
		}
		row := []string{fmt.Sprintf("%d", Ind), *Akey.AccessKeyId, inUseStr, Akey.CreateDate.String(), string(Akey.Status), *Akey.UserName}
		p.AddRow(row)
	}
	err := p.Flush()
	fmt.Printf("---------------------------------------------------------------------------------------------------------------\n")
	return err
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
	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
	o.log.Debug(o.ctx, "Checking user:'%s' existing access-keys...\n", iamUser)
	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
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
		o.log.Warn(o.ctx, "user:'%s' already has max number of access keys:'%d'\n", iamUser, len(currKeys.AccessKeyMetadata))

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
						o.log.Error(o.ctx, "(attempt %d/%d) Invalid access key ID entered:'%s'", attempt, max_attempts, keyInput)
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
				o.log.Info(o.ctx, "Attempting to delete AccessKey:%s\n", deleteKeyID)
				_, err = o.awsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{UserName: awsSdk.String(iamUser), AccessKeyId: awsSdk.String(deleteKeyID)})
				if err != nil {
					o.log.Error(o.ctx, "Error attempting to delete IAM user:'%s' accesskey:'%s'. Err:'%s'\n", iamUser, deleteKeyID, err)
					return err
				} else {
					o.log.Info(o.ctx, "Delete AccessKey:'%s', Success\n", deleteKeyID)
					deleteSuccess = true
				}
			} else {
				o.log.Warn(o.ctx, "IAM user:'%s', AccessKey:'%s', not found in iam.ListAccessKeysOutput\n", iamUser, deleteKeyID)
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
	o.log.Debug(o.ctx, "Check if AWS user '%s' exists\n", osdManagedAdminUsername)
	checkUser, err := o.awsClient.GetUser(&iam.GetUserInput{UserName: awsSdk.String(osdManagedAdminUsername)})
	var nse *iamTypes.NoSuchEntityException
	if (err != nil && errors.As(err, &nse)) || checkUser == nil {
		o.log.Warn(o.ctx, "User Not Found: '%s', trying user:'%s' instead...\n", osdManagedAdminUsername, common.OSDManagedAdminIAM)
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

func (o *rotateCredOptions) doRotateManagedAdminAWSCreds() error {
	if o.awsClient == nil {
		return fmt.Errorf("AWS client has not been created yet for doRotateAWSCreds()")
	}
	// Update osdManagedAdmin secrets
	osdManagedAdminUsername, err := o.checkOsdManagedUsername()
	if err != nil {
		return err
	}
	o.log.Info(o.ctx, "Using osdManagedAdminUsername: '%s' \n", osdManagedAdminUsername)
	// Check for max keys before attempting to create, use user provided input to delete a key if needed...
	err = o.checkAccessKeysMaxDelete(osdManagedAdminUsername, common.AWSAccountNamespace, o.secretName, "")
	if err != nil {
		return err
	}

	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")
	o.log.Debug(o.ctx, "Creating new access key for '%s'\n", osdManagedAdminUsername)
	o.log.Debug(o.ctx, "----------------------------------------------------------------------\n")

	o.log.Info(o.ctx, "Creating new access key...\n")
	createAccessKeyOutput, err := o.awsClient.CreateAccessKey(&iam.CreateAccessKeyInput{UserName: awsSdk.String(osdManagedAdminUsername)})
	if err != nil {
		o.log.Warn(o.ctx, "Failed to create access key for user:'%s'\n", osdManagedAdminUsername)
		return err
	}

	fmt.Printf("New creds:\nuser:'%s'\nAccessKey:'%s'\n", *createAccessKeyOutput.AccessKey.UserName, *createAccessKeyOutput.AccessKey.AccessKeyId)
	// Todo: Make displaying the secret a cli provided arg? If this is useful for error/script failure recovery?
	if o.saveSecretKeyToFile {
		err := os.WriteFile(saveFileManaged, []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey), 0600)
		if err != nil {
			o.log.Error(o.ctx, "Error! Saving key info to file:'%s', err:%s", saveFileManaged, err)
			//return err
		} else {
			fmt.Printf("\nSaved key to: '%s'\n", saveFileManaged)
		}
	}
	// Place new credentials into body for secret
	newOsdManagedAdminSecretData := map[string][]byte{
		"aws_user_name":         []byte(*createAccessKeyOutput.AccessKey.UserName),
		"aws_access_key_id":     []byte(*createAccessKeyOutput.AccessKey.AccessKeyId),
		"aws_secret_access_key": []byte(*createAccessKeyOutput.AccessKey.SecretAccessKey),
	}

	// Update existing osdManagedAdmin secret
	// UpdateSecret() includes a check for existing secret
	o.log.Info(o.ctx, "Updating osdManagedAdmin secret:'%s.%s' with new AWS cred values...\n", common.AWSAccountNamespace, o.secretName)
	err = common.UpdateSecret(o.hiveKubeClient, o.secretName, common.AWSAccountNamespace, newOsdManagedAdminSecretData)
	if err != nil {
		return err
	}

	// Update secret in ClusterDeployment's namespace
	o.log.Info(o.ctx, "Updating ClusterDeployment secret:'%s.aws' with new AWS cred values...\n", o.account.Spec.ClaimLinkNamespace)
	err = common.UpdateSecret(o.hiveKubeClient, "aws", o.account.Spec.ClaimLinkNamespace, newOsdManagedAdminSecretData)
	if err != nil {
		return err
	}

	o.log.Debug(o.ctx, "---------------------------------------------------------\n")
	o.log.Info(o.ctx, "AWS creds updated on hive.\n")
	o.log.Debug(o.ctx, "---------------------------------------------------------\n")

	o.log.Info(o.ctx, "Begin syncset ops to sync to cluster....\n")

	o.log.Debug(o.ctx, "Fetching cluster deployments list from hive...\n")
	clusterDeployments := &hiveapiv1.ClusterDeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(o.account.Spec.ClaimLinkNamespace),
	}

	err = o.hiveKubeClient.List(o.ctx, clusterDeployments, listOpts...)
	if err != nil {
		return err
	}

	if len(clusterDeployments.Items) == 0 {
		return fmt.Errorf("failed to retreive cluster deployments")
	}
	cdName := clusterDeployments.Items[0].ObjectMeta.Name
	o.log.Debug(o.ctx, "Got cluster deployment name:'%s'\n", cdName)

	const syncSetName = "aws-sync"
	syncSet := &hiveapiv1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      syncSetName,
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
	o.log.Info(o.ctx, "Creating syncset:'%s.%s', to deploy the updated creds to the cluster for CCO...\n", o.account.Spec.ClaimLinkNamespace, syncSetName)
	o.log.Debug(o.ctx, "Syncing AWS creds down to cluster.")
	err = o.hiveKubeClient.Create(o.ctx, syncSet)
	if err != nil {
		return err
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
			return err
		}
		elapsed := time.Since(start)
		for _, status := range foundStatus.Status.SyncSets {
			if status.Name == syncSetName {
				if status.FirstSuccessTime != nil {
					o.log.Debug(o.ctx, "Syncset '%s' first success time set, elapsed:'%s'\n", syncSetName, elapsed)
					isSSSynced = true
					break
				}
				if status.FailureMessage != "" {
					o.log.Info(o.ctx, "Found SyncSet:'%s' current status failureMessage:'%s'\n", syncSetName, status.FailureMessage)
				}
			}
		}

		if isSSSynced {
			o.log.Info(o.ctx, "\nSync '%s' completed. Elapsed:'%s'\n", syncSetName, time.Since(start))
			break
		}

		fmt.Printf(".")
		time.Sleep(time.Second * 5)
	}
	if !isSSSynced {
		elapsed := time.Since(start)
		return fmt.Errorf("syncset failed to sync after elapsed:'%s'. Please verify manually", elapsed)
	}

	o.log.Debug(o.ctx, "Clean up the SS on hive...\n")
	err = o.hiveKubeClient.Delete(o.ctx, syncSet)
	if err != nil {
		o.log.Warn(o.ctx, "Error deleting syncset:'%s.%s' on hive: '%s'\n", syncSet.Namespace, syncSet.Name, err)
		return err
	}

	o.log.Info(o.ctx, "Successfully rotated secrets for user:'%s'\n", osdManagedAdminUsername)
	return nil
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

	o.log.Debug(o.ctx, "----------------------------------------------------------------------------\n")
	o.log.Info(o.ctx, "ccs cli flag was set. Attempting to update '%s' user creds...\n", userName)
	o.log.Debug(o.ctx, "----------------------------------------------------------------------------\n")
	// Only update if the Account CR is actually CCS
	if o.account.Spec.BYOC {

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

		newOsdCcsAdminSecretData := map[string][]byte{
			"aws_user_name":         []byte(*createAccessKeyOutputCCS.AccessKey.UserName),
			"aws_access_key_id":     []byte(*createAccessKeyOutputCCS.AccessKey.AccessKeyId),
			"aws_secret_access_key": []byte(*createAccessKeyOutputCCS.AccessKey.SecretAccessKey),
		}
		fmt.Printf("Created '%s' creds: UserName:'%s', AccessKey:'%s'\n", userName, *createAccessKeyOutputCCS.AccessKey.UserName, *createAccessKeyOutputCCS.AccessKey.AccessKeyId)
		// Make displaying the secret a cli provided arg? If this is useful for error/script failure recovery?
		if o.saveSecretKeyToFile {
			err := os.WriteFile(saveFileCcs, []byte(*createAccessKeyOutputCCS.AccessKey.SecretAccessKey), 0600)
			if err != nil {
				o.log.Error(o.ctx, "Error! Saving key info to file:'%s', err:'%s'", saveFileCcs, err)
				//return err
			} else {
				fmt.Printf("\nSaved key to: '%s'\n", saveFileCcs)
			}
		}
		o.log.Debug(o.ctx, "Updating '%s.%s' secret with new creds...\n", o.account.Spec.ClaimLinkNamespace, secretName)
		err = common.UpdateSecret(o.hiveKubeClient, secretName, o.account.Spec.ClaimLinkNamespace, newOsdCcsAdminSecretData)
		if err != nil {
			o.log.Warn(o.ctx, "Error updating '%s.%s' secret with new creds. Err:'%s\n", o.account.Spec.ClaimLinkNamespace, secretName, err)
			return err
		}
		o.log.Debug(o.ctx, "Successfully updated secrets for '%s'\n", userName)
	} else {
		// Thi is a secondary check, and should have also been performed early on.
		// before any intrusive ops are attempted to avoid confusing the end user as
		// to what ops will-be/have-been completed
		o.log.Warn(o.ctx, "account:'%s' is not BYOC/CCS, skipping osdCcsAdmin credential rotation", o.accountID)
		return nil
	}
	return nil
}
