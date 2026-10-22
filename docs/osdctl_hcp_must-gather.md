## osdctl hcp must-gather

Create a must-gather for an HCP cluster (and optionally its management/service clusters)

### Synopsis

Create a must-gather for a HyperShift / ROSA HCP cluster.

--cluster-id is the cluster you want data about; --gather selects which cluster(s) are
actually collected from. The supplied ID and the gather targets are distinct: the default
target 'hcp' requires an HCP cluster ID, while 'mc'/'sc'/'sc_acm' can also be run directly
against a management- or service-cluster ID.

Gather targets:
  hcp     HCP control-plane logs & events from RHOBS, plus an ACM hypershift dump (requires an HCP ID)
  mc      management-cluster infrastructure must-gather
  sc      service-cluster infrastructure must-gather
  sc_acm  service-cluster ACM must-gather

Supported workflows:
  HCP cluster ID:         --gather hcp (default) | --gather mc | --gather sc,sc_acm | any combination
  Management cluster ID:  --gather mc
  Service cluster ID:     --gather sc,sc_acm

The 'hcp' target reads control-plane logs from RHOBS and therefore requires RHOBS client
credentials (RHOBS_CLIENT_ID / RHOBS_CLIENT_SECRET as env vars, in ~/.config/osdctl, or via Vault).

```
osdctl hcp must-gather --cluster-id <cluster-identifier> [flags]
```

### Examples

```
  # HCP control-plane logs/events + hypershift dump (default)
  osdctl hcp must-gather --cluster-id ${HCP_CLUSTER_ID} --reason ${REASON}

  # HCP cluster plus its management and service clusters
  osdctl hcp must-gather --cluster-id ${HCP_CLUSTER_ID} --gather hcp,mc,sc,sc_acm --reason ${REASON}

  # Directly against a management-cluster ID
  osdctl hcp must-gather --cluster-id ${MC_CLUSTER_ID} --gather mc --reason ${REASON}
```

### Options

```
      --acm_image string      Overrides the acm must-gather image being used for acm mc, sc as well as hcp must-gathers. (default "quay.io/redhat-user-workloads/crt-redhat-acm-tenant/must-gather-acm-51:203e21767f29c24069a92565638befed9fc7cc41")
  -C, --cluster-id string     Internal ID of the cluster to gather data from
      --gather string         Comma-separated list of gather targets (available: hcp, mc, sc, sc_acm). (default "hcp")
  -h, --help                  help for must-gather
      --hive-ocm-url string   OCM environment for the RHOBS/hive lookups used by the hcp target - aliases: "production", "staging", "integration". (default "production")
      --max-log-lines int     Maximum number of most-recent log lines to collect per namespace from RHOBS for the hcp target; a positive cap is written newest-first (0 = unlimited, oldest-first). (default 100000)
      --output-dir string     Directory to write the must-gather into (defaults to the current working directory). Avoid RAM-backed paths like /tmp (tmpfs) for large gathers.
      --reason string         The reason for this command, which requires elevation (e.g., OHSS ticket or PD incident).
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

