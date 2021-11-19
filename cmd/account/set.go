package account

import (
	"context"
	"encoding/json"
	"errors"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/k8s"
)

// newCmdSet implements the set command which sets fields in account cr status
func newCmdSet(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newSetOptions(streams, flags, client)
	setCmd := &cobra.Command{
		Use:               "set <account name>",
		Short:             "Set AWS Account CR status",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	setCmd.Flags().StringVarP(&ops.accountNamespace, "account-namespace", "a", common.AWSAccountNamespace,
		"The namespace to keep AWS accounts. The default value is aws-account-operator.")

	setCmd.Flags().StringVar(&ops.state, "state", "", "set status.state field in the specified account")
	setCmd.Flags().BoolVarP(&ops.rotateCredentials, "rotate-credentials", "r", false,
		"set status.rotateCredentials in the specified account")

	setCmd.Flags().StringVarP(&ops.patchPayload, "patch", "p", "", "the raw payload used to patch the account status")
	setCmd.Flags().StringVarP(&ops.patchType, "type", "t", "merge",
		"The type of patch being provided; one of [merge json]. The strategic patch is not supported.")

	return setCmd
}

// setOptions defines the struct for running set command
type setOptions struct {
	accountName      string
	accountNamespace string

	state             string
	rotateCredentials bool

	// if patchPayload is set, it will use raw data to patch the object
	patchPayload string
	patchType    string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newSetOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *setOptions {
	return &setOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *setOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "The name of Account CR is required for set command")
	}
	o.accountName = args[0]

	switch o.state {
	// state doesn't set, continue
	case "":

	// valid value, continue
	case "Creating", "Pending", "PendingVerification",
		"Failed", "Ready":

	// throw error
	default:
		return cmdutil.UsageErrorf(cmd, "unsupported account state "+o.state)
	}

	return nil
}

func (o *setOptions) run() error {
	ctx := context.TODO()

	acc, err := k8s.GetAWSAccount(ctx, o.kubeCli, o.accountNamespace, o.accountName)
	if err != nil {
		return err
	}

	// patch account using the provided raw payload
	if o.patchPayload != "" {
		return o.rawPatch(ctx, acc)
	}

	payload := map[string]interface{}{
		"status": map[string]interface{}{
			"rotateCredentials": o.rotateCredentials,
		},
	}

	if o.state != "" {
		statusMap, _ := payload["status"].(map[string]interface{})
		statusMap["state"] = o.state
	}

	// set fields in status
	var mergePatch []byte
	mergePatch, _ = json.Marshal(payload)

	return o.kubeCli.Status().Patch(ctx, acc, client.RawPatch(types.MergePatchType, mergePatch))
}

// patch account status with raw input data
func (o *setOptions) rawPatch(ctx context.Context, account *awsv1alpha1.Account) error {
	var patchType types.PatchType
	switch o.patchType {
	case "merge":
		patchType = types.MergePatchType
	case "json":
		patchType = types.JSONPatchType
	default:
		return errors.New("unsupported patch type " + o.patchType)
	}
	return o.kubeCli.Status().Patch(ctx, account, client.RawPatch(patchType, []byte(o.patchPayload)))
}
