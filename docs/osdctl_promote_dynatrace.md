## osdctl promote dynatrace

Utilities to promote dynatrace

```
osdctl promote dynatrace [flags]
```

### Examples

```

		# List all Dynatrace components available for promotion
		osdctl promote dynatrace --list

		# Promote a dynatrace component
		osdctl promote dynatrace --component <component> --gitHash <git-hash>

		# List all dynatrace-config modules available for promotion
		osdctl promote dynatrace --terraform --list

		# Promote a dynatrace module
		osdctl promote dynatrace --terraform --module=<module-name>
```

### Options

```
      --appInterfaceDir pwd         location of app-interface checkout. Falls back to pwd
  -c, --component string            Dynatrace component getting promoted
      --dynatraceConfigDir string   location of dynatrace-config checkout. Falls back to `pwd'
  -g, --gitHash string              Git hash of the SaaS service/operator commit getting promoted
  -h, --help                        help for dynatrace
  -l, --list                        List all SaaS services/operators
  -m, --module string               module to promote
  -t, --terraform                   deploy dynatrace-config terraform job
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

