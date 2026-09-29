## osdctl rhobs

RHOBS.next related utilities

### Synopsis

RHOBS.next related utilities.

For headless automation, have the trusted launcher inject RHOBS_CLIENT_ID and
RHOBS_CLIENT_SECRET into the osdctl process environment from your secret manager.

Alternatively, set rhobs_client_id and rhobs_client_secret in ~/.config/osdctl:
  rhobs_client_id: "<client-id>"
  rhobs_client_secret: "<client-secret>"
Restrict access to this file (for example, chmod 600 ~/.config/osdctl).

Each environment variable overrides its corresponding config value. An explicitly
empty environment variable returns an error instead of falling back to config.
A complete pair bypasses Vault; an incomplete pair returns an error. Vault is
used only when neither credential is supplied. Direct credentials apply to all
RHOBS environments queried by the process; supply the pair for your target environment.

Keep secret values out of agent prompts, tool calls, and shell tracing.
Environment variables remain accessible to the receiving process and may be
inherited by child processes.

### Options

```
  -C, --cluster-id string     Name or Internal ID of the cluster (defaults to current cluster context)
  -h, --help                  help for rhobs
      --hive-ocm-url string   OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl](osdctl.md)	 - OSD CLI
* [osdctl rhobs alerts](osdctl_rhobs_alerts.md)	 - List or silence RHOBS alerts
* [osdctl rhobs cell](osdctl_rhobs_cell.md)	 - Get the RHOBS cell for a given cluster
* [osdctl rhobs hcp-dashboard](osdctl_rhobs_hcp-dashboard.md)	 - Get the HCP dashboard URL for a given HCP cluster
* [osdctl rhobs logs](osdctl_rhobs_logs.md)	 - Fetch logs from RHOBS for a given cluster or cell
* [osdctl rhobs mcp](osdctl_rhobs_mcp.md)	 - RHOBS MCP server for AI agent integration
* [osdctl rhobs metrics](osdctl_rhobs_metrics.md)	 - Fetch metrics from RHOBS for a given cluster

