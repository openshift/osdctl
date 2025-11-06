## osdctl cluster reports list

List cluster reports from backplane-api

### Synopsis

List all reports for a specific cluster.

This command retrieves and displays all reports associated with a cluster,
showing the report ID, summary, and creation timestamp. You can optionally
limit the number of reports returned to the most recent N reports.

Examples:
  # List all reports for a cluster (defaults to 10 most recent)
  osdctl cluster reports list --cluster-id 1a2b3c4d

  # List the 5 most recent reports
  osdctl cluster reports list --cluster-id 1a2b3c4d --last 5

  # List reports with JSON output
  osdctl cluster reports list --cluster-id my-cluster --output json

```
osdctl cluster reports list [flags]
```

### Options

```
  -C, --cluster-id string   Cluster ID (internal or external)
  -h, --help                help for list
  -l, --last int            Number of most recent reports to retrieve (backend defaults to 10)
  -o, --output string       Output format: table or json (default "table")
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

* [osdctl cluster reports](osdctl_cluster_reports.md)	 - Manage cluster reports in backplane-api

