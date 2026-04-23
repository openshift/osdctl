## osdctl cluster cad run

Run a manual investigation on the CAD cluster

### Synopsis

Run a manual investigation on the Configuration Anomaly Detection (CAD) cluster.

This command schedules a Tekton PipelineRun on the appropriate CAD cluster (stage or production)
to run an investigation against a target cluster. The results will be written to a backplane report.

Prerequisites:
  - Connected to the target cluster's OCM environment (production or stage)
  - The CAD clusters themselves are always in production OCM

Available Investigations:
  chgm, cmbb, can-not-retrieve-updates, ai, cpd, etcd-quota-low,
  insightsoperatordown, machine-health-check, must-gather, upgrade-config,
  restart-controlplane, describe-nodes

Examples:
```bash
# Run a change management investigation on a production cluster
osdctl cluster cad run \
  --cluster-id 1a2b3c4d5e6f7g8h9i0j \
  --investigation chgm \
  --environment production \
  --reason "OHSS-12345"

# Run a dry-run investigation (does not create a report)
osdctl cluster cad run \
  --cluster-id 1a2b3c4d5e6f7g8h9i0j \
  --investigation chgm \
  --environment production \
  --reason "OHSS-12345" \
  --dry-run

# Run describe-nodes on master nodes only
osdctl cluster cad run \
  --cluster-id 1a2b3c4d5e6f7g8h9i0j \
  --investigation describe-nodes \
  --environment production \
  --reason "OHSS-12345" \
  --params MASTER=true
```

Note:
  After the investigation completes (may take several minutes), view results using:
```bash
osdctl cluster reports list -C <cluster-id> -l 1
```

  You must be connected to the target cluster's OCM environment to view its reports.

```
osdctl cluster cad run [flags]
```

### Options

```
  -C, --cluster-id string      Cluster ID (internal or external)
  -d, --dry-run                Dry-Run: Run the investigation with the dry-run flag. This will not create a report.
  -e, --environment string     Environment in which the target cluster runs. Allowed values: "stage" or "production"
  -h, --help                   help for run
  -i, --investigation string   Investigation name
  -p, --params stringArray     Investigation-specific parameters as KEY=VALUE (can be specified multiple times)
      --reason string          Provide a reason for running a manual investigation, used for backplane. Eg: 'OHSS-XXXX', or '#ITN-2024-XXXXX.
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

* [osdctl cluster cad](osdctl_cluster_cad.md)	 - Provides commands to run CAD tasks

