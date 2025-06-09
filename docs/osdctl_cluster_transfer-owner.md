## osdctl cluster transfer-owner

Transfer cluster ownership to a new user (to be done by Region Lead)

```
osdctl cluster transfer-owner [flags]
```

### Examples

```

  # Transfer ownership
  osdctl cluster transfer-owner --new-owner "new_OCM_userName" --cluster-id 1kfmyclusteristhebesteverp8m --reason "transfer ownership per jira-id"

```

### Options

```
  -C, --cluster-id string   The Internal Cluster ID/External Cluster ID/ Cluster Name
  -d, --dry-run             Dry-run - show all changes but do not apply them
  -h, --help                help for transfer-owner
      --new-owner string    The new owner's username to transfer the cluster to
      --old-owner string    The old owner's username to transfer the cluster from
      --reason string       The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
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

* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster

