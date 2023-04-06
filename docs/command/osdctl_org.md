## osdctl org

Provides information for a specified organization

```
osdctl org [flags]
```

### Options

```
  -h, --help   help for org
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
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO

* [osdctl](osdctl.md)	 - OSD CLI
* [osdctl org aws-accounts](osdctl_org_aws-accounts.md)	 - get organization AWS Accounts
* [osdctl org clusters](osdctl_org_clusters.md)	 - get organization clusters
* [osdctl org current](osdctl_org_current.md)	 - gets current organization
* [osdctl org customers](osdctl_org_customers.md)	 - get paying/non-paying organizations
* [osdctl org describe](osdctl_org_describe.md)	 - describe organization
* [osdctl org get](osdctl_org_get.md)	 - get organization by users
* [osdctl org labels](osdctl_org_labels.md)	 - get organization labels
* [osdctl org users](osdctl_org_users.md)	 - get organization users

