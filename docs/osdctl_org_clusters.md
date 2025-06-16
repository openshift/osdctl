## osdctl org clusters

get all active organization clusters

### Synopsis

By default, returns all active clusters for a given organization. The organization can either be specified with an argument
passed in, or by providing both the --aws-profile and --aws-account-id flags. You can request all clusters regardless of status by providing the --all flag.

```
osdctl org clusters [flags]
```

### Examples

```
Retrieving all active clusters for a given organizational unit:
osdctl org clusters 123456789AbcDEfGHiJklMnopQR

Retrieving all active clusters for a given organizational unit in JSON format:
osdctl org clusters 123456789AbcDEfGHiJklMnopQR -o json

Retrieving all clusters for a given organizational unit regardless of status:
osdctl org clusters 123456789AbcDEfGHiJklMnopQR --all

Retrieving all active clusters for a given AWS profile:
osdctl org clusters --aws-profile my-aws-profile --aws-account-id 123456789
```

### Options

```
  -A, --all                     get all clusters regardless of status
  -a, --aws-account-id string   specify AWS Account Id
  -p, --aws-profile string      specify AWS profile
  -h, --help                    help for clusters
  -o, --output string           valid output formats are ['', 'json']
```

### Options inherited from parent commands

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl org](osdctl_org.md)	 - Provides information for a specified organization

