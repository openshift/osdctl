## osdctl rhobs mcp

RHOBS MCP server for AI agent integration

### Synopsis

MCP (Model Context Protocol) server that exposes RHOBS metrics, logs,
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
  - osdctl config: ~/.config/osdctl must have rhobs_<env>_vault_path entries

### Options

```
  -h, --help   help for mcp
```

### Options inherited from parent commands

```
      --client-id string       RHOBS SSO client ID - skips Vault lookup. Falls back to the RHOBS_CLIENT_ID env var.
      --client-secret string   RHOBS SSO client secret - skips Vault lookup. Falls back to the RHOBS_CLIENT_SECRET env var.
  -C, --cluster-id string      Name or Internal ID of the cluster (defaults to current cluster context)
      --hive-ocm-url string    OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
  -S, --skip-version-check     skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl rhobs](osdctl_rhobs.md)	 - RHOBS.next related utilities
* [osdctl rhobs mcp config](osdctl_rhobs_mcp_config.md)	 - Print MCP client configuration JSON
* [osdctl rhobs mcp server](osdctl_rhobs_mcp_server.md)	 - Start the RHOBS MCP server

