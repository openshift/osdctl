package backup

import "github.com/spf13/cobra"

func NewCmdBackup() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "backup",
		Short:             "Manage Velero backups for HCP clusters",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
	}

	cmd.AddCommand(NewCmdCreate())
	cmd.AddCommand(NewCmdDiscover())

	return cmd
}
