## osdctl cluster imdsv2

Migrate cluster nodes to enforce IMDSv2 (Instance Metadata Service v2)

### Synopsis

Migrate ROSA Classic cluster nodes to enforce IMDSv2.

This automates the SOP for migrating machines to IMDSv2 by:
- Patching Hive MachinePools to require IMDSv2
- Replacing infra nodes (one at a time)
- Updating ControlPlaneMachineSet for automatic master node rollout
- Validating all nodes/machines are using IMDSv2

Pre-flight checks verify cluster health before making changes.

```
osdctl cluster imdsv2 [flags]
```

### Examples

```
  # Migrate all nodes (infra + masters)
  osdctl cluster imdsv2 -C ${CLUSTER_ID} --reason "JIRA-12345"

  # Migrate only infra nodes
  osdctl cluster imdsv2 -C ${CLUSTER_ID} --reason "CASE-67890" --nodes infra

  # Migrate only master nodes
  osdctl cluster imdsv2 -C ${CLUSTER_ID} --reason "JIRA-12345" --nodes master
```

### Options

```
  -C, --cluster-id string   The internal/external ID of the cluster
  -h, --help                help for imdsv2
      --nodes string        Node roles to migrate: all, master, infra, workers (default "all")
      --reason string       Reason for elevation (OHSS/PD/JIRA ticket)
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

* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster

