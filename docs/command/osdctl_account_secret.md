## osdctl account secret

secret <command>

### Synopsis

secret <command>

```
osdctl account secret [flags]
```

### Options

```
  -h, --help   help for secret
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

* [osdctl account](osdctl_account.md)	 - AWS Account related utilities
* [osdctl account secret check](osdctl_account_secret_check.md)	 - Check AWS Account CR IAM User credentials
* [osdctl account secret rotate](osdctl_account_secret_rotate.md)	 - Rotate IAM credentials secret

