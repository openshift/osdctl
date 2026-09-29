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
  - Vault login: VAULT_ADDR=https://vault.devshift.net vault login -method=oidc (required when direct RHOBS client ID and secret credentials are not supplied)
  - osdctl config: ~/.config/osdctl must have rhobs_<env>_vault_path entries when direct RHOBS credentials are not supplied through --client-id/--client-secret or RHOBS_CLIENT_ID/RHOBS_CLIENT_SECRET

### Options

```
  -h, --help   help for mcp
```

### Options inherited from parent commands

```
      --client-id string       RHOBS SSO client ID (falls back to RHOBS_CLIENT_ID when empty). A complete ID/secret pair from flags and/or env vars bypasses Vault credential lookup. If neither is set, Vault is used; an incomplete pair returns an error.
      --client-secret string   RHOBS SSO client secret (falls back to RHOBS_CLIENT_SECRET when empty). A complete ID/secret pair from flags and/or env vars bypasses Vault credential lookup. If neither is set, Vault is used; an incomplete pair returns an error.
  -C, --cluster-id string      Name or Internal ID of the cluster (defaults to current cluster context)
      --hive-ocm-url string    OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
  -S, --skip-version-check     skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl rhobs](osdctl_rhobs.md)	 - RHOBS.next related utilities
* [osdctl rhobs mcp config](osdctl_rhobs_mcp_config.md)	 - Print MCP client configuration JSON
* [osdctl rhobs mcp server](osdctl_rhobs_mcp_server.md)	 - Start the RHOBS MCP server

