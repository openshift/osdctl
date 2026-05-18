package rhobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func checkVaultToken(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "vault", "token", "lookup")
	cmd.Env = append(os.Environ(), "VAULT_ADDR=https://vault.devshift.net")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("vault CLI not found in PATH; install Vault and retry")
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("vault token lookup timed out; verify VAULT_ADDR and network connectivity")
		}
		return fmt.Errorf("vault token expired or missing, run: VAULT_ADDR=https://vault.devshift.net vault login -method=oidc")
	}
	return nil
}

func newCmdMcp() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "RHOBS MCP server for AI agent integration",
		Long: `MCP (Model Context Protocol) server that exposes RHOBS metrics, logs,
and alerts querying as tools for AI agents.

Compatible with any MCP client (Claude Code, Cursor, Windsurf, custom agents).

Subcommands:
  server    Start the stdio MCP server
  config    Print MCP client configuration JSON

Quick start:
  claude --mcp-config "$(osdctl rhobs mcp config)"

Prerequisites:
  - OCM login: ocm login --use-auth-code --url <environment>
  - Vault login: VAULT_ADDR=https://vault.devshift.net vault login -method=oidc
  - osdctl config: ~/.config/osdctl must have rhobs_<env>_vault_path entries`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.AddCommand(newCmdMcpServer())
	cmd.AddCommand(newCmdMcpConfig())

	return cmd
}

func newCmdMcpServer() *cobra.Command {
	return &cobra.Command{
		Use:           "server",
		Short:         "Start the RHOBS MCP server",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetOutput(io.Discard)

			go func(ctx context.Context) {
				if err := checkVaultToken(ctx); err != nil {
					fmt.Fprintln(os.Stderr, "WARNING:", err)
					fmt.Fprintln(os.Stderr, "MCP server starting anyway. Tool calls will fail until vault is authenticated.")
				}
			}(cmd.Context())

			server := mcp.NewServer(&mcp.Implementation{
				Name:    "osdctl-rhobs",
				Version: "1.0.0",
			}, nil)

			registerMcpTools(server)

			return server.Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}

func newCmdMcpConfig() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print MCP client configuration JSON",
		Long: `Print MCP client configuration JSON for use with AI agents.

Usage with Claude Code:
  claude --mcp-config "$(osdctl rhobs mcp config)"

Or add to ~/.claude/mcp_settings.json manually.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			execPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to determine osdctl binary path: %v", err)
			}

			config := map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"osdctl-rhobs": map[string]interface{}{
						"command": execPath,
						"args":    []string{"rhobs", "mcp", "server"},
					},
				},
			}

			output, err := json.MarshalIndent(config, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal config: %v", err)
			}

			fmt.Println(string(output))
			return nil
		},
	}
}
