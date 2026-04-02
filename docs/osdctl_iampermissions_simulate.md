## osdctl iampermissions simulate

Simulate IAM policies against required permissions to detect mismatches

### Synopsis

Simulate validates that ROSA managed IAM policies grant all permissions
required by OCP components. It uses AWS IAM SimulateCustomPolicy to test
each required action against the managed policy, including condition key
contexts that CredentialsRequest diffing alone cannot catch.

Managed policies are automatically fetched from the managed-cluster-config
repository for the corresponding major.minor version, and all managed
policies are compared against the respective operator CredentialsRequests
extracted from the release image.

Examples:

```bash
  # Compare all managed policies for a version against operator CRs
  osdctl iampermissions simulate \
    --release-version 4.17.0

  # Also run supplementary test manifests (e.g. for condition key scenarios)
  osdctl iampermissions simulate \
    --release-version 4.17.0 \
    --manifest-file ./ebs-csi-driver.yaml

  # Output JUnit XML for CI integration
  osdctl iampermissions simulate \
    --release-version 4.17.0 \
    --output junit \
    --output-file results.xml
```

```
osdctl iampermissions simulate [flags]
```

### Options

```
  -h, --help                     help for simulate
      --manifest-dir string      Path to a directory of supplementary test manifest YAMLs
      --manifest-file string     Path to a supplementary test manifest YAML
  -o, --output string            Output format: table, json, junit (default "table")
      --output-file string       Write output to file instead of stdout
      --region string            AWS region for IAM API calls (default "us-east-1")
  -r, --release-version string   OCP release version (required)
```

### Options inherited from parent commands

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -c, --cloud CloudSpec                  cloud for which the policies should be retrieved. supported values: [aws, sts, gcp, wif] (default aws)
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

* [osdctl iampermissions](osdctl_iampermissions.md)	 - STS/WIF utilities

