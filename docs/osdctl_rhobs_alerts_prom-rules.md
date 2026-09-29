## osdctl rhobs alerts prom-rules

The Prometheus rules (alerts & recording rules) defined on the RHOBS cell

### Options

```
  -h, --help   help for prom-rules
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
* [osdctl rhobs alerts prom-rules get](osdctl_rhobs_alerts_prom-rules_get.md)	 - List the Prometheus rules defined on the RHOBS cell

