## osdctl hcp transition-to-eus

Transition ROSA HCP clusters from stable to EUS channel (Even Y-Stream EOL handling)

### Synopsis

Transition ROSA HCP clusters from stable to EUS channel during End-of-Life handling.

⚠️ IMPORTANT GUARDRAILS ⚠️
This command is specifically designed for EVEN Y-STREAM end-of-life transitions (4.14, 4.16, 4.18, etc.).

The command validates:
- Cluster must be HCP (not Classic)
- Cluster must be on an even y-stream (4.14, 4.16, 4.18, etc.)
- Cluster must be on 'stable' channel (not already on 'eus')
- Cluster must be in 'ready' state

WORKFLOW:
For clusters with recurring update policies:
1. Saves the existing recurring update policy settings
2. Deletes the recurring update policy
3. Transitions the channel from 'stable' to 'eus'
4. Verifies the channel change
5. Restores the recurring update policy with original settings
6. Prompts to send service log notification

For clusters with individual updates:
1. Transitions the channel from 'stable' to 'eus'
2. Verifies the channel change
3. Prompts to send service log notification

SERVICE LOG BEHAVIOR:
- After each successful transition, you will be prompted to optionally send a service log notification
- On failures where recurring policy was modified and restored: You will be prompted to send an 'attempted' notification to the customer

This approach extends the support lifecycle for clusters on even y-streams without forcing upgrades.

```
osdctl hcp transition-to-eus [flags]
```

### Examples

```
  # Transition single cluster (will prompt to send service log after success)
  osdctl hcp transition-to-eus -C cluster123

  # Multiple clusters from file
  osdctl hcp transition-to-eus --clusters-file clusters.json

  # Dry-run to preview changes
  osdctl hcp transition-to-eus --clusters-file clusters.json --dry-run

```

### Options

```
  -C, --cluster-id string      ID of the target HCP cluster
  -c, --clusters-file string   JSON file containing cluster IDs (format: {"clusters":["$CLUSTERID1", "$CLUSTERID2"]})
      --dry-run                Simulate the transition without making any changes
  -h, --help                   help for transition-to-eus
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

