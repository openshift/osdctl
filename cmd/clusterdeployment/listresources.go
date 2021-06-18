package clusterdeployment

import (
	"context"
	"fmt"
	"strings"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	gcpv1alpha1 "github.com/openshift/gcp-project-operator/pkg/apis/gcp/v1alpha1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/spf13/cobra"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
)

// newCmdList implements the list command to list
func newCmdListResources(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newListResourcesOptions(streams, flags)
	lrCmd := &cobra.Command{
		Use:               "listresources",
		Short:             "List all resources on a hive cluster related to a given cluster",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(runListResources(cmd, ops))
		},
	}
	lrCmd.Flags().StringVarP(&ops.clusterId, "cluster-id", "C", "", "Cluster ID")

	return lrCmd
}

// listOptions defines the struct for running list command
type listResourcesOptions struct {
	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli   client.Client
	clusterId string
}

func newListResourcesOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *listResourcesOptions {
	return &listResourcesOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *listResourcesOptions) complete(_ *cobra.Command, _ []string) error {
	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func runListResources(cmd *cobra.Command, o *listResourcesOptions) error {
	if o.clusterId == "" {
		return cmdutil.UsageErrorf(cmd, "No cluster ID specified, use -C to set one")
	}
	clusterDeployment, err := getClusterDeployment(cmd, o)
	if err != nil {
		return err
	}

	p := printer.NewTablePrinter(o.IOStreams.Out, 20, 1, 3, ' ')
	p.AddRow([]string{"Group", "Version", "Kind", "Namespace", "Name"})
	p.AddRow(createRow(clusterDeployment.ObjectMeta, clusterDeployment.TypeMeta))

	if clusterDeployment.Spec.Platform.AWS != nil {
		accountClaim, err := getAccountClaim(clusterDeployment, cmd, o)
		if err != nil {
			return err
		}
		p.AddRow(createRow(accountClaim.ObjectMeta, accountClaim.TypeMeta))
		account, err := getAccount(accountClaim, cmd, o)
		if err != nil {
			return err
		}
		p.AddRow(createRow(account.ObjectMeta, account.TypeMeta))
	}

	if clusterDeployment.Spec.Platform.GCP != nil {
		projectClaim, err := getProjectClaim(clusterDeployment, cmd, o)
		if err != nil {
			return err
		}
		p.AddRow(createRow(projectClaim.ObjectMeta, projectClaim.TypeMeta))
		projectReference, err := getProjectReference(projectClaim, cmd, o)
		fmt.Printf("%s\n", projectReference.Kind)
		if err != nil {
			return err
		}
		p.AddRow(createRow(projectReference.ObjectMeta, projectReference.TypeMeta))
	}

	return p.Flush()
}

func getClusterDeployment(cmd *cobra.Command, o *listResourcesOptions) (hivev1.ClusterDeployment, error) {
	var cds hivev1.ClusterDeploymentList
	if err := o.kubeCli.List(context.TODO(), &cds, &client.ListOptions{}); err != nil {
		return hivev1.ClusterDeployment{}, err
	}

	for _, cd := range cds.Items {
		if strings.Contains(cd.Namespace, o.clusterId) {
			return cd, nil
		}
	}
	return hivev1.ClusterDeployment{}, cmdutil.UsageErrorf(cmd, "ClusterDeployment not found")
}

func createRow(m v1.ObjectMeta, t v1.TypeMeta) []string {
	group, version := getGroupVersion(t.APIVersion)
	return []string{group, version, t.Kind, m.Namespace, m.Name}
}

func getGroupVersion(apiVersion string) (group, version string) {
	substrings := strings.Split(apiVersion, "/")
	if len(substrings) < 2 {
		return
	}
	group = substrings[0]
	version = substrings[1]
	return
}

func getAccountClaim(cd hivev1.ClusterDeployment, cmd *cobra.Command, o *listResourcesOptions) (awsv1alpha1.AccountClaim, error) {
	var accountClaims awsv1alpha1.AccountClaimList
	if err := o.kubeCli.List(context.TODO(), &accountClaims, &client.ListOptions{Namespace: cd.Namespace}); err != nil {
		return awsv1alpha1.AccountClaim{}, err
	}

	if len(accountClaims.Items) < 1 {
		return awsv1alpha1.AccountClaim{}, cmdutil.UsageErrorf(cmd, "AccountClaim not found")
	}
	if len(accountClaims.Items) > 1 {
		return awsv1alpha1.AccountClaim{}, cmdutil.UsageErrorf(cmd, "more than 1 AccountClaim found")
	}
	return accountClaims.Items[0], nil
}

func getAccount(claim awsv1alpha1.AccountClaim, cmd *cobra.Command, o *listResourcesOptions) (awsv1alpha1.Account, error) {
	var accounts awsv1alpha1.AccountList
	if err := o.kubeCli.List(context.TODO(), &accounts, &client.ListOptions{Namespace: "aws-account-operator"}); err != nil {
		return awsv1alpha1.Account{}, err
	}

	for _, acc := range accounts.Items {
		if acc.Name == claim.Spec.AccountLink {
			return acc, nil
		}
	}
	return awsv1alpha1.Account{}, cmdutil.UsageErrorf(cmd, "Account not found")
}

func getProjectClaim(cd hivev1.ClusterDeployment, cmd *cobra.Command, o *listResourcesOptions) (gcpv1alpha1.ProjectClaim, error) {
	var projectClaims gcpv1alpha1.ProjectClaimList
	if err := o.kubeCli.List(context.TODO(), &projectClaims, &client.ListOptions{Namespace: cd.Namespace}); err != nil {
		return gcpv1alpha1.ProjectClaim{}, err
	}

	if len(projectClaims.Items) < 1 {
		return gcpv1alpha1.ProjectClaim{}, cmdutil.UsageErrorf(cmd, "ProjectClaim not found")
	}
	if len(projectClaims.Items) > 1 {
		return gcpv1alpha1.ProjectClaim{}, cmdutil.UsageErrorf(cmd, "more than 1 ProjectClaim found")
	}

	return projectClaims.Items[0], nil
}

func getProjectReference(claim gcpv1alpha1.ProjectClaim, cmd *cobra.Command, o *listResourcesOptions) (gcpv1alpha1.ProjectReference, error) {
	var projectReferences gcpv1alpha1.ProjectReferenceList
	if err := o.kubeCli.List(context.TODO(), &projectReferences, &client.ListOptions{Namespace: claim.Spec.ProjectReferenceCRLink.Namespace}); err != nil {
		return gcpv1alpha1.ProjectReference{}, err
	}

	for _, projectReference := range projectReferences.Items {
		if projectReference.Name == claim.Spec.ProjectReferenceCRLink.Name {
			return projectReference, nil
		}
	}
	return gcpv1alpha1.ProjectReference{}, cmdutil.UsageErrorf(cmd, "ProjectReference not found")
}
