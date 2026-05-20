## osdctl rhobs logs

Fetch logs from RHOBS for a given cluster

### Synopsis

Fetch logs from RHOBS for a given cluster. The cluster can be a management cluster (MC) or whatever cluster sending logs to RHOBS; the command works as if the management cluster ID was passed if given a hosted cluster (HCP) ID. By default, logs from all the pods in the given namespace are returned but it is possible to specify a single pod as an argument or filter pods using their labels. Logs themselves can be also filtered to only keep the ones containing a given regexp (--contain-regex option) or a given log level (--level option).

```
osdctl rhobs logs [pod] [flags]
```

### Options

```
      --contain stringArray             Text the log message must contain - flag can be repeated
      --contain-regex stringArray       Regular expression the log message must contain - flag can be repeated
  -c, --container string                Name of the container - print all containers logs if not specified
      --direction string                Direction of the logs to return - allowed values: "forward" or "backward" - "backward" returns the most recent & interesting logs first, while "forward" matches the behavior of "kubectl logs" by returning the oldest logs first (default to "backward" unless --follow is set in which case it is forced to "forward")
      --end-time time                   End time for the logs (default to now)
      --field strings                   Fields to print with the log message - not possible with the "json" output format - flag can be repeated / values can also be aggregated with one flag using the comma as separator - possible values: "k8s_namespace_name", "k8s_pod_name", "k8s_container_name" - use the "json" output format to know about all possible fields (default [k8s_pod_name])
  -f, --follow                          Specify if the logs should be streamed - exclusive with --start-time, --end-time, --since, --direction, --limit and --no-limit flags
  -h, --help                            help for logs
      --include-events                  Include events in the logs output - may add significant noise, use with caution
      --level strings                   Log level to retain - allowed values: "default", "trace", "info", "warn", "error" - flag can be repeated / values can also be aggregated with one flag using the comma as separator
      --limit int                       Maximum number of logs to return - allowed range: [1 100000] (default to 10000 unless --follow is set in which case there is no limit)
  -n, --namespace string                Name of the namespace (default "default")
      --no-limit                        Do not limit the number of logs to return - exclusive with --limit flag
      --not-contain stringArray         Text the log message must not contain - flag can be repeated
      --not-contain-regex stringArray   Regular expression the log message must not contain - flag can be repeated
  -o, --output string                   Format of the output - allowed values: "text", "csv" or "json" (default "text")
  -q, --query string                    LogQL expression - exclusive with many other flags
  -l, --selector string                 Label selector for filtering pods - exclusive with the pod argument
      --since duration                  Only return logs newer than a relative duration (e.g. 1h, 30m) - exclusive with --start-time & --end-time
      --start-time time                 Start time for the logs - alternate alias: --since-time (default to 5 minutes ago)
      --ts                              Print metadata timestamps - to be used when log messages do not have a timestamp - not possible with the "json" output format
```

### Options inherited from parent commands

```
  -C, --cluster-id string     Name or Internal ID of the cluster (defaults to current cluster context)
      --hive-ocm-url string   OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
  -S, --skip-version-check    skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl rhobs](osdctl_rhobs.md)	 - RHOBS.next related utilities

