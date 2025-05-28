## osdctl cloudtrail write-events

Prints cloudtrail write events to console with advanced filtering options

### Synopsis


	Lists AWS CloudTrail write events for a specific OpenShift/ROSA cluster with advanced 
	filtering capabilities to help investigate cluster-related activities.

	The command automatically authenticates with OpenShift Cluster Manager (OCM) and assumes 
	the appropriate AWS role for the target cluster to access CloudTrail logs.

	By default, the command filters out system and service account events using patterns 
	from the osdctl configuration file. 

```
osdctl cloudtrail write-events [flags]
```

### Examples

```

    # Time range with user and include events where username=(john.doe or system) and event=(CreateBucket or AssumeRole); print custom format
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,09:00:00 --until 2025-07-15,17:00:00 \
      -I username=john.doe -I event=CreateBucket -E event=AssumeRole -E username=system --print-format event,time,username,resource-name

    # Get all events from a specific time onwards for a 2h duration; print url
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,15:00:00 --since 2h --url

    # Get all events until the specified time since the last 2 hours; print raw-event
    $ osdctl cloudtrail write-events -C cluster-id --after 2025-07-15,15:00:00 --since 2h --raw-event
```

### Options

```
      --after string           Specifies all events that occur after the specified time. Format "YY-MM-DD,hh:mm:ss".
      --cache                  Enable/Disable cache file for write-events (default true)
  -C, --cluster-id string      Cluster ID
  -E, --exclude strings        Filter events by exclusion. (i.e. "-E username=, -E event=, -E resource-name=, -E resource-type=, -E arn=")
  -h, --help                   help for write-events
  -I, --include strings        Filter events by inclusion. (i.e. "-I username=, -I event=, -I resource-name=, -I resource-type=, -I arn=")
  -l, --log-level string       Options: "info", "debug", "warn", "error". (default=info) (default "info")
      --print-fields strings   Prints all cloudtrail write events in selected format. Can specify (username, time, event, arn, resource-name, resource-type, arn). i.e --print-format username,time,event (default [event,time,username,arn])
  -r, --raw-event              Prints the cloudtrail events to the console in raw json format
      --since string           Specifies that only events that occur within the specified time are returned. Defaults to 1h.Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". (default "1h")
      --until string           Specifies all events that occur before the specified time. Format "YY-MM-DD,hh:mm:ss".
  -u, --url                    Generates Url link to cloud console cloudtrail event
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

* [osdctl cloudtrail](osdctl_cloudtrail.md)	 - AWS CloudTrail related utilities

