## osdctl alert silence org

Add new silence for alert for org

### Synopsis

Add a new silence for specific alerts or all alerts with a comment and duration for an organization. OHSS required for org-wide silence.

```
osdctl alert silence org <org-id> [--all --duration --comment | --alertname --duration --comment] [flags]
```

### Examples

```
  # Silence all alerts for an organization
  osdctl alerts silence org ${ORG_ID} --all --comment "${REASON}: org-wide silence"

  # Silence a specific alert for an organization
  osdctl alerts silence org ${ORG_ID} --alertname "KubePodNotReady" --comment "${REASON}: investigating pod issue"
```

### Options

```
      --alertname strings   alertname (comma-separated)
  -a, --all                 add silences for all alert
  -c, --comment string      add comment about silence. OHSS required for org-wide silence
  -d, --duration string     add duration for silence (default "15d")
  -h, --help                help for org
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

* [osdctl alert silence](osdctl_alert_silence.md)	 - add, expire and list silence associated with alerts

