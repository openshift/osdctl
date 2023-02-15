package jumphost

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// newCmdGenerateKey builds a new Key Generation Command
func newCmdGenerateKey(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := GenerateKeyOptions{}

	keyGenCommand := &cobra.Command{
		Use:               "generate-key",
		Short:             "Generates a key in AWS for ssh'ing to a jumphost",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.Run())
		},
	}
	keyGenCommand.Flags().StringVarP(&ops.keyName, "key-name", "k", "$$USER$$-SRE-Jumphost", "Key Name")
	return keyGenCommand
}

type GenerateKeyOptions struct {
	keyName string
}

func (o *GenerateKeyOptions) Validate() error {
	if o.keyName == "" {
		return fmt.Errorf("Key name must be specified")
	}
	return nil
}

func (o *GenerateKeyOptions) Run() error {
	err := o.Validate()
	if err != nil {
		return err
	}

	fmt.Println("Generating Key")

	return nil
}
