## osdctl account mgmt iam

Creates an IAM user in a given AWS account and prints out the credentials

```
osdctl account mgmt iam [flags]
```

### Options

```
  -i, --accountId string   AWS account ID to run this against
  -h, --help               help for iam
  -p, --profile string     AWS Profile
  -r, --region string      AWS Region
  -R, --rotate             Rotate an IAM user's credentials and print the output
  -u, --user string        Kerberos username to run this for
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

* [osdctl account mgmt](osdctl_account_mgmt.md)	 - AWS Account Management

