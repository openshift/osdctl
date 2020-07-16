package cmd

import (
	"bytes"
	"io"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
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
func newCmdCompletion(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newCompletionOptions(streams)
	cmd := &cobra.Command{
		Use:                   "completion SHELL",
		Short:                 "Output shell completion code for the specified shell (bash or zsh)",
		Example:               completionExample,
		DisableAutoGenTag:     true,
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run(cmd, args))
		},
	}

	return cmd
}

type completionOptions struct {
	supportedShells []string

	genericclioptions.IOStreams
}

func newCompletionOptions(streams genericclioptions.IOStreams) *completionOptions {
	return &completionOptions{
		IOStreams: streams,
	}
}

func (o *completionOptions) run(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmdutil.UsageErrorf(cmd, "Shell not specified.")
	}
	if len(args) > 1 {
		return cmdutil.UsageErrorf(cmd, "Too many arguments. Expected only the shell type.")
	}
	switch args[0] {
	case "bash":
		return cmd.Parent().GenBashCompletion(o.Out)
	case "zsh":
		return genCompletionZsh(o.Out, cmd.Parent())
	default:
		return cmdutil.UsageErrorf(cmd, "Unsupported shell type %q.", args[0])
	}
}

// Followed the trick https://github.com/kubernetes/kubectl/blob/master/pkg/cmd/completion/completion.go#L145.
func genCompletionZsh(out io.Writer, osdctl *cobra.Command) error {
	zshHead := "#compdef osdctl\n"
	out.Write([]byte(zshHead))

	zshInitialization := `
__osdctl_bash_source() {
	alias shopt=':'
	emulate -L sh
	setopt kshglob noshglob braceexpand

	source "$@"
}

__osdctl_type() {
	# -t is not supported by zsh
	if [ "$1" == "-t" ]; then
		shift

		# fake Bash 4 to disable "complete -o nospace". Instead
		# "compopt +-o nospace" is used in the code to toggle trailing
		# spaces. We don't support that, but leave trailing spaces on
		# all the time
		if [ "$1" = "__osdctl_compopt" ]; then
			echo builtin
			return 0
		fi
	fi
	type "$@"
}

__osdctl_compgen() {
	local completions w
	completions=( $(compgen "$@") ) || return $?

	# filter by given word as prefix
	while [[ "$1" = -* && "$1" != -- ]]; do
		shift
		shift
	done
	if [[ "$1" == -- ]]; then
		shift
	fi
	for w in "${completions[@]}"; do
		if [[ "${w}" = "$1"* ]]; then
			echo "${w}"
		fi
	done
}

__osdctl_compopt() {
	true # don't do anything. Not supported by bashcompinit in zsh
}

__osdctl_ltrim_colon_completions()
{
	if [[ "$1" == *:* && "$COMP_WORDBREAKS" == *:* ]]; then
		# Remove colon-word prefix from COMPREPLY items
		local colon_word=${1%${1##*:}}
		local i=${#COMPREPLY[*]}
		while [[ $((--i)) -ge 0 ]]; do
			COMPREPLY[$i]=${COMPREPLY[$i]#"$colon_word"}
		done
	fi
}

__osdctl_get_comp_words_by_ref() {
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[${COMP_CWORD}-1]}"
	words=("${COMP_WORDS[@]}")
	cword=("${COMP_CWORD[@]}")
}

__osdctl_filedir() {
	# Don't need to do anything here.
	# Otherwise we will get trailing space without "compopt -o nospace"
	true
}

autoload -U +X bashcompinit && bashcompinit

# use word boundary patterns for BSD or GNU sed
LWORD='[[:<:]]'
RWORD='[[:>:]]'
if sed --help 2>&1 | grep -q 'GNU\|BusyBox'; then
	LWORD='\<'
	RWORD='\>'
fi

__osdctl_convert_bash_to_zsh() {
	sed \
	-e 's/declare -F/whence -w/' \
	-e 's/_get_comp_words_by_ref "\$@"/_get_comp_words_by_ref "\$*"/' \
	-e 's/local \([a-zA-Z0-9_]*\)=/local \1; \1=/' \
	-e 's/flags+=("\(--.*\)=")/flags+=("\1"); two_word_flags+=("\1")/' \
	-e 's/must_have_one_flag+=("\(--.*\)=")/must_have_one_flag+=("\1")/' \
	-e "s/${LWORD}_filedir${RWORD}/__osdctl_filedir/g" \
	-e "s/${LWORD}_get_comp_words_by_ref${RWORD}/__osdctl_get_comp_words_by_ref/g" \
	-e "s/${LWORD}__ltrim_colon_completions${RWORD}/__osdctl_ltrim_colon_completions/g" \
	-e "s/${LWORD}compgen${RWORD}/__osdctl_compgen/g" \
	-e "s/${LWORD}compopt${RWORD}/__osdctl_compopt/g" \
	-e "s/${LWORD}declare${RWORD}/builtin declare/g" \
	-e "s/\\\$(type${RWORD}/\$(__osdctl_type/g" \
	<<'BASH_COMPLETION_EOF'
`
	out.Write([]byte(zshInitialization))

	buf := new(bytes.Buffer)
	osdctl.GenBashCompletion(buf)
	out.Write(buf.Bytes())

	zshTail := `
BASH_COMPLETION_EOF
}

__osdctl_bash_source <(__osdctl_convert_bash_to_zsh)
`
	out.Write([]byte(zshTail))
	return nil
}
