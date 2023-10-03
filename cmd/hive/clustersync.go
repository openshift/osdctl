package hive

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveapiv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/spf13/cobra"

	// "k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const hiveVersionMajorMinorPatchLabel string = "hive.openshift.io/version-major-minor-patch"

// listOptions defines the struct for running clustersync command
type clusterSyncFailuresOptions struct {
	includeLimitedSupport  bool
	includeHibernating     bool
	includeFailingSyncSets bool
	noHeaders              bool
	sortField              string
	sortDescending         bool

	genericclioptions.IOStreams
	kubeCli client.Client
}

type failingClusterSync struct {
	Name            string
	Namespace       string
	Timestamp       string
	LimitedSupport  bool
	Hibernating     bool
	FailingSyncSets string
}

// newCmdList implements the list command to list cluster deployment crs
func newCmdClusterSyncFailures(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	opts := newClusterSyncOptions(streams, flags, client)
	clusterSyncCmd := &cobra.Command{
		Use:               "clustersync-failures [flags]",
		Short:             "list clustersync failures",
		Args:              cobra.NoArgs,
		Aliases:           []string{"csf"},
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(opts.complete(cmd, args))
			cmdutil.CheckErr(opts.run())
		},
	}
	clusterSyncCmd.Flags().BoolVarP(&opts.includeLimitedSupport, "include-limited-support", "L", false, "Include clusters in limited support.")
	clusterSyncCmd.Flags().BoolVarP(&opts.includeHibernating, "include-hibernating", "H", false, "Include hibernating clusters.")
	clusterSyncCmd.Flags().BoolVarP(&opts.includeFailingSyncSets, "include-failing-syncsets", "F", false, "Include failing syncsets.")
	clusterSyncCmd.Flags().BoolVar(&opts.noHeaders, "no-headers", false, "Don't print headers.")
	clusterSyncCmd.Flags().BoolVar(&opts.sortDescending, "sort-desc", false, "Sort in a descending order. Default false.")
	clusterSyncCmd.Flags().StringVar(&opts.sortField, "sort-by", "timestamp", "Sort by field.")

	return clusterSyncCmd
}

func newClusterSyncOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *clusterSyncFailuresOptions {
	return &clusterSyncFailuresOptions{
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *clusterSyncFailuresOptions) complete(_ *cobra.Command, _ []string) error {
	return nil
}

func (o *clusterSyncFailuresOptions) run() error {
	csList, err := o.failingClusterSyncs()
	if err != nil {
		return err
	}

	if err = o.sortBy(csList); err != nil {
		return err
	}

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

	for _, cs := range csList {
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

	p.Flush()

	return nil
}

func (o *clusterSyncFailuresOptions) failingClusterSyncs() ([]failingClusterSync, error) {
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
		for _, sss := range cs.Status.SelectorSyncSets {
			if sss.Result == "Failure" {
				failingSyncSets.WriteString(sss.Name)
				failingSyncSets.WriteString(" ")
			}
		}
		for _, ss := range cs.Status.SyncSets {
			if ss.Result == "Failure" {
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
		}

		fcsList = append(fcsList, fc)
	}

	return fcsList, nil
}

func (o *clusterSyncFailuresOptions) sortBy(failingClusterSyncList []failingClusterSync) error {
	switch strings.ToLower(o.sortField) {
	case "name":
		sort.Slice(failingClusterSyncList, func(i, j int) bool {
			res := failingClusterSyncList[i].Name < failingClusterSyncList[j].Name
			if o.sortDescending {
				return !res
			}
			return res
		})
	case "timestamp":
		sort.Slice(failingClusterSyncList, func(i, j int) bool {
			res := failingClusterSyncList[i].Timestamp < failingClusterSyncList[j].Timestamp
			if o.sortDescending {
				return !res
			}
			return res
		})
	case "failingsyncsets":
		sort.Slice(failingClusterSyncList, func(i, j int) bool {
			res := failingClusterSyncList[i].FailingSyncSets < failingClusterSyncList[j].FailingSyncSets
			if o.sortDescending {
				return !res
			}
			return res
		})
	default:
		return fmt.Errorf("Specify one of the following fields as a sort argument: name, timestamp, failingsyncsets.")

	}

	return nil
}
