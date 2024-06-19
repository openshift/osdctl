package iampermissions

import (
	"fmt"
	"github.com/openshift/osdctl/pkg/policies"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type getOptions struct {
	ReleaseVersion string
	Cloud          policies.CloudSpec
}

func newCmdGet() *cobra.Command {
	ops := &getOptions{}
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
	directory, err := policies.DownloadCredentialRequests(o.ReleaseVersion, o.Cloud)
	if err != nil {
		return err
	}

	output := "OCP CredentialsRequests for " + o.Cloud.String() + " have been saved in " + directory + " directory"
	fmt.Println(output)

	return nil
}
