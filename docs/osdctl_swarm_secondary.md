## osdctl swarm secondary

List unassigned JIRA issues based on criteria

### Synopsis

Lists unassigned Jira issues from the 'OHSS' project
		for the following Products
		- OpenShift Dedicated
		- Openshift Online Pro
		- OpenShift Online Starter
		- Red Hat OpenShift Service on AWS
		- HyperShift Preview
		- Empty 'Products' field in Jira
		with the 'Summary' field  of the new ticket not matching the following
		- Compliance Alert
		and the 'Work Type' is not one of the RFE or Change Request 

```
osdctl swarm secondary [flags]
```

### Examples

```
#Collect tickets for secondary swarm
		osdctl swarm secondary
```

### Options

```
  -h, --help   help for secondary
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

* [osdctl swarm](osdctl_swarm.md)	 - Provides a set of commands for swarming activity

