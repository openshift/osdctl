## osdctl network verify-egress

Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.

### Synopsis

Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.

  This command is an opinionated wrapper around running https://github.com/openshift/osd-network-verifier for SREs.
  Given an OCM cluster name or id, this command will attempt to automatically detect the security group, subnet, and
  cluster-wide proxy configuration to run osd-network-verifier's egress verification. The purpose of this check is to
  verify whether a ROSA cluster's VPC allows for all required external URLs are reachable. The exact cause can vary and
  typically requires a customer to remediate the issue themselves.

  The osd-network-verifier supports two modes:
  1. Traditional mode: launches a probe instance in a given subnet and checks egress to external required URLs.
     Since October 2022, the probe is an instance without a public IP address. For this reason, the probe's requests
     will fail for subnets that don't have a NAT gateway. This mode will always fail and give a false negative for
     public subnets (in non-privatelink clusters), since they have an internet gateway and no NAT gateway.
  2. Pod mode (--pod-mode): runs verification as Kubernetes Jobs within the target cluster. This mode requires
     cluster admin access but provides more accurate results as it tests from within the actual cluster environment.
     
     Pod mode uses the following Kubernetes client configuration priority:
     1. In-cluster configuration (when ServiceAccount token exists)
     2. Backplane credentials (when --cluster-id is provided)
     3. User-provided kubeconfig (when --kubeconfig is specified)
     4. Default kubeconfig (from ~/.kube/config)

  Docs: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-aws-prereqs.html#osd-aws-privatelink-firewall-prerequisites_prerequisites

```
osdctl network verify-egress [flags]
```

### Examples

```

  # Run against a cluster registered in OCM
  osdctl network verify-egress --cluster-id my-rosa-cluster

  # Run against a cluster registered in OCM with a cluster-wide-proxy
  touch cacert.txt
  osdctl network verify-egress --cluster-id my-rosa-cluster --cacert cacert.txt

  # Override automatic selection of a subnet or security group id
  osdctl network verify-egress --cluster-id my-rosa-cluster --subnet-id subnet-abcd --security-group sg-abcd

  # Run against multiple manually supplied subnet IDs
  osdctl network verify-egress --cluster-id my-rosa-cluster --subnet-id subnet-abcd --subnet-id subnet-efgh

  # Override automatic selection of the list of endpoints to check
  osdctl network verify-egress --cluster-id my-rosa-cluster --platform hostedcluster

  # Run in pod mode using Kubernetes jobs (requires cluster access)
  osdctl network verify-egress --cluster-id my-rosa-cluster --pod-mode

  # Run in pod mode using ServiceAccount (when running inside a Kubernetes Pod)
  osdctl network verify-egress --pod-mode --region us-east-1 --namespace my-namespace

  # Run in pod mode with custom namespace and kubeconfig
  osdctl network verify-egress --pod-mode --region us-east-1 --namespace my-namespace --kubeconfig ~/.kube/config

  # Run network verification without sending service logs on failure
  osdctl network verify-egress --cluster-id my-rosa-cluster --no-service-log

  # (Not recommended) Run against a specific VPC, without specifying cluster-id
  <export environment variables like AWS_ACCESS_KEY_ID or use aws configure>
  osdctl network verify-egress --subnet-id subnet-abcdefg123 --security-group sg-abcdefgh123 --region us-east-1
```

### Options

```
  -A, --all-subnets               (optional) an option for AWS Privatelink clusters to run osd-network-verifier against all subnets listed by ocm.
      --cacert string             (optional) path to a file containing the additional CA trust bundle. Typically set so that the verifier can use a configured cluster-wide proxy.
  -C, --cluster-id string         (optional) OCM internal/external cluster id to run osd-network-verifier against.
      --cpu-arch string           (optional) compute instance CPU architecture. E.g., 'x86' or 'arm' (default "x86")
      --debug                     (optional) if provided, enable additional debug-level logging
      --egress-timeout duration   (optional) timeout for individual egress verification requests (default 5s)
      --gcp-project-id string     (optional) the GCP project ID to run verification for
  -h, --help                      help for verify-egress
      --kubeconfig string         (optional) path to kubeconfig file for pod mode (uses default kubeconfig if not specified)
      --namespace string          (optional) Kubernetes namespace to run verification pods in (default "openshift-network-diagnostics")
      --no-service-log            (optional) disable automatic service log sending when verification fails
      --no-tls                    (optional) if provided, ignore all ssl certificate validations on client-side.
      --platform string           (optional) override for cloud platform/product. E.g., 'aws-classic' (OSD/ROSA Classic), 'aws-hcp' (ROSA HCP), or 'aws-hcp-zeroegress'
      --pod-mode                  (optional) run verification using Kubernetes pods instead of cloud instances
      --probe string              (optional) select the probe to be used for egress testing. Either 'curl' (default) or 'legacy' (default "curl")
      --region string             (optional) AWS region, required for --pod-mode if not passing a --cluster-id
      --security-group string     (optional) security group ID override for osd-network-verifier, required if not specifying --cluster-id
      --subnet-id stringArray     (optional) private subnet ID override, required if not specifying --cluster-id and can be specified multiple times to run against multiple subnets
      --version                   When present, prints out the version of osd-network-verifier being used
      --vpc string                (optional) VPC name for cases where it can't be fetched from OCM
```

### Options inherited from parent commands

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl network](osdctl_network.md)	 - network related utilities

