## osdctl cluster transfer-owner

Transfer cluster ownership to a new user (to be done by Region Lead)

```
osdctl cluster transfer-owner [flags]
```

### Options

```
  -C, --cluster-id string            The Internal Cluster ID/External Cluster ID/ Cluster Name
  -d, --dry-run                      Dry-run - show all changes but do not apply them
  -h, --help                         help for transfer-owner
      --new-organization-id string   Organization of the new owner
      --new-owner string             The new owners username to transfer the cluster to
      --old-organization-id string   Organization of the old owner
      --old-owner string             The old owners username to transfer the cluster from
```

### Options inherited from parent commands

```
      --alsologtostderr                  log to standard error as well as files
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --logtostderr                      log to standard error instead of files
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO

* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster

