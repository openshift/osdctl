## osdctl cluster support post

Send limited support reason to a given cluster

### Synopsis

Sends limited support reason to a given cluster, along with an internal service log detailing why the cluster was placed into limited support.
The caller will be prompted to continue before sending the limited support reason.

```
osdctl cluster support post --cluster-id <cluster-identifier> [flags]
```

### Examples

```
# Post a limited support reason for a cluster misconfiguration
osdctl cluster support post --cluster-id=1a2B3c4DefghIjkLMNOpQrSTUV5 --misconfiguration cluster --problem="The cluster has a second failing ingress controller, which is not supported and can cause issues with SLA." \
--resolution="Remove the additional ingress controller 'my-custom-ingresscontroller'. 'oc get ingresscontroller -n openshift-ingress-operator' should yield only 'default'" \
--evidence="See OHSS-1234"

Will result in the following limited-support text sent to the customer:
The cluster has a second failing ingress controller, which is not supported and can cause issues with SLA. Remove the additional ingress controller 'my-custom-ingresscontroller'. 'oc get ingresscontroller -n openshift-ingress-operator' should yield only 'default'.

```

### Options

```
  -c, --cluster-id string        Intenal Cluster ID (required)
      --evidence string          (optional) The reasoning that led to the decision to place the cluster in limited support. Can also be a link to a Jira case. Used for internal service log only.
  -h, --help                     help for post
      --misconfiguration cloud   The type of misconfiguration responsible for the cluster being placed into limited support. Valid values are cloud or `cluster`.
  -p, --param stringArray        Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.
      --problem string           Complete sentence(s) describing the problem responsible for the cluster being placed into limited support. Will form the limited support message with the contents of --resolution appended
      --resolution string        Complete sentence(s) describing the steps for the customer to take to resolve the issue and move out of limited support. Will form the limited support message with the contents of --problem prepended
  -t, --template string          Message template file or URL
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

* [osdctl cluster support](osdctl_cluster_support.md)	 - Cluster Support

