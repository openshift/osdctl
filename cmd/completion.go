package cmd

import (
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
)

var (
	completionExample = templates.Examples(`
		# Installing bash completion on macOS using homebrew
		## If running Bash 3.2 included with macOS
		    brew install bash-completion
		## or, if running Bash 4.1+
		    brew install bash-completion@2


		# Installing bash completion on Linux
		## If bash-completion is not installed on Linux, please install the 'bash-completion' package
		## via your distribution's package manager.
		## Load the osdctl completion code for bash into the current shell
		    source <(osdctl completion bash)
		## Write bash completion code to a file and source if from .bash_profile
		    osdctl completion bash > ~/.completion.bash.inc
		    printf "
		      # osdctl shell completion
		      source '$HOME/.completion.bash.inc'
		      " >> $HOME/.bash_profile
		    source $HOME/.bash_profile


		# Load the osdctl completion code for zsh[1] into the current shell
		    source <(osdctl completion zsh)
		# Set the osdctl completion code for zsh[1] to autoload on startup
		    osdctl completion zsh > "${fpath[1]}/_osdctl"`)
)

// newCmdCompletion creates the `completion` command
func newCmdCompletion() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "completion SHELL",
		Short:             "Output shell completion code for the specified shell (bash or zsh)",
		Example:           completionExample,
		DisableAutoGenTag: true,
		Args:              cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:         []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Parent().GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return cmdutil.UsageErrorf(cmd, "Unsupported shell type %q.", args[0])
			}
		},
	}

	return cmd
}
