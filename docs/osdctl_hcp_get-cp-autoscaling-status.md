## osdctl hcp get-cp-autoscaling-status

Get control plane autoscaling status for hosted clusters on a management cluster

### Synopsis

Query a single HCP management cluster to retrieve autoscaling status for all hosted clusters.

This command is useful for checking the autoscaling configuration status of hosted clusters
on a specific management cluster during day-to-day operations.

```
osdctl hcp get-cp-autoscaling-status [flags]
```

### Examples

```

  # Get autoscaling status for all hosted clusters on a management cluster
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID}

  # Get status with CSV output
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID} --output csv > status.csv

  # Show only clusters ready for migration
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID} --show-only ready-for-migration

  # Show only clusters that need annotation removal
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID} --show-only needs-removal

  # Show only clusters safe to remove override
  osdctl hcp get-cp-autoscaling-status --mgmt-cluster-id ${MGMT_CLUSTER_ID} --show-only safe-to-remove-override
```

### Options

```
  -h, --help                     help for get-cp-autoscaling-status
      --mgmt-cluster-id string   Management cluster ID or name (required)
      --no-headers               Skip table headers in output
      --output string            Output format: text, json, yaml, csv (default "text")
      --show-only string         Filter output: needs-removal, ready-for-migration, safe-to-remove-override
```

### Options inherited from parent commands

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl hcp](osdctl_hcp.md)	 - 

