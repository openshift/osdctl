## osdctl dynatrace gather-logs

Gather all Pod logs and Application event from HCP

### Synopsis

Gathers pods logs and evnets of a given HCP from Dynatrace.

  This command fetches the logs from the HCP namespace, the hypershift namespace and cert-manager related namespaces.
  Logs will be dumped to a directory with prefix hcp-must-gather.
		

```
osdctl dynatrace gather-logs --cluster-id <cluster-identifier> [flags]
```

### Examples

```

  # Gather logs for a HCP cluster with cluster id hcp-cluster-id-123
  osdctl dt gather-logs --cluster-id hcp-cluster-id-123
```

### Options

```
      --cluster-id string   Internal ID of the HCP cluster to gather logs from (required)
      --dest-dir string     Destination directory for the logs dump, defaults to the local directory.
  -h, --help                help for gather-logs
      --since int           Number of hours (integer) since which to pull logs and events (default 10)
      --sort string         Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc' (default "asc")
      --tail int            Last 'n' logs and events to fetch. By default it will pull everything
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

* [osdctl dynatrace](osdctl_dynatrace.md)	 - Dynatrace related utilities

