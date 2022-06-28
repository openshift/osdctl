package sts

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/coreos/go-semver/semver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type policyOptions struct {
	ReleaseVersion string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newPolicyOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *policyOptions {
	return &policyOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func newCmdPolicy(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newPolicyOptions(streams, flags, client)
	policyCmd := &cobra.Command{
		Use:               "policy",
		Short:             "Get OCP STS policy",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run(args))
		},
	}

	policyCmd.Flags().StringVarP(&ops.ReleaseVersion, "release-version", "r", "", "")

	return policyCmd
}

func (o *policyOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Release version is required for policy command")
	}

	_, err := semver.NewVersion(args[0])
	if err != nil {
		return cmdutil.UsageErrorf(cmd, "Release version must satisfy the semantic version format: %s", err.Error())
	}

	return nil
}

func (o *policyOptions) run(args []string) error {
	// save crs files in /tmp/crs- dir for given release version
	crs := "oc adm release extract quay.io/openshift-release-dev/ocp-release:" + args[0] + "-x86_64 --credentials-requests --cloud=aws --to=/tmp/crs-" + args[0]
	_, err := exec.Command("bash", "-c", crs).Output() //#nosec G204 -- Subprocess launched with variable
	if err != nil {
		fmt.Print(err)
		return err
	}

	output := "OCP STS policy files have been saved in /tmp/crs-" + args[0] + " directory"
	fmt.Println(output)

	return nil
}
