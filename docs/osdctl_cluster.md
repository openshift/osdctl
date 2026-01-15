## osdctl cluster

Provides information for a specified cluster

### Options

```
  -h, --help   help for cluster
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

* [osdctl](osdctl.md)	 - OSD CLI
* [osdctl cluster break-glass](osdctl_cluster_break-glass.md)	 - Emergency access to a cluster
* [osdctl cluster check-banned-user](osdctl_cluster_check-banned-user.md)	 - Checks if the cluster owner is a banned user.
* [osdctl cluster context](osdctl_cluster_context.md)	 - Shows the context of a specified cluster
* [osdctl cluster cpd](osdctl_cluster_cpd.md)	 - Runs diagnostic for a Cluster Provisioning Delay (CPD)
* [osdctl cluster detach-stuck-volume](osdctl_cluster_detach-stuck-volume.md)	 - Detach openshift-monitoring namespace's volume from a cluster forcefully
* [osdctl cluster etcd-health-check](osdctl_cluster_etcd-health-check.md)	 - Checks the etcd components and member health
* [osdctl cluster etcd-member-replace](osdctl_cluster_etcd-member-replace.md)	 - Replaces an unhealthy etcd node
* [osdctl cluster from-infra-id](osdctl_cluster_from-infra-id.md)	 - Get cluster ID and external ID from a given infrastructure ID commonly used by Splunk
* [osdctl cluster get-env-vars](osdctl_cluster_get-env-vars.md)	 - Print a cluster's ID/management namespaces, optionally as env variables
* [osdctl cluster health](osdctl_cluster_health.md)	 - Describes health of cluster nodes and provides other cluster vitals.
* [osdctl cluster hypershift-info](osdctl_cluster_hypershift-info.md)	 - Pull information about AWS objects from the cluster, the management cluster and the privatelink cluster
* [osdctl cluster logging-check](osdctl_cluster_logging-check.md)	 - Shows the logging support status of a specified cluster
* [osdctl cluster orgId](osdctl_cluster_orgId.md)	 - Get the OCM org ID for a given cluster
* [osdctl cluster owner](osdctl_cluster_owner.md)	 - List the clusters owned by the user (can be specified to any user, not only yourself)
* [osdctl cluster reports](osdctl_cluster_reports.md)	 - Manage cluster reports in backplane-api
* [osdctl cluster resize](osdctl_cluster_resize.md)	 - resize control-plane/infra nodes
* [osdctl cluster resync](osdctl_cluster_resync.md)	 - Force a resync of a cluster from Hive
* [osdctl cluster sre-operators](osdctl_cluster_sre-operators.md)	 - SRE operator related utilities
* [osdctl cluster ssh](osdctl_cluster_ssh.md)	 - utilities for accessing cluster via ssh
* [osdctl cluster support](osdctl_cluster_support.md)	 - Cluster Support
* [osdctl cluster transfer-owner](osdctl_cluster_transfer-owner.md)	 - Transfer cluster ownership to a new user (to be done by Region Lead)
* [osdctl cluster validate-pull-secret](osdctl_cluster_validate-pull-secret.md)	 - Checks if the pull secret email matches the owner email
* [osdctl cluster validate-pull-secret-ext](osdctl_cluster_validate-pull-secret-ext.md)	 - Extended checks to confirm pull-secret data is synced with current OCM data
* [osdctl cluster verify-dns](osdctl_cluster_verify-dns.md)	 - Verify DNS resolution for HCP cluster public endpoints

