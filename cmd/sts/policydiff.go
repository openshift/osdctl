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

type policyDiffOptions struct {
	oldReleaseVersion string
	newReleaseVersion string

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newPolicyDiffOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *policyDiffOptions {
	return &policyDiffOptions{
		flags:     flags,
		IOStreams: streams,
	}
}

func newCmdPolicyDiff(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newPolicyDiffOptions(streams, flags)
	policyDiffCmd := &cobra.Command{
		Use:               "policy-diff",
		Short:             "Get diff between two versions of OCP STS policy",
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run(args))
		},
	}

	policyDiffCmd.Flags().StringVarP(&ops.oldReleaseVersion, "old-version", "o", "", "")
	policyDiffCmd.Flags().StringVarP(&ops.newReleaseVersion, "new-version", "n", "", "")

	return policyDiffCmd
}

func (o *policyDiffOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return cmdutil.UsageErrorf(cmd, "Old and new release version is required for policy-diff command")
	}

	for _, s := range args {
		re := regexp.MustCompile(`^[0-9]{1}\.[0-9]{1,2}\.[0-9]{1,2}$`)
		if !re.MatchString(s) {
			return cmdutil.UsageErrorf(cmd, "Release version have to be in the x.y.z format ")
		}
	}

	// only initialize kubernetes client when versions are set
	if o.oldReleaseVersion != "" && o.newReleaseVersion != "" {
		var err error
		o.kubeCli, err = k8s.NewClient(o.flags)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *policyDiffOptions) run(args []string) error {
	// save crs files in /tmp/crs- dirs for each release version
	for _, s := range args {
		crs := "oc adm release extract quay.io/openshift-release-dev/ocp-release:" + s + "-x86_64 --credentials-requests --cloud=aws --to=/tmp/crs-" + s
		_, err := exec.Command("bash", "-c", crs).Output()
		if err != nil {
			fmt.Print(err)
			return err
		}
	}

	diff := "diff /tmp/crs-" + string(args[0]) + " /tmp/crs-" + string(args[1])
	output, _ := exec.Command("bash", "-c", diff).Output()
	fmt.Println(string(output))

	return nil
}
