## osdctl rhobs alerts get

List alerts from RHOBS for a given cluster

```
osdctl rhobs alerts get [flags]
```

### Options

```
  -f, --filter          Only keep the results matching the given cluster - only effective if some of those results have a _id, _mc_id or mc_name label
  -h, --help            help for get
  -o, --output string   Format of the output - allowed values: "text", "csv" or "json" (default "text")
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

* [osdctl rhobs alerts](osdctl_rhobs_alerts.md)	 - List or silence RHOBS alerts

