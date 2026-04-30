## osdctl rhobs logs

Fetch logs from RHOBS

```
osdctl rhobs logs [pod] [flags]
```

### Options

```
      --contain stringArray             Text the log message must contain - flag can be repeated
      --contain-regex stringArray       Regular expression the log message must contain - flag can be repeated
  -c, --container string                Name of the container - print all containers logs if not specified
      --direction string                Direction of the logs to return - allowed values: "forward" or "backward" - "backward" returns the most recent & interesting logs first, while "forward" matches the behavior of "kubectl logs" by returning the oldest logs first (default "backward")
      --end-time time                   End time for the log query (default: now)
      --fields string                   Fields to print with the log message - not applicable for the "json" output format - comma-separated values without spaces - for instance: "k8s_namespace_name,k8s_pod_name,k8s_container_name" - use the "json" output format to print all fields in JSON format (default "k8s_pod_name")
  -h, --help                            help for logs
      --include-events                  Include events in the logs output - may add significant noise, use with caution
      --level strings                   Log level to retain - allowed values: "default", "trace", "info", "warn", "error" - flag can be repeated / values can also be aggregated with a single flag using the comma as separator
      --limit int                       Maximum number of logs to return - allowed range: [1 100000] (default 10000)
  -n, --namespace string                Name of the namespace (default "default")
      --no-limit                        Do not limit the number of logs to return
      --not-contain stringArray         Text the log message must not contain - flag can be repeated
      --not-contain-regex stringArray   Regular expression the log message must not contain - flag can be repeated
  -o, --output string                   Format of the output - allowed values: "text", "csv" or "json" (default "text")
  -q, --query string                    Loki query - exclusive with many other flags
  -l, --selector string                 Label selector for filtering pods - exclusive with the pod argument
      --since duration                  Only return logs newer than a relative duration (e.g. 1h, 30m) - exclusive with --start-time & --end-time
      --start-time time                 Start time for the log query - alternate alias: --since-time (default: 30 minutes ago)
      --ts                              Print metadata timestamps - to be used when log messages do not have a timestamp - not applicable for the "json" output format
```

### Options inherited from parent commands

```
  -C, --cluster-id string     Name or Internal ID of the cluster (defaults to current cluster context)
      --hive-ocm-url string   OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
  -S, --skip-version-check    skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl rhobs](osdctl_rhobs.md)	 - RHOBS.next related utilities

