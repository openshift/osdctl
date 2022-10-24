## osdctl network verify-egress

Verify essential openshift domains are reachable from given subnet ID.

### Synopsis

Verify essential openshift domains are reachable from given subnet ID.

```
osdctl network verify-egress [flags]
```

### Examples

```
For AWS, ensure your credential environment vars 
AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY (also AWS_SESSION_TOKEN for STS credentials) 
are set correctly before execution.

# Verify that essential openshift domains are reachable from a given SUBNET_ID
osdctl network verify-egress --subnet-id $(SUBNET_ID) --region $(AWS_REGION)
```

### Options

```
      --cacert string               (optional) path to cacert file to be used upon https requests being made by verifier
      --cloud-tags stringToString   (optional) comma-seperated list of tags to assign to cloud resources e.g. --cloud-tags key1=value1,key2=value2 (default [osd-network-verifier=owned,red-hat-managed=true,Name=osd-network-verifier])
      --debug                       (optional) if true, enable additional debug-level logging
  -h, --help                        help for verify-egress
      --http-proxy string           (optional) http-proxy to be used upon http requests being made by verifier, format: http://user:pass@x.x.x.x:8978
      --https-proxy string          (optional) https-proxy to be used upon https requests being made by verifier, format: https://user:pass@x.x.x.x:8978
      --image-id string             (optional) cloud image for the compute instance
      --instance-type string        (optional) compute instance type (default "t3.micro")
      --kms-key-id string           (optional) ID of KMS key used to encrypt root volumes of compute instances. Defaults to cloud account default key
      --no-tls                      (optional) if true, ignore all ssl certificate validations on client-side.
  -p, --profile string              (optional) AWS Profile
      --region string               (optional) compute instance region. If absent, environment var AWS_REGION will be used, if set (default "us-east-1")
      --security-group string       (optional) Security group to use for EC2 instance
      --subnet-id string            source subnet ID
      --timeout duration            (optional) timeout for individual egress verification requests (default 1s)
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
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          log level for V logs
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### SEE ALSO

* [osdctl network](osdctl_network.md)	 - network related utilities

