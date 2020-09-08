## osdctl account secret rotate

Rotate IAM credentials secret

### Synopsis

Rotate IAM credentials secret

```
osdctl account secret rotate <IAM User name> [flags]
```

### Options

```
  -i, --account-id string          AWS Account ID
  -a, --account-name string        AWS Account CR name
      --account-namespace string   The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
  -c, --aws-config string          specify AWS config file path
  -p, --aws-profile string         specify AWS profile
  -r, --aws-region string          Specify AWS region (default "us-east-1")
  -h, --help                       help for rotate
  -o, --output string              Output path for secret yaml file
      --print                      Print the generated secret (default true)
      --secret-name string         Specify name of the generated secret (default "byoc")
      --secret-namespace string    Specify namespace of the generated secret (default "aws-account-operator")
```

### Options inherited from parent commands

```
      --cluster string             The name of the kubeconfig cluster to use
      --context string             The name of the kubeconfig context to use
      --insecure-skip-tls-verify   If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string          Path to the kubeconfig file to use for CLI requests.
      --request-timeout string     The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string              The address and port of the Kubernetes API server
```

### SEE ALSO

* [osdctl account secret](osdctl_account_secret.md)	 - secret <command>

