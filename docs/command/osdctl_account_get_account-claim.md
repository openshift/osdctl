## osdctl account get account-claim

Get AWS Account Claim CR

### Synopsis

Get AWS Account Claim CR

```
osdctl account get account-claim [flags]
```

### Options

```
  -i, --account-id string          AWS account ID
      --account-namespace string   The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
  -h, --help                       help for account-claim
  -o, --output string              Output format. One of: json|yaml|jsonpath=...|jsonpath-file=... see jsonpath template [http://kubernetes.io/docs/user-guide/jsonpath].
      --template string            Template string or path to template file to use when --output=jsonpath, --output=jsonpath-file.
```

### Options inherited from parent commands

```
      --cluster string             The name of the kubeconfig cluster to use
      --context string             The name of the kubeconfig context to use
      --insecure-skip-tls-verify   If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string          Path to the kubeconfig file to use for CLI requests.
      --request-timeout string     The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string              The address and port of the Kubernetes API server
```

### SEE ALSO

* [osdctl account get](osdctl_account_get.md)	 - get resources

