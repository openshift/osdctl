## osdctl cost

Cost Management related utilities

### Synopsis

<<<<<<< HEAD
The cost command allows for cost management on the AWS platform (other 
platforms may be added in the future)

```
osdctl cost [flags]
```
=======
The cost command allows for cost management on the AWS platform (other
platforms may be added in the future. Its functions include:

- Managing the AWS Cost Explorer with `osdctl cost aws`. This leaves the possibility of adding cost 
management support for other platforms e.g. `osdctl cost gcp`

- Get cost of OUs with `osdctl cost aws get`

- Create cost category with `osdctl cost aws create`

- Reconcile cost categories with `osdctl cost aws reconcile`
>>>>>>> completed osdctl_cost.md

### Options

```
  -h, --help   help for cost
```

### Options inherited from parent commands

```
      --cluster string             The name of the kubeconfig cluster to use
      --context string             The name of the kubeconfig context to use
<<<<<<< HEAD
      --insecure-skip-tls-verify   If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string          Path to the kubeconfig file to use for CLI requests.
      --request-timeout string     The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
=======
      --insecure-skip-tls-verify   If true, the server's certificate will not be checked for validity. This will make your HTTPS onnections insecure
      --kubeconfig string          Path to the kubeconfig file to use for CLI requests.
      --request-timeout string     The length of time to wait before giving up on a single server request. Non-zero values hould contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
>>>>>>> completed osdctl_cost.md
  -s, --server string              The address and port of the Kubernetes API server
```

### SEE ALSO

* [osdctl cost aws](osdctl_cost_aws.md)	 - A brief description of your command
