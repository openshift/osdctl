## osdctl hive clustersync-failures

List clustersync failures

### Synopsis


  Helps investigate ClusterSyncs in a failure state on OSD/ROSA hive shards.

  This command by default will list ClusterSyncs that are in a failure state
  for clusters that are not in limited support or hibernating.

  Error messages are include in all output format except the text format.


```
osdctl hive clustersync-failures [flags]
```

### Examples

```

  # List clustersync failures using the short version of the command
  $ osdctl hive csf

  # Output in a yaml format, excluding which syncsets are failing and sorting
  # by timestamp in a descending order
  $ osdctl hive csf --syncsets=false --output=yaml --sort-by=timestamp --order=desc

  # Include limited support and hibernating clusters
  $ osdctl hive csf --limited-support -hibernating

  # List failures and error message for a single cluster
  $ osdctl hive csf -C <cluster-id>

```

### Options

```
  -C, --cluster-id string   Internal ID to list failing syncsets and relative errors for a specific cluster.
  -h, --help                help for clustersync-failures
  -i, --hibernating         Include hibernating clusters.
  -l, --limited-support     Include clusters in limited support.
      --no-headers          Don't print headers when output format is set to text.
      --order string        Set the sorting order. Options: asc, desc. (default "asc")
  -o, --output string       Set the output format. Options: yaml, json, csv, text. (default "text")
      --sort-by string      Sort the output by a specified field. Options: name, timestamp, failingsyncsets. (default "timestamp")
      --syncsets            Include failing syncsets. (default true)
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

* [osdctl hive](osdctl_hive.md)	 - hive related utilities

