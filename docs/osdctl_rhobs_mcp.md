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
  - RHOBS credentials: resolve each value from RHOBS_CLIENT_ID/RHOBS_CLIENT_SECRET,
    then rhobs_client_id/rhobs_client_secret in ~/.config/osdctl.
    A complete pair bypasses Vault; an incomplete pair or explicitly empty
    environment variable returns an error.
  - Vault fallback (when neither credential resolves): configure rhobs_<env>_vault_path
    in ~/.config/osdctl and log in with VAULT_ADDR=https://vault.devshift.net vault login -method=oidc

### Options

```
  -h, --help   help for mcp
```

### Options inherited from parent commands

```
  -C, --cluster-id string     Name or Internal ID of the cluster (defaults to current cluster context)
      --hive-ocm-url string   OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
  -S, --skip-version-check    skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl rhobs](osdctl_rhobs.md)	 - RHOBS.next related utilities
* [osdctl rhobs mcp config](osdctl_rhobs_mcp_config.md)	 - Print MCP client configuration JSON
* [osdctl rhobs mcp server](osdctl_rhobs_mcp_server.md)	 - Start the RHOBS MCP server

