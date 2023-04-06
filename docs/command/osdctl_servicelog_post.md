## osdctl servicelog post

Send a servicelog message to a given cluster

```
osdctl servicelog post CLUSTER_ID [flags]
```

### Options

```
  -c, --clusters-file string     Read a list of clusters to post the servicelog to. the format of the file is: {"clusters":["$CLUSTERID"]}
  -d, --dry-run                  Dry-run - print the service log about to be sent but don't send it.
  -h, --help                     help for post
  -i, --internal                 Internal only service log. Use MESSAGE for template parameter (eg. -p MESSAGE='My super secret message').
  -p, --param stringArray        Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.
  -q, --query stringArray        Specify a search query (eg. -q "name like foo") for a bulk-post to matching clusters.
  -f, --query-file stringArray   File containing search queries to apply. All lines in the file will be concatenated into a single query. If this flag is called multiple times, every file's search query will be combined with logical AND.
  -t, --template string          Message template file or URL
  -y, --yes                      Skips all prompts.
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

* [osdctl servicelog](osdctl_servicelog.md)	 - OCM/Hive Service log

