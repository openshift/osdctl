package restore

import (
	"fmt"
	"io"

	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	internalcmd "github.com/openshift/osdctl/cmd/hcp/internal/cmd"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

func NewCmdManifestWork() *cobra.Command {
	cmd := &cobra.Command{Use: "manifestwork", Short: "Compare and relocate HCP ManifestWorks", Args: cobra.NoArgs, DisableAutoGenTag: true}
	cmd.AddCommand(newCmdManifestWorkCompare(), newCmdManifestWorkMove())
	return cmd
}

func newCmdManifestWorkCompare() *cobra.Command {
	flags := &manifestWorkFlags{}
	cmd := &cobra.Command{
		Use:               "compare --cluster-id <cluster-id> --replacement-mc <management-cluster> --reason <reason> [--source-mc <management-cluster>]",
		Short:             "Compare HCP ManifestWorks between management clusters",
		Example:           "  osdctl hcp restore manifestwork compare --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}\n  osdctl hcp restore manifestwork compare --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			manager := internalcmd.NewDefaultManifestWorkManager(internalclient.NewOCMClient(ocmConn, logger))
			comparisons, err := manager.CompareManifestWorks(cmd.Context(), manifestWorkRequest(flags))
			if err != nil {
				return err
			}
			return printManifestWorkComparison(cmd.OutOrStdout(), comparisons)
		},
	}
	flags.AddFlags(cmd)
	return cmd
}

func newCmdManifestWorkMove() *cobra.Command {
	cmd := &cobra.Command{Use: "move", Short: "Move HCP Velero and cluster resources in phases", Args: cobra.NoArgs, DisableAutoGenTag: true}
	cmd.AddCommand(newCmdManifestWorkMoveVeleroResources(), newCmdManifestWorkMoveClusterResources())
	return cmd
}

func newCmdManifestWorkMoveVeleroResources() *cobra.Command {
	flags := &manifestWorkFlags{}
	cmd := &cobra.Command{
		Use:               "velero-resources --cluster-id <cluster-id> --replacement-mc <management-cluster> --reason <reason> [--source-mc <management-cluster>]",
		Short:             "Move the Velero resource ManifestWorks",
		Example:           "  osdctl hcp restore manifestwork move velero-resources --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}\n  osdctl hcp restore manifestwork move velero-resources --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			manager := internalcmd.NewDefaultManifestWorkManager(internalclient.NewOCMClient(ocmConn, logger))
			plan, err := manager.PrepareVeleroManifestWorkMove(cmd.Context(), manifestWorkRequest(flags))
			if err != nil {
				return err
			}
			if len(plan.ManifestWorks) == 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "All %s ManifestWorks already exist in %s. Source ManifestWorks were retained.\n", plan.Phase, plan.ReplacementMC)
				return err
			}
			message := fmt.Sprintf("Create %d %s ManifestWork(s) in %s on service cluster %s?\nSource ManifestWorks will be retained. [y/N] ", plan.Count, plan.Phase, plan.ReplacementMC, plan.ServiceClusterName)
			if !confirm(cmd.InOrStdin(), cmd.OutOrStdout(), message) {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "ManifestWork move cancelled.")
				return err
			}
			result, err := manager.MoveManifestWorks(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if result.AlreadyExists {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "All %s ManifestWorks already exist in %s. Source ManifestWorks were retained.\n", result.Phase, result.ReplacementMC)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created %d %s ManifestWork(s) in %s. Source ManifestWorks were retained.\n", result.Created, result.Phase, result.ReplacementMC)
			return err
		},
	}
	flags.AddFlags(cmd)
	return cmd
}

func newCmdManifestWorkMoveClusterResources() *cobra.Command {
	flags := &manifestWorkFlags{}
	cmd := &cobra.Command{
		Use:               "cluster-resources --cluster-id <cluster-id> --replacement-mc <management-cluster> --reason <reason> [--source-mc <management-cluster>]",
		Short:             "Move the namespace, HostedCluster, and NodePool ManifestWorks",
		Example:           "  osdctl hcp restore manifestwork move cluster-resources --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}\n  osdctl hcp restore manifestwork move cluster-resources --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())
			manager := internalcmd.NewDefaultManifestWorkManager(internalclient.NewOCMClient(ocmConn, logger))
			plan, err := manager.PrepareClusterManifestWorkMove(cmd.Context(), manifestWorkRequest(flags))
			if err != nil {
				return err
			}
			message := fmt.Sprintf("Pause HostedCluster and NodePool reconciliation in the source, then move %d cluster ManifestWork(s) to %s on service cluster %s?\nSource ManifestWorks will be retained and both source and destination resources will remain paused. [y/N] ", plan.Count, plan.ReplacementMC, plan.ServiceClusterName)
			if !confirm(cmd.InOrStdin(), cmd.OutOrStdout(), message) {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "ManifestWork move cancelled.")
				return err
			}
			result, err := manager.MoveManifestWorks(cmd.Context(), plan)
			if err != nil {
				return err
			}
			if result.AlreadyExists {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "All %s ManifestWorks already exist in %s. Source ManifestWorks were retained.\n", result.Phase, result.ReplacementMC)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created %d %s ManifestWork(s) in %s. Source ManifestWorks were retained.\n", result.Created, result.Phase, result.ReplacementMC)
			return err
		},
	}
	flags.AddFlags(cmd)
	return cmd
}

type manifestWorkFlags struct{ clusterID, sourceMC, replacementMC, reason string }

func (flags *manifestWorkFlags) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVar(&flags.sourceMC, "source-mc", "", "Management cluster whose namespace contains the ManifestWorks (defaults to the HCP management cluster)")
	cmd.Flags().StringVar(&flags.replacementMC, "replacement-mc", "", "Management cluster whose namespace contains the replacement ManifestWorks")
	cmd.Flags().StringVar(&flags.reason, "reason", "", "Reason for elevated access")
	for _, flag := range []string{"cluster-id", "replacement-mc", "reason"} {
		_ = cmd.MarkFlagRequired(flag)
	}
}

func manifestWorkRequest(flags *manifestWorkFlags) internalcmd.ManifestWorkRequest {
	return internalcmd.ManifestWorkRequest{ClusterID: flags.clusterID, SourceMC: flags.sourceMC, ReplacementMC: flags.replacementMC, Reason: flags.reason}
}

func printManifestWorkComparison(out io.Writer, comparisons []internalcmd.ManifestWorkComparison) error {
	table := printer.NewTablePrinter(out, 0, 4, 2, ' ')
	table.AddRow([]string{"NAME", "SOURCE", "SOURCE APPLIED", "SOURCE AVAILABLE", "REPLACEMENT", "REPLACEMENT APPLIED", "REPLACEMENT AVAILABLE"})
	for _, comparison := range comparisons {
		table.AddRow([]string{comparison.Name, manifestWorkPresence(comparison.Source), manifestWorkCondition(comparison.Source, "Applied"), manifestWorkCondition(comparison.Source, "Available"), manifestWorkPresence(comparison.Replacement), manifestWorkCondition(comparison.Replacement, "Applied"), manifestWorkCondition(comparison.Replacement, "Available")})
	}
	return table.Flush()
}

func manifestWorkPresence(work *workv1.ManifestWork) string {
	if work == nil {
		return "No"
	}
	return "Yes"
}

func manifestWorkCondition(work *workv1.ManifestWork, conditionType string) string {
	if work == nil {
		return "-"
	}
	for _, condition := range work.Status.Conditions {
		if condition.Type == conditionType {
			return string(condition.Status)
		}
	}
	return string(metav1.ConditionUnknown)
}
