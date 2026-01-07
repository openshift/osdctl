## osdctl hive login-tests

Test utility to exercise OSDCTL client connections for both Target Cluster and it's Hive Cluster.

### Synopsis


This test utility attempts to exercise and validate OSDCTL's functions related to
OCM and backplane client connections. 
	
This test utility can be run against an OSD/Rosa Classic target cluster. This utility
will attempt to discover the Hive cluster, and create both
OCM and kube client connections, and perform basic requests for each to connection in 
order to validate functionality of the related OSDCTL utility functions.  
	
This test utility allows for the target cluster to exist in a separate OCM 
environment (ie integration, staging) from the hive cluster (ie production).

The default OCM environment vars should be set for the target cluster. 
If the target cluster exists outside of the OCM 'production' environment, the user 
has the option to provide the production OCM config (with valid token set), 
or provide the production OCM API url as a command argument, or set the value in the osdctl 
config yaml file (ie: "hive_ocm_url: https://api.openshift.com" or "hive_ocm_url: production" ).
For testing purposes comment out 'hive_ocm_url' from osdctl's config if testing an empty value. 


```
osdctl hive login-tests [flags]
```

### Options

```
  -C, --cluster-id string        Cluster ID
  -h, --help                     help for login-tests
      --hive-ocm-config string   OCM config for hive if different than Cluster
      --hive-ocm-url string      OCM URL for hive, this will fallback to reading from the osdctl config value: 'hive_ocm_url' if left empty
      --verbose                  Verbose output
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

* [osdctl hive](osdctl_hive.md)	 - hive related utilities

