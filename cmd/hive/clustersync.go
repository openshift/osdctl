package hive

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveapiv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// failingClusterSync represents a failing ClusterSync
type failingClusterSync struct {
	Name            string
	Namespace       string
	Timestamp       string
	LimitedSupport  bool
	Hibernating     bool
	FailingSyncSets string
	ErrorMessage    string
}

// clusterSyncFailuresOptions defines the struct for running clustersync command
type clusterSyncFailuresOptions struct {
	clusterID              string
	includeLimitedSupport  bool
	includeHibernating     bool
	includeFailingSyncSets bool
	noHeaders              bool
	output                 string
	sortField              string
	sortOrder              string

	genericclioptions.IOStreams
	kubeCli client.Client
}

const (
	clusterSyncFailuresLongDescription = `
  Helps investigate ClusterSyncs in a failure state on OSD/ROSA hive shards.

  This command by default will list ClusterSyncs that are in a failure state
  for clusters that are not in limited support or hibernating.

  Error messages are include in all output format except the text format.
`
	clusterSyncFailuresExample = `
  # List clustersync failures using the short version of the command
  $ osdctl hive csf

  # Output in a yaml format, excluding which syncsets are failing and sorting
  # by timestamp in a descending order
  $ osdctl hive csf --syncsets=false --output=yaml --sort-by=timestamp --order=desc

  # Include limited support and hibernating clusters
  $ osdctl hive csf --limited-support -hibernating

  # List failures and error message for a single cluster
  $ osdctl hive csf -C <cluster-id>
`
)

// NewCmdList implements the list command to list cluster deployment crs
func NewCmdClusterSyncFailures(streams genericclioptions.IOStreams, client client.Client) *cobra.Command {
	opts := &clusterSyncFailuresOptions{
		IOStreams: streams,
		kubeCli:   client,
	}
	clusterSyncCmd := &cobra.Command{
		Use:               "clustersync-failures [flags]",
		Short:             "List clustersync failures",
		Long:              clusterSyncFailuresLongDescription,
		Example:           clusterSyncFailuresExample,
		Args:              cobra.NoArgs,
		Aliases:           []string{"csf"},
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(opts.complete(cmd, args))
			cmdutil.CheckErr(opts.run())
		},
	}
	clusterSyncCmd.Flags().BoolVarP(&opts.includeLimitedSupport, "limited-support", "l", false, "Include clusters in limited support.")
	clusterSyncCmd.Flags().BoolVarP(&opts.includeHibernating, "hibernating", "i", false, "Include hibernating clusters.")
	clusterSyncCmd.Flags().BoolVarP(&opts.includeFailingSyncSets, "syncsets", "", true, "Include failing syncsets.")
	clusterSyncCmd.Flags().BoolVar(&opts.noHeaders, "no-headers", false, "Don't print headers when output format is set to text.")
	clusterSyncCmd.Flags().StringVarP(&opts.output, "output", "o", "text", "Set the output format. Options: yaml, json, csv, text.")
	clusterSyncCmd.Flags().StringVar(&opts.sortField, "sort-by", "timestamp", "Sort the output by a specified field. Options: name, timestamp, failingsyncsets.")
	clusterSyncCmd.Flags().StringVar(&opts.sortOrder, "order", "asc", "Set the sorting order. Options: asc, desc.")
	clusterSyncCmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Internal ID to list failing syncsets and relative errors for a specific cluster.")

	return clusterSyncCmd
}

func (o *clusterSyncFailuresOptions) complete(cmd *cobra.Command, args []string) error {
	if o.sortField != "name" && o.sortField != "timestamp" && o.sortField != "failingsyncsets" {
		return cmdutil.UsageErrorf(cmd, "invalid sort field.")
	}

	if o.sortOrder != "asc" && o.sortOrder != "desc" {
		return cmdutil.UsageErrorf(cmd, "sort order must be 'asc' or 'desc'")
	}

	if o.output != "yaml" && o.output != "json" && o.output != "csv" && o.output != "text" {
		return cmdutil.UsageErrorf(cmd, "invalid output field")
	}

	if _, err := config.GetConfig(); err != nil {
		return cmdutil.UsageErrorf(cmd, "could not find KUBECONFIG, please make sure you are logged into an hive shard")
	}

	return nil
}

func (o *clusterSyncFailuresOptions) run() error {
	if o.clusterID != "" {
		return o.printFailingCluster()
	}

	csList, err := o.listFailingClusterSyncs()
	if err != nil {
		return err
	}

	if err = o.sortBy(csList); err != nil {
		return err
	}

	switch o.output {
	case "json":
		if err = o.printJson(csList); err != nil {
			return err
		}
	case "yaml":
		if err = o.printYaml(csList); err != nil {
			return err
		}
	case "csv":
		if err = o.printCsv(csList); err != nil {
			return err
		}
	default:
		if err = o.printText(csList); err != nil {
			return err
		}
	}

	return nil
}

// printFailingCluster print sync failures relative to a specified cluster
func (o *clusterSyncFailuresOptions) printFailingCluster() error {
	opts := client.ListOptions{
		Namespace: "uhc-production-" + o.clusterID,
	}

	var cdList hivev1.ClusterDeploymentList
	if err := o.kubeCli.List(context.TODO(), &cdList, &opts); err != nil {
		return fmt.Errorf("could not retrieve ClusterDeployment, please make sure you are logged into the correct hive shard: %v", err)
	}

	var csList hiveapiv1alpha1.ClusterSyncList
	if err := o.kubeCli.List(context.TODO(), &csList, &opts); err != nil || len(csList.Items) == 0 {
		return fmt.Errorf("could not retrieve ClusterSync, please make sure you are logged into the correct hive shard: %v", err)
	}

	clusterDeployment := cdList.Items[0]
	clusterSync := csList.Items[0]

	_, isInLimitedSupport := clusterDeployment.Labels["api.openshift.com/limited-support"]

	isHibernating := false
	for _, condition := range clusterDeployment.Status.Conditions {
		if condition.Type == "Hibernating" && condition.Status == v1.ConditionTrue {
			isHibernating = true
			break
		}
	}

	fmt.Println("Cluster Name:", clusterDeployment.ObjectMeta.Name)
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("Status:")
	fmt.Printf("  Limited Support: %v\n", isInLimitedSupport)
	fmt.Printf("  Hibernating: %v\n", isHibernating)

	selectorSyncSetFailures := ""
	for _, sss := range clusterSync.Status.SelectorSyncSets {
		if sss.Result != "Success" {
			errorMessage := regexp.MustCompile(fmt.Sprintf("(.{%d})", 90)).ReplaceAllString(sss.FailureMessage, "$1\n")
			errorMessage = strings.ReplaceAll(errorMessage, "\n", "\n      ")

			selectorSyncSetFailures += fmt.Sprintf("  - Name: %s\n", sss.Name)
			selectorSyncSetFailures += fmt.Sprintf("    Error:\n      %s\n\n", errorMessage)
		}
	}

	if selectorSyncSetFailures != "" {
		fmt.Printf("\nSelectorSyncSet Failures:\n%s", selectorSyncSetFailures)
	}

	syncSetFailures := ""
	for _, ss := range clusterSync.Status.SyncSets {
		if ss.Result != "Success" {
			errorMessage := regexp.MustCompile(fmt.Sprintf("(.{%d})", 80)).ReplaceAllString(ss.FailureMessage, "$1\n")
			errorMessage = strings.ReplaceAll(errorMessage, "\n", "\n      ")

			syncSetFailures += fmt.Sprintf("  - Name: %s\n", ss.Name)
			syncSetFailures += fmt.Sprintf("    Error:\n      %s\n\n", errorMessage)
		}
	}

	if syncSetFailures != "" {
		fmt.Printf("\nSyncSet Failures:\n%s", syncSetFailures)
	}

	if selectorSyncSetFailures == "" && syncSetFailures == "" {
		fmt.Printf("\n\nNo failures\n\n")
	}

	fmt.Println(strings.Repeat("-", 40))
	return nil
}

// listFailingClusterSyncs list ClusterSyncs in a failure state
func (o *clusterSyncFailuresOptions) listFailingClusterSyncs() ([]failingClusterSync, error) {
	// Retrieve all clusterdeployments
	var cdList hivev1.ClusterDeploymentList
	if err := o.kubeCli.List(context.TODO(), &cdList, &client.ListOptions{}); err != nil {
		return nil, fmt.Errorf("could not retrieve ClusterDeployments, please make sure you are logged into an hive cluster: %v", err)
	}

	cdMap := make(map[string]hivev1.ClusterDeployment)
	for _, cd := range cdList.Items {
		cdMap[cd.Namespace] = cd
	}

	// Retrieve all clustersyncs
	var csList hiveapiv1alpha1.ClusterSyncList
	if err := o.kubeCli.List(context.TODO(), &csList, &client.ListOptions{}); err != nil {
		return nil, fmt.Errorf("could not retrieve ClusterSyncs, please make sure you are logged into an hive cluster: %v", err)
	}

	var fcsList []failingClusterSync
	for _, cs := range csList.Items {
		if len(cs.Status.Conditions) == 0 {
			continue
		}

		condition := cs.Status.Conditions[0]

		if condition.Reason != "Failure" {
			continue
		}

		_, isInLimitedSupport := cdMap[cs.Namespace].Labels["api.openshift.com/limited-support"]

		isHibernating := false
		for _, condition := range cdMap[cs.Namespace].Status.Conditions {
			if condition.Type == "Hibernating" && condition.Status == v1.ConditionTrue {
				isHibernating = true
				break
			}
		}

		var failingSyncSets strings.Builder
		errorMessage := ""
		for _, sss := range cs.Status.SelectorSyncSets {
			if sss.Result == "Failure" {
				errorMessage += sss.FailureMessage + "\n\n"
				failingSyncSets.WriteString(sss.Name)
				failingSyncSets.WriteString(" ")
			}
		}
		for _, ss := range cs.Status.SyncSets {
			if ss.Result == "Failure" {
				errorMessage += ss.FailureMessage + "\n\n"
				failingSyncSets.WriteString(ss.Name)
				failingSyncSets.WriteString(" ")
			}
		}

		fc := failingClusterSync{
			Name:            cs.Name,
			Namespace:       cs.Namespace,
			Timestamp:       condition.LastTransitionTime.Format(time.RFC3339),
			LimitedSupport:  isInLimitedSupport,
			Hibernating:     isHibernating,
			FailingSyncSets: failingSyncSets.String(),
			ErrorMessage:    errorMessage,
		}

		fcsList = append(fcsList, fc)
	}

	return fcsList, nil
}

// sortBy sort the ClusterSync failure list by the specified field
func (o *clusterSyncFailuresOptions) sortBy(failingClusterSyncList []failingClusterSync) error {
	switch strings.ToLower(o.sortField) {
	case "name":
		sort.Slice(failingClusterSyncList, func(i, j int) bool {
			res := failingClusterSyncList[i].Name < failingClusterSyncList[j].Name
			if o.sortOrder == "desc" {
				return !res
			}
			return res
		})
	case "timestamp":
		sort.Slice(failingClusterSyncList, func(i, j int) bool {
			res := failingClusterSyncList[i].Timestamp < failingClusterSyncList[j].Timestamp
			if o.sortOrder == "desc" {
				return !res
			}
			return res
		})
	case "failingsyncsets":
		sort.Slice(failingClusterSyncList, func(i, j int) bool {
			res := failingClusterSyncList[i].FailingSyncSets < failingClusterSyncList[j].FailingSyncSets
			if o.sortOrder == "desc" {
				return !res
			}
			return res
		})
	default:
		return fmt.Errorf("Specify one of the following fields as a sort argument: name, timestamp, failingsyncsets.")

	}

	return nil
}

// printJson prints the ClusterSync failures list in json format
func (o *clusterSyncFailuresOptions) printJson(failingClusterSyncList []failingClusterSync) error {
	filteredFailingClusterSyncList := []failingClusterSync{}
	for _, cs := range failingClusterSyncList {
		if !o.includeLimitedSupport && cs.LimitedSupport {
			continue
		}
		if !o.includeHibernating && cs.Hibernating {
			continue
		}
		filteredFailingClusterSyncList = append(filteredFailingClusterSyncList, cs)
	}

	if err := json.NewEncoder(os.Stdout).Encode(filteredFailingClusterSyncList); err != nil {
		return err
	}
	return nil
}

// printYaml prints the ClusterSync failures list in yaml format
func (o *clusterSyncFailuresOptions) printYaml(failingClusterSyncList []failingClusterSync) error {
	filteredFailingClusterSyncList := []failingClusterSync{}
	for _, cs := range failingClusterSyncList {
		if !o.includeLimitedSupport && cs.LimitedSupport {
			continue
		}
		if !o.includeHibernating && cs.Hibernating {
			continue
		}
		filteredFailingClusterSyncList = append(filteredFailingClusterSyncList, cs)
	}

	if err := yaml.NewEncoder(os.Stdout).Encode(failingClusterSyncList); err != nil {
		return err
	}
	return nil
}

// printCsv prints the ClusterSync failures list in csv format
func (o *clusterSyncFailuresOptions) printCsv(failingClusterSyncList []failingClusterSync) error {
	writer := csv.NewWriter(os.Stdout)

	headers := []string{"NAME", "NAMESPACE", "TIMESTAMP", "LIMITED SUPPORT", "HIBERNATING", "FAILING SYNCSETS", "ERROR MESSAGE"}
	if err := writer.Write(headers); err != nil {
		return err
	}

	for _, f := range failingClusterSyncList {
		if !o.includeLimitedSupport && f.LimitedSupport {
			continue
		}

		if !o.includeHibernating && f.Hibernating {
			continue
		}

		row := []string{
			f.Name,
			f.Namespace,
			f.Timestamp,
			strconv.FormatBool(f.LimitedSupport),
			strconv.FormatBool(f.Hibernating),
			f.FailingSyncSets,
			f.ErrorMessage,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	writer.Flush()

	return nil
}

// printText prints the ClusterSync failures list in text format
func (o *clusterSyncFailuresOptions) printText(failingClusterSyncList []failingClusterSync) error {
	p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')

	if !o.noHeaders {
		headers := []string{"NAMESPACE", "NAME", "TIMESTAMP"}
		if o.includeLimitedSupport {
			headers = append(headers, "LS")
		}
		if o.includeHibernating {
			headers = append(headers, "HIBERNATING")
		}

		if o.includeFailingSyncSets {
			headers = append(headers, "SYNCSETS")
		}
		p.AddRow(headers)
	}

	for _, cs := range failingClusterSyncList {
		if !o.includeLimitedSupport && cs.LimitedSupport {
			continue
		}

		if !o.includeHibernating && cs.Hibernating {
			continue
		}

		row := []string{cs.Namespace, cs.Name, cs.Timestamp}

		if o.includeLimitedSupport {
			row = append(row, strconv.FormatBool(cs.LimitedSupport))
		}

		if o.includeHibernating {
			row = append(row, strconv.FormatBool(cs.Hibernating))
		}

		if o.includeFailingSyncSets {
			row = append(row, cs.FailingSyncSets)
		}

		p.AddRow(row)
	}

	if err := p.Flush(); err != nil {
		return err
	}

	return nil
}
