// Package docgen provides functionality for generating osdctl documentation
package docgen

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/openshift/osdctl/cmd"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	DefaultCmdPath      = "./cmd"
	DefaultDocsDir      = "./docs"
	DefaultCommandsFile = "osdctl_commands.md"
)

// CommandCategory represents a categorized command
type CommandCategory struct {
	Name        string
	Description string
	Category    string
}

// Options holds the configuration for the documentation generator
type Options struct {
	// CmdPath is the path to the cmd directory
	CmdPath string
	// DocsDir is the output directory for generated docs
	DocsDir string
	// CommandsFile is the file to write the command reference to
	CommandsFile string
	// Logger for output
	Logger *log.Logger
	// IOStreams for command initialization
	IOStreams genericclioptions.IOStreams
}

// NewDefaultOptions returns a new Options with default values
func NewDefaultOptions() *Options {
	return &Options{
		CmdPath:      DefaultCmdPath,
		DocsDir:      DefaultDocsDir,
		CommandsFile: DefaultCommandsFile,
		Logger:       log.New(os.Stdout, "", log.LstdFlags),
		IOStreams: genericclioptions.IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
	}
}

// categorizeCommand determines the command category based on its hierarchy
func categorizeCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return "General Commands"
	}

	parent := cmd.Parent()
	if parent != nil && parent.Name() != "osdctl" {
		return strings.Title(parent.Name()) + " Commands"
	}
	return "General Commands"
}

// extractCommands builds a map of all commands and their categories
func extractCommands(cmd *cobra.Command) map[string]CommandCategory {
	commands := make(map[string]CommandCategory)

	if cmd.Name() != "" && !cmd.HasParent() {
		commands[cmd.Name()] = CommandCategory{
			Name:        cmd.Name(),
			Description: cmd.Short,
			Category:    categorizeCommand(cmd),
		}
	}

	for _, subCmd := range cmd.Commands() {
		if !subCmd.Hidden && subCmd.Deprecated == "" {
			commands[subCmd.Name()] = CommandCategory{
				Name:        subCmd.Name(),
				Description: subCmd.Short,
				Category:    categorizeCommand(subCmd),
			}
			// Recursively get subcommands
			for k, v := range extractCommands(subCmd) {
				commands[k] = v
			}
		}
	}

	return commands
}

// generateCommandReferenceMd creates a standalone markdown file with command references
func generateCommandReferenceMd(commandsFile string, commands map[string]CommandCategory) error {
	var output strings.Builder

	// Add header
	output.WriteString("# OSDCTL Command Reference\n\n")
	output.WriteString("This document provides a comprehensive list of all available osdctl commands, organized by category.\n\n")

	// Group commands by category
	categorizedCommands := make(map[string][]CommandCategory)
	for _, cmd := range commands {
		category := cmd.Category
		if category == "" {
			category = "General Commands"
		}
		categorizedCommands[category] = append(categorizedCommands[category], cmd)
	}

	// Write commands by category
	for category, cmds := range categorizedCommands {
		output.WriteString(fmt.Sprintf("## %s\n\n", category))
		for _, cmd := range cmds {
			output.WriteString(fmt.Sprintf("* `%s` - %s\n", cmd.Name, cmd.Description))
		}
		output.WriteString("\n")
	}

	// Write the content to the output file
	return os.WriteFile(commandsFile, []byte(output.String()), 0644)
}

// GenerateDocs generates the documentation for osdctl commands
func GenerateDocs(opts *Options) error {
	if opts == nil {
		opts = NewDefaultOptions()
	}

	// Ensure docs directory exists
	if err := os.MkdirAll(opts.DocsDir, 0755); err != nil {
		return errors.Wrap(err, "creating docs directory")
	}

	if _, err := os.Stat(opts.CmdPath); os.IsNotExist(err) {
		return errors.Errorf("cmd directory '%s' does not exist", opts.CmdPath)
	}

	opts.Logger.Println("🔄 Generating documentation...")

	// Initialize root command
	rootCmd := cmd.NewCmdRoot(opts.IOStreams)

	// Extract all commands and their information
	commands := extractCommands(rootCmd)

	// Generate markdown documentation for all commands in the docs directory
	if err := doc.GenMarkdownTree(rootCmd, opts.DocsDir); err != nil {
		return errors.Wrap(err, "generating command documentation")
	}

	// Generate the standalone command reference file in the root directory
	if err := generateCommandReferenceMd(opts.CommandsFile, commands); err != nil {
		return errors.Wrap(err, "creating command reference file")
	}

	opts.Logger.Printf("✅ Documentation successfully generated in %s and %s created in root directory",
		opts.DocsDir, opts.CommandsFile)

	return nil
}

// Command returns a cobra command that can be used to generate documentation
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

	// Add flags
	cmd.Flags().StringVar(&opts.CmdPath, "cmd-path", opts.CmdPath, "Path to the cmd directory")
	cmd.Flags().StringVar(&opts.DocsDir, "docs-dir", opts.DocsDir, "Path to the docs output directory")
	cmd.Flags().StringVar(&opts.CommandsFile, "commands-file", opts.CommandsFile, "Filename for the command reference file")

	return cmd
}

// Main function that can be called directly from a main.go file
func Main() {
	cmd := Command()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
