package docgen

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/osdctl/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	DefaultCmdPath      = "./cmd"
	DefaultDocsDir      = "./docs"
	DefaultCommandsFile = "osdctl_commands.md"
	StateFile           = ".docgen_state"
)

type CommandCategory struct {
	Name        string
	Description string
	Category    string
}

type Options struct {
	CmdPath      string
	DocsDir      string
	CommandsFile string
	Logger       *log.Logger
	IOStreams    genericclioptions.IOStreams
}

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

func getDirectoryHash(dir string) (string, error) {
	hasher := sha256.New()

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			hasher.Write([]byte(path))
			hasher.Write([]byte(info.ModTime().String()))

			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(hasher, f); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func loadState() (string, error) {
	data, err := os.ReadFile(StateFile)
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(data), err
}

func saveState(state string) error {
	return os.WriteFile(StateFile, []byte(state), 0644)
}

func hasCmdDirChanged(cmdPath string) (bool, error) {
	currentHash, err := getDirectoryHash(cmdPath)
	if err != nil {
		return false, err
	}

	previousHash, err := loadState()
	if err != nil {
		return false, err
	}

	return currentHash != previousHash, nil
}

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
			for k, v := range extractCommands(subCmd) {
				commands[k] = v
			}
		}
	}

	return commands
}

func generateCommandReferenceMd(commandsFile string, commands map[string]CommandCategory) error {
	var output strings.Builder

	output.WriteString("# OSDCTL Command Reference\n\n")
	output.WriteString("This document provides a comprehensive list of all available osdctl commands, organized by category.\n\n")

	categorizedCommands := make(map[string][]CommandCategory)
	for _, cmd := range commands {
		category := cmd.Category
		if category == "" {
			category = "General Commands"
		}
		categorizedCommands[category] = append(categorizedCommands[category], cmd)
	}

	for category, cmds := range categorizedCommands {
		output.WriteString(fmt.Sprintf("## %s\n\n", category))
		for _, cmd := range cmds {
			output.WriteString(fmt.Sprintf("* `%s` - %s\n", cmd.Name, cmd.Description))
		}
		output.WriteString("\n")
	}

	return os.WriteFile(commandsFile, []byte(output.String()), 0644)
}

func GenerateDocs(opts *Options) error {
	if opts == nil {
		opts = NewDefaultOptions()
	}

	changed, err := hasCmdDirChanged(opts.CmdPath)
	if err != nil {
		return fmt.Errorf("checking cmd directory state: %w", err)
	}

	if !changed {
		opts.Logger.Println("📋 No changes detected in cmd directory, skipping documentation generation")
		return nil
	}

	if err := os.MkdirAll(opts.DocsDir, 0755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}

	if _, err := os.Stat(opts.CmdPath); os.IsNotExist(err) {
		return fmt.Errorf("cmd directory '%s' does not exist", opts.CmdPath)
	}

	opts.Logger.Println("🔄 Generating documentation...")

	rootCmd := cmd.NewCmdRoot(opts.IOStreams)

	commands := extractCommands(rootCmd)

	if err := doc.GenMarkdownTree(rootCmd, opts.DocsDir); err != nil {
		return fmt.Errorf("generating command documentation: %w", err)
	}

	if err := generateCommandReferenceMd(opts.CommandsFile, commands); err != nil {
		return fmt.Errorf("creating command reference file: %w", err)
	}

	newHash, err := getDirectoryHash(opts.CmdPath)
	if err != nil {
		return fmt.Errorf("calculating new state hash: %w", err)
	}
	if err := saveState(newHash); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	opts.Logger.Printf("✅ Documentation successfully generated in %s and %s created in root directory",
		opts.DocsDir, opts.CommandsFile)

	return nil
}

func Command() *cobra.Command {
	opts := NewDefaultOptions()
	cmd := &cobra.Command{
		Use:   "docgen",
		Short: "Generate osdctl documentation",
		Long:  "Generate markdown documentation for osdctl commands when cmd directory changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return GenerateDocs(opts)
		},
	}

	cmd.Flags().StringVar(&opts.CmdPath, "cmd-path", opts.CmdPath, "Path to the cmd directory")
	cmd.Flags().StringVar(&opts.DocsDir, "docs-dir", opts.DocsDir, "Path to the docs output directory")
	cmd.Flags().StringVar(&opts.CommandsFile, "commands-file", opts.CommandsFile, "Filename for the command reference file")

	return cmd
}

func Main() {
	cmd := Command()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
