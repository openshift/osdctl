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
	DefaultCmdPath = "./cmd"
	DefaultDocsDir = "./docs"
	StateFile      = ".docgen_state"
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

// Calculate hash of directory contents to detect changes
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

// Load previous state hash from file
func loadState() (string, error) {
	data, err := os.ReadFile(StateFile)
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(data), err
}

// Save current state hash to file
func saveState(state string) error {
	return os.WriteFile(StateFile, []byte(state), 0644)
}

// Check if cmd directory has changed since last run
func hasChanged(cmdPath string) (bool, error) {
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

// Generate a single commands markdown file in README format in the root directory
func generateCommandsMarkdown(rootCmd *cobra.Command) error {
	// Place the file in the root directory
	filename := CommandsMdFile
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write header
	fmt.Fprintf(f, "# osdctl Commands\n\n")
	fmt.Fprintf(f, "## Command Overview\n\n")

	// Helper function to recursively list commands
	var writeCommands func(cmd *cobra.Command, depth int)
	writeCommands = func(cmd *cobra.Command, depth int) {
		if !cmd.IsAvailableCommand() || cmd.IsAdditionalHelpTopicCommand() {
			return
		}

		// Indent based on depth
		indent := strings.Repeat("  ", depth)

		// Write command with proper heading level
		fmt.Fprintf(f, "%s- `%s` - %s\n", indent, cmd.Use, cmd.Short)

		// Process subcommands
		for _, subcmd := range cmd.Commands() {
			writeCommands(subcmd, depth+1)
		}
	}

	// Process all commands
	for _, cmd := range rootCmd.Commands() {
		writeCommands(cmd, 0)
	}

	// Add detailed command documentation
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

		// Add command header
		fmt.Fprintf(f, "### %s\n\n", cmdPath)

		// Add description
		if cmd.Long != "" {
			fmt.Fprintf(f, "%s\n\n", cmd.Long)
		} else if cmd.Short != "" {
			fmt.Fprintf(f, "%s\n\n", cmd.Short)
		}

		// Add usage if available
		fmt.Fprintf(f, "```\n%s\n```\n\n", cmd.UseLine())

		// Add flags if available
		if len(cmd.Flags().FlagUsages()) > 0 {
			fmt.Fprintf(f, "#### Flags\n\n```\n%s```\n\n", cmd.Flags().FlagUsages())
		}

		// Process subcommands
		for _, subcmd := range cmd.Commands() {
			writeCommandDetails(subcmd, cmdPath)
		}
	}

	// Start with root command
	writeCommandDetails(rootCmd, "")

	return nil
}

// Generate documentation for all commands
func GenerateDocs(opts *Options) error {
	if opts == nil {
		opts = NewDefaultOptions()
	}

	// Check if cmd directory changed
	changed, err := hasChanged(opts.CmdPath)
	if err != nil {
		return fmt.Errorf("checking cmd directory state: %w", err)
	}

	if !changed {
		opts.Logger.Println("📋 No changes detected in cmd directory, skipping documentation generation")
		return nil
	}

	// Create docs directory if it doesn't exist
	if err := os.MkdirAll(opts.DocsDir, 0755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}

	if _, err := os.Stat(opts.CmdPath); os.IsNotExist(err) {
		return fmt.Errorf("cmd directory '%s' does not exist", opts.CmdPath)
	}

	opts.Logger.Println("🔄 Generating documentation...")

	// Get root command
	rootCmd := cmd.NewCmdRoot(opts.IOStreams)

	// Generate markdown documentation (individual files)
	if err := doc.GenMarkdownTree(rootCmd, opts.DocsDir); err != nil {
		return fmt.Errorf("generating command documentation: %w", err)
	}

	// Generate single commands markdown file in README format in the root directory
	if err := generateCommandsMarkdown(rootCmd); err != nil {
		return fmt.Errorf("generating commands markdown file: %w", err)
	}

	// Save new state
	newHash, err := getDirectoryHash(opts.CmdPath)
	if err != nil {
		return fmt.Errorf("calculating new state hash: %w", err)
	}
	if err := saveState(newHash); err != nil {
		return fmt.Errorf("saving state: %w", err)
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
		Long:  "Generate markdown documentation for osdctl commands when cmd directory changes",
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
