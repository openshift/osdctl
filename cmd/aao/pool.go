package aao

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// newCmdPool gets the current status of the AWS Account Operator AccountPool
func newCmdPool(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newPoolOptions(streams, flags)
	poolCmd := &cobra.Command{
		Use:               "pool",
		Short:             "Get the status of the AWS Account Operator AccountPool",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	return poolCmd
}

// poolOptions defines the struct for running the pool command
type poolOptions struct {
	genericclioptions.IOStreams
}

func newPoolOptions(streams genericclioptions.IOStreams, _ *genericclioptions.ConfigFlags) *poolOptions {
	return &poolOptions{
		IOStreams: streams,
	}
}

func (o *poolOptions) complete(cmd *cobra.Command) error {
	return nil
}

func (o *poolOptions) run() error {
	cmd := "oc get accounts -o json -n aws-account-operator | jq '.items | map(select(.status.claimed==\"false\" or .status.claimed==null)) | map(select(.status.state!=\"Failed\")) | map(select(.spec.legalEntity==null or .spec.legalEntity.id==\"\" or .spec.legalEntity.id==null)) | length'"
	output, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		fmt.Println(string(output))
		fmt.Print(err)
		return err
	}
	fmt.Printf("Available Accounts: %s", string(output))
	return nil
}
