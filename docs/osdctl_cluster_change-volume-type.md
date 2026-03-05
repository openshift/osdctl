## osdctl cluster change-volume-type

Change EBS volume type (e.g., io1 to gp3) for cluster volumes

### Synopsis

Change the type of an EBS volume attached to a cluster.

Common use cases:
  - Migrate master node volumes from io1 to gp3 for cost optimization
  - Change volume performance characteristics without data loss

IMPORTANT:
  - This operation is performed online (no downtime required)
  - AWS will migrate the volume in the background
  - The node and cluster remain operational during the change
  - Requires backplane elevation (--reason flag)
  - IOPS and throughput are only set if explicitly provided

```
osdctl cluster change-volume-type --cluster-id <cluster-id> --volume-id <volume-id> --target-type <type> [flags]
```

### Examples

```
  # Migrate a master volume from io1 to gp3 with custom IOPS
  osdctl cluster change-volume-type \
    --cluster-id 2abc123def456 \
    --volume-id vol-1234567890abcdef0 \
    --target-type gp3 \
    --iops 3000 \
    --throughput 125 \
    --reason "SREP-3811 - Master volume cost optimization"

  # Change to gp3 with default performance (AWS defaults)
  osdctl cluster change-volume-type \
    --cluster-id 2abc123def456 \
    --volume-id vol-1234567890abcdef0 \
    --target-type gp3 \
    --reason "SREP-3811"

  # Dry run to preview changes
  osdctl cluster change-volume-type \
    --cluster-id 2abc123def456 \
    --volume-id vol-1234567890abcdef0 \
    --target-type gp3 \
    --reason "SREP-3811" \
    --dry-run
```

### Options

```
  -C, --cluster-id string    Cluster ID (internal ID or external ID)
      --dry-run              Dry run - show what would be changed without executing
  -h, --help                 help for change-volume-type
      --iops int32           Provisioned IOPS (for io1, io2, gp3). If not specified, keeps current value or uses AWS defaults.
      --reason string        Reason for elevation (OHSS/PD/JIRA ticket)
  -t, --target-type string   Target volume type (gp2, gp3, io1, io2, st1, sc1)
      --throughput int32     Throughput in MB/s (for gp3 only). If not specified, uses AWS defaults.
  -v, --volume-id string     EBS volume ID (e.g., vol-1234567890abcdef0)
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

