## osdctl cluster pull-secret update

Refresh a cluster's pull secret from the cluster owner's OCM account

### Synopsis

Refresh a cluster's pull secret from the cluster owner's OCM account.

This updates the pull secret on a ROSA HCP or Classic cluster without performing
an ownership transfer. The pull secret is rebuilt from the latest credentials
in the cluster owner's OCM account.

A pre-flight check always runs first. If any checks fail, the command exits
unless --force is specified (requires typing YES to confirm).

See documentation prior to executing:
https://github.com/openshift/ops-sop/blob/master/hypershift/knowledge_base/howto/replace-pull-secret.md
https://github.com/openshift/ops-sop/blob/master/v4/howto/transfer_cluster_ownership.md

```
osdctl cluster pull-secret update [flags]
```

### Examples

```
  # Update pull secret on a cluster
  osdctl cluster pull-secret update --cluster-id 1kfmyclusterid --reason "OHSS-1234"

  # Dry-run to preview without making changes
  osdctl cluster pull-secret update --cluster-id 1kfmyclusterid --reason "OHSS-1234" --dry-run

  # Force proceed despite pre-flight failures (e.g. missing pull secret)
  osdctl cluster pull-secret update --cluster-id 1kfmyclusterid --reason "OHSS-1234" --force
```

### Options

```
  -C, --cluster-id string     The Internal/External Cluster ID or Cluster Name
  -d, --dry-run               Dry-run - show what would change but do not apply
      --force                 Proceed despite pre-flight failures or no-op detection (requires YES confirmation)
  -h, --help                  help for update
      --hive-ocm-url string   OCM environment for Hive operations (aliases: production, staging, integration)
      --reason string         The reason for this command (usually an OHSS or PD ticket)
```

### Options inherited from parent commands

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl cluster pull-secret](osdctl_cluster_pull-secret.md)	 - Diagnose and manage cluster pull secrets

