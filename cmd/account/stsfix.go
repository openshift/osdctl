package account

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newCmdReset implements the reset command which resets the specified account cr
func newCmdStsFix(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newStsFixOptions(streams, flags, client)
	resetCmd := &cobra.Command{
		Use:               "stsFix <account name>",
		Short:             "Reset AWS Account CR to Ready and AccountClaim to Ready",
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
	// mark this flag hidden because it is not recommended to use
	_ = resetCmd.Flags().MarkHidden("skip-check")

	return resetCmd
}

type State string

var stateReady State = "Ready"

// resetOptions defines the struct for running reset command
type stsFixOptions struct {
	accountName      string
	accountNamespace string
	skipCheck        bool
	setState         State

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newStsFixOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *stsFixOptions {
	return &stsFixOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *stsFixOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "The name of Account CR is required for reset command")
	}

	o.setState = stateReady
	o.accountName = args[0]

	return nil
}

func (o *stsFixOptions) run() error {
	if !o.skipCheck {
		reader := bufio.NewReader(o.In)
		fmt.Fprintf(o.Out, fmt.Sprintf("Reset account %s? (Y/N) ", o.accountName))
		text, _ := reader.ReadSlice('\n')

		input := strings.ToLower(strings.Trim(string(text), "\n"))
		if input != "y" {
			return nil
		}
	}

	fmt.Fprintln(o.Out, fmt.Sprintf("Resetting account %s", o.accountName))

	ctx := context.TODO()

	// get account CR
	account, err := k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, o.accountName)
	if err != nil {
		fmt.Println(fmt.Sprintf("Could not get account %s", err))
		os.Exit(99)
	}

	// get claimlink details from account CR for accountClaim
	accClaimName := account.Spec.ClaimLink
	accClaimNamespace := account.Spec.ClaimLinkNamespace

	// get AccountClaim CR
	accountClaim, err := k8s.GetAWSAccountClaim(ctx, o.kubeCli, accClaimNamespace, accClaimName)
	if err != nil {
		fmt.Println(fmt.Sprintf("Error Getting Accountclaim: %s", err))
		os.Exit(100)
	}

	accountReadyError := o.readyAccount(ctx, account)
	if accountReadyError != nil {
		fmt.Println(fmt.Sprintf("Error Setting Account %s to Ready: %s", o.accountName, accountReadyError))
		os.Exit(101)
	}

	accClaimReadyError := o.readyAccountClaim(ctx, accountClaim)
	if accClaimReadyError != nil {
		fmt.Println(fmt.Sprintf("Error Setting AccountClaim %s to Ready: %s", accountClaim.Name, accClaimReadyError))
		os.Exit(102)
	}
	return nil
}

func (o *stsFixOptions) readyAccount(ctx context.Context, account *awsv1alpha1.Account) error {
	fmt.Fprintln(o.Out, fmt.Sprintf("Changing Account %s state to %s", account.Name, o.setState))
	//reset fields in status
	var mergePatch []byte

	status := map[string]interface{}{
		"state": o.setState,
	}
	mergePatch, _ = json.Marshal(map[string]interface{}{
		"status": status,
	})
	return o.kubeCli.Status().Patch(ctx, account, client.RawPatch(types.MergePatchType, mergePatch))
}

func (o *stsFixOptions) readyAccountClaim(ctx context.Context, accountClaim *awsv1alpha1.AccountClaim) error {
	fmt.Fprintln(o.Out, fmt.Sprintf("Changing AccountClaim %s/%s state to %s", accountClaim.Namespace, accountClaim.Name, o.setState))
	//reset fields in status
	var mergePatch []byte

	status := map[string]interface{}{
		"state": o.setState,
	}
	mergePatch, _ = json.Marshal(map[string]interface{}{
		"status": status,
	})
	return o.kubeCli.Status().Patch(ctx, accountClaim, client.RawPatch(types.MergePatchType, mergePatch))
}
