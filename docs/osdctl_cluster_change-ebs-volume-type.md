## osdctl cluster change-ebs-volume-type

Change EBS volume type for control plane and/or infra nodes by replacing machines

### Synopsis

Change the EBS volume type for control plane and/or infra nodes on a ROSA/OSD cluster.

This command replaces machines to change volume types (not in-place modification).
For control plane nodes, it patches the ControlPlaneMachineSet (CPMS) which automatically
rolls nodes one at a time. For infra nodes, it uses the Hive MachinePool dance to safely
replace all infra nodes with new ones using the target volume type.

Pre-flight checks are performed automatically before making changes.

```
osdctl cluster change-ebs-volume-type [flags]
```

### Examples

```
  # Change both control plane and infra volumes to gp3
  osdctl cluster change-ebs-volume-type -C ${CLUSTER_ID} --type gp3 --reason "SREP-3811"

  # Change only control plane volumes to gp3
  osdctl cluster change-ebs-volume-type -C ${CLUSTER_ID} --type gp3 --role control-plane --reason "SREP-3811"

  # Change only infra volumes to gp3
  osdctl cluster change-ebs-volume-type -C ${CLUSTER_ID} --type gp3 --role infra --reason "SREP-3811"
```

### Options

```
  -C, --cluster-id string   The internal/external ID of the cluster
  -h, --help                help for change-ebs-volume-type
      --reason string       Reason for elevation (OHSS/PD/JIRA ticket)
      --role string         Node role to change: control-plane, infra (default: both)
      --type string         Target EBS volume type (gp3)
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

