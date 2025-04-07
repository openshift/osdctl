## osdctl cluster resync

Force a resync of a cluster from Hive

### Synopsis

Force a resync of a cluster from Hive

  Normally, clusters are periodically synced by Hive every two hours at minimum. This command deletes a cluster's
  clustersync from its managing Hive cluster, causing the clustersync to be recreated in most circumstances and forcing
  a resync of all SyncSets and SelectorSyncSets. The command will also wait for the clustersync to report its status
  again (Success or Failure) before exiting.


```
osdctl cluster resync [flags]
```

### Examples

```

  # Force a cluster resync by deleting its clustersync CustomResource
  osdctl cluster resync --cluster-id ${CLUSTER_ID}

```

### Options

```
  -C, --cluster-id string   OCM internal/external cluster id or cluster name to delete the clustersync for.
  -h, --help                help for resync
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

