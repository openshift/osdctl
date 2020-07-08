## osdctl account list account-claim

List AWS Account Claim CR

### Synopsis

List AWS Account Claim CR

```
osdctl account list account-claim [flags]
```

### Options

```
  -h, --help           help for account-claim
      --state string   Account cr state. If not specified, it will list all crs by default.
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

* [osdctl account list](osdctl_account_list.md)	 - List resources

