package sts

import (
	"fmt"
	"os/exec"
	"regexp"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/pkg/k8s"
)

type policyOptions struct {
	ReleaseVersion string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newPolicyOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *policyOptions {
	return &policyOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func newCmdPolicy(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newPolicyOptions(streams, flags)
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

	re := regexp.MustCompile(`^[0-9]{1}\.[0-9]{1,2}\.[0-9]{1,2}$`)
	if !re.MatchString(args[0]) {
		return cmdutil.UsageErrorf(cmd, "Release version have to be in the x.y.z format ")
	}

	// only initialize kubernetes client when versions are set
	if o.ReleaseVersion != "" {
		var err error
		o.kubeCli, err = k8s.NewClient(o.flags)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *policyOptions) run(args []string) error {
	// save crs files in /tmp/crs- dir for given release version
	crs := "oc adm release extract quay.io/openshift-release-dev/ocp-release:" + args[0] + "-x86_64 --credentials-requests --cloud=aws --to=/tmp/crs-" + args[0]
	_, err := exec.Command("bash", "-c", crs).Output()
	if err != nil {
		fmt.Print(err)
		return err
	}

	output := "OCP STS policy files have been saved in /tmp/crs-" + args[0] + " directory"
	fmt.Println(output)

	return nil
}
