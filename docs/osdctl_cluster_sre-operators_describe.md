## osdctl cluster sre-operators describe

Describe SRE operators

### Synopsis


  Helps obtain various health information about a specified SRE operator within a cluster,
  including CSV, Subscription, OperatorGroup, Deployment, and Pod health statuses.

  A git_access token is required to fetch the latest version of the operators, and can be 
  set within the config file using the 'osdctl setup' command.

  The command creates a Kubernetes client to access the current cluster context, and GitLab/GitHub
  clients to fetch the latest versions of each operator from its respective repository.
	

```
osdctl cluster sre-operators describe [flags]
```

### Examples

```

		# Describe SRE operators
		$ osdctl cluster sre-operators describe <operator-name>
	
```

### Options

```
  -h, --help   help for describe
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

* [osdctl cluster sre-operators](osdctl_cluster_sre-operators.md)	 - SRE operator related utilities

