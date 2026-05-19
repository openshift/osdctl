package sreagent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	pdURL       string
	autoExecute bool
	outputDir   string
)

const (
	sreAgentDescription = `
  SRE Agent is an AI-powered tool that helps SREs triage alerts and diagnose issues.
  It automatically fetches incident details from PagerDuty, finds relevant SOPs,
  and executes diagnostic commands on clusters.
`

	sreAgentExample = `
  # Interactive mode (asks for confirmation at each step)
  osdctl ai sre-agent --pd-url "${PD_URL}"

  # Fully automated mode (no confirmations)
  osdctl ai sre-agent --pd-url "${PD_URL}" --auto-execute

  # Specify output directory for sre-agent files
  osdctl ai sre-agent --pd-url "${PD_URL}" --output /tmp/sre-agent-output
`
)

func NewCmdSreAgent() *cobra.Command {
	sreAgentCmd := &cobra.Command{
		Use:           "sre-agent",
		Short:         "Run SRE Agent for automated incident investigation",
		Long:          sreAgentDescription,
		Example:       sreAgentExample,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			// Step 1: Validate sre-agent installation
			if !validateSreAgent() {
				return
			}

			// Step 2: Check/Setup config (includes ops-sop setup)
			if !checkSreAgentConfig() {
				return
			}

			// Step 3: Execute sre-agent
			sreAgentPath := filepath.Join(xdg.DataHome, "sre-agent/venv/bin/sre-agent")
			sreAgentArgs := buildSreAgentArgs(args)

			err := executeSreAgent(sreAgentPath, sreAgentArgs, outputDir)
			if err != nil {
				cmdutil.CheckErr(err)
			}
		},
	}

	sreAgentCmd.Flags().StringVar(&pdURL, "pd-url", "", "PagerDuty incident URL (required)")
	sreAgentCmd.Flags().BoolVar(&autoExecute, "auto-execute", false, "Fully automated mode without confirmations")
	sreAgentCmd.Flags().StringVar(&outputDir, "output", "", "Output directory for sre-agent files (default: current directory)")

	// Mark pd-url as required
	if err := sreAgentCmd.MarkFlagRequired("pd-url"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to mark pd-url as required: %v\n", err)
	}

	return sreAgentCmd
}

// buildSreAgentArgs constructs the argument list for sre-agent command
func buildSreAgentArgs(additionalArgs []string) []string {
	args := []string{}

	if pdURL != "" {
		args = append(args, "--pd-url", pdURL)
	}

	if autoExecute {
		args = append(args, "--auto-execute")
	}

	// Add any additional arguments passed
	args = append(args, additionalArgs...)

	return args
}

// executeSreAgent runs the sre-agent command with provided arguments
func executeSreAgent(sreAgentPath string, args []string, outputDir string) error {
	cmd := exec.Command(sreAgentPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set working directory if output directory is specified
	if outputDir != "" {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		cmd.Dir = outputDir
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sre-agent execution failed: %w", err)
	}

	return nil
}
