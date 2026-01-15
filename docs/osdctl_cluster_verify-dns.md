## osdctl cluster verify-dns

Verify DNS resolution for HCP cluster public endpoints

### Synopsis

Performs DNS resolution tests for HCP clusters.

Note: This command should be run when on the Red Hat VPN

This command tests DNS resolution for cluster public endpoints:
- Wildcard A record: *.apps.rosa.<cluster-name>.<base-domain>
- Apps CNAME: apps.rosa.<cluster-name>.<base-domain>
- ACME challenge CNAME: _acme-challenge.apps.rosa.<cluster-name>.<base-domain>
- Cluster ID CNAME: <cluster-id>.rosa.<cluster-name>.<base-domain>
- ACME cluster CNAME: _acme-challenge.<cluster-id>.rosa.<cluster-name>.<base-domain>
- API record: api.<cluster-name>.<base-domain> (A record or CNAME based on PrivateLink)
- OAuth record: oauth.<cluster-name>.<base-domain> (A record or CNAME based on PrivateLink)

This command only supports HCP (Hosted Control Plane) clusters.

Output Formats:
- table (default): Human-readable table format with summary and recommendations
- json: JSON format for programmatic consumption

```
osdctl cluster verify-dns --cluster-id <cluster-id> [flags]
```

### Options

```
  -C, --cluster-id string   Cluster ID (internal or external)
  -h, --help                help for verify-dns
  -o, --output string       Output format: 'table' or 'json' (default "table")
  -v, --verbose             Verbose output
```

### Options inherited from parent commands

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster

