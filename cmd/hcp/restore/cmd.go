package restore

import "github.com/spf13/cobra"

func NewCmdRestore() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "restore",
		Short:             "Prepare and restore HCP clusters during disaster recovery",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	cmd.AddCommand(NewCmdCreate())
	cmd.AddCommand(NewCmdManifestWork())
	cmd.AddCommand(NewCmdReassign())
	cmd.AddCommand(NewCmdReleaseDNS())
	cmd.AddCommand(NewCmdUpdateTrustPolicy())
	cmd.AddCommand(NewCmdCopySnapshots())

	return cmd
}
