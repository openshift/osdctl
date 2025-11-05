## osdctl cluster reports create

Create a new cluster report in backplane-api

```
osdctl cluster reports create [flags]
```

### Options

```
  -C, --cluster-id string   Cluster ID (internal or external)
  -d, --data string         Report data as a string (will be base64 encoded)
  -f, --file string         Path to file containing report data (will be base64 encoded)
  -h, --help                help for create
  -o, --output string       Output format: table or json (default "table")
      --summary string      Summary/title for the report
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

* [osdctl cluster reports](osdctl_cluster_reports.md)	 - Cluster Reports from backplane-api

