## osdctl cluster snapshot

Capture a point-in-time snapshot of cluster state

### Synopsis

Capture a point-in-time snapshot of cluster state for evidence collection.

This command captures the current state of key cluster resources including:
- Namespace states
- Node conditions and readiness  
- ClusterOperator status
- Custom resources (optional)

The snapshot can be saved to a YAML file and later compared using 
'osdctl cluster diff' to identify changes during feature testing.

```
osdctl cluster snapshot [flags]
```

### Examples

```
  # Capture cluster snapshot to a file
  osdctl cluster snapshot -C <cluster-id> -o before.yaml

  # Capture snapshot with specific namespaces
  osdctl cluster snapshot -C <cluster-id> -o snapshot.yaml --namespaces openshift-monitoring,openshift-operators

  # Capture additional resource types
  osdctl cluster snapshot -C <cluster-id> -o snapshot.yaml --resources pods,deployments,services
```

### Options

```
  -C, --cluster-id string    Cluster ID (internal, external, or name)
  -h, --help                 help for snapshot
      --namespaces strings   Specific namespaces to include (default: all openshift-* namespaces)
  -o, --output string        Output file path (YAML format)
      --resources strings    Additional resource types to capture (e.g., pods,deployments)
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

* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster

