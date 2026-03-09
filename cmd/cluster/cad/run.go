package cad

import (
	"context"
	"fmt"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cadClusterIDProd  = "2fbi9mjhqpobh20ot5d7e5eeq3a8gfhs" // These IDs are hard-coded in app-interface
	cadClusterIDStage = "2f9ghpikkv446iidcv7b92em2hgk13q9"
)

var validInvestigations = []string{
	"chgm",
	"cmbb",
	"can-not-retrieve-updates",
	"ai",
	"cpd",
	"etcd-quota-low",
	"insightsoperatordown",
	"machine-health-check",
	"must-gather",
	"upgrade-config",
	"restart-controlplane",
}

var validEnvironments = []string{
	"stage",
	"production",
}

type cadRunOptions struct {
	clusterID       string
	investigation   string
	elevationReason string
	environment     string
}

func newCmdRun() *cobra.Command {
	opts := &cadRunOptions{}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run a manual investigation on the CAD cluster",
		Long: `Run a manual investigation on the Configuration Anomaly Detection (CAD) cluster.

This command schedules a Tekton PipelineRun on the appropriate CAD cluster (stage or production)
to run an investigation against a target cluster.

Prerequisites:
  - Connected to the target cluster's OCM environment (production or stage)
  - The CAD clusters themselves are always in production OCM

Available Investigations:
  chgm, cmbb, can-not-retrieve-updates, ai, cpd, etcd-quota-low,
  insightsoperatordown, machine-health-check, must-gather, upgrade-config

Example:
  # Run a change management investigation on a production cluster
  osdctl cluster cad run \
    --cluster-id 1a2b3c4d5e6f7g8h9i0j \
    --investigation chgm \
    --environment production \
    --reason "OHSS-12345"

Note:
  After the investigation completes (may take several minutes), view results using:
    osdctl cluster reports list -C <cluster-id> -l 1

  You must be connected to the target cluster's OCM environment to view its reports.`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run()
		},
	}

	runCmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Cluster ID (internal or external)")
	runCmd.Flags().StringVarP(&opts.investigation, "investigation", "i", "", "Investigation name")
	runCmd.Flags().StringVarP(&opts.environment, "environment", "e", "", "Environment of the cluster we want to run the investigation on. Allowed values: \"stage\" or \"production\"")
	runCmd.Flags().StringVar(&opts.elevationReason, "reason", "", "Provide a reason for running a manual investigation, used for backplane. Eg: 'OHSS-XXXX', or '#ITN-2024-XXXXX.")

	_ = runCmd.RegisterFlagCompletionFunc("investigation", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return validInvestigations, cobra.ShellCompDirectiveNoFileComp
	})

	_ = runCmd.RegisterFlagCompletionFunc("environment", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return validEnvironments, cobra.ShellCompDirectiveNoFileComp
	})

	return runCmd
}

func (o *cadRunOptions) run() error {
	if err := o.validate(); err != nil {
		return err
	}

	cadClusterID, cadNamespace := o.getCADClusterConfig()

	// CAD clusters are always in production OCM, so explicitly create a production connection
	ocmConn, err := utils.CreateConnectionWithUrl("production")
	if err != nil {
		return fmt.Errorf("failed to create production OCM connection: %w", err)
	}
	defer ocmConn.Close()

	k8sClient, err := k8s.NewAsBackplaneClusterAdminWithConn(cadClusterID, client.Options{}, ocmConn, o.elevationReason, "Need elevation for cad cluster in order to schedule a Tekton pipeline run")
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	u := o.pipelineRunTemplate(cadNamespace)

	err = k8sClient.Create(context.Background(), u)
	if err != nil {
		return fmt.Errorf("failed to schedule task: %w", err)
	}

	reportCmd := fmt.Sprintf("'osdctl cluster reports list -C %s -l 1'", o.clusterID)
	fmt.Println("Successfully scheduled manual investigation. It can take several minutes until a report is available. Run this command to check the latest report for the results while being connected to the right OCM backplane environment. " + reportCmd)

	return nil
}

func (o *cadRunOptions) validate() error {
	conn, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer conn.Close()

	if o.clusterID == "" {
		return fmt.Errorf("cluster-id is required")
	}

	validInvestigation := false
	for _, v := range validInvestigations {
		if o.investigation == v {
			validInvestigation = true
			break
		}
	}
	if !validInvestigation {
		return fmt.Errorf("invalid investigation %q, must be one of: %v", o.investigation, validInvestigations)
	}

	validEnvironment := false
	for _, v := range validEnvironments {
		if o.environment == v {
			validEnvironment = true
			break
		}
	}
	if !validEnvironment {
		return fmt.Errorf("invalid environment %q, must be one of: %v", o.environment, validEnvironments)
	}

	if o.elevationReason == "" {
		return fmt.Errorf("elevation reason is required")
	}

	return nil
}

func (o *cadRunOptions) getCADClusterConfig() (clusterID, namespace string) {
	if o.environment == "stage" {
		return cadClusterIDStage, "configuration-anomaly-detection-stage"
	}
	return cadClusterIDProd, "configuration-anomaly-detection-production"
}

func (o *cadRunOptions) pipelineRunTemplate(cadNamespace string) *unstructured.Unstructured {
	u := unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"apiVersion": "tekton.dev/v1beta1",
		"kind":       "PipelineRun",
		"metadata": map[string]interface{}{
			"generateName": "cad-manual-",
			"namespace":    cadNamespace,
		},
		"spec": map[string]interface{}{
			"params": []map[string]interface{}{
				{
					"name":  "cluster-id",
					"value": o.clusterID,
				},
				{
					"name":  "investigation",
					"value": o.investigation,
				},
				{
					"name":  "dry-run",
					"value": "false",
				},
			},
			"pipelineRef": map[string]interface{}{
				"name": "cad-manual-investigation-pipeline",
			},
			"serviceAccountName": "cad-sa",
			"timeout":            "30m",
		},
	}

	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "tekton.dev",
		Version: "v1beta1",
		Kind:    "PipelineRun",
	})

	return &u
}
