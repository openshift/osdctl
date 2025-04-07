## osdctl promote saas

Utilities to promote SaaS services/operators

```
osdctl promote saas [flags]
```

### Examples

```

		# List all SaaS services/operators
		osdctl promote saas --list

		# Promote a SaaS service/operator
		osdctl promote saas --serviceName <service-name> --gitHash <git-hash> --osd
		or
		osdctl promote saas --serviceName <service-name> --gitHash <git-hash> --hcp
```

### Options

```
      --appInterfaceDir pwd   location of app-interfache checkout. Falls back to pwd and $HOME/git/app-interface
  -g, --gitHash string        Git hash of the SaaS service/operator commit getting promoted
      --hcp                   HCP service/operator getting promoted
  -h, --help                  help for saas
  -l, --list                  List all SaaS services/operators
  -n, --namespaceRef string   SaaS target namespace reference name
      --osd                   OSD service/operator getting promoted
      --serviceName string    SaaS service/operator getting promoted
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

* [osdctl promote](osdctl_promote.md)	 - Utilities to promote services/operators

