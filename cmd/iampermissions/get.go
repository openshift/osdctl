package iampermissions

import (
	"fmt"
	"io"
	"os"

	"github.com/openshift/osdctl/pkg/policies"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type getOptions struct {
	ReleaseVersion string
	Cloud          policies.CloudSpec

	// Injected for testability
	downloadFunc func(string, policies.CloudSpec) (string, error)
	outputWriter io.Writer
}

func newCmdGet() *cobra.Command {
	ops := &getOptions{
		downloadFunc: policies.DownloadCredentialRequests,
		outputWriter: os.Stdout,
	}

	policyCmd := &cobra.Command{
		Use:               "get",
		Short:             "Get OCP CredentialsRequests",
		Args:              cobra.ExactArgs(0),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ops.Cloud = *cmd.Flag(cloudFlagName).Value.(*policies.CloudSpec)
			cmdutil.CheckErr(ops.run())
		},
	}

	policyCmd.Flags().StringVarP(&ops.ReleaseVersion, "release-version", "r", "", "")
	policyCmd.MarkFlagRequired("release-version")

	return policyCmd
}

func (o *getOptions) run() error {
	directory, err := o.downloadFunc(o.ReleaseVersion, o.Cloud)
	if err != nil {
		return err
	}

	output := fmt.Sprintf("OCP CredentialsRequests for %s have been saved in %s directory", o.Cloud.String(), directory)
	fmt.Fprintln(o.outputWriter, output)

	return nil
}
