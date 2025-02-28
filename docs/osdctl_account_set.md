## osdctl account set

Set AWS Account CR status

```
osdctl account set <account name> [flags]
```

### Options

```
  -a, --account-namespace string   The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
  -h, --help                       help for set
  -p, --patch string               the raw payload used to patch the account status
  -r, --rotate-credentials         set status.rotateCredentials in the specified account
      --state string               set status.state field in the specified account
  -t, --type string                The type of patch being provided; one of [merge json]. The strategic patch is not supported. (default "merge")
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

* [osdctl account](osdctl_account.md)	 - AWS Account related utilities

