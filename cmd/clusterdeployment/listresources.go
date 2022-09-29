package clusterdeployment

import (
	"context"
	"strings"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	gcpv1alpha1 "github.com/openshift/gcp-project-operator/api/v1alpha1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/pkg/printer"
)

// newCmdList implements the list command to list
func newCmdListResources(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	l := newListResources(streams, flags, client)
	lrCmd := &cobra.Command{
		Use:               "listresources",
		Short:             "List all resources on a hive cluster related to a given cluster",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(l.complete(cmd, args))
			cmdutil.CheckErr(l.RunListResources())
		},
	}
	lrCmd.Flags().StringVarP(&l.ClusterId, "cluster-id", "C", "", "Cluster ID")
	lrCmd.Flags().BoolVarP(&l.ExternalResourcesOnly, "external", "e", false, "only list external resources (i.e. exclude resources in cluster namespace)")

	return lrCmd
}

// listOptions defines the struct for running list command
type ListResources struct {
	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams

	ClusterDeployment     hivev1.ClusterDeployment
	ClusterId             string
	P                     Printer
	ExternalResourcesOnly bool
	KubeCli               client.Client
	Cmd                   *cobra.Command
}

//go:generate mockgen -destination ./mock/k8s/client.go -package client sigs.k8s.io/controller-runtime/pkg/client Client
//go:generate mockgen -destination ./mock/printer/printer.go -package printer -source listresources.go Printer
type Printer interface {
	AddRow(row []string)
	Flush() error
}

func newListResources(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *ListResources {
	return &ListResources{
		flags:     flags,
		IOStreams: streams,
		P:         printer.NewTablePrinter(streams.Out, 20, 1, 3, ' '),
		KubeCli:   client,
	}
}

func (l *ListResources) complete(cmd *cobra.Command, _ []string) error {
	var err error

	err = l.getClusterDeployment()
	if err != nil {
		return err
	}

	l.Cmd = cmd

	return nil
}

func (l *ListResources) RunListResources() error {
	if l.ClusterId == "" {
		return cmdutil.UsageErrorf(l.Cmd, "No cluster ID specified, use -C to set one")
	}
	l.P.AddRow([]string{"Group", "Version", "Kind", "Namespace", "Name"})
	l.PrintRow(l.ClusterDeployment.ObjectMeta, l.ClusterDeployment.TypeMeta)

	if l.ClusterDeployment.Spec.Platform.AWS != nil {
		accountClaim, err := l.getAccountClaim(l.ClusterDeployment)
		if err != nil {
			return err
		}
		l.PrintRow(accountClaim.ObjectMeta, accountClaim.TypeMeta)
		account, err := l.getAccount(accountClaim)
		if err != nil {
			return err
		}
		l.PrintRow(account.ObjectMeta, account.TypeMeta)
		iamSecret, err := l.getSecret(account)
		if err != nil {
			return err
		}
		l.PrintRow(iamSecret.ObjectMeta, iamSecret.TypeMeta)
	}

	if l.ClusterDeployment.Spec.Platform.GCP != nil {
		projectClaim, err := l.getProjectClaim(l.ClusterDeployment)
		if err != nil {
			return err
		}
		l.PrintRow(projectClaim.ObjectMeta, projectClaim.TypeMeta)
		projectReference, err := l.getProjectReference(projectClaim)
		if err != nil {
			return err
		}
		l.PrintRow(projectReference.ObjectMeta, projectReference.TypeMeta)
	}

	return l.P.Flush()
}

func (l *ListResources) PrintRow(m v1.ObjectMeta, t v1.TypeMeta) {
	if m.Namespace != l.ClusterDeployment.Namespace || !l.ExternalResourcesOnly {
		l.P.AddRow(createRow(m, t))
	}
}

func (l *ListResources) getClusterDeployment() error {
	var cds hivev1.ClusterDeploymentList
	if err := l.KubeCli.List(context.TODO(), &cds, &client.ListOptions{}); err != nil {
		return err
	}

	for _, cd := range cds.Items {
		if strings.Contains(cd.Namespace, l.ClusterId) {
			l.ClusterDeployment = cd
			return nil
		}
	}
	return cmdutil.UsageErrorf(l.Cmd, "ClusterDeployment not found")
}

func createRow(m v1.ObjectMeta, t v1.TypeMeta) []string {
	group, version := getGroupVersion(t.APIVersion)
	if group == "" {
		group = `""`
	}
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

func (l *ListResources) getAccountClaim(cd hivev1.ClusterDeployment) (awsv1alpha1.AccountClaim, error) {
	var accountClaims awsv1alpha1.AccountClaimList
	if err := l.KubeCli.List(context.TODO(), &accountClaims, &client.ListOptions{Namespace: cd.Namespace}); err != nil {
		return awsv1alpha1.AccountClaim{}, err
	}

	if len(accountClaims.Items) < 1 {
		return awsv1alpha1.AccountClaim{}, cmdutil.UsageErrorf(l.Cmd, "AccountClaim not found")
	}
	if len(accountClaims.Items) > 1 {
		return awsv1alpha1.AccountClaim{}, cmdutil.UsageErrorf(l.Cmd, "more than 1 AccountClaim found")
	}
	return accountClaims.Items[0], nil
}

func (l *ListResources) getAccount(claim awsv1alpha1.AccountClaim) (awsv1alpha1.Account, error) {
	var accounts awsv1alpha1.AccountList
	if err := l.KubeCli.List(context.TODO(), &accounts, &client.ListOptions{Namespace: "aws-account-operator"}); err != nil {
		return awsv1alpha1.Account{}, err
	}

	for _, acc := range accounts.Items {
		if acc.Name == claim.Spec.AccountLink {
			return acc, nil
		}
	}
	return awsv1alpha1.Account{}, cmdutil.UsageErrorf(l.Cmd, "Account not found")
}

func (o *ListResources) getSecret(account awsv1alpha1.Account) (corev1.Secret, error) {

	// SREP User is not allowed to list Secrets, hence we need to fake one until osdctl can impersonate
	secret := corev1.Secret{
		TypeMeta: v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "/v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      account.Spec.IAMUserSecret,
			Namespace: account.Namespace,
		},
	}
	return secret, nil
}

func (l *ListResources) getProjectClaim(cd hivev1.ClusterDeployment) (gcpv1alpha1.ProjectClaim, error) {
	var projectClaims gcpv1alpha1.ProjectClaimList
	if err := l.KubeCli.List(context.TODO(), &projectClaims, &client.ListOptions{Namespace: cd.Namespace}); err != nil {
		return gcpv1alpha1.ProjectClaim{}, err
	}

	if len(projectClaims.Items) < 1 {
		return gcpv1alpha1.ProjectClaim{}, cmdutil.UsageErrorf(l.Cmd, "ProjectClaim not found")
	}
	if len(projectClaims.Items) > 1 {
		return gcpv1alpha1.ProjectClaim{}, cmdutil.UsageErrorf(l.Cmd, "more than 1 ProjectClaim found")
	}

	return projectClaims.Items[0], nil
}

func (l *ListResources) getProjectReference(claim gcpv1alpha1.ProjectClaim) (gcpv1alpha1.ProjectReference, error) {
	var projectReferences gcpv1alpha1.ProjectReferenceList
	if err := l.KubeCli.List(context.TODO(), &projectReferences, &client.ListOptions{Namespace: claim.Spec.ProjectReferenceCRLink.Namespace}); err != nil {
		return gcpv1alpha1.ProjectReference{}, err
	}

	for _, projectReference := range projectReferences.Items {
		if projectReference.Name == claim.Spec.ProjectReferenceCRLink.Name {
			return projectReference, nil
		}
	}
	return gcpv1alpha1.ProjectReference{}, cmdutil.UsageErrorf(l.Cmd, "ProjectReference not found")
}
