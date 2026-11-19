## osdctl hcp recover

Recover ExternalDNS and AWSEndpointService reconciliation after disaster recovery

### Synopsis

Recover ExternalDNS and AWSEndpointService reconciliation after HCP management cluster disaster recovery.

When a hosted cluster is restored on a replacement management cluster, the AWSEndpointService
controller may fail to reconcile the existing load balancer endpoint. This command
automates recovery by:

  1. Verifying DNS cleanup (old Route 53 TXT ownership records removed)
  2. Confirming cluster resource state (private-router AWSEndpointService)
  3. Deleting private-router to force controller rediscovery
  4. Polling for resource recreation with automatic reconciliation
  5. Outputting new/updated resources after reconciliation

The command supports --dry-run to preview changes without making modifications, and
--skip-dns-verification to resume after DNS cleanup has been verified separately.

```
osdctl hcp recover --cluster-id <hcp-id> --management-cluster <mc-name> --reason <reason> [flags]
```

### Examples

```
  # Preview recovery without making changes
  osdctl hcp recover --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR recovery"

  # Preview with all flags
  osdctl hcp recover --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR recovery" --dry-run

  # Resume recovery after DNS cleanup
  osdctl hcp recover --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR recovery" --skip-dns-verification
```

### Options

```
  -C, --cluster-id string            Internal ID, name, or external ID of the HCP cluster
      --debug                        Enable debug logging output
      --dry-run                      Preview changes without deleting resources
  -h, --help                         help for recover
  -M, --management-cluster string    Name of the replacement management cluster (required)
      --reason string                Reason for elevated access to management cluster
      --restart-network-components   Restart kube-apiserver, konnectivity, CPO, HCCO, OVN, and router after recovery
      --skip-dns-verification        Skip Route 53 TXT ownership record verification (use if DNS cleanup already verified)
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

* [osdctl hcp](osdctl_hcp.md)	 - 
* [osdctl hcp recover validate](osdctl_hcp_recover_validate.md)	 - Validate DNS and networking health after HCP disaster recovery

