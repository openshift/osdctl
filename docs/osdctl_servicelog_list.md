## osdctl servicelog list

Get service logs for a given cluster identifier.

### Synopsis

Get service logs for a given cluster identifier.

# To return just service logs created by SREs
osdctl servicelog list --cluster-id=my-cluster-id

# To return all service logs, including those by automated systems
osdctl servicelog list --cluster-id=my-cluster-id --all-messages

# To return all service logs, as well as internal service logs
osdctl servicelog list --cluster-id=my-cluster-id --all-messages --internal


```
osdctl servicelog list --cluster-id <cluster-identifier> [flags] [options]
```

### Options

```
  -A, --all-messages        Toggle if we should see all of the messages or only SRE-P specific ones
  -C, --cluster-id string   Internal Cluster identifier (required)
  -h, --help                help for list
  -i, --internal            Toggle if we should see internal messages
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

* [osdctl servicelog](osdctl_servicelog.md)	 - OCM/Hive Service log

