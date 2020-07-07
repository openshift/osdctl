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
      --request-timeout string     The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string              The address and port of the Kubernetes API server
```

### SEE ALSO

* [osdctl account](osdctl_account.md)	 - AWS Account related utilities
<<<<<<< HEAD
* [osdctl clusterdeployment](osdctl_clusterdeployment.md)	 - cluster deployment related utilities
* [osdctl completion](osdctl_completion.md)	 - Output shell completion code for the specified shell (bash or zsh)
=======
>>>>>>> added spaces for lint. also edited osdctl.md to include cost command added blank osdctl_cost.md file
* [osdctl cost](osdctl_cost.md)	 - Cost Management related utilities
* [osdctl metrics](osdctl_metrics.md)	 - Display metrics of aws-account-operator
* [osdctl options](osdctl_options.md)	 - Print the list of flags inherited by all commands

