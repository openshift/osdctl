package sre_operators

import (
	"fmt"

	// csvutil "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/cmd/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type sreOperatorsListOptions struct {
	clusterID string
	short     bool
	outdated  bool

	kubeCli client.Client
}

const (
	sreOperatorsListExample = `
	# List SRE operators
	$ osdctl cluster sre-operators list
	`
	appInterfaceURL   = "git@gitlab.cee.redhat.com:service/app-interface.git"
	referenceYamlPath = "data/services/osd-operators/app.yml"
)

func newCmdList(client client.Client) *cobra.Command {
	opts := &sreOperatorsListOptions{
		kubeCli: client,
	}

	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List the current and latest version of SRE operators",
		Example:           sreOperatorsListExample,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(opts.checks(cmd))
			opts.ListOperators(cmd)
		},
	}

	listCmd.Flags().BoolVar(&opts.short, "short", false, "Excluse fetching the latest version from app-interface for faster output")
	listCmd.Flags().BoolVar(&opts.outdated, "outdated", false, "Filter to only show operators running outdated versions")

	return listCmd
}

// Command validity checking
func (ctx *sreOperatorsListOptions) checks(cmd *cobra.Command) error {
	if _, err := config.GetConfig(); err != nil {
		return util.UsageErrorf(cmd, "could not find KUBECONFIG, please make sure you are logged into a cluster")
	}
	fmt.Println("success")
	return nil
}

// main
func (ctx *sreOperatorsListOptions) ListOperators(cmd *cobra.Command) error {

	// list of operators to check (SRE only)
	listOfOperators := []string{
		"addon-operator",
		"aws-vpce-operator",
		"custom-domains-operator",
		"managed-node-metadata-operator",
		"managed-upgrade-operator",
		"must-gather-operator",
		"ocm-agent-operator",
		"osd-metrics-exporter",
		"rbac-permissions-operator",
		"openshift-splunk-forwarder-operator",
		"aws-account-operator",
		"certman-operator",
		"cloud-ingress-operator",
		"configure-alertmanager-operator",
		"deadmanssnitch-operator",
		"deployment-validation-operator",
		"dynatrace-operator",
		"gcp-project-operator",
		"managed-velero-operator",
		"observability-operator",
		"opentelemetry-operator",
		"pagerduty-operator",
		"route-monitor-operator",
	}
	// csvList := csvutil.ClusterServiceVersion{}

	// one simple reason for this instead of empty slice:
	// allows for later on easier removal of operators with no version,
	// i.e. not present on cluster.
	currentVersion := make([]string, len(listOfOperators))
	expectedVersion := make([]string, len(listOfOperators))

	fmt.Printf("%-40s %-10s %-10s\n", "OPERATOR", "CURRENT", "EXPECTED")
	for operator := range listOfOperators {
		// TODO: get current version of each operator via kube API

		// TODO: get expected version of each operator via app-interface

		// TODO: insert versions into slices below
		currentVersion[operator] = "test"
		expectedVersion[operator] = "test"

		fmt.Printf("%-40s %-10s %-10s\n", listOfOperators[operator], currentVersion[operator], expectedVersion[operator])
		fmt.Println() // returns to newline at end of output
	}

	return nil
}
