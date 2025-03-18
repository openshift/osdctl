## osdctl cluster resize infra

Resize an OSD/ROSA cluster's infra nodes

### Synopsis

Resize an OSD/ROSA cluster's infra nodes

  This command automates most of the "machinepool dance" to safely resize infra nodes for production classic OSD/ROSA 
  clusters. This DOES NOT work in non-production due to environmental differences.

  Remember to follow the SOP for preparation and follow up steps:

    https://github.com/openshift/ops-sop/blob/master/v4/howto/resize-infras-workers.md


```
osdctl cluster resize infra [flags]
```

### Examples

```

  # Automatically vertically scale infra nodes to the next size
  osdctl cluster resize infra --cluster-id ${CLUSTER_ID}

  # Resize infra nodes to a specific instance type
  osdctl cluster resize infra --cluster-id ${CLUSTER_ID} --instance-type "r5.xlarge"

```

### Options

```
  -C, --cluster-id string      OCM internal/external cluster id or cluster name to resize infra nodes for.
  -h, --help                   help for infra
      --instance-type string   (optional) Override for an AWS or GCP instance type to resize the infra nodes to, by default supported instance types are automatically selected.
      --justification string   The justification behind resize
      --ohss string            OHSS ticket tracking this infra node resize
      --reason string          The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)
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

