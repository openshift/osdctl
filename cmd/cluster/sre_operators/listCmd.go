package sre_operators

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xanzy/go-gitlab"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type sreOperatorsListOptions struct {
	short    bool
	outdated bool

	genericclioptions.IOStreams
	kubeCli client.Client
}

const (
	sreOperatorsListExample = `
	# List SRE operators
	$ osdctl cluster sre-operators list
	`
	repositoryBranch = "production"
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

// Command validity check
func (ctx *sreOperatorsListOptions) checks(cmd *cobra.Command) error {
	if _, err := config.GetConfig(); err != nil {
		return util.UsageErrorf(cmd, "could not find KUBECONFIG, please make sure you are logged into a cluster")
	}
	return nil
}

func (ctx *sreOperatorsListOptions) ListOperators(cmd *cobra.Command) error {

	// list of SRE operators to check
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

	listOfOperatorNames := []string{
		"addon-operator",
		"aws-vpce-operator",
		"custom-domains-operator",
		"managed-node-metadata-operator",
		"managed-upgrade-operator",
		"must-gather-operator",
		"ocm-agent-operator",
		"osd-metrics-exporter",
		"rbac-permissions-operator",
		"splunk-forwarder-operator",
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

	currentVersion := make([]string, len(listOfOperators))
	// expectedVersion := make([]string, len(listOfOperators))
	operatorChannel := ""

	// Unstructured csv and csvList
	csv := &unstructured.Unstructured{}
	csv.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "ClusterServiceVersion",
	})

	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	})

	csvList := &unstructured.UnstructuredList{}
	csvList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "ClusterServiceVersionList",
	})

	fmt.Printf("%-45s %-20s %-20s %-15s %-15s\n", "NAME", "CURRENT", "EXPECTED", "STATUS", "CHANNEL")

	for operator := range listOfOperators {

		if err := ctx.kubeCli.List(context.TODO(), csvList, client.InNamespace(listOfOperators[operator])); err != nil {
			continue
		} else {
			// iterates through namespace to find SRE operator
			operatorStatus := ""
			for _, item := range csvList.Items {
				if strings.Contains(item.GetName(), listOfOperatorNames[operator]) {
					operatorStatus = item.Object["status"].(map[string]interface{})["phase"].(string)
					currentVersion[operator] = item.GetName()
				}
			}
			currentVersion[operator] = extractVersion(currentVersion[operator])

			latestVersion := getLatestVersion(listOfOperatorNames[operator])

			// p := printer.NewTablePrinter(ctx.IOStreams.Out, 20, 1, 3, ' ')

			// if ctx.short {
			// 	p.AddRow(listOfOperatorNames[operator], currentVersion[operator], latestVersion, operatorStatus, "")
			// 	p.Print()
			// } else {
			// 	p.AddRow(listOfOperatorNames[operator], currentVersion[operator], latestVersion, operatorStatus, "")
			// 	p.Print()
			// }

			// if ctx.outdated && currentVersion[operator] == latestVersion {

			fmt.Printf("%-45s %-20s %-20s %-15s %-15s\n", listOfOperatorNames[operator], currentVersion[operator], latestVersion, operatorStatus, operatorChannel)
		}
	}
	fmt.Println()
	return nil
}
func extractVersion(input string) string {
	// extracts version number from image name; might want to get hash later as well
	regex := regexp.MustCompile(`.*(v[0-9\.]+)-`)
	match := regex.FindStringSubmatch(input)

	if len(match) > 1 {
		return match[1]
	} else {
		return ""
	}
}

func getLatestVersion(operatorName string) string {

	// obtain personal access token from osdctl config
	gitlab_access := viper.GetString("gitlab_access")
	if gitlab_access == "" {
		fmt.Println("gitlab access token not found, please ensure your gitlab access token is set in the osdctl config")
	}
	// Generate gitlab client
	gitClient, err := gitlab.NewClient(gitlab_access, gitlab.WithBaseURL("https://gitlab.cee.redhat.com/"))
	if err != nil {
		fmt.Println("failed to create gitlab client:", err)
	}

	repoLink := "service/saas-" + operatorName + "-bundle"
	filePath := operatorName + "/" + operatorName + ".package.yaml"

	fileYaml, _, err := gitClient.RepositoryFiles.GetFile(repoLink, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(repositoryBranch)})
	if err != nil {
		fmt.Printf("failed to get file: %s", err)
		return ""
	}

	// decode base64
	decodedYamlString, err := base64.StdEncoding.DecodeString(fileYaml.Content)
	if err != nil {
		fmt.Printf("failed to decode file: %s", err)
	}

	expectedVersion := extractVersion(string(decodedYamlString))

	return expectedVersion
}
