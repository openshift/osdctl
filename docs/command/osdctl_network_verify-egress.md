## osdctl network verify-egress

Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.

### Synopsis

Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.

  This command is an opinionated wrapper around running https://github.com/openshift/osd-network-verifier for SREs.
  Given an OCM cluster name or id, this command will attempt to automatically detect the security group, subnet, and
  cluster-wide proxy configuration to run osd-network-verifier's egress verification. The purpose of this check is to
  verify whether a ROSA cluster's VPC allows for all required external URLs are reachable. The exact cause can vary and
  typically requires a customer to remediate the issue themselves.

  Docs: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-aws-prereqs.html#osd-aws-privatelink-firewall-prerequisites_prerequisites

```
osdctl network verify-egress [flags]
```

### Examples

```

  # Run against a cluster registered in OCM
  ocm-backplane tunnel -D
  osdctl network verify-egress --cluster-id my-rosa-cluster

  # Run against a cluster registered in OCM with a cluster-wide-proxy
  ocm-backplane tunnel -D
  touch cacert.txt
  osdctl network verify-egress --cluster-id my-rosa-cluster --cacert cacert.txt

  # Override automatic selection of a subnet or security group id
  ocm-backplane tunnel -D
  osdctl network verify-egress --cluster-id my-rosa-cluster --subnet-id subnet-abcd --security-group sg-abcd

  # (Not recommended) Run against a specific VPC, without specifying cluster-id
  <export environment variables like AWS_ACCESS_KEY_ID or use aws configure>
  osdctl network verify-egress --subnet-id subnet-abcdefg123 --security-group sg-abcdefgh123 --region us-east-1
```

### Options

```
      --cacert string           (optional) path to a file containing the additional CA trust bundle. Typically set so that the verifier can use a configured cluster-wide proxy.
      --cluster-id string       (optional) OCM internal/external cluster id to run osd-network-verifier against.
      --debug                   (optional) if provided, enable additional debug-level logging
  -h, --help                    help for verify-egress
      --no-tls                  (optional) if provided, ignore all ssl certificate validations on client-side.
      --region string           (optional) AWS region
      --security-group string   (optional) security group ID override for osd-network-verifier, required if not specifying --cluster-id
      --subnet-id string        (optional) private subnet ID override, required if not specifying --cluster-id
```

### Options inherited from parent commands

```
      --alsologtostderr                  log to standard error as well as files
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --log_backtrace_at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log_dir string                   If non-empty, write log files in this directory
      --logtostderr                      log to standard error instead of files
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO

* [osdctl network](osdctl_network.md)	 - network related utilities

