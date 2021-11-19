package clusterdeployment

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/pkg/printer"
)

// newCmdList implements the list command to list cluster deployment crs
func newCmdList(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newListOptions(streams, flags)
	listCmd := &cobra.Command{
		Use:               "list",
		Short:             "List cluster deployment crs",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	return listCmd
}

// listOptions defines the struct for running list command
type listOptions struct {
	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newListOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *listOptions {
	return &listOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *listOptions) complete(_ *cobra.Command, _ []string) error {
	return nil
}

func (o *listOptions) run() error {
	ctx := context.TODO()
	var cds hivev1.ClusterDeploymentList
	if err := o.kubeCli.List(ctx, &cds, &client.ListOptions{}); err != nil {
		return err
	}

	var (
		matched  bool
		platform string
		region   string
		version  string
	)
	p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
	p.AddRow([]string{"NameSpace", "Name", "API URL", "Completed Version", "Platform", "Region"})
	for _, cd := range cds.Items {
		// TODO: add more options when we support more platforms
		switch p := cd.Spec.Platform; {
		case p.AWS != nil:
			platform = "aws"
			region = p.AWS.Region
		case p.GCP != nil:
			platform = "gcp"
			region = p.GCP.Region
		case p.Azure != nil:
			platform = "azure"
			region = p.Azure.Region
		default:
			platform = ""
			region = ""
		}

		if cd.Status.ClusterVersionStatus != nil {
			for _, history := range cd.Status.ClusterVersionStatus.History {
				if history.State == configv1.CompletedUpdate {
					version = history.Version
					break
				}
			}
		}

		p.AddRow([]string{cd.Namespace, cd.Name, cd.Status.APIURL, version, platform, region})

		matched = true
	}

	if matched {
		return p.Flush()
	}
	return nil
}
