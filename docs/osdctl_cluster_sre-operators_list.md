## osdctl cluster sre-operators list

List the current and latest version of SRE operators

### Synopsis


	Lists the current version, channel, and status of SRE operators running in the current 
	cluster context, and by default fetches the latest version from the operators' repositories.
	
	A git_access token is required to fetch the latest version of the operators, and can be 
	set within the config file using the 'osdctl setup' command.
	
	The command creates a Kubernetes client to access the current cluster context, and GitLab/GitHub
	clients to fetch the latest versions of each operator from its respective repository.
	

```
osdctl cluster sre-operators list [flags]
```

### Examples

```

	# List SRE operators
	$ osdctl cluster sre-operators list
	
	# List SRE operators without fetching the latest version for faster output
	$ osdctl cluster sre-operators list --short
	
	# List only SRE operators that are running outdated versions
	$ osdctl cluster sre-operators list --outdated

	# List SRE operators without their commit shas and repositry URL
	$ osdctl cluster sre-operators list --no-commit
	
	# List a specific SRE operator
	$ osdctl cluster sre-operators list --operator='OPERATOR_NAME'
	
```

### Options

```
  -h, --help              help for list
      --no-commit         Excluse commit shas and repository URL from the output
      --no-headers        Exclude headers from the output
      --operator string   Filter to only show the specified operator.
      --outdated          Filter to only show operators running outdated versions
      --short             Exclude fetching the latest version from repositories for faster output
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

