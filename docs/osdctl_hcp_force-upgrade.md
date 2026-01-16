## osdctl hcp force-upgrade

Schedule forced control plane upgrade for HCP clusters (Requires ForceUpgrader permissions)

### Synopsis

Schedule forced control plane upgrades for ROSA HCP clusters. This command skips all validation checks
(critical alerts, cluster conditions, node pool checks, and version gate agreements).

⚠️ REQUIRES ForceUpgrader PERMISSIONS ⚠️

This command can target clusters in two ways:
- Single cluster: --cluster-id <ID>
- Multiple clusters from file: --clusters-file <file.json>

UPGRADE BEHAVIOR:
The command explicitly upgrades clusters to the LATEST Z-STREAM version of the specified Y-stream.
This serves two purposes:
1. Force upgrades to latest z-stream of the SAME y-stream for critical bug fixes
2. Force upgrades to latest z-stream of a SUBSEQUENT y-stream when current y-stream goes out of support

Example: --target-y 4.15 will upgrade to the latest available 4.15.z version (e.g., 4.15.32).

```
osdctl hcp force-upgrade [flags]
```

### Examples

```
  # Force upgrade without service log
  osdctl hcp force-upgrade -C cluster123 --target-y 4.15

  # Force upgrade with end-of-support service log
  osdctl hcp force-upgrade -C cluster123 --target-y 4.16 --send-service-log end-of-support

  # Multiple clusters from file with end-of-support service log
  osdctl hcp force-upgrade --clusters-file clusters.json --target-y 4.16 --send-service-log end-of-support

  # Force upgrade with custom service log template file
  osdctl hcp force-upgrade -C cluster123 --target-y 4.15 --send-service-log /path/to/custom-template.json


```

### Options

```
  -C, --cluster-id string         ID of the target HCP cluster
  -c, --clusters-file string      JSON file containing cluster IDs (format: {"clusters":["$CLUSTERID1", "$CLUSTERID2"]})
      --dry-run                   Simulate the upgrade without making any changes
  -h, --help                      help for force-upgrade
      --next-run-minutes int      Offset in minutes for scheduling upgrade (minimum 6 for the scheduling to take place) (default 10)
      --send-service-log string   Send service log notification after scheduling upgrade. Specify template name (e.g., 'end-of-support') or file path (e.g., '/path/to/template.json')
      --target-y string           Target Y-stream version (e.g., 4.15) - will upgrade to the LATEST Z-stream of this Y-stream
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

