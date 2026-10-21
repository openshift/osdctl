## osdctl hcp restore

Prepare and restore HCP clusters during disaster recovery

### Options

```
  -h, --help   help for restore
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
* [osdctl hcp restore create](osdctl_hcp_restore_create.md)	 - Create and monitor a Velero restore for an HCP backup
* [osdctl hcp restore manifestwork](osdctl_hcp_restore_manifestwork.md)	 - Compare and relocate HCP ManifestWorks
* [osdctl hcp restore release-dns](osdctl_hcp_restore_release-dns.md)	 - Release central DNS records for an HCP cluster
* [osdctl hcp restore update-trust-policy](osdctl_hcp_restore_update-trust-policy.md)	 - Allow a replacement management cluster to access HCP backups

