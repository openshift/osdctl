## osdctl cloudtrail errors

Prints CloudTrail error events (permission/IAM issues) to console.

### Synopsis

Surfaces permission and IAM-related errors from AWS CloudTrail.

By default, matches these error patterns:
  - AccessDenied
  - UnauthorizedOperation / Client.UnauthorizedOperation
  - Forbidden
  - InvalidClientTokenId
  - AuthFailure
  - ExpiredToken
  - SignatureDoesNotMatch

Use --error-types to filter for specific error patterns.

```
osdctl cloudtrail errors [flags]
```

### Examples

```
  # Check for permission errors in the last hour
  osdctl cloudtrail errors -C <cluster-id> --since 1h

  # Check for specific error types only
  osdctl cloudtrail errors -C <cluster-id> --error-types AccessDenied,Forbidden

  # Output as JSON for scripting
  osdctl cloudtrail errors -C <cluster-id> --json

  # Include console links for each event
  osdctl cloudtrail errors -C <cluster-id> --url
```

### Options

```
  -C, --cluster-id string     Cluster ID
      --error-types strings   Comma-separated list of error patterns to match (default: all common permission errors)
  -h, --help                  help for errors
      --json                  Output results as JSON
  -r, --raw-event             Print raw CloudTrail event JSON
      --since string          Time window to search (e.g., 30m, 1h, 24h). Valid units: ns, us, ms, s, m, h. (default "1h")
  -u, --url                   Include console URL links for each event
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

