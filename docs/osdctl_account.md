## osdctl account

AWS Account related utilities

### Options

```
  -h, --help   help for account
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
* [osdctl account clean-velero-snapshots](osdctl_account_clean-velero-snapshots.md)	 - Cleans up S3 buckets whose name start with managed-velero
* [osdctl account cli](osdctl_account_cli.md)	 - Generate temporary AWS CLI credentials on demand
* [osdctl account console](osdctl_account_console.md)	 - Generate an AWS console URL on the fly
* [osdctl account generate-secret](osdctl_account_generate-secret.md)	 - Generates IAM credentials secret
* [osdctl account get](osdctl_account_get.md)	 - Get resources
* [osdctl account list](osdctl_account_list.md)	 - List resources
* [osdctl account mgmt](osdctl_account_mgmt.md)	 - AWS Account Management
* [osdctl account reset](osdctl_account_reset.md)	 - Reset AWS Account CR
* [osdctl account rotate-secret](osdctl_account_rotate-secret.md)	 - Rotate IAM credentials secret
* [osdctl account servicequotas](osdctl_account_servicequotas.md)	 - Interact with AWS service-quotas
* [osdctl account set](osdctl_account_set.md)	 - Set AWS Account CR status
* [osdctl account verify-secrets](osdctl_account_verify-secrets.md)	 - Verify AWS Account CR IAM User credentials

