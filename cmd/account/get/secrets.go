package get

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/spf13/cobra"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/printer"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secret = "-secret"
)

// newCmdGetSecrets implements the get secrets command which get
// the name of secrets related to the specified AWS Account ID
func newCmdGetSecrets(streams genericclioptions.IOStreams, client client.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newGetSecretsOptions(streams, client, globalOpts)
	getSecretsCmd := &cobra.Command{
		Use:               "secrets",
		Short:             "Get AWS Account CR related secrets",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	getSecretsCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	getSecretsCmd.Flags().StringVarP(&ops.accountID, "account-id", "i", "", "AWS account ID")

	return getSecretsCmd
}

// getSecretsOptions defines the structure for getting account related secrets
type getSecretsOptions struct {
	accountID        string
	accountNamespace string
	output           string

	printFlags *printer.PrintFlags
	genericclioptions.IOStreams
	kubeCli       client.Client
	GlobalOptions *globalflags.GlobalOptions
}

func newGetSecretsOptions(streams genericclioptions.IOStreams, client client.Client, globalOpts *globalflags.GlobalOptions) *getSecretsOptions {
	return &getSecretsOptions{
		IOStreams:     streams,
		kubeCli:       client,
		GlobalOptions: globalOpts,
	}
}

func (o *getSecretsOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.accountID == "" {
		return cmdutil.UsageErrorf(cmd, accountIDRequired)
	}

	o.output = o.GlobalOptions.Output
	return nil
}

func (o *getSecretsOptions) run() error {
	ctx := context.TODO()

	var (
		accounts      awsv1alpha1.AccountList
		accountCRName string
	)
	if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
		Namespace: o.accountNamespace,
	}); err != nil {
		return err
	}

	for _, account := range accounts.Items {
		if account.Spec.AwsAccountID == o.accountID {
			accountCRName = account.Name
		}
	}

	if accountCRName == "" {
		return fmt.Errorf("Account matches for AWS Account ID %s not found\n", o.accountID)
	}

	secretSuffixes := []string{secret}
	var secret v1.Secret
	for _, suffix := range secretSuffixes {
		if err := o.kubeCli.Get(ctx, types.NamespacedName{
			Namespace: o.accountNamespace,
			Name:      accountCRName + suffix,
		}, &secret); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		}
		if o.output == "" {
			fmt.Fprintln(o.IOStreams.Out, secret.Name)
		}

		resourcePrinter, err := o.printFlags.ToPrinter(o.output)
		if err != nil {
			return err
		}

		return resourcePrinter.PrintObj(&secret, o.Out)
	}

	return nil
}
