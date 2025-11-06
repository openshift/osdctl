## osdctl cluster reports get

Get a specific cluster report from backplane-api

### Synopsis

Retrieve and display a specific report by its ID.

This command fetches a report by its report ID and displays the decoded
report data. Use 'list' to find available report IDs.

Examples:
  # Get a specific report and display its contents
  osdctl cluster reports get --cluster-id 1a2b3c4d --report-id abc-123-def

  # Get a report with JSON output (includes encoded data)
  osdctl cluster reports get -C my-cluster -r report-456 --output json

  # Get a report and pipe the output to a file
  osdctl cluster reports get -C 1a2b3c4d -r abc-123 > report-output.txt

```
osdctl cluster reports get [flags]
```

### Options

```
  -C, --cluster-id string   Cluster ID (internal or external)
  -h, --help                help for get
  -o, --output string       Output format: text or json (default "text")
  -r, --report-id string    Report ID to retrieve
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

