## osdctl rhobs metrics

Fetch metrics from RHOBS for a given cluster

### Synopsis

Fetch metrics from RHOBS for a given cluster. The cluster can be a hosted cluster (HCP), a management cluster (MC) or whatever cluster sending metrics to RHOBS. The prometheus expression provided as an argument can be either an instant query or a range query; it is optional if the --url option is set. By default, the command will try to evaluate the expression as an instant query at the current time, but it is possible to specify a different evaluation time using the --time option or a time range using the --start-time, --end-time and --since options. Results can be filtered to only keep the ones matching the given cluster (--cluster-id option) with the --filter option even if it is more efficient to do that filtering at the prometheus expression level.

```
osdctl rhobs metrics [PromQL-expression] [flags]
```

### Options

```
  -b, --browser           Open in the default browser the URL computed with the --url option - only applicable if --url is set
      --end-time time     End time at which the PromQL expression must be evaluated - can only be set if --start-time or --url is set (default to now)
  -f, --filter            Only keep the results matching the given cluster - only effective if some of those results have a _id, _mc_id or mc_name label - exclusive with --url
  -h, --help              help for metrics
  -o, --output string     Format of the output - allowed values: "table", "csv" or "json" - exclusive with --url (default "table")
      --since duration    Only return values newer than a relative duration (e.g. 1h, 30m) - enable time range mode - exclusive with --time, --start-time & --end-time
      --start-time time   Start time at which the PromQL expression must be evaluated - enable time range mode - exclusive with --time (default to 30 minutes ago)
      --step duration     Duration between data points (e.g. 30s, 2m) - can only be set if in time range mode (i.e. --start-time or --since is set)
      --time time         Time at which the PromQL expression must be evaluated - exclusive with --url (default to now)
  -u, --url               Only compute and print the grafana URL
```

### Options inherited from parent commands

```
  -C, --cluster-id string     Name or Internal ID of the cluster (defaults to current cluster context)
      --hive-ocm-url string   OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
  -S, --skip-version-check    skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl rhobs](osdctl_rhobs.md)	 - RHOBS.next related utilities

