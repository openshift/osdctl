package jumphost

import (
	"fmt"

	osdctlCommand "github.com/openshift/osdctl/pkg/osdctlCommand"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// newCmdInit builds a new Jumphost from start to finish
func newCmdInit(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := &InitOptions{}

	initCmd := &cobra.Command{
		Use:               "init",
		Short:             "Initializes a jumphost in a cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.Run())
		},
	}

	initCmd.Flags().StringVarP(&ops.keyName, "key-name", "k", "$$USER$$-SRE-Jumphost", "Key Name")
	return initCmd
}

type InitOptions struct {
	keyName string

	generateKeyOpts osdctlCommand.Command
}

func (o *InitOptions) Init() error {
	if o.generateKeyOpts == nil {
		o.generateKeyOpts = &GenerateKeyOptions{
			keyName: o.keyName,
		}
	}

	return nil
}

// Validate validates the options that are passed to the command. These are generally
// populated by flags, but in the cases where we would be calling a command from another
// command these would be manually populated, so it would be a good idea to have a separate
// set of validation in that case.
func (o *InitOptions) Validate() error {
	if o.keyName == "" {
		return fmt.Errorf("Key name must be specified")
	}
	return nil
}

// This should be the entire functionality of the command, from start to end. There's
// nothing saying that we can't break out into smaller functions here.  Printing output is
// also something that we should consider how to do effectively, as we might not always
// want to _print_ the output after running, but we might want to just _use_ the output.
func (o *InitOptions) Run() error {
	err := o.Validate()
	if err != nil {
		return err
	}

	err = o.Init()
	if err != nil {
		return err
	}

	fmt.Println("Initializing Jumphost")

	err = o.generateKeyOpts.Run()
	if err != nil {
		return err
	}

	return nil
}
