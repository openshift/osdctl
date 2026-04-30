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
  -C, --cluster-id string   Name or Internal ID of the cluster (defaults to current cluster context)
      --console             Print the url to the dynatrace web console instead of outputting the logs
      --container strings   Container name(s) (comma-separated)
      --contains string     Include logs which contain a phrase
      --dry-run             Only builds the query without fetching any logs from the tenant
      --from time           Datetime from which to filter logs, in the format "YYYY-MM-DD HH:MM"
  -h, --help                help for logs
  -n, --namespace strings   Namespace(s) (comma-separated)
      --node strings        Node name(s) (comma-separated)
      --since int           Number of hours (integer) since which to search (default 1)
      --sort string         Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc'. (default "asc")
      --status strings      Status(Info/Warn/Error) (comma-separated)
      --tail int            Last 'n' logs to fetch (default 1000)
      --to time             Datetime until which to filter logs to, in the format "YYYY-MM-DD HH:MM"
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl dynatrace](osdctl_dynatrace.md)	 - Dynatrace related utilities

