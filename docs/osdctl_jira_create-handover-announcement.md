## osdctl jira create-handover-announcement

Create a new Handover announcement for SREPHOA Project

### Synopsis


Create a new Handover announcement for SREPHOA Project. To fill the fields, use the following instructions:

1. Cluster ID
	- If a specific cluster is affected, enter the ID (internal or external).
	- If not applicable: enter None, N/A, or All for fleet-wide impact.

2. Customer Name 	
	- Use the exact name from the output of the command you ran. Do not modify or abbreviate. Copy-paste exactly.
	- If not applicable: enter None or N/A
	Note : To find the Customer Name you can get it from ocm describe cluster | grep -i organization or run the following command ocm get $(ocm get $(ocm get cluster $CLUSTER_ID  | jq -r .subscription.href) | jq -r '.creator.href') |  jq -r '.organization.name'

3. Version
Use the Openshift version number in the format:
	- 4.16 if it affects entire Y-stream versions
	- 4.16.5 if it affects a specific version

4. Product Type
Select the appropriate product type:
	- Choose Multiple if it affects the fleet
	- Otherwise, select the specific product involved

5. Description - Add a brief description of the announcement.

```
osdctl jira create-handover-announcement [flags]
```

### Options

```
      --cluster string       Cluster ID
      --customer string      Customer name
      --description string   Enter Description for the Announcment
  -h, --help                 help for create-handover-announcement
      --products string      Comma-separated list of products (e.g. 'Product A,Product B')
      --summary string       Enter Summary/Title for the Announcment
      --version string       Affected Openshift Version (e.g 4.16 or 4.15.32)
```

### Options inherited from parent commands

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
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

* [osdctl jira](osdctl_jira.md)	 - Provides a set of commands for interacting with Jira

