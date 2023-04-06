## osdctl jumphost delete

Delete a jumphost created by `osdctl jumphost create`

### Synopsis

Delete a jumphost created by "osdctl jumphost create"

  This command cleans up AWS resources created by "osdctl jumphost create" if it
  fails the customer should be notified as there will be leftover AWS resources
  in their account. This command is idempotent and safe to run over and over.

  Requires these permissions:
  {
    "Version": "2012-10-17",
    "Statement": [
      {
        "Action": [
          "ec2:AuthorizeSecurityGroupIngress",
          "ec2:CreateKeyPair",
          "ec2:CreateSecurityGroup",
          "ec2:CreateTags",
          "ec2:DeleteKeyPair",
          "ec2:DeleteSecurityGroup",
          "ec2:DescribeImages",
          "ec2:DescribeInstances",
          "ec2:DescribeKeyPairs",
          "ec2:DescribeSecurityGroups",
          "ec2:DescribeSubnets",
          "ec2:RunInstances",
          "ec2:TerminateInstances"
        ],
        "Effect": "Allow",
        "Resource": "*"
      }
    ]
  }

```
osdctl jumphost delete [flags]
```

### Examples

```

  # Create and delete a jumphost
  osdctl jumphost create --subnet-id public-subnet-id
  osdctl jumphost delete --subnet-id public-subnet-id
```

### Options

```
  -h, --help               help for delete
      --subnet-id string   subnet id to search for and delete a jumphost in
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

* [osdctl jumphost](osdctl_jumphost.md)	 - 

