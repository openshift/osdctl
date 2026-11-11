## osdctl hcp recover validate

Validate DNS and networking health after HCP disaster recovery

### Synopsis

Validate DNS and networking health after HCP management cluster disaster recovery.

After running recover to restore DNS reconciliation, this command performs
comprehensive health validation:

  1. Verify Route 53 records are properly reconciled (A/CNAME for api/oauth endpoints)
  2. Validate networking resources (AWSEndpointService, private-router Service)
  3. Check control plane components (kube-apiserver, etcd, networking controllers)
  4. Verify HCP cluster is reachable via OCM and backplane

Use this command to confirm that DNS recovery completed successfully and the cluster
is fully accessible.

```
osdctl hcp recover validate --cluster-id <hcp-id> --management-cluster <mc-name> --reason <reason> [flags]
```

### Examples

```
  # Validate cluster after DNS recovery
  osdctl hcp recover validate --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR validation"

  # Validate with debug logging
  osdctl hcp recover validate --cluster-id my-hcp-cluster --management-cluster dest-mgmt-cluster --reason "DR validation" --debug
```

### Options

```
  -C, --cluster-id string           Internal ID, name, or external ID of the HCP cluster
      --debug                       Enable debug logging output
  -h, --help                        help for validate
  -M, --management-cluster string   Name of the management cluster
  -R, --reason string               Reason for elevated access to management cluster
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

* [osdctl hcp recover](osdctl_hcp_recover.md)	 - Recover ExternalDNS and AWSEndpointService reconciliation after disaster recovery

