## osdctl account verify-secrets

Verify AWS Account CR IAM User credentials

### Synopsis

Verify AWS Account CR IAM User credentials

```
osdctl account verify-secrets [<account name>] [flags]
```

### Options

```
      --account-namespace string   The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
  -A, --all                        Verify all Account CRs
  -h, --help                       help for verify-secrets
  -m, --verbose                    Verbose output
```

### Options inherited from parent commands

```
      --alsologtostderr                  log to standard error as well as files
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --logtostderr                      log to standard error instead of files
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO

* [osdctl account](osdctl_account.md)	 - AWS Account related utilities

