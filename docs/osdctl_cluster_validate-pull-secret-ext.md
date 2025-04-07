## osdctl cluster validate-pull-secret-ext

Extended checks to confirm pull-secret data is synced with current OCM data

### Synopsis


	Attempts to validate if a cluster's pull-secret auth values are in sync with the account's email, 
	registry_credential, and access token data stored in OCM.  
	If this is being executed against a cluster which is not owned by the current OCM account, 
	Region Lead permissions are required to view and validate the OCM AccessToken. 


```
osdctl cluster validate-pull-secret-ext [CLUSTER_ID] [flags]
```

### Examples

```

	# Compare OCM Access-Token, OCM Registry-Credentials, and OCM Account Email against cluster's pull secret
	osdctl cluster validate-pull-secret-ext ${CLUSTER_ID} --reason "OSD-XYZ"

	# Exclude Access-Token, and Registry-Credential checks...
	osdctl cluster validate-pull-secret-ext ${CLUSTER_ID} --reason "OSD-XYZ" --skip-access-token --skip-registry-creds

```

### Options

```
  -h, --help                  help for validate-pull-secret-ext
  -l, --log-level string      debug, info, warn, error. (default=info) (default "info")
      --reason string         Mandatory reason for this command to be run (usually includes an OHSS or PD ticket)
      --skip-access-token     Exclude OCM AccessToken checks against cluster secret
      --skip-registry-creds   Exclude OCM Registry Credentials checks against cluster secret
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

