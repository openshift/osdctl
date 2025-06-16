## osdctl jira create-handover-announcement

Create a new Handover announcement for SREPHOA Project

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
      --version string       Affects version
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

