package sre_operators

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/openshift/osdctl/pkg/printer"
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
	short     bool
	outdated  bool
	noHeaders bool

	genericclioptions.IOStreams
	kubeCli client.Client
}

type sreOperator struct {
	Name     string
	Current  string
	Expected string
	Status   string
	Channel  string
}

var opList []sreOperator

const (
	sreOperatorsListExample = `
	# List SRE operators
	$ osdctl cluster sre-operators list
	
	# List SRE operators without fetching the latest version for faster output
	$ osdctl cluster sre-operators list --short
	
	# List only SRE operators that are running outdated versions
	$ osdctl cluster sre-operators list --outdated
	`
	repositoryBranch = "production"
)

func newCmdList(streams genericclioptions.IOStreams, client client.Client) *cobra.Command {
	opts := &sreOperatorsListOptions{
		kubeCli:   client,
		IOStreams: streams,
	}

	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List the current and latest version of SRE operators",
		Example:           sreOperatorsListExample,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(opts.checks(cmd))
			output, _ := opts.ListOperators(cmd)
			opts.printText(output)
		},
	}

	listCmd.Flags().BoolVar(&opts.short, "short", false, "Exclude fetching the latest version from repositories for faster output")
	listCmd.Flags().BoolVar(&opts.outdated, "outdated", false, "Filter to only show operators running outdated versions")
	listCmd.Flags().BoolVar(&opts.noHeaders, "no-headers", false, "Exclude headers from the output")

	return listCmd
}

// Command validity check
func (ctx *sreOperatorsListOptions) checks(cmd *cobra.Command) error {
	if _, err := config.GetConfig(); err != nil {
		return util.UsageErrorf(cmd, "could not find KUBECONFIG, please make sure you are logged into a cluster")
	}
	if ctx.outdated && ctx.short {
		return util.UsageErrorf(cmd, "cannot use both --short and --outdated flags together")
	}
	return nil
}

// Print output in table format
func (ctx *sreOperatorsListOptions) printText(opList []sreOperator) error {
	p := printer.NewTablePrinter(ctx.IOStreams.Out, 20, 1, 3, ' ')

	if !ctx.noHeaders {
		header := []string{"NAME", "CURRENT", "EXPECTED", "STATUS", "CHANNEL"}
		if ctx.short {
			header = []string{"NAME", "CURRENT", "STATUS", "CHANNEL"}
		}
		p.AddRow(header)
	}

	for _, op := range opList {
		if ctx.outdated && op.Current == op.Expected {
			continue
		}
		row := []string{op.Name, op.Current, op.Status, op.Channel}
		if !ctx.short {
			row = []string{op.Name, op.Current, op.Expected, op.Status, op.Channel}
		}
		p.AddRow(row)
	}
	if err := p.Flush(); err != nil {
		return err
	}

	return nil
}

func (ctx *sreOperatorsListOptions) ListOperators(cmd *cobra.Command) ([]sreOperator, error) {

	listOfOperators := []string{
		"openshift-addon-operator",
		"aws-vpce-operator",
		"openshift-custom-domains-operator",
		"openshift-managed-node-metadata-operator",
		"openshift-managed-upgrade-operator",
		"openshift-must-gather-operator",
		"openshift-ocm-agent-operator",
		"openshift-osd-metrics",
		"openshift-rbac-permissions",
		"openshift-splunk-forwarder-operator",
		"aws-account-operator",
		"certman-operator",
		"openshift-cloud-ingress-operator",
		"openshift-config-operator",
		"deadmanssnitch-operator",
		"openshift-deployment-validation-operator",
		// "dynatrace-operator", // skip for now
		"gcp-project-operator",
		"openshift-velero",
		// "openshift-observability-operator", // skip for now
		// "opentelemetry-operator", // skip for now
		"pagerduty-operator",
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
		// "dynatrace-operator", // skip for now
		"gcp-project-operator",
		"managed-velero-operator",
		// "observability-operator", // skip for now
		// "opentelemetry-operator", // skip for now
		"pagerduty-operator",
		"route-monitor-operator",
	}

	currentVersion := make([]string, len(listOfOperators))
	latestVersion, operatorStatus, operatorChannel := "", "", ""
	gitClient := &gitlab.Client{}

	csv := &unstructured.Unstructured{}
	csv.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "ClusterServiceVersion",
	})
	csvList := &unstructured.UnstructuredList{}
	csvList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "ClusterServiceVersionList",
	})
	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "Subscription",
	})
	subList := &unstructured.UnstructuredList{}
	subList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1alpha1",
		Kind:    "SubscriptionList",
	})

	// Initialize gitlab client
	if !ctx.short {
		gitlab_access := viper.GetString("gitlab_access")
		if gitlab_access == "" {
			fmt.Println("gitlab access token not found, please ensure your gitlab access token is set in the osdctl config")
			return nil, nil
		}
		gitClient, _ = gitlab.NewClient(gitlab_access, gitlab.WithBaseURL("https://gitlab.cee.redhat.com/"))
	}

	// iterates through list of operators
	for operator := range listOfOperators {
		if err := ctx.kubeCli.List(context.TODO(), csvList, client.InNamespace(listOfOperators[operator])); err != nil {
			continue
		} else {

			for _, item := range csvList.Items {
				if strings.Contains(item.GetName(), listOfOperatorNames[operator]) {
					currentVersion[operator] = item.GetName()
					operatorStatus = item.Object["status"].(map[string]interface{})["phase"].(string)
				}
			}

			if currentVersion[operator] == "" {
				continue
			}

			// get channel
			if err := ctx.kubeCli.List(context.TODO(), subList, client.InNamespace(listOfOperators[operator])); err != nil {
				continue
			} else {
				for _, item := range subList.Items {
					if strings.Contains(item.GetName(), listOfOperatorNames[operator]) {
						operatorChannel = item.Object["spec"].(map[string]interface{})["channel"].(string)
					}
				}
			}

			currentVersion[operator] = extractVersion(currentVersion[operator])

			if !ctx.short {
				latestVersion = getLatestVersion(gitClient, listOfOperatorNames[operator])
			}
		}

		op := sreOperator{
			Name:     listOfOperatorNames[operator],
			Current:  currentVersion[operator],
			Expected: latestVersion,
			Status:   operatorStatus,
			Channel:  operatorChannel,
		}

		opList = append(opList, op)
	}

	return opList, nil
}

func extractVersion(input string) string {
	// extract version from csv name
	regex := regexp.MustCompile(`(?:.?)(v[0-9\.]+)(?:-.*)?`)
	extracted := regex.FindStringSubmatch(input)

	if len(extracted) > 1 {
		return extracted[1]
	} else {
		return ""
	}
}

func getLatestVersion(gitClient *gitlab.Client, operatorName string) string {

	// Special case for deployment-validation-operator: version is stored in a text file
	if operatorName == "deployment-validation-operator" {
		repoLink := "service/saas-operator-versions"
		filePath := "deployment-validation-operator/deployment-validation-operator-versions.txt"

		fileTxt, _, err := gitClient.RepositoryFiles.GetFile(repoLink, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr("master")})
		if err != nil {
			fmt.Println(operatorName, "- Failed to obtain GitLab file: ", err)
			return ""
		}
		decodedFileTxt, err := base64.StdEncoding.DecodeString(fileTxt.Content)
		if err != nil {
			fmt.Println(operatorName, "failed to decode file: ", err)
		}
		line := strings.Split(string(decodedFileTxt), "\n")
		for i := len(line) - 1; i >= 0; i-- {
			if line[i] != "" {
				expectedVersion := extractVersion("v" + line[i])
				return expectedVersion
			}
		}
	}

	repoLink := "service/saas-" + operatorName + "-bundle"
	filePath := operatorName + "/" + operatorName + ".package.yaml"

	fileYaml, _, err := gitClient.RepositoryFiles.GetFile(repoLink, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(repositoryBranch)})
	if err != nil {
		fmt.Println(operatorName, "- Failed to obtain GitLab file: ", err)
		return ""
	}

	// decode base64
	decodedYamlString, err := base64.StdEncoding.DecodeString(fileYaml.Content)
	if err != nil {
		fmt.Println(operatorName, "failed to decode file: ", err)
	}

	expectedVersion := extractVersion(string(decodedYamlString))

	return expectedVersion
}