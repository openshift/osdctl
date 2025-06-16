## osdctl cluster ssh key

Retrieve a cluster's SSH key from Hive

### Synopsis

Retrieve a cluster's SSH key from Hive. If a cluster-id is provided, then the key retrieved will be for that cluster. If no cluster-id is provided, then the key for the cluster backplane is currently logged into will be used instead. This command should only be used as a last resort, when all other means of accessing a node are lost.

```
osdctl cluster ssh key --reason $reason [--cluster-id $CLUSTER_ID] [flags]
```

### Examples

```
$ osdctl cluster ssh key --cluster-id $CLUSTER_ID --reason "OHSS-XXXX"
INFO[0005] Backplane URL retrieved via OCM environment: https://api.backplane.openshift.com
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----

Providing a --cluster-id allows you to specify the cluster who's private ssh key you want to view, regardless if you're logged in or not.


$ osdctl cluster ssh key --reason "OHSS-XXXX"
INFO[0005] Backplane URL retrieved via OCM environment: https://api.backplane.openshift.com
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----

Omitting --cluster-id will print the ssh key for the cluster you're currently logged into.


$ osdctl cluster ssh key -y --reason "OHSS-XXXX" > /tmp/ssh.key
INFO[0005] Backplane URL retrieved via OCM environment: https://api.backplane.openshift.com
$ cat /tmp/ssh.key
-----BEGIN RSA PRIVATE KEY-----
...
-----END RSA PRIVATE KEY-----

Despite the logs from backplane, the ssh key is the only output channelled through stdout. This means you can safely redirect the output to a file for greater convienence.
```

### Options

```
      --cluster-id string   Cluster identifier (internal ID, UUID, name, etc) to retrieve the SSH key for. If not specified, the current cluster will be used.
  -h, --help                help for key
      --reason string       Provide a reason for accessing the clusters SSH key, used for backplane. Eg: 'OHSS-XXXX', or '#ITN-2024-XXXXX
  -y, --yes                 Skip any confirmation prompts and print the key automatically. Useful for redirects and scripting.
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

* [osdctl cluster ssh](osdctl_cluster_ssh.md)	 - utilities for accessing cluster via ssh

