## osdctl hcp backup

Trigger a Velero backup for an HCP cluster

### Synopsis

Trigger a Velero backup for an HCP cluster using the cluster's existing daily schedule.

This command:
  1. Logs into the Management Cluster for the given HCP cluster (unprivileged)
  2. Validates the Velero schedule exists in the openshift-adp namespace
  3. Logs into the Management Cluster again with elevated permissions (backplane-cluster-admin)
  4. Triggers an immediate backup from the schedule via velero backup create --from-schedule

Optional metadata can be attached to the Velero Backup CR:

  --label key=value       Add a label to the Backup CR (may be repeated)
  --annotation key=value  Add an annotation to the Backup CR (may be repeated)

This command only triggers the backup. It does not wait for completion.
To monitor the backup status after triggering, log into the management cluster
and review the Backup CR:
  ocm backplane login --manager <CLUSTER_ID>
  oc get backup <backup-id> -n openshift-adp


```
osdctl hcp backup --cluster-id <cluster-id> --reason <reason> [flags]
```

### Examples

```
  osdctl hcp backup --cluster-id 1abc2def3ghi --reason OHSS-12345
  osdctl hcp backup --cluster-id 1abc2def3ghi --reason OHSS-12345 --label env=prod --label incident=OHSS-12345
  osdctl hcp backup --cluster-id 1abc2def3ghi --reason OHSS-12345 --annotation owner=sre-team
```

### Options

```
      --annotation stringToString   Annotation to add to the Velero Backup CR (key=value); may be repeated (default [])
  -C, --cluster-id string           Internal ID, name, or external ID of the HCP cluster
  -h, --help                        help for backup
      --label stringToString        Label to add to the Velero Backup CR (key=value); may be repeated (default [])
      --reason string               Reason for privilege elevation (e.g., OHSS-1234 or PD incident ID)
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

