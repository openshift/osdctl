## osdctl alert silence add

Add new silence for alert

### Synopsis

add new silence for specfic or all alert with comment and duration of alert

```
osdctl alert silence add <cluster-id> [--all --duration --comment | --alertname --duration --comment] [flags]
```

### Options

```
      --alertname strings   alertname (comma-separated)
  -a, --all                 Adding silences for all alert
  -c, --comment string      add comment about silence (default "Adding silence using the osdctl alert command")
  -d, --duration string     Adding duration for silence as 15 days (default "15d")
  -h, --help                help for add
      --reason string       The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
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

