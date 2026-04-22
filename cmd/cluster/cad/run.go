package cad

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/openshift/osdctl/cmd/setup"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cadClusterIDProd  = "2fbi9mjhqpobh20ot5d7e5eeq3a8gfhs" // These IDs are hard-coded in app-interface
	cadClusterIDStage = "2f9ghpikkv446iidcv7b92em2hgk13q9"
	cadNamespaceProd  = "configuration-anomaly-detection-production"
	cadNamespaceStage = "configuration-anomaly-detection-stage"
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
	"describe-nodes",
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
	isDryRun        bool
	params          []string
}

func newCmdRun() *cobra.Command {
	opts := &cadRunOptions{}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run a manual investigation on the CAD cluster",
		Long: `Run a manual investigation on the Configuration Anomaly Detection (CAD) cluster.

This command schedules a Tekton PipelineRun on the appropriate CAD cluster (stage or production)
to run an investigation against a target cluster. The results will be written to a backplane report.

Prerequisites:
  - Connected to the target cluster's OCM environment (production or stage)
  - The CAD clusters themselves are always in production OCM

Available Investigations:
  chgm, cmbb, can-not-retrieve-updates, ai, cpd, etcd-quota-low,
  insightsoperatordown, machine-health-check, must-gather, upgrade-config,
  restart-controlplane, describe-nodes

Examples:
` + "```bash" + `
# Run a change management investigation on a production cluster
osdctl cluster cad run \
  --cluster-id 1a2b3c4d5e6f7g8h9i0j \
  --investigation chgm \
  --environment production \
  --reason "OHSS-12345"

# Run a dry-run investigation (does not create a report)
osdctl cluster cad run \
  --cluster-id 1a2b3c4d5e6f7g8h9i0j \
  --investigation chgm \
  --environment production \
  --reason "OHSS-12345" \
  --dry-run

# Run describe-nodes on master nodes only
osdctl cluster cad run \
  --cluster-id 1a2b3c4d5e6f7g8h9i0j \
  --investigation describe-nodes \
  --environment production \
  --reason "OHSS-12345" \
  --params MASTER=true
` + "```" + `

Note:
  After the investigation completes (may take several minutes), view results using:
` + "```bash" + `
osdctl cluster reports list -C <cluster-id> -l 1
` + "```" + `

  You must be connected to the target cluster's OCM environment to view its reports.`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run()
		},
	}

	runCmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Cluster ID (internal or external)")
	runCmd.Flags().StringVarP(&opts.investigation, "investigation", "i", "", "Investigation name")
	runCmd.Flags().StringVarP(&opts.environment, "environment", "e", "", "Environment in which the target cluster runs. Allowed values: \"stage\" or \"production\"")
	runCmd.Flags().BoolVarP(&opts.isDryRun, "dry-run", "d", false, "Dry-Run: Run the investigation with the dry-run flag. This will not create a report.")
	runCmd.Flags().StringVar(&opts.elevationReason, "reason", "", "Provide a reason for running a manual investigation, used for backplane. Eg: 'OHSS-XXXX', or '#ITN-2024-XXXXX.")
	runCmd.Flags().StringArrayVarP(&opts.params, "params", "p", nil,
		"Investigation-specific parameters as KEY=VALUE (can be specified multiple times)")

	_ = runCmd.MarkFlagRequired("cluster-id")
	_ = runCmd.MarkFlagRequired("investigation")
	_ = runCmd.MarkFlagRequired("environment")
	_ = runCmd.MarkFlagRequired("reason")

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

	grafanaURL := viper.GetString(setup.CADGrafanaURL)
	awsAccountID := viper.GetString(setup.CADAWSAccountID)

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

	// Get the generated name created by the API server
	pipelineRunName := u.GetName()

	var logsLink string
	if grafanaURL != "" && awsAccountID != "" {
		logsLink = fmt.Sprintf("%s/explore?schemaVersion=1&panes=%%7B%%22buh%%22:%%7B%%22datasource%%22:%%22P1A97A9592CB7F392%%22,%%22queries%%22:%%5B%%7B%%22id%%22:%%22%%22,%%22region%%22:%%22us-east-1%%22,%%22namespace%%22:%%22%%22,%%22refId%%22:%%22A%%22,%%22datasource%%22:%%7B%%22type%%22:%%22cloudwatch%%22,%%22uid%%22:%%22P1A97A9592CB7F392%%22%%7D,%%22queryMode%%22:%%22Logs%%22,%%22logGroups%%22:%%5B%%7B%%22arn%%22:%%22arn:aws:logs:us-east-1:%[2]s:log-group:cads01ue1.configuration-anomaly-detection-stage:%%2A%%22,%%22name%%22:%%22cads01ue1.configuration-anomaly-detection-stage%%22,%%22accountId%%22:%%22%[2]s%%22%%7D,%%7B%%22arn%%22:%%22arn:aws:logs:us-east-1:%[2]s:log-group:cadp01ue1.configuration-anomaly-detection-production:%%2A%%22,%%22name%%22:%%22cadp01ue1.configuration-anomaly-detection-production%%22,%%22accountId%%22:%%22%[2]s%%22%%7D%%5D,%%22expression%%22:%%22fields%%20message%%5Cn%%7C%%20filter%%20kubernetes.pod_name%%20like%%20%%5C%%22%s%%5C%%22%%22,%%22statsGroups%%22:%%5B%%5D%%7D%%5D,%%22range%%22:%%7B%%22from%%22:%%22now-1h%%22,%%22to%%22:%%22now%%22%%7D,%%22panelsState%%22:%%7B%%22logs%%22:%%7B%%22visualisationType%%22:%%22logs%%22%%7D%%7D%%7D%%7D&orgId=1", grafanaURL, awsAccountID, pipelineRunName)
	}

	if !o.isDryRun {
		reportCmd := fmt.Sprintf("'osdctl cluster reports list -C %s -l 1'", o.clusterID)
		msg := "Successfully scheduled manual investigation. It can take several minutes until a report is available. \n" +
			"Run this command to check the latest report for the results while being connected to the right OCM backplane environment. " + reportCmd + " \n"

		if logsLink != "" {
			msg += "If a report fails to show up, check the TaskRun pod logs here after a few minutes: " + logsLink
		} else {
			msg += "To view TaskRun pod logs, configure 'cad_grafana_url' and 'cad_aws_account_id' using 'osdctl setup'"
		}
		fmt.Println(msg)
	} else {
		if logsLink != "" {
			fmt.Println("Dry-run investigation scheduled. Check for logs here: ", logsLink)
		} else {
			fmt.Println("Dry-run investigation scheduled. To view logs, configure 'cad_grafana_url' and 'cad_aws_account_id' using 'osdctl setup'")
		}
	}

	return nil
}

func (o *cadRunOptions) validate() error {
	if o.clusterID == "" {
		return fmt.Errorf("cluster-id is required")
	}

	if !slices.Contains(validInvestigations, o.investigation) {
		return fmt.Errorf("invalid investigation %q, must be one of: %v", o.investigation, validInvestigations)
	}

	if !slices.Contains(validEnvironments, o.environment) {
		return fmt.Errorf("invalid environment %q, must be one of: %v", o.environment, validEnvironments)
	}

	if o.elevationReason == "" {
		return fmt.Errorf("elevation reason is required")
	}

	for _, p := range o.params {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid param %q: must be in KEY=VALUE format", p)
		}
	}

	return nil
}

func (o *cadRunOptions) getCADClusterConfig() (clusterID, namespace string) {
	if o.environment == "stage" {
		return cadClusterIDStage, cadNamespaceStage
	}
	return cadClusterIDProd, cadNamespaceProd
}

func (o *cadRunOptions) pipelineRunTemplate(cadNamespace string) *unstructured.Unstructured {
	pipelineParams := []map[string]interface{}{
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
			"value": o.isDryRun,
		},
	}

	if len(o.params) > 0 {
		pipelineParams = append(pipelineParams, map[string]interface{}{
			"name":  "investigation-params",
			"value": strings.Join(o.params, ","),
		})
	}

	u := unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"apiVersion": "tekton.dev/v1beta1",
		"kind":       "PipelineRun",
		"metadata": map[string]interface{}{
			"generateName": "cad-manual-",
			"namespace":    cadNamespace,
		},
		"spec": map[string]interface{}{
			"params":             pipelineParams,
			"pipelineRef":        map[string]interface{}{"name": "cad-manual-investigation-pipeline"},
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
