package support

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	slv1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/openshift/osdctl/internal/utils"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

const (
	LimitedSupportSummaryCluster                         = "Cluster is in Limited Support due to unsupported cluster configuration"
	LimitedSupportSummaryCloud                           = "Cluster is in Limited Support due to unsupported cloud provider configuration"
	MisconfigurationFlag                                 = "misconfiguration"
	cloud                         MisconfigurationReason = "cloud"
	cluster                       MisconfigurationReason = "cluster"
	ProblemFlag                                          = "problem"
	ResolutionFlag                                       = "resolution"
	EvidenceFlag                                         = "evidence"
	InternalServiceLogSeverity                           = "Warning"
	InternalServiceLogServiceName                        = "SREManualAction"
	InternalServiceLogSummary                            = "LimitedSupportEvidence"
)

type Post struct {
	Template         string
	TemplateParams   []string
	Misconfiguration MisconfigurationReason
	Problem          string
	Resolution       string
	Evidence         string
	cluster          *cmv1.Cluster
}

type TemplateFile struct {
	Severity       string             `json:"severity"`
	Summary        string             `json:"summary"`
	Log_type       string             `json:"log_type"`
	Details        string             `json:"details"`
	Detection_type cmv1.DetectionType `json:"detection_type"`
}

var (
	userParameterNames, userParameterValues []string
)

func newCmdpost() *cobra.Command {
	p := &Post{}

	postCmd := &cobra.Command{
		Use:   "post CLUSTER_ID",
		Short: "Send limited support reason to a given cluster",
		Long: `Sends limited support reason to a given cluster, along with an internal service log detailing why the cluster was placed into limited support.
The caller will be prompted to continue before sending the limited support reason.`,
		Example: `# Post a limited support reason for a cluster misconfiguration
osdctl cluster support post 1a2B3c4DefghIjkLMNOpQrSTUV5 --misconfiguration cluster --problem="The cluster has a second failing ingress controller, which is not supported and can cause issues with SLA." \
--resolution="Remove the additional ingress controller 'my-custom-ingresscontroller'. 'oc get ingresscontroller -n openshift-ingress-operator' should yield only 'default'" \
--evidence="See OHSS-1234"

Will result in the following limited-support text sent to the customer:
The cluster has a second failing ingress controller, which is not supported and can cause issues with SLA. Remove the additional ingress controller 'my-custom-ingresscontroller'. 'oc get ingresscontroller -n openshift-ingress-operator' should yield only 'default'.
`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := p.Run(args[0]); err != nil {
				return fmt.Errorf("error posting limited support reason: %w", err)
			}
			return nil
		},
	}

	// Define required flags
	postCmd.Flags().StringVarP(&p.Template, "template", "t", "", "Message template file or URL")
	postCmd.Flags().StringArrayVarP(&p.TemplateParams, "param", "p", p.TemplateParams, "Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.")
	postCmd.Flags().Var(&p.Misconfiguration, MisconfigurationFlag, "The type of misconfiguration responsible for the cluster being placed into limited support. Valid values are `cloud` or `cluster`.")
	postCmd.Flags().StringVar(&p.Problem, ProblemFlag, "", "Complete sentence(s) describing the problem responsible for the cluster being placed into limited support. Will form the limited support message with the contents of --resolution appended")
	postCmd.Flags().StringVar(&p.Resolution, ResolutionFlag, "", "Complete sentence(s) describing the steps for the customer to take to resolve the issue and move out of limited support. Will form the limited support message with the contents of --problem prepended")
	postCmd.Flags().StringVar(&p.Evidence, EvidenceFlag, "", "(optional) The reasoning that led to the decision to place the cluster in limited support. Can also be a link to a Jira case. Used for internal service log only.")
	return postCmd
}

func (p *Post) Init() error {
	userParameterNames = []string{}
	userParameterValues = []string{}
	return nil
}

func (p *Post) setup() error {
	switch p.Problem[len(p.Problem)-1:] {
	case ".", "?", "!":
		return errors.New("--problem should not end in punctuation")
	}
	switch p.Resolution[len(p.Resolution)-1:] {
	case ".", "?", "!":
		return errors.New("--resolution should not end in punctuation")
	}
	return nil
}

func validateResolutionString(res string) error {
	if res[len(res)-1:] == "." {
		return errors.New("resolution string should not end with a `.` as it is already added in the email template")
	}
	return nil
}

func (p *Post) check() error {
	if p.Template != "" {
		if p.Problem != "" || p.Resolution != "" || p.Misconfiguration != "" || p.Evidence != "" {
			return fmt.Errorf("\nIf Template flag is present, --problem, --resolution, --misconfiguration and --evidence flags cannot be used")
		}
	} else {
		if p.Problem == "" || p.Resolution == "" || p.Misconfiguration == "" {
			return fmt.Errorf("\nIn the absence of Template -t flag, --problem, --resolution and --misconfiguration flags are mandatory")
		}
		if err := validateResolutionString(p.Resolution); err != nil {
			return err
		}
		if err := p.setup(); err != nil {
			return err
		}
	}
	return nil
}

func (p *Post) Run(clusterID string) error {
	if err := p.Init(); err != nil {
		return err
	}

	if err := p.check(); err != nil {
		return err
	}

	// Check that the cluster key (name, identifier or external identifier) given by the user
	// is reasonably safe so that there is no risk of SQL injection
	if err := ctlutil.IsValidClusterKey(clusterID); err != nil {
		return err
	}

	connection, err := ctlutil.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		if err = connection.Close(); err != nil {
			fmt.Printf("Cannot close the connection: %q\n", err)
			os.Exit(1)
		}
	}()

	p.cluster, err = ctlutil.GetCluster(connection, clusterID)
	if err != nil {
		return fmt.Errorf("can't retrieve cluster: %w", err)
	}
	var limitedSupport *cmv1.LimitedSupportReason
	if p.Template != "" {
		limitedSupport, err = p.buildLimitedSupportTemplate()
		if err != nil {
			return err
		}
	} else {
		limitedSupport, err = p.buildLimitedSupport()
		if err != nil {
			return err
		}
	}

	fmt.Printf("The following limited support reason will be sent to %s:\n", clusterID)
	if err = printLimitedSupportReason(limitedSupport); err != nil {
		return fmt.Errorf("failed to print limited support reason template: %w", err)
	}

	if !ctlutil.ConfirmPrompt() {
		return nil
	}

	postLimitedSupportResponse, err := sendLimitedSupportPostRequest(connection, p.cluster.ID(), limitedSupport)
	if err != nil {
		return fmt.Errorf("failed to post limited support reason: %w", err)
	}
	fmt.Printf("Successfully added new limited support reason with ID %v\n", postLimitedSupportResponse.Body().ID())

	if p.Evidence != "" {
		var subscriptionId string
		if subscription, ok := p.cluster.GetSubscription(); ok {
			subscriptionId = subscription.ID()
		}
		log, err := p.buildInternalServiceLog(postLimitedSupportResponse.Body().ID(), subscriptionId)
		if err != nil {
			return err
		}

		fmt.Printf("Sending the following internal service log to %s:\n", clusterID)
		if err = printInternalServiceLog(log); err != nil {
			return fmt.Errorf("failed to print internal service log template: %w", err)
		}

		postServiceLogResponse, err := sendInternalServiceLogPostRequest(connection, log)
		if err != nil {
			return fmt.Errorf("failed to post internal service log: %w", err)
		}
		fmt.Printf("Successfully sent internal service log with ID %v\n", postServiceLogResponse.Body().ID())
	}

	return nil
}

func (p *Post) buildLimitedSupport() (*cmv1.LimitedSupportReason, error) {
	limitedSupportBuilder := cmv1.NewLimitedSupportReason().
		Details(fmt.Sprintf("%s %s", p.Problem, p.Resolution)).
		DetectionType(cmv1.DetectionTypeManual)
	switch p.Misconfiguration {
	case cloud:
		limitedSupportBuilder.Summary(LimitedSupportSummaryCloud)
	case cluster:
		limitedSupportBuilder.Summary(LimitedSupportSummaryCluster)
	}

	limitedSupport, err := limitedSupportBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build new limited support reason: %w", err)
	}
	return limitedSupport, nil
}

func (p *Post) buildLimitedSupportTemplate() (*cmv1.LimitedSupportReason, error) {
	t := p.readTemplate() // parse the given JSON template provided via '-t' flag

	p.parseUserParameters() // parse all the '-p' user flags
	// For every '-p' flag, replace its related placeholder in the template
	for k := range userParameterNames {
		p.replaceFlags(t, userParameterNames[k], userParameterValues[k])
	}
	p.checkLeftovers(t)

	limitedSupportBuilder := cmv1.NewLimitedSupportReason().Summary(t.Summary).Details(t.Details).DetectionType(t.Detection_type)
	limitedSupport, err := limitedSupportBuilder.Build()

	if err != nil {
		return nil, fmt.Errorf("failed to build new limited support reason: %w", err)
	}
	return limitedSupport, nil
}

// parseUserParameters parse all the '-p FOO=BAR' parameters and checks for syntax errors
func (p *Post) parseUserParameters() {
	for _, v := range p.TemplateParams {
		if !strings.Contains(v, "=") {
			log.Fatalf("Wrong syntax of '-p' flag. Please use it like this: '-p FOO=BAR'")
		}

		param := strings.SplitN(v, "=", 2)
		if param[0] == "" || param[1] == "" {
			log.Fatalf("Wrong syntax of '-p' flag. Please use it like this: '-p FOO=BAR'")
		}

		userParameterNames = append(userParameterNames, fmt.Sprintf("${%v}", param[0]))
		userParameterValues = append(userParameterValues, param[1])
	}
}

func (p *Post) readTemplate() *TemplateFile {
	if p.Template == "" {
		log.Fatalf("Template file is not provided. Use '-t' to fix this.")
	} else {
		templateObj, err := p.accessFile(p.Template)
		if err != nil { //check the presence of this URL or file and also if this can be accessed
			log.Fatal(err)
		}

		var t1 TemplateFile
		json.Unmarshal(templateObj, &t1)
		return &t1
	}
	return nil
}

// accessFile returns the contents of a local file or url, and any errors encountered
func (p *Post) accessFile(filePath string) ([]byte, error) {

	if utils.IsValidUrl(filePath) {
		urlPage, _ := url.Parse(filePath)
		if err := utils.IsOnline(*urlPage); err != nil {
			return nil, fmt.Errorf("host %q is not accessible", filePath)
		}
		return utils.CurlThis(urlPage.String())
	}

	filePath = filepath.Clean(filePath)
	if utils.FileExists(filePath) {
		// template is file on the disk
		file, err := os.ReadFile(filePath) //#nosec G304 -- Potential file inclusion via variable
		if err != nil {
			return file, fmt.Errorf("cannot read the file.\nError: %q", err)
		}
		return file, nil
	}
	if utils.FolderExists(filePath) {
		return nil, fmt.Errorf("the provided path %q is a directory, not a file", filePath)
	}
	return nil, fmt.Errorf("cannot read the file %q", filePath)
}

func (p *Post) replaceFlags(template *TemplateFile, flagName string, flagValue string) {
	if flagValue == "" {
		log.Fatalf("The selected template is using '%[1]s' parameter, but '%[1]s' flag was not set. Use '-p %[1]s=\"FOOBAR\"' to fix this.", flagName)
	}

	found := false

	if strings.Contains(template.Details, flagName) {
		found = true
		template.Details = strings.ReplaceAll(template.Details, flagName, flagValue)
	}

	if !found {
		log.Fatalf("The selected template is not using '%s' parameter, but '--param' flag was set. Do not use '-p %s=%s' to fix this.", flagName, flagName, flagValue)
	}
}

func (p *Post) findLeftovers(s string) (matches []string) {
	r := regexp.MustCompile(`\${[^{}]*}`)
	matches = r.FindAllString(s, -1)
	return matches
}

func (p *Post) checkLeftovers(template *TemplateFile) {
	unusedParameters := p.findLeftovers(template.Details)
	var numberOfMissingParameters int
	for _, v := range unusedParameters {
		// Ignore parameters in the exclude list, ie ${CLUSTER_UUID}, which will be replaced later for each cluster a servicelog is sent to
		if strings.Contains(template.Details, v) {
			numberOfMissingParameters++
			regex := strings.NewReplacer("${", "", "}", "")
			log.Printf("The one of the template files is using '%s' parameter, but '--param' flag is not set for this one. Use '-p %v=\"FOOBAR\"' to fix this.", v, regex.Replace(v))
		}
	}
	if numberOfMissingParameters == 1 {
		log.Fatal("Please define this missing parameter properly.")
	} else if numberOfMissingParameters > 1 {
		log.Fatalf("Please define all %v missing parameters properly.", numberOfMissingParameters)
	}
}

func printLimitedSupportReason(limitedSupport *cmv1.LimitedSupportReason) error {
	buf := bytes.Buffer{}
	err := cmv1.MarshalLimitedSupportReason(limitedSupport, &buf)
	if err != nil {
		return fmt.Errorf("failed to marshal limited support reason: %w", err)
	}

	return dump.Pretty(os.Stdout, buf.Bytes())
}

func sendLimitedSupportPostRequest(ocmClient *sdk.Connection, clusterID string, limitedSupport *cmv1.LimitedSupportReason) (*cmv1.LimitedSupportReasonsAddResponse, error) {
	response, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(clusterID).LimitedSupportReasons().Add().Body(limitedSupport).Send()
	if err != nil {
		return nil, fmt.Errorf("failed to post new limited support reason: %w", err)
	}
	return response, nil
}

func (p *Post) buildInternalServiceLog(limitedSupportId string, subscriptionId string) (*slv1.LogEntry, error) {
	logEntryBuilder := slv1.NewLogEntry().
		ClusterUUID(p.cluster.ExternalID()).
		ClusterID(p.cluster.ID()).
		InternalOnly(true).
		Severity(InternalServiceLogSeverity).
		ServiceName(InternalServiceLogServiceName).
		Summary(InternalServiceLogSummary).
		Description(fmt.Sprintf("%v - %v", limitedSupportId, p.Evidence))
	if subscriptionId != "" {
		logEntryBuilder.SubscriptionID(subscriptionId)
	}
	logEntry, err := logEntryBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create log entry: %w", err)
	}
	return logEntry, nil
}

func printInternalServiceLog(logEntry *slv1.LogEntry) error {
	buf := bytes.Buffer{}
	err := slv1.MarshalLogEntry(logEntry, &buf)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}
	return dump.Pretty(os.Stdout, buf.Bytes())
}

func sendInternalServiceLogPostRequest(ocmClient *sdk.Connection, logEntry *slv1.LogEntry) (*slv1.ClusterLogsAddResponse, error) {
	response, err := ocmClient.ServiceLogs().V1().ClusterLogs().Add().Body(logEntry).Send()
	if err != nil {
		return nil, fmt.Errorf("failed to post new internal service log: %w", err)
	}
	return response, nil
}

type MisconfigurationReason string

func (m *MisconfigurationReason) String() string {
	return string(*m)
}

func (m *MisconfigurationReason) Set(v string) error {
	switch v {
	case "cloud", "cluster":
		*m = MisconfigurationReason(v)
		return nil
	default:
		return errors.New(`must be one of "cloud" or "cluster"`)
	}
}

func (m *MisconfigurationReason) Type() string {
	return "MisconfigurationReason"
}
