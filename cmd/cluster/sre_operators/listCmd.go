package sre_operators

import (
	"context"
	"fmt"
	"regexp"

	// csvutil "github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubectl/pkg/cmd/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type sreOperatorsListOptions struct {
	short    bool
	outdated bool

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
	return nil
}

// main
func (ctx *sreOperatorsListOptions) ListOperators(cmd *cobra.Command) error {

	// list of operators to check (SRE only)
	listOfOperators := []string{
		"openshift-addon-operator",
		"aws-vpce-operator", // unverified
		"openshift-custom-domains-operator",
		"openshift-managed-node-metadata-operator",
		"openshift-managed-upgrade-operator",
		"openshift-must-gather-operator",
		"openshift-ocm-agent-operator",
		"openshift-osd-metrics",
		"openshift-rbac-permissions",
		"openshift-splunk-forwarder-operator",
		"aws-account-operator", // unverified
		"certman-operator",     // unverified
		"openshift-cloud-ingress-operator",
		"openshift-config-operator",
		"deadmanssnitch-operator", // unverified
		"openshift-deployment-validation-operator",
		"dynatrace-operator",   // unverified
		"gcp-project-operator", // unverified
		"openshift-velero",
		"openshift-observability-operator",
		"opentelemetry-operator",       // unverified
		"openshift-pagerduty-operator", // unverified
		"openshift-route-monitor-operator",
	}
	// csvList := csvutil.ClusterServiceVersion{}
	// csvutil.ClusterServiceVersion{}

	// one simple reason for this instead of empty slice:
	// allows for later on easier removal of operators with no version,
	// i.e. not present on cluster.
	// listOfExistingOperators := make(map[string]string)
	currentVersion := make([]string, len(listOfOperators))
	expectedVersion := make([]string, len(listOfOperators))

	fmt.Printf("%-40s %-10s %-10s %-10s %-10s\n", "OPERATOR", "CURRENT", "EXPECTED", "CHANNEL", "STATUS")
	for operator := range listOfOperators {

		podList := &v1.PodList{}

		if err := ctx.kubeCli.List(context.TODO(), podList, client.InNamespace(listOfOperators[operator])); err != nil {
			// fmt.Println("failed to retrieve pods", err)
			// return fmt.Errorf("failed to retrieve pods: %v", err)
			continue
		} else {
			if envVar := podList.Items[0].Spec.Containers[0].Env; envVar != nil {
				version := envVar[len(envVar)-1] // gets last element of envVar slice
				formattedVersion := extractVersion(version.Value)
				currentVersion[operator] = formattedVersion
			} else if envVar := podList.Items[1].Spec.Containers[0].Env; envVar != nil {
				version := envVar[len(envVar)-1]
				formattedVersion := extractVersion(version.Value)
				currentVersion[operator] = formattedVersion
			} else {
				// TODO: find a way to handle operators with no version
				// operators such as ocm-agent-operator and config-operator
				// do not seem to contain any version numbers within metadata.
			}

			// TODO: get expected version of each operator via app-interface

			// TODO: insert versions into slices below
			// listOfExistingOperators[listOfOperators[operator]] =
			// currentVersion[operator] = formattedVersion
			expectedVersion[operator] = "test"

			fmt.Printf("%-40s %-10s %-10s\n", listOfOperators[operator], currentVersion[operator], expectedVersion[operator])
			fmt.Println() // returns to newline at end of output
		}

	}
	return nil
}
func extractVersion(input string) string {

	regex := regexp.MustCompile(`.*v([0-9\.]+)-`)
	match := regex.FindStringSubmatch(input)

	if len(match) > 1 {
		return match[1]
	} else {
		return ""
	}
}
