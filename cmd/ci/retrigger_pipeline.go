package ci

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// appsrep09ue1 hosts all SAPM Tekton PipelineRuns for osd-operators. It is always in production OCM.
const appsrep09ue1ClusterID = "29nmp5rhf8rgclg4a02lju4eld79js9e"

var pipelineRunGVK = schema.GroupVersionKind{
	Group:   "tekton.dev",
	Version: "v1",
	Kind:    "PipelineRun",
}

type retriggerPipelineOptions struct {
	namespace string
	runName   string
	reason    string
}

func newCmdRetriggerPipeline() *cobra.Command {
	opts := &retriggerPipelineOptions{}

	cmd := &cobra.Command{
		Use:   "retrigger-pipeline",
		Short: "Retrigger a failed SAPM Tekton PipelineRun on appsrep09ue1",
		Long: `Retrigger a failed SAPM Tekton PipelineRun on appsrep09ue1.

Use this when a PipelineRun fails for infrastructure reasons unrelated to the operator code
or e2e tests (e.g. network blip, transient API failure). A successful retrigger publishes
to the SAPM success channel, unblocking downstream promotions.

To find the failed PipelineRun name:
  ocm backplane login --multi ` + appsrep09ue1ClusterID + `
  kubectl get pipelineruns -n <operator>-pipelines --sort-by=.metadata.creationTimestamp

Common namespaces: certman-operator-pipelines, cloud-ingress-operator-pipelines,
  rbac-permissions-operator-pipelines, ocm-agent-operator-pipelines`,
		Example: `  # Retrigger a failed certman int PipelineRun
  osdctl ci retrigger-pipeline -n certman-operator-pipelines \
    -r saas-co-prow-e2e-osd-integration-hivei01ue1-abc12 \
    --reason ROSAENG-62330`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "", "Namespace containing the failed PipelineRun (e.g. certman-operator-pipelines)")
	cmd.Flags().StringVarP(&opts.runName, "run", "r", "", "Name of the failed PipelineRun")
	cmd.Flags().StringVar(&opts.reason, "reason", "", "Backplane elevation reason (Jira key, PD URL, or ITN key)")

	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("run")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

func (o *retriggerPipelineOptions) run(ctx context.Context) error {
	ocmConn, err := utils.CreateConnectionWithUrl("production")
	if err != nil {
		return fmt.Errorf("failed to create production OCM connection: %w", err)
	}
	defer ocmConn.Close()

	k8sClient, err := k8s.NewAsBackplaneClusterAdminWithConn(
		appsrep09ue1ClusterID,
		client.Options{},
		ocmConn,
		o.reason,
		"Retriggering failed SAPM PipelineRun",
	)
	if err != nil {
		return fmt.Errorf("failed to create k8s client for appsrep09ue1: %w", err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(pipelineRunGVK)

	fmt.Printf("Fetching PipelineRun %s/%s...\n", o.namespace, o.runName)
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: o.runName, Namespace: o.namespace}, existing); err != nil {
		return fmt.Errorf("failed to get PipelineRun %s: %w", o.runName, err)
	}

	if err := requireFailedPipelineRun(existing); err != nil {
		return err
	}

	spec, found, err := unstructured.NestedMap(existing.Object, "spec")
	if err != nil {
		return fmt.Errorf("failed to extract spec from PipelineRun: %w", err)
	}
	if !found {
		return fmt.Errorf("PipelineRun %s has no spec", o.runName)
	}
	// Strip spec.status control field (StoppedRunFinally, CancelledRunFinally, etc.) so the
	// new run starts fresh. Inheriting it would cause the Tekton controller to immediately
	// stop or cancel the retriggered run.
	delete(spec, "status")

	generateName := existing.GetGenerateName()
	if generateName == "" {
		// Fall back to stripping the last random suffix from the run name.
		parts := strings.Split(o.runName, "-")
		if len(parts) < 2 {
			return fmt.Errorf("run name %q has no hyphen; cannot derive generateName", o.runName)
		}
		generateName = strings.Join(parts[:len(parts)-1], "-") + "-"
	}

	newRun := &unstructured.Unstructured{}
	newRun.Object = map[string]interface{}{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]interface{}{
			"generateName": generateName,
			"namespace":    o.namespace,
		},
		"spec": spec,
	}
	newRun.SetGroupVersionKind(pipelineRunGVK)
	newRun.SetLabels(existing.GetLabels())
	newRun.SetAnnotations(existing.GetAnnotations())

	fmt.Println("Creating new PipelineRun...")
	if err := k8sClient.Create(ctx, newRun); err != nil {
		return fmt.Errorf("failed to create PipelineRun: %w", err)
	}

	name := newRun.GetName()
	fmt.Printf("Created: %s\n\n", name)
	fmt.Printf("Monitor with:\n  kubectl get pipelinerun -n %s %s -w\n", o.namespace, name)

	return nil
}

// requireFailedPipelineRun returns an error if the PipelineRun is not in a
// terminal failed state (i.e. it is still running, succeeded, or was cancelled).
func requireFailedPipelineRun(pr *unstructured.Unstructured) error {
	conditions, _, _ := unstructured.NestedSlice(pr.Object, "status", "conditions")
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] != "Succeeded" {
			continue
		}
		status, _ := cond["status"].(string)
		reason, _ := cond["reason"].(string)
		switch status {
		case "Unknown":
			return fmt.Errorf("PipelineRun %s is still running (status: Unknown, reason: %s); retrigger is only allowed for failed runs", pr.GetName(), reason)
		case "True":
			return fmt.Errorf("PipelineRun %s already succeeded; retrigger is only allowed for failed runs", pr.GetName())
		case "False":
			if reason == "Cancelled" || reason == "PipelineRunCancelled" ||
				reason == "StoppedRunFinally" || reason == "CancelledRunFinally" || reason == "PipelineRunStopped" {
				return fmt.Errorf("PipelineRun %s was stopped/cancelled (reason: %s); retrigger is only allowed for failed runs", pr.GetName(), reason)
			}
			// Failed, timed out, or other terminal failure — allow retrigger.
			return nil
		}
	}
	// No Succeeded condition found (e.g. very new run); treat as unknown.
	return fmt.Errorf("PipelineRun %s has no Succeeded condition; cannot determine state", pr.GetName())
}
