package account

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
)

// newCmdReset implements the reset command which resets the specified account cr
func newCmdReset(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newResetOptions(streams, flags)
	resetCmd := &cobra.Command{
		Use:               "reset <account name>",
		Short:             "Reset AWS Account CR",
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	resetCmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")
	resetCmd.Flags().BoolVarP(&ops.skipCheck, "skip-check", "y", false,
		"Skip the prompt check")
	resetCmd.Flags().BoolVar(&ops.resetLegalEntity, "reset-legalentity", false,
		`This will wipe the legalEntity, claimLink and reused fields, allowing accounts to be used for different Legal Entities.`)

	// mark this flag hidden because it is not recommended to use
	_ = resetCmd.Flags().MarkHidden("skip-check")

	return resetCmd
}

// resetOptions defines the struct for running reset command
type resetOptions struct {
	accountName      string
	accountNamespace string

	skipCheck        bool
	resetLegalEntity bool

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newResetOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *resetOptions {
	return &resetOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func (o *resetOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "The name of Account CR is required for reset command")
	}
	o.accountName = args[0]

	var err error
	o.kubeCli, err = k8s.NewClient(o.flags)
	if err != nil {
		return err
	}

	return nil
}

func (o *resetOptions) run() error {
	if !o.skipCheck {
		reader := bufio.NewReader(o.In)
		fmt.Fprintf(o.Out, fmt.Sprintf("Reset account %s? (Y/N) ", o.accountName))
		text, _ := reader.ReadSlice('\n')

		input := strings.ToLower(strings.Trim(string(text), "\n"))
		if input != "y" {
			return nil
		}
	}

	ctx := context.TODO()

	//cleanup secrets
	var secrets v1.SecretList
	if err := o.kubeCli.List(ctx, &secrets, &client.ListOptions{
		Namespace: o.accountNamespace,
	}); err != nil {
		return err
	}
	for _, secret := range secrets.Items {
		if strings.HasPrefix(secret.Name, o.accountName) {
			fmt.Fprintln(o.Out, "Deleting secret "+secret.Name)
			if err := o.kubeCli.Delete(ctx, &secret, &client.DeleteOptions{}); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
		}
	}

	account, err := k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, o.accountName)
	if err != nil {
		return err
	}

	// reset fields in spec
	account.Spec.ClaimLink = ""
	account.Spec.ClaimLinkNamespace = ""
	account.Spec.IAMUserSecret = ""
	if o.resetLegalEntity {
		account.Spec.LegalEntity = v1alpha1.LegalEntity{}
	}

	if err := o.kubeCli.Update(ctx, account, &client.UpdateOptions{}); err != nil {
		return err
	}

	// reset fields in status
	var mergePatch []byte

	status := map[string]interface{}{
		"rotateCredentials":        false,
		"rotateConsoleCredentials": false,
		"claimed":                  false,
		"state":                    "",
		"conditions":               []interface{}{},
	}

	if o.resetLegalEntity {
		status["reused"] = false
	}

	mergePatch, _ = json.Marshal(map[string]interface{}{
		"status": status,
	})

	return o.kubeCli.Status().Patch(ctx, account, client.RawPatch(types.MergePatchType, mergePatch))
}
