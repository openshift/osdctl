## osdctl

OSD CLI

### Synopsis

CLI tool to provide OSD related utilities

### Options

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for osdctl
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl aao](osdctl_aao.md)	 - AWS Account Operator Debugging Utilities
* [osdctl account](osdctl_account.md)	 - AWS Account related utilities
* [osdctl alert](osdctl_alert.md)	 - List alerts
* [osdctl cloudtrail](osdctl_cloudtrail.md)	 - AWS CloudTrail related utilities
* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster
* [osdctl cost](osdctl_cost.md)	 - Cost Management related utilities
* [osdctl dynatrace](osdctl_dynatrace.md)	 - Dynatrace related utilities
* [osdctl env](osdctl_env.md)	 - Create an environment to interact with a cluster
* [osdctl hcp](osdctl_hcp.md)	 - 
* [osdctl hive](osdctl_hive.md)	 - hive related utilities
* [osdctl iampermissions](osdctl_iampermissions.md)	 - STS/WIF utilities
* [osdctl jira](osdctl_jira.md)	 - Provides a set of commands for interacting with Jira
* [osdctl jumphost](osdctl_jumphost.md)	 - 
* [osdctl mc](osdctl_mc.md)	 - 
* [osdctl mcp](osdctl_mcp.md)	 - Start osdctl in MCP server mode
* [osdctl network](osdctl_network.md)	 - network related utilities
* [osdctl org](osdctl_org.md)	 - Provides information for a specified organization
* [osdctl promote](osdctl_promote.md)	 - Utilities to promote services/operators
* [osdctl servicelog](osdctl_servicelog.md)	 - OCM/Hive Service log
* [osdctl setup](osdctl_setup.md)	 - Setup the configuration
* [osdctl swarm](osdctl_swarm.md)	 - Provides a set of commands for swarming activity
* [osdctl upgrade](osdctl_upgrade.md)	 - Upgrade osdctl
* [osdctl version](osdctl_version.md)	 - Display the version

