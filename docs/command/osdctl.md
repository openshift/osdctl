## osdctl

OSD CLI

### Synopsis

CLI tool to provide OSD related utilities

### Options

```
      --alsologtostderr                  log to standard error as well as files
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for osdctl
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

* [osdctl aao](osdctl_aao.md)	 - AWS Account Operator Debugging Utilities
* [osdctl account](osdctl_account.md)	 - AWS Account related utilities
* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster
* [osdctl clusterdeployment](osdctl_clusterdeployment.md)	 - cluster deployment related utilities
* [osdctl completion](osdctl_completion.md)	 - Output shell completion code for the specified shell (bash or zsh)
* [osdctl cost](osdctl_cost.md)	 - Cost Management related utilities
* [osdctl env](osdctl_env.md)	 - Create an environment to interact with a cluster
* [osdctl jumphost](osdctl_jumphost.md)	 - 
* [osdctl network](osdctl_network.md)	 - network related utilities
* [osdctl options](osdctl_options.md)	 - Print the list of flags inherited by all commands
* [osdctl org](osdctl_org.md)	 - Provides information for a specified organization
* [osdctl servicelog](osdctl_servicelog.md)	 - OCM/Hive Service log
* [osdctl sts](osdctl_sts.md)	 - STS related utilities
* [osdctl upgrade](osdctl_upgrade.md)	 - Upgrade osdctl
* [osdctl version](osdctl_version.md)	 - Display the version

