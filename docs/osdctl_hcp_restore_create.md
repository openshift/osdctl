## osdctl hcp restore create

Create and monitor a Velero restore for an HCP backup

### Synopsis

Create and monitor a Velero Restore in the target management cluster. The target defaults to the management cluster currently bound to the HCP; an explicit target must be in the HCP's region. Repeated invocations safely resume an active restore or report an already completed restore; failed attempts receive a new retry Restore.

```
osdctl hcp restore create --cluster-id <cluster-id> --backup-name <backup-name> --reason <reason> [--target-mc <management-cluster>] [--timeout <duration>] [flags]
```

### Examples

```
  osdctl hcp restore create --cluster-id ${CLUSTER_ID} --backup-name ${BACKUP_NAME} --reason ${REASON}
  osdctl hcp restore create --cluster-id ${CLUSTER_ID} --target-mc ${TARGET_MC} --backup-name ${BACKUP_NAME} --reason ${REASON} --timeout 30m
```

### Options

```
      --backup-name string   Name of the completed Velero backup to restore
  -C, --cluster-id string    Internal ID, name, or external ID of the HCP cluster
  -h, --help                 help for create
      --reason string        Reason for elevated access
      --target-mc string     Internal ID, name, or external ID of the management cluster that will run the restore
      --timeout duration     Maximum time to wait for the restore to complete (default 30m0s)
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

* [osdctl hcp restore](osdctl_hcp_restore.md)	 - Prepare and restore HCP clusters during disaster recovery

