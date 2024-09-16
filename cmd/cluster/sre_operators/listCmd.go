package sre_operators

import (
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
)

func newCmdList() *cobra.Command {
	opts := &sreOperatorsListOptions{}

	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List the current and latest version of SRE operators",
		Example:           sreOperatorsListExample,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ListOperators()
		},
	}

	listCmd.Flags().BoolVar(&opts.short, "short", false, "Excluse fetching the latest version from app-interface for faster output")
	listCmd.Flags().BoolVar(&opts.outdated, "outdated", false, "Filter to only show operators running outdated versions")

	return listCmd
}

// main
func ListOperators() {
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

	// one simple reason for this instead of empty slice:
	// allows for later on easier removal of operators with no version,
	// i.e. not present on cluster.
	currentVersion := make([]string, len(listOfOperators))
	expectedVersion := "test"

	fmt.Printf("%-40s %-10s %-10s\n", "OPERATOR", "CURRENT", "EXPECTED")
	for operator := range listOfOperators {
		// TODO: get current version of each operator via kube API
		// AS EXAMPLE => cmd := "oc get csv -n " + listOfOperators[operator] + "-o json | jq '.items[].spec.version' "

		// TODO: insert current version into currentVersion slice below
		currentVersion[operator] = "test"

		fmt.Printf("%-40s %-10s %-10s\n", listOfOperators[operator], currentVersion[operator], expectedVersion)
		fmt.Println() // returns to newline at end of output
	}

}
