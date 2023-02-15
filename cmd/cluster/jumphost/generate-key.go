package jumphost

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// newCmdGenerateKey builds a new Key Generation Command
func newCmdGenerateKey(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	genKeyCmd := GenerateKeyCommand{}

	cobraCmd := &cobra.Command{
		Use:               "generate-key",
		Short:             "Generates a key in AWS for ssh'ing to a jumphost",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			output, err := genKeyCmd.Run()
			cmdUtil.CheckErr(err)
			output.Print()
		},
	}
	cobraCmd.Flags().StringVarP(&ops.keyName, "key-name", "k", "$$USER$$-SRE-Jumphost", "Key Name")
	return cobraCmd
}

type GenerateKeyCommand struct {
	keyName string
}

type GenerateKeyOutput struct {
	keyArn  string
	keyName string
}

func (o *GenerateKeyOutput) Print() {
	fmt.Println("Key Generated:")
	fmt.Printf("  Name: %s\n  Arn: %s\n", o.keyName, o.keyArn)
}

func (c *GenerateKeyCommand) Validate() error {
	if c.keyName == "" {
		return fmt.Errorf("Key name must be specified")
	}
	return nil
}

func (c *GenerateKeyOptions) Run() (*GenerateKeyOutput, error) {
	err := c.Validate()
	if err != nil {
		return err
	}

	fmt.Println("Generating Key")

	return nil
}
