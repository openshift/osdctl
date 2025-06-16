## osdctl org context

fetches information about the given organization

### Synopsis

Fetches information about the given organization. This data is presented as a table where each row includes the name, version, ID, cloud provider, and plan for the cluster. Rows will also include the number of recent service logs, active PD Alerts, Jira Issues, and limited support status for that specific cluster.

```
osdctl org context orgId [flags]
```

### Examples

```
# Get context data for a cluster
osdctl org context 1a2B3c4DefghIjkLMNOpQrSTUV5

# Get context data in JSON format
osdctl org context 1a2B3c4DefghIjkLMNOpQrSTUV5 -o json
```

### Options

```
  -h, --help            help for context
  -o, --output string   output format for the results. only supported value currently is 'json'
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

* [osdctl org](osdctl_org.md)	 - Provides information for a specified organization

