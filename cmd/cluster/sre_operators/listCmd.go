package sre_operators

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/go-github/v63/github"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	gitlab "gitlab.com/gitlab-org/api/client-go"
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
	operator  string
	noCommit  bool

	genericclioptions.IOStreams
	kubeCli client.Client
}

type sreOperator struct {
	Name           string
	Current        string
	CurrentCommit  string
	Expected       string
	ExpectedCommit string
	Status         string
	Channel        string
	RepositoryURL  string
}

const (
	sreOperatorsListExample = `
	# List SRE operators
	$ osdctl cluster sre-operators list
	
	# List SRE operators without fetching the latest version for faster output
	$ osdctl cluster sre-operators list --short
	
	# List only SRE operators that are running outdated versions
	$ osdctl cluster sre-operators list --outdated

	# List SRE operators without their commit shas and repositry URL
	$ osdctl cluster sre-operators list --no-commit
	
	# List a specific SRE operator
	$ osdctl cluster sre-operators list --operator='OPERATOR_NAME'
	`
	sreOperatorsListDescription = `
	Lists the current version, channel, and status of SRE operators running in the current 
	cluster context, and by default fetches the latest version from the operators' repositories.
	
	A git_access token is required to fetch the latest version of the operators, and can be 
	set within the config file using the 'osdctl setup' command.
	
	The command creates a Kubernetes client to access the current cluster context, and GitLab/GitHub
	clients to fetch the latest versions of each operator from its respective repository.
	`

	repositoryBranch = "production"
)

var listOfOperators = []string{
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
	"openshift-monitoring",
	"deadmanssnitch-operator",
	"openshift-deployment-validation-operator",
	"gcp-project-operator",
	"openshift-velero",
	"pagerduty-operator",
	"openshift-route-monitor-operator",
	"openshift-observability-operator",
}

var listOfOperatorNames = []string{
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
	"gcp-project-operator",
	"managed-velero-operator",
	"pagerduty-operator",
	"route-monitor-operator",
	"observability-operator",
}

var (
	RestoreColor = "\033[0m"
	Red          = "\033[31m"
	Bold         = "\u001b[1m"
	Gap          = "         "
)

func newCmdList(streams genericclioptions.IOStreams, client client.Client) *cobra.Command {
	opts := &sreOperatorsListOptions{
		kubeCli:   client,
		IOStreams: streams,
	}

	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List the current and latest version of SRE operators",
		Long:              sreOperatorsListDescription,
		Example:           sreOperatorsListExample,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(opts.checks(cmd))
			output, _ := opts.ListOperators(cmd)
			util.CheckErr(opts.printText(output))
		},
	}

	listCmd.Flags().BoolVar(&opts.short, "short", false, "Exclude fetching the latest version from repositories for faster output")
	listCmd.Flags().BoolVar(&opts.outdated, "outdated", false, "Filter to only show operators running outdated versions")
	listCmd.Flags().BoolVar(&opts.noHeaders, "no-headers", false, "Exclude headers from the output")
	listCmd.Flags().StringVar(&opts.operator, "operator", "", "Filter to only show the specified operator.")
	listCmd.Flags().BoolVar(&opts.noCommit, "no-commit", false, "Excluse commit shas and repository URL from the output")

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
	p := printer.NewTablePrinter(ctx.IOStreams.Out, 18, 1, 3, ' ')

	if opList == nil {
		return nil
	}

	if !ctx.noHeaders {
		p.AddRow([]string{Bold})
		header := []string{"NAME", "CURRENT", "CUR. COMMIT", "EXPECTED", "EXP. COMMIT", "STATUS", "CHANNEL", "Operator Repository URL" + RestoreColor}
		if ctx.noCommit {
			header = []string{"NAME", "CURRENT", "EXPECTED", "STATUS", "CHANNEL" + RestoreColor}
		}
		if ctx.short {
			header = []string{"NAME", "CURRENT", "CUR. COMMIT", "STATUS", "CHANNEL" + RestoreColor}
		}
		if ctx.short && ctx.noCommit {
			header = []string{"NAME", "CURRENT", "STATUS", "CHANNEL" + RestoreColor}
		}
		p.AddRow(header)
	}

	sort.Slice(opList, func(i, j int) bool {
		return opList[i].Name < opList[j].Name
	})

	for _, op := range opList {
		if ctx.outdated && op.Current == op.Expected {
			continue
		}
		row := []string{op.Name, op.Current, op.CurrentCommit, op.Status, op.Channel}
		if !ctx.short {
			row = []string{op.Name, op.Current, op.CurrentCommit, op.Expected, op.ExpectedCommit, op.Status, op.Channel, op.RepositoryURL}
			if ctx.noCommit {
				row = []string{op.Name, op.Current, op.Expected, op.Status, op.Channel}
			}
			if op.Expected == "" {
				row = []string{Red + Bold + op.Name, Gap + op.Current, Gap + op.CurrentCommit, Gap + "not found (Gitlab token may be missing or expired)", Gap + op.ExpectedCommit, Gap + op.Status, Gap + op.Channel, Gap + op.RepositoryURL + RestoreColor}
			} else if op.Current != op.Expected {
				row = []string{Red + Bold + op.Name, Gap + op.Current, Gap + op.CurrentCommit, Gap + op.Expected, Gap + op.ExpectedCommit, Gap + op.Status, Gap + op.Channel, Gap + op.RepositoryURL + RestoreColor}
				if ctx.noCommit {
					row = []string{Red + Bold + op.Name, Gap + op.Current, Gap + op.Expected, Gap + op.Status, Gap + op.Channel + RestoreColor}
				}
			}
		}
		if ctx.short && ctx.noCommit {
			row = []string{op.Name, op.Current, op.Status, op.Channel}
		}

		p.AddRow(row)
	}
	if err := p.Flush(); err != nil {
		return err
	}

	return nil
}

type operatorResult struct {
	Operator sreOperator
	Error    error
}

func (ctx *sreOperatorsListOptions) ListOperators(cmd *cobra.Command) ([]sreOperator, error) {

	// dynamically allocates number of workers based on CPU cores
	workerLimit := runtime.NumCPU() * 2
	resultChannel := make(chan operatorResult, len(listOfOperators))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workerLimit)

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

	gitlabClient := &gitlab.Client{}
	if !ctx.short {
		gitlab_access := viper.GetString("gitlab_access")
		if gitlab_access == "" {
			fmt.Println("gitlab access token not found, please ensure your gitlab access token is set in the .config/osdctl file in the format: 'gitlab_access: \"<TOKEN>\"'.")
			return nil, nil
		}
		gitlabClient, _ = gitlab.NewClient(gitlab_access, gitlab.WithBaseURL("https://gitlab.cee.redhat.com/"))
	}

	if ctx.operator != "" {
		for i, operator := range listOfOperators {
			if operator == ctx.operator || listOfOperatorNames[i] == ctx.operator {
				listOfOperators = []string{operator}
				listOfOperatorNames = []string{listOfOperatorNames[i]}
				break
			} else if i == len(listOfOperators)-1 {
				fmt.Printf("Error: Operator '%s' not found", ctx.operator)
				return nil, nil
			}
		}
	}

	// iterate through list of operators
	for operator := range listOfOperators {
		wg.Add(1)
		go func(oper, operatorName string, i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			currentVersion := make([]string, len(listOfOperators))
			latestVersion := make([]string, len(listOfOperators))
			currentCommit, expectedCommit, operatorChannel, operatorStatus, repositoryUrl := "", "", "", "", ""

			if !ctx.short {
				latestVersion[i], repositoryUrl, expectedCommit = getLatestVersion(gitlabClient, listOfOperatorNames[i])
			}

			csvListCopy := csvList.DeepCopy()
			subListCopy := subList.DeepCopy()

			if err := ctx.kubeCli.List(context.TODO(), csvListCopy, client.InNamespace(listOfOperators[i])); err != nil {
				return
			} else {
				for _, item := range csvListCopy.Items {
					if strings.Contains(item.GetName(), listOfOperatorNames[i]) {
						currentVersion[i] = item.GetName()
						operatorStatus = item.Object["status"].(map[string]interface{})["phase"].(string)
					}
				}
				if currentVersion[i] == "" {
					return
				}

				if err := ctx.kubeCli.List(context.TODO(), subListCopy, client.InNamespace(listOfOperators[i])); err != nil {
					return
				} else {
					for _, item := range subListCopy.Items {
						if strings.Contains(item.GetName(), listOfOperatorNames[i]) {
							operatorChannel = item.Object["spec"].(map[string]interface{})["channel"].(string)
						}
					}
				}

				currentCommit = extractCommit(currentVersion[i])
				currentVersion[i] = extractVersion(currentVersion[i])
			}

			op := sreOperator{
				Name:           listOfOperatorNames[i],
				Current:        currentVersion[i],
				CurrentCommit:  currentCommit,
				Expected:       latestVersion[i],
				ExpectedCommit: expectedCommit,
				Status:         operatorStatus,
				Channel:        operatorChannel,
				RepositoryURL:  repositoryUrl,
			}
			resultChannel <- operatorResult{Operator: op, Error: nil}
		}(listOfOperators[operator], listOfOperatorNames[operator], operator)
	}

	go func() {
		wg.Wait()
		close(resultChannel)
	}()

	var opList []sreOperator
	for result := range resultChannel {
		if result.Error != nil {
			fmt.Println("Error: ", result.Error)
		}
		opList = append(opList, result.Operator)
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

func extractCommit(input string) string {
	regex := regexp.MustCompile(`((?:)v[0-9\.]+-(.*))`)
	extracted := regex.FindStringSubmatch(input)

	if len(extracted) > 1 {
		if len(extracted[2]) > 7 {
			extracted[2] = extracted[2][1:]
			return extracted[2]
		}
		return extracted[2]
	} else {
		return ""
	}
}

func getLatestVersion(gitClient *gitlab.Client, operatorName string) (string, string, string) {

	// Special case for deployment-validation-operator: version is stored in a text file
	if operatorName == "deployment-validation-operator" {
		repoLink := "service/saas-operator-versions"
		filePath := "deployment-validation-operator/deployment-validation-operator-versions.txt"

		fileTxt, _, err := gitClient.RepositoryFiles.GetFile(repoLink, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr("master")})
		if err != nil {
			fmt.Println(operatorName, "- Failed to obtain GitLab file: ", err)
			return "", "", ""
		}
		decodedFileTxt, err := base64.StdEncoding.DecodeString(fileTxt.Content)
		if err != nil {
			fmt.Println(operatorName, "failed to decode file: ", err)
		}
		line := strings.Split(string(decodedFileTxt), "\n")
		for i := len(line) - 1; i >= 0; i-- {
			if line[i] != "" {
				expectedVersion := extractVersion("v" + line[i])
				expectedCommit := extractCommit("v" + line[i])
				return expectedVersion, "https://github.com/app-sre/deployment-validation-operator/", expectedCommit
			}
		}
	}
	// Special case for must-gather-operator
	if operatorName == "must-gather-operator" {
		repoLink := "service/saas-" + operatorName + "-bundle"
		filePath := operatorName + "/" + operatorName + ".package.yaml"
		// Get the latest release branch
		repositoryBranches, _, err := gitClient.Branches.ListBranches("service/saas-must-gather-operator-bundle", &gitlab.ListBranchesOptions{})
		if err != nil {
			fmt.Println("Failed to list branches: ", err)
			return "", "", ""
		}
		repositoryBranch := ""
		highestVersion := 0.0
		re := regexp.MustCompile(`release-(\d+\.\d+)`)

		for _, branch := range repositoryBranches {
			matches := re.FindStringSubmatch(branch.Name)
			if len(matches) == 0 {
				continue
			}
			version, _ := strconv.ParseFloat(matches[1], 64)
			if version > highestVersion {
				highestVersion = version
				repositoryBranch = branch.Name
			}
		}
		fileYaml, _, err := gitClient.RepositoryFiles.GetFile(repoLink, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(repositoryBranch)})
		if err != nil {
			fmt.Println(operatorName, "- Failed to obtain GitLab file: ", err)
			return "", "", ""
		}
		decodedYamlString, err := base64.StdEncoding.DecodeString(fileYaml.Content)
		if err != nil {
			fmt.Println(operatorName, "failed to decode file: ", err)
		}
		expectedVersion := extractVersion(string(decodedYamlString))
		return expectedVersion, "https://github.com/openshift/must-gather-operator/", extractCommit(string(decodedYamlString))
	}
	// Special case for observability-operator
	if operatorName == "observability-operator" {
		repoLink := "service/app-interface"
		filePath := "data/services/osd-operators/cicd/saas/saas-observability-operator.yaml"
		fileYaml, _, err := gitClient.RepositoryFiles.GetFile(repoLink, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr("master")})
		if err != nil {
			fmt.Println(operatorName, "- Failed to obtain GitLab file: ", err)
			return "", "", ""
		}
		// decode base64
		decodedYamlString, err := base64.StdEncoding.DecodeString(fileYaml.Content)
		if err != nil {
			fmt.Println(operatorName, "failed to decode file: ", err)
		}
		yamlContent := string(decodedYamlString)
		re := regexp.MustCompile(`hivep01ue1/cluster-scope.yml
    ref:\s*(\S+)`)
		matches := re.FindStringSubmatch(yamlContent)

		if len(matches) == 0 {
			fmt.Println("Failed to extract version from observability-operator")
		}

		version, sha := getObservabilityOperatorVersion(matches[1])

		return version, "https://github.com/rhobs/observability-operator/", sha

	}

	repoLink := "service/saas-" + operatorName + "-bundle"
	filePath := operatorName + "/" + operatorName + ".package.yaml"

	fileYaml, _, err := gitClient.RepositoryFiles.GetFile(repoLink, filePath, &gitlab.GetFileOptions{Ref: gitlab.Ptr(repositoryBranch)})
	if err != nil {
		fmt.Println(operatorName, "- Failed to obtain GitLab file: ", err)
		return "", "", ""
	}

	// decode base64
	decodedYamlString, err := base64.StdEncoding.DecodeString(fileYaml.Content)
	if err != nil {
		fmt.Println(operatorName, "failed to decode file: ", err)
	}

	expectedVersion := extractVersion(string(decodedYamlString))

	repositoryUrl := "https://github.com/openshift/" + operatorName

	return expectedVersion, repositoryUrl, extractCommit(string(decodedYamlString))
}

func getObservabilityOperatorVersion(sha string) (string, string) {
	githubClient := github.NewClient(nil)

	fileGithub, _, _ := githubClient.Repositories.ListTags(context.TODO(), "rhobs", "observability-operator", &github.ListOptions{})

	for _, tag := range fileGithub {
		if *tag.Commit.SHA == sha {
			expectedVersion := *tag.Name
			sha = sha[:7]
			return expectedVersion, sha
		}
	}

	return "", ""
}
