## osdctl hcp restore migrate-storage

Migrate HCP snapshot storage to a replacement management cluster

### Synopsis

Discovers EBS VolumeSnapshotContents for the HCP on the replacement management cluster, stores the relevant inventory and snapshot mappings in a restartable journal, copies snapshots through a temporary source-account KMS key, replaces the target VSCs with the copied snapshot IDs, and resumes the control plane only after all replacement VSCs are ready to use.

```
osdctl hcp restore migrate-storage --cluster-id <cluster-id> --replacement-mc <management-cluster> --reason <reason> [flags]
```

### Examples

```
  osdctl hcp restore migrate-storage --cluster-id ${CLUSTER_ID} --replacement-mc ${REPLACEMENT_MC} --reason ${REASON}
```

### Options

```
  -C, --cluster-id string         Internal ID, name, or external ID of the HCP cluster
      --concurrency int           Maximum number of snapshot copy chains to run concurrently (default 4)
  -h, --help                      help for migrate-storage
  -p, --profile string            AWS profile used to assume management-cluster account roles
      --reason string             Reason for elevated access to the replacement management cluster
      --replacement-mc string     Internal ID, name, or external ID of the replacement management cluster
      --snapshot-journal string   Path for the restartable snapshot journal (defaults to a cluster-specific file; existing journals prompt to resume or create a numbered file)
      --source-mc string          Internal ID, name, or external ID of the source management cluster (defaults to the HCP's current management cluster)
      --timeout duration          Maximum time to wait for all snapshot copies (default 2h0m0s)
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

