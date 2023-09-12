package support

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	slv1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"os"
)

const (
	LimitedSupportSummaryCluster  = "Cluster is in Limited Support due to unsupported cluster configuration"
	LimitedSupportSummaryCloud    = "Cluster is in Limited Support due to unsupported cloud provider configuration"
	MisconfigurationFlag          = "misconfiguration"
	ProblemFlag                   = "problem"
	ResolutionFlag                = "resolution"
	EvidenceFlag                  = "evidence"
	InternalServiceLogSeverity    = "Warning"
	InternalServiceLogServiceName = "SREManualAction"
	InternalServiceLogSummary     = "LimitedSupportEvidence"
)

func newCmdpost() *cobra.Command {

	postCmd := &cobra.Command{
		Use:   "post CLUSTER_ID",
		Short: "Send limited support reason to a given cluster",
		Long: `Sends limited support reason to a given cluster, along with an internal service log detailing why the cluster was placed into limited support.
The caller will be prompted to continue before sending the limited support reason.`,
		Example: `# Post a limited support reason for a cluster misconfiguration
osdctl cluster support post 1a2B3c4DefghIjkLMNOpQrSTUV5 --misconfiguration cluster --problem="the cluster has a second failing ingress controller, which is not supported and can cause issues with SLA" \
--resolution="remove the additional ingress controller 'my-custom-ingresscontroller'. 'oc get ingresscontroller -n openshift-ingress-operator' should yield only 'default'" \
--evidence="See OHSS-1234"`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			misconfiguration, err := cmd.Flags().GetString(MisconfigurationFlag)
			if err != nil {
				return fmt.Errorf("error reading --%v flag", MisconfigurationFlag)
			}
			if misconfiguration != "cloud" && misconfiguration != "cluster" {
				return errors.New("--misconfiguration flag must be either `cloud` or `cluster`")
			}

			problem, err := cmd.Flags().GetString(ProblemFlag)
			if err != nil {
				return fmt.Errorf("error reading --%v flag", ProblemFlag)
			}

			resolution, err := cmd.Flags().GetString(ResolutionFlag)
			if err != nil {
				return fmt.Errorf("error reading --%v flag", ResolutionFlag)
			}

			evidence, err := cmd.Flags().GetString(EvidenceFlag)
			if err != nil {
				return fmt.Errorf("error reading --%v flag", EvidenceFlag)
			}

			err = PostLimitedSupport(args[0], misconfiguration, problem, resolution, evidence)
			if err != nil {
				return fmt.Errorf("error posting limited support reason: %w", err)
			}

			return nil
		},
	}

	// Define required flags
	postCmd.Flags().String(MisconfigurationFlag, "", "The type of misconfiguration responsible for the cluster being placed into limited support. Valid values are `cloud` or `cluster`.")
	postCmd.Flags().String(ProblemFlag, "", "The problem responsible for the cluster being placed into limited support.")
	postCmd.Flags().String(ResolutionFlag, "", "Steps for the customer to take to resolve the issue and move out of limited support.")
	postCmd.Flags().String(EvidenceFlag, "", "The reasoning that led to the decision to place the cluster in limited support. Can also be a link to a Jira case. Used for internal service log only.")

	return postCmd
}

func PostLimitedSupport(clusterID string, misconfiguration string, problem string, resolution string, evidence string) error {
	if misconfiguration != "cloud" && misconfiguration != "cluster" {
		return errors.New("misconfiguration must be either `cloud` or `cluster`")
	}

	// Check that the cluster key (name, identifier or external identifier) given by the user
	// is reasonably safe so that there is no risk of SQL injection
	err := ctlutil.IsValidClusterKey(clusterID)
	if err != nil {
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

	limitedSupport, err := buildLimitedSupport(misconfiguration, problem, resolution)
	if err != nil {
		return err
	}

	fmt.Printf("The following limited support reason will be sent to %s:\n", clusterID)
	if err = printLimitedSupportReason(limitedSupport); err != nil {
		return fmt.Errorf("failed to print limited support reason template: %w", err)
	}

	if !ctlutil.ConfirmPrompt() {
		return nil
	}

	cluster, err := ctlutil.GetCluster(connection, clusterID)
	if err != nil {
		return fmt.Errorf("can't retrieve cluster: %w", err)
	}

	postLimitedSupportResponse, err := sendLimitedSupportPostRequest(connection, cluster.ID(), limitedSupport)
	if err != nil {
		return fmt.Errorf("failed to post limited support reason: %w", err)
	}

	fmt.Printf("Successfully added new limited support reason with ID %v\n", postLimitedSupportResponse.Body().ID())

	var subscriptionId string
	if subscription, ok := cluster.GetSubscription(); ok {
		subscriptionId = subscription.ID()
	}
	log, err := buildInternalServiceLog(cluster.ExternalID(), cluster.ID(), postLimitedSupportResponse.Body().ID(), evidence, subscriptionId)
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

	return nil
}

func buildLimitedSupport(misconfiguration string, problem string, resolution string) (*cmv1.LimitedSupportReason, error) {
	limitedSupportBuilder := cmv1.NewLimitedSupportReason().
		Details(fmt.Sprintf("%v. %v", problem, resolution)).
		DetectionType(cmv1.DetectionTypeManual)
	if misconfiguration == "cloud" {
		limitedSupportBuilder.Summary(LimitedSupportSummaryCloud)
	} else if misconfiguration == "cluster" {
		limitedSupportBuilder.Summary(LimitedSupportSummaryCluster)
	}

	limitedSupport, err := limitedSupportBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build new limited support reason: %w", err)
	}
	return limitedSupport, nil
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

func buildInternalServiceLog(externalId string, internalId string, limitedSupportId string, evidence string, subscriptionId string) (*slv1.LogEntry, error) {
	logEntryBuilder := slv1.NewLogEntry().
		ClusterUUID(externalId).
		ClusterID(internalId).
		InternalOnly(true).
		Severity(InternalServiceLogSeverity).
		ServiceName(InternalServiceLogServiceName).
		Summary(InternalServiceLogSummary).
		Description(fmt.Sprintf("%v - %v", limitedSupportId, evidence))
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
