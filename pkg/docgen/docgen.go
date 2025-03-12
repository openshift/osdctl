package docgen

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/openshift/osdctl/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	DefaultCmdPath = "./cmd"
	DefaultDocsDir = "./docs"
	CommandsMdFile = "osdctl_commands.md"
)

type Options struct {
	CmdPath   string
	DocsDir   string
	Logger    *log.Logger
	IOStreams genericclioptions.IOStreams
}

func NewDefaultOptions() *Options {
	return &Options{
		CmdPath: DefaultCmdPath,
		DocsDir: DefaultDocsDir,
		Logger:  log.New(os.Stdout, "", log.LstdFlags),
		IOStreams: genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
	}
}

func generateCommandsMarkdown(rootCmd *cobra.Command) error {
	filename := CommandsMdFile
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "# osdctl Commands\n\n")
	fmt.Fprintf(f, "## Command Overview\n\n")

	var writeCommands func(cmd *cobra.Command, depth int)
	writeCommands = func(cmd *cobra.Command, depth int) {
		if !cmd.IsAvailableCommand() || cmd.IsAdditionalHelpTopicCommand() {
			return
		}

		indent := strings.Repeat("  ", depth)

		fmt.Fprintf(f, "%s- `%s` - %s\n", indent, cmd.Use, cmd.Short)

		for _, subcmd := range cmd.Commands() {
			writeCommands(subcmd, depth+1)
		}
	}

	for _, cmd := range rootCmd.Commands() {
		writeCommands(cmd, 0)
	}

	fmt.Fprintf(f, "\n## Command Details\n\n")

	var writeCommandDetails func(cmd *cobra.Command, path string)
	writeCommandDetails = func(cmd *cobra.Command, path string) {
		if !cmd.IsAvailableCommand() || cmd.IsAdditionalHelpTopicCommand() {
			return
		}

		cmdPath := path
		if path != "" {
			cmdPath = path + " " + cmd.Name()
		} else {
			cmdPath = cmd.Name()
		}

		fmt.Fprintf(f, "### %s\n\n", cmdPath)

		if cmd.Long != "" {
			fmt.Fprintf(f, "%s\n\n", cmd.Long)
		} else if cmd.Short != "" {
			fmt.Fprintf(f, "%s\n\n", cmd.Short)
		}

		fmt.Fprintf(f, "```\n%s\n```\n\n", cmd.UseLine())

		if len(cmd.Flags().FlagUsages()) > 0 {
			fmt.Fprintf(f, "#### Flags\n\n```\n%s```\n\n", cmd.Flags().FlagUsages())
		}

		for _, subcmd := range cmd.Commands() {
			writeCommandDetails(subcmd, cmdPath)
		}
	}

	writeCommandDetails(rootCmd, "")

	return nil
}

func GenerateDocs(opts *Options) error {
	if opts == nil {
		opts = NewDefaultOptions()
	}

	if err := os.MkdirAll(opts.DocsDir, 0755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}

	if _, err := os.Stat(opts.CmdPath); os.IsNotExist(err) {
		return fmt.Errorf("cmd directory '%s' does not exist", opts.CmdPath)
	}

	opts.Logger.Println("🔄 Generating documentation...")

	rootCmd := cmd.NewCmdRoot(opts.IOStreams)

	if err := doc.GenMarkdownTree(rootCmd, opts.DocsDir); err != nil {
		return fmt.Errorf("generating command documentation: %w", err)
	}

	if err := generateCommandsMarkdown(rootCmd); err != nil {
		return fmt.Errorf("generating commands markdown file: %w", err)
	}

	opts.Logger.Printf("✅ Documentation successfully generated in %s", opts.DocsDir)
	opts.Logger.Printf("✅ Commands overview generated in %s", CommandsMdFile)

	return nil
}

func Command() *cobra.Command {
	opts := NewDefaultOptions()
	cmd := &cobra.Command{
		Use:   "docgen",
		Short: "Generate osdctl documentation",
		Long:  "Generate markdown documentation for osdctl commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return GenerateDocs(opts)
		},
	}

	cmd.Flags().StringVar(&opts.CmdPath, "cmd-path", opts.CmdPath, "Path to the cmd directory")
	cmd.Flags().StringVar(&opts.DocsDir, "docs-dir", opts.DocsDir, "Path to the docs output directory")

	return cmd
}

func Main() {
	cmd := Command()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
