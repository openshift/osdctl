## osdctl hcp backup discover

Discover HCP backups from the disaster recovery S3 bucket

```
osdctl hcp backup discover --cluster-id <cluster-id> [flags]
```

### Examples

```
  osdctl hcp backup discover --cluster-id ${CLUSTER_ID}
  osdctl hcp backup discover --cluster-id ${CLUSTER_ID} --limit 5
  osdctl hcp backup discover --cluster-id ${CLUSTER_ID} --limit -1
  osdctl hcp backup discover --cluster-id ${CLUSTER_ID} --profile ${AWS_PROFILE}
```

### Options

```
  -C, --cluster-id string   Internal ID, name, or external ID of the HCP cluster
  -h, --help                help for discover
  -l, --limit int           Number of most recent backups to display (-1 displays all) (default 1)
  -p, --profile string      AWS profile used to assume the DR account role
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

* [osdctl hcp backup](osdctl_hcp_backup.md)	 - Trigger a Velero backup for an HCP cluster

