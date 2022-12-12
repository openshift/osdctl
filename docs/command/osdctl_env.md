## osdctl env

Create an environment to interact with a cluster

```
osdctl env [flags] [env-alias]
```

### Options

```
  -a, --api string            OpenShift API URL for individual cluster login
  -c, --cluster-id string     Cluster ID
  -d, --delete                Delete environment
  -k, --export-kubeconfig     Output export kubeconfig statement, to use environment outside of the env directory
  -h, --help                  help for env
  -K, --kubeconfig string     KUBECONFIG file to use in this env (will be copied to the environment dir)
  -l, --login-script string   OCM login script to execute in a loop in ocb every 30 seconds
  -p, --password string       Password for individual cluster login
  -r, --reset                 Reset environment
  -t, --temp                  Delete environment on exit
  -u, --username string       Username for individual cluster login
```

### Options inherited from parent commands

```
      --alsologtostderr                  log to standard error as well as files
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
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

* [osdctl](osdctl.md)	 - OSD CLI

