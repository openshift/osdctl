## osdctl cluster validate-pull-secret

Checks if the pull secret email matches the owner email

### Synopsis

Checks if the pull secret email matches the owner email.

The command will first attempt to create a managedjob on the cluster to complete the task.
However if this fails (e.g. pod fails to run on the cluster), the fallback option of elevating
with backplane (requires reason and cluster-id) can be run.


```
osdctl cluster validate-pull-secret --cluster-id <cluster-identifier> [flags]
```

### Options

```
  -C, --cluster-id string   The internal ID of the cluster to check (only required if elevating, and ID is not found within context.)
      --elevate             Skip managed job approach and use backplane elevation directly
  -h, --help                help for validate-pull-secret
      --reason string       The reason for this command to be run (usually an OHSS or PD ticket)
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

