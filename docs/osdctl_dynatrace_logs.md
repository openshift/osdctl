## osdctl dynatrace logs

Fetch logs from Dynatrace

### Synopsis


  Fetch logs of current cluster context (by default) from Dynatrace and display the logs like oc logs.

  This command also prints the Dynatrace URL and the corresponding DQL in the output.



```
osdctl dynatrace logs --cluster-id <cluster-identifier> [flags]
```

### Examples

```

  # Get the logs of the cluster in the current context.
  $ osdctl dt logs

  # Get the logs of a specific cluster
  $ osdctl dt logs --cluster-id <cluster-id>

 # Get a link to the dynatrace UI for the current cluster context.
  $ osdctl dt logs --console

  # Get the logs of the pod alertmanager-main-0 in namespace openshift-monitoring in the current cluster context.
  $ osdctl dt logs alertmanager-main-0 -n openshift-monitoring

 # Get the logs of the pod alertmanager-main-0 in namespace openshift-monitoring for a specific HCP cluster
  $ osdctl dt logs alertmanager-main-0 -n openshift-monitoring --cluster-id <cluster-id>

  # Only return logs newer than 2 hours old (an integer in hours)
  $ osdctl dt logs alertmanager-main-0 -n openshift-monitoring --since 2

  # Get logs for a specific time range using --from and --to flags
  $ osdctl dt logs alertmanager-main-0 -n openshift-monitoring --from "2025-06-15 04:00" --to "2025-06-17 13:00"

  # Restrict return of logs to those that contain a specific phrase
  $ osdctl dt logs alertmanager-main-0 -n openshift-monitoring --contains <phrase>

```

### Options

```
      --cluster-id string   Name or Internal ID of the cluster (defaults to current cluster context)
      --console             Print the url to the dynatrace web console instead of outputting the logs
      --container strings   Container name(s) (comma-separated)
      --contains string     Include logs which contain a phrase
      --dry-run             Only builds the query without fetching any logs from the tenant
      --from time           Datetime from which to filter logs, in the format "YYYY-MM-DD HH:MM" (default 0001-01-01T00:00:00Z)
  -h, --help                help for logs
  -n, --namespace strings   Namespace(s) (comma-separated)
      --node strings        Node name(s) (comma-separated)
      --since int           Number of hours (integer) since which to search (defaults to 1 hour) (default 1)
      --sort string         Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc'. Defaults to 'asc' (default "asc")
      --status strings      Status(Info/Warn/Error) (comma-separated)
      --tail int            Last 'n' logs to fetch (defaults to 100) (default 1000)
      --to time             Datetime until which to filter logs to, in the format "YYYY-MM-DD HH:MM" (default 0001-01-01T00:00:00Z)
```

### Options inherited from parent commands

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl dynatrace](osdctl_dynatrace.md)	 - Dynatrace related utilities

