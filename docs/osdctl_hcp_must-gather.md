## osdctl hcp must-gather

Create a must-gather for HCP cluster

### Synopsis

Create a must-gather for an HCP cluster with optional gather targets

```
osdctl hcp must-gather --cluster-id <cluster-identifier> [flags]
```

### Examples

```
osdctl hcp must-gather --cluster-id CLUSTER_ID --gather sc,mc,sc_acm --reason OHSS-1234
```

### Options

```
      --acm_image string    Overrides the acm must-gather image being used for acm mc, sc as well as hcp must-gathers. (default "quay.io/stolostron/must-gather:2.11.4-SNAPSHOT-2024-12-02-15-19-44")
      --cluster-id string   Internal ID of the cluster to gather data from
      --gather string       Comma-separated list of gather targets (available: sc, sc_acm, mc, hcp). (default "hcp")
  -h, --help                help for must-gather
      --reason string       The reason for this command, which requires elevation (e.g., OHSS ticket or PD incident).
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

* [osdctl hcp](osdctl_hcp.md)	 - 

