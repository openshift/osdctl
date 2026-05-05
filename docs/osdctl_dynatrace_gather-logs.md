## osdctl dynatrace gather-logs

Gather all Pod logs and Application event from HCP

### Synopsis

Gathers pods logs and evnets of a given HCP from Dynatrace.

  This command fetches the logs from the HCP namespace, the hypershift namespace and cert-manager related namespaces.
  Logs will be dumped to a directory with prefix hcp-must-gather.
		

```
osdctl dynatrace gather-logs --cluster-id <cluster-identifier> [flags]
```

### Examples

```

  # Gather logs for a HCP cluster with cluster id hcp-cluster-id-123
  osdctl dt gather-logs --cluster-id hcp-cluster-id-123
```

### Options

```
  -C, --cluster-id string   Internal ID of the HCP cluster to gather logs from (required)
      --dest-dir string     Destination directory for the logs dump, defaults to the local directory.
  -h, --help                help for gather-logs
      --since int           Number of hours (integer) since which to pull logs and events (default 10)
      --sort string         Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc' (default "asc")
      --tail int            Last 'n' logs and events to fetch. By default it will pull everything
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl dynatrace](osdctl_dynatrace.md)	 - Dynatrace related utilities

