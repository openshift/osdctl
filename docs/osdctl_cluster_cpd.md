## osdctl cluster cpd

Runs diagnostic for a Cluster Provisioning Delay (CPD)

### Synopsis


Helps investigate OSD/ROSA cluster provisioning delays (CPD) or failures

  This command only supports AWS at the moment and will:
	
  * Check the cluster's dnszone.hive.openshift.io custom resource
  * Check whether a known OCM error code and message has been shared with the customer already
  * Check that the cluster's VPC and/or subnet route table(s) contain a route for 0.0.0.0/0 if it's BYOVPC


```
osdctl cluster cpd [flags]
```

### Examples

```

  # Investigate a CPD for a cluster using an AWS profile named "rhcontrol"
  osdctl cluster cpd --cluster-id 1kfmyclusteristhebesteverp8m --profile rhcontrol

```

### Options

```
  -C, --cluster-id string   The internal (OCM) Cluster ID
  -h, --help                help for cpd
  -p, --profile string      AWS profile name
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

* [osdctl cluster](osdctl_cluster.md)	 - Provides information for a specified cluster

