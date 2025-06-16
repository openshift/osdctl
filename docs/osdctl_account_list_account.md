## osdctl account list account

List AWS Account CR

```
osdctl account list account [flags]
```

### Options

```
      --account-namespace string   The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
  -c, --claim string               Filter account CRs by claimed or not. Supported values are true, false. Otherwise it lists all accounts
  -h, --help                       help for account
  -r, --reuse string               Filter account CRs by reused or not. Supported values are true, false. Otherwise it lists all accounts
      --show-managed-fields        If true, keep the managedFields when printing objects in JSON or YAML format.
      --state string               Account cr state. The default value is all to display all the crs (default "all")
      --template string            Template string or path to template file to use when --output=jsonpath, --output=jsonpath-file.
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

* [osdctl account list](osdctl_account_list.md)	 - List resources

