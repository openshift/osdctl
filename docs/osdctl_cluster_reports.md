## osdctl cluster reports

Manage cluster reports in backplane-api

### Synopsis

Manage cluster reports stored in backplane-api.

Cluster reports are used to store and retrieve diagnostic information
and other data related to cluster operations. Reports are associated with a
specific cluster and include a summary and base64-encoded data.

```
osdctl cluster reports [flags]
```

### Options

```
  -h, --help   help for reports
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
* [osdctl cluster reports create](osdctl_cluster_reports_create.md)	 - Create a new cluster report in backplane-api
* [osdctl cluster reports get](osdctl_cluster_reports_get.md)	 - Get a specific cluster report from backplane-api
* [osdctl cluster reports list](osdctl_cluster_reports_list.md)	 - List cluster reports from backplane-api

