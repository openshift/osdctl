## osdctl cluster resize control-plane

Resize an OSD/ROSA cluster's control plane nodes

### Synopsis

Resize an OSD/ROSA cluster's control plane nodes

  Requires previous login to the api server via "ocm backplane login".
  The user will be prompted to send a service log after initiating the resize. The resize process runs asynchronously,
  and this command exits immediately after sending the service log. Any issues with the resize will be reported via PagerDuty.

```
osdctl cluster resize control-plane [flags]
```

### Examples

```

  # Resize all control plane instances to m5.4xlarge using control plane machine sets
  osdctl cluster resize control-plane -c "${CLUSTER_ID}" --machine-type m5.4xlarge --reason "${OHSS}"
```

### Options

```
  -C, --cluster-id string     The internal ID of the cluster to perform actions on
  -h, --help                  help for control-plane
      --machine-type string   The target AWS machine type to resize to (e.g. m5.2xlarge)
      --reason string         The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)
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

* [osdctl cluster resize](osdctl_cluster_resize.md)	 - resize control-plane/infra nodes

