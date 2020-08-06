## osdctl cost

Cost Management related utilities

### Synopsis

The cost command allows for cost management on the AWS platform (other 
platforms may be added in the future)

### Options

```
  -a, --aws-access-key-id string       AWS Access Key ID
  -c, --aws-config string              specify AWS config file path
  -p, --aws-profile string             specify AWS profile
  -g, --aws-region string              specify AWS region (default "us-east-1")
  -x, --aws-secret-access-key string   AWS Secret Access Key
  -h, --help                           help for cost
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

* [osdctl](osdctl.md)	 - OSD CLI
* [osdctl cost create](osdctl_cost_create.md)	 - Create a cost category for the given OU
* [osdctl cost get](osdctl_cost_get.md)	 - Get total cost of a given OU. If no OU given, then gets total cost of v4 OU.
* [osdctl cost list](osdctl_cost_list.md)	 - List the cost of each OU under given OU
* [osdctl cost reconcile](osdctl_cost_reconcile.md)	 - Checks if there's a cost category for every OU. If an OU is missing a cost category, creates the cost category

