## osdctl

OSD CLI

### Synopsis

CLI tool to provide OSD related utilities

```
osdctl [flags]
```

### Options

```
      --cluster string             The name of the kubeconfig cluster to use
      --context string             The name of the kubeconfig context to use
  -h, --help                       help for osdctl
      --insecure-skip-tls-verify   If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string          Path to the kubeconfig file to use for CLI requests.
  -n, --namespace string           If present, the namespace scope for this CLI request
      --request-timeout string     The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string              The address and port of the Kubernetes API server
```

### SEE ALSO

* [osdctl clean-velero-snapshots](osdctl_clean-velero-snapshots.md)	 - cleans up S3 buckets whose name start with managed-velero
* [osdctl console](osdctl_console.md)	 - generate an AWS console URL on the fly
* [osdctl list](osdctl_list.md)	 - list resources
* [osdctl metrics](osdctl_metrics.md)	 - display metrics of aws-account-operator
* [osdctl options](osdctl_options.md)	 - Print the list of flags inherited by all commands
* [osdctl reset](osdctl_reset.md)	 - reset AWS account
* [osdctl set](osdctl_set.md)	 - set AWS account cr status

