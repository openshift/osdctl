## osdctl rhobs mcp config

Print MCP client configuration JSON

### Synopsis

Print MCP client configuration JSON for use with AI agents.

Usage with Claude Code:
  claude --mcp-config "$(osdctl rhobs mcp config)"

Or add to ~/.claude/mcp_settings.json manually.

```
osdctl rhobs mcp config [flags]
```

### Options

```
  -h, --help   help for config
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

* [osdctl rhobs mcp](osdctl_rhobs_mcp.md)	 - RHOBS MCP server for AI agent integration

