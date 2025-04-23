## osdctl cluster hypershift-info

Pull information about AWS objects from the cluster, the management cluster and the privatelink cluster

### Synopsis

This command aggregates AWS objects from the cluster, management cluster and privatelink for hypershift cluster.
It attempts to render the relationships as graphviz if that output format is chosen or will simply print the output as tables.

```
osdctl cluster hypershift-info [flags]
```

### Options

```
  -c, --cluster-id string           Provide internal ID of the cluster
  -h, --help                        help for hypershift-info
  -o, --output string               output format ['table', 'graphviz'] (default "graphviz")
  -l, --privatelinkaccount string   Privatelink account ID
  -p, --profile string              AWS Profile
  -r, --region string               AWS Region
      --verbose                     Verbose output
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

* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster

