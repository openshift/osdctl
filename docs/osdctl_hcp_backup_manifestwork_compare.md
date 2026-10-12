## osdctl hcp backup manifestwork compare

Compare HCP ManifestWorks between management clusters

```
osdctl hcp backup manifestwork compare --cluster-id <cluster-id> --replacement-mc <management-cluster> --reason <reason> [--source-mc <management-cluster>] [flags]
```

### Examples

```
  osdctl hcp backup manifestwork compare --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}
  osdctl hcp backup manifestwork compare --cluster-id ${CLUSTER_ID} --source-mc ${SOURCE_MC} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}
```

### Options

```
  -C, --cluster-id string       Internal ID, name, or external ID of the HCP cluster
  -h, --help                    help for compare
      --reason string           Reason for elevated access
      --replacement-mc string   Management cluster whose namespace contains the replacement ManifestWorks
      --source-mc string        Management cluster whose namespace contains the ManifestWorks (defaults to the HCP management cluster)
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

* [osdctl hcp backup manifestwork](osdctl_hcp_backup_manifestwork.md)	 - Compare and relocate HCP ManifestWorks

