## osdctl evidence

Evidence collection utilities for feature testing

### Synopsis

Evidence collection utilities for feature testing.

This command group provides tools to help SRE teams collect evidence
during feature validation testing. The collected evidence can include
CloudTrail logs, cluster snapshots, and other diagnostic information.

```
osdctl evidence [flags]
```

### Options

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for evidence
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl](osdctl.md)	 - OSD CLI
* [osdctl evidence collect](osdctl_evidence_collect.md)	 - Collect evidence from cluster and AWS for feature testing

