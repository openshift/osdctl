package sre_operators

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/openshift/osdctl/pkg/printer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xanzy/go-gitlab"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type sreOperatorsDescribeOptions struct {
	genericclioptions.IOStreams
	kubeCli client.Client
}

type sreOperatorDetails struct {
	basic sreOperator

	CsvHealthPhase   string
	CsvHealthMessage string

	SubscriptionMessage string

	OperartorGroupHealth string

	DeploymentHealth string
	PodHealth        string
}

const (
	sreOperatorsDescribeExample = `
		# Describe SRE operators
		$ osdctl cluster sre-operators describe <operator-name>
	`
	sreOperatorsLongDescription = `
  Helps obtain various health information about a specified SRE operator within a cluster,
  including CSV, Subscription, OperatorGroup, Deployment, and Pod health statuses.

  A git_access token is required to fetch the latest version of the operators, and can be 
  set within the config file using the 'osdctl setup' command.

  The command creates a Kubernetes client to access the current cluster context, and GitLab/GitHub
  clients to fetch the latest versions of each operator from its respective repository.
	`
)

func newCmdDescribe(streams genericclioptions.IOStreams, client client.Client) *cobra.Command {
	opts := &sreOperatorsDescribeOptions{
		kubeCli:   client,
		IOStreams: streams,
	}

	describeCmd := &cobra.Command{
		Use:     "describe",
		Short:   "Describe SRE operators",
		Long:    sreOperatorsLongDescription,
		Example: sreOperatorsDescribeExample,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(opts.checks(cmd))
			output, _ := opts.DescribeOperator(cmd, args[0])
			util.CheckErr(opts.printText(output))
		},
	}

	return describeCmd
}

func (ctx *sreOperatorsDescribeOptions) checks(cmd *cobra.Command) error {
	if _, err := config.GetConfig(); err != nil {
		return util.UsageErrorf(cmd, "could not find KUBECONFIG, please make sure you are logged into a cluster")
	}

	return nil
}

func (ctx *sreOperatorsDescribeOptions) printText(output []sreOperatorDetails) error {
	p := printer.NewTablePrinter(ctx.IOStreams.Out, 20, 1, 3, ' ')

	for _, op := range output {
		p.AddRow([]string{"Operator Name:", op.basic.Name})
		if op.basic.Current != op.basic.Expected {
			p.AddRow([]string{"Current Version:", Red + op.basic.Current + " (outdated)" + RestoreColor})
			p.AddRow([]string{"Expected Version:", op.basic.Expected})
		} else {
			p.AddRow([]string{"Current Version:", op.basic.Current})
			p.AddRow([]string{"Expected Version:", op.basic.Expected})
		}
		p.AddRow([]string{"Expected Version Commit URL:", op.basic.CommitURL})
		p.AddRow([]string{"Channel:", op.basic.Channel})
		p.AddRow([]string{"CSV Status:", op.basic.Status})
		p.AddRow([]string{"CSV Phase:", op.CsvHealthPhase})
		p.AddRow([]string{"Latest CSV Health Message:", op.CsvHealthMessage})
		p.AddRow([]string{"Latest Subscription Message:", op.SubscriptionMessage})
		p.AddRow([]string{"OperatorGroup Status:", op.OperartorGroupHealth})
		p.AddRow([]string{"Deployment Status:", op.DeploymentHealth})
		p.AddRow([]string{"Pod Status:", op.PodHealth})
	}

	if err := p.Flush(); err != nil {
		return err
	}

	return nil
}

func (ctx *sreOperatorsDescribeOptions) DescribeOperator(cmd *cobra.Command, operatorName string) ([]sreOperatorDetails, error) {

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

	opGroup := &unstructured.Unstructured{}
	opGroup.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1",
		Kind:    "OperatorGroup",
	})

	opGroupList := &unstructured.UnstructuredList{}
	opGroupList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operators.coreos.com",
		Version: "v1",
		Kind:    "OperatorGroupList",
	})

	gitlabClient := &gitlab.Client{}
	if 1 != 2 {
		gitlab_access := viper.GetString("gitlab_access")
		if gitlab_access == "" {
			fmt.Println("gitlab access token not found, please ensure your gitlab access token is set in the osdctl config")
			return nil, nil
		}
		gitlabClient, _ = gitlab.NewClient(gitlab_access, gitlab.WithBaseURL("https://gitlab.cee.redhat.com/"))
	}

	// check both lists for operator
	if !slices.Contains(listOfOperators, operatorName) && !slices.Contains(listOfOperatorNames, operatorName) {
		fmt.Println("Invalid operator name, please select one of the following operators:")
		for _, operator := range listOfOperatorNames {
			fmt.Println(operator)
		}
		return nil, nil
	}
	// Convert operator to the correct name
	if slices.Contains(listOfOperators, operatorName) {
		operatorName = listOfOperatorNames[slices.Index(listOfOperators, operatorName)]
	}

	opIndex := slices.Index(listOfOperatorNames, operatorName)

	currentVersion, csvStatus, operatorChannel, csvHealthPhase, csvHealthMessage, subMessage, opGroupStatus, podHealth, deploymentHealth := "", "", "", "", "", "", "", "", ""
	ExpectedVersion, commitUrl := getLatestVersion(gitlabClient, operatorName)

	if err := ctx.kubeCli.List(context.TODO(), csvList, client.InNamespace(listOfOperators[opIndex])); err != nil {
		fmt.Println("Error retrieving CSV details")
		return nil, err
	} else {
		for _, item := range csvList.Items {
			if strings.Contains(item.GetName(), operatorName) {
				currentVersion = item.GetName()
				csvStatus = item.Object["status"].(map[string]interface{})["phase"].(string)
				conditions := item.Object["status"].(map[string]interface{})["conditions"]
				lastCondition := conditions.([]interface{})[len(conditions.([]interface{}))-1].(map[string]interface{})
				csvHealthPhase = lastCondition["phase"].(string)
				csvHealthMessage = lastCondition["message"].(string) + ", reason: " + lastCondition["reason"].(string)
			}
		}
	}

	if err := ctx.kubeCli.List(context.TODO(), subList, client.InNamespace(listOfOperators[opIndex])); err != nil {
		fmt.Println("Error retrieving subscription details")
		return nil, err
	} else {
		for _, item := range subList.Items {
			if strings.Contains(item.GetName(), operatorName) {
				operatorChannel = item.Object["spec"].(map[string]interface{})["channel"].(string)
				subMessage = item.Object["status"].(map[string]interface{})["conditions"].([]interface{})[0].(map[string]interface{})["message"].(string)
			}
		}
	}

	if err := ctx.kubeCli.List(context.TODO(), opGroupList, client.InNamespace(listOfOperators[opIndex])); err != nil {
		fmt.Println("Error retrieving Operator Group details")
		return nil, err
	} else {
		for _, item := range opGroupList.Items {
			if strings.Contains(item.GetName(), operatorName) {
				opGroupStatus = "Exists, last updated: " + item.Object["status"].(map[string]interface{})["lastUpdated"].(string)
			} else {
				opGroupStatus = "Not Found"
			}
		}
	}

	podList := &corev1.PodList{}
	if err := ctx.kubeCli.List(context.TODO(), podList, client.InNamespace(listOfOperators[opIndex])); err != nil {
		fmt.Println("Error retrieving Pod details")
		return nil, err
	} else {
		for _, item := range podList.Items {
			if strings.Contains(item.GetName(), operatorName) {
				podHealth = string(item.Status.Phase)
			}
		}
	}

	deploymentList := &unstructured.UnstructuredList{}
	deploymentList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "DeploymentList",
	})
	if err := ctx.kubeCli.List(context.TODO(), deploymentList, client.InNamespace(listOfOperators[opIndex])); err != nil {
		fmt.Println("Error retrieving operator details")
		return nil, err
	} else {
		for _, item := range deploymentList.Items {
			if strings.Contains(item.GetName(), operatorName) {
				deploymentHealth = "Exists, last updated: " + item.Object["status"].(map[string]interface{})["conditions"].([]interface{})[0].(map[string]interface{})["lastUpdateTime"].(string)
			} else {
				deploymentHealth = "Not Found"
			}
		}
	}
	currentVersion = extractVersion(currentVersion)
	op := sreOperatorDetails{
		basic: sreOperator{operatorName, currentVersion, ExpectedVersion, csvStatus, operatorChannel, commitUrl},

		CsvHealthPhase:   csvHealthPhase,
		CsvHealthMessage: csvHealthMessage,

		SubscriptionMessage: subMessage,

		OperartorGroupHealth: opGroupStatus,

		DeploymentHealth: deploymentHealth,
		PodHealth:        podHealth,
	}

	var operator []sreOperatorDetails
	operator = append(operator, op)

	return operator, nil
}
