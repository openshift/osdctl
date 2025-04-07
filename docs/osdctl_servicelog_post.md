## osdctl servicelog post

Post a service log to a cluster or list of clusters

### Synopsis

Post a service log to a cluster or list of clusters

  Docs: https://docs.openshift.com/rosa/logging/sd-accessing-the-service-logs.html

```
osdctl servicelog post --cluster-id <cluster-identifier> [flags]
```

### Examples

```

  # Post a service log to a single cluster via a local file
  osdctl servicelog post --cluster-id ${CLUSTER_ID} -t ~/path/to/file.json

  # Post a service log to a single cluster via a remote URL, providing a parameter
  osdctl servicelog post --cluster-id ${CLUSTER_ID} -t https://raw.githubusercontent.com/openshift/managed-notifications/master/osd/incident_resolved.json -p ALERT_NAME="alert"

  # Post an internal-only service log message
  osdctl servicelog post --cluster-id ${CLUSTER_ID} -i -p "MESSAGE=This is an internal message"

  # Post a short external message
  osdctl servicelog post --cluster-id ${CLUSTER_ID} -r "summary=External Message" -r "description=This is an external message" -r internal_only=False

  # Post a service log to a group of clusters, determined by an OCM query
  ocm list cluster -p search="cloud_provider.id is 'gcp' and managed='true' and state is 'ready'"
  osdctl servicelog post -q "cloud_provider.id is 'gcp' and managed='true' and state is 'ready'" -t file.json

```

### Options

```
  -C, --cluster-id string        Internal ID of the cluster to post the service log to
  -c, --clusters-file string     Read a list of clusters to post the servicelog to. the format of the file is: {"clusters":["$CLUSTERID"]}
  -d, --dry-run                  Dry-run - print the service log about to be sent but don't send it.
  -h, --help                     help for post
  -i, --internal                 Internal only service log. Use MESSAGE for template parameter (eg. -p MESSAGE='My super secret message').
  -r, --override Info            Specify a key-value pair (eg. -r FOO=BAR) to replace a JSON key in the document, only supports string fields, specifying -r without -t or -i will use a default template with severity Info and internal_only=True unless these are also overridden.
  -p, --param stringArray        Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.
  -q, --query stringArray        Specify a search query (eg. -q "name like foo") for a bulk-post to matching clusters.
  -f, --query-file stringArray   File containing search queries to apply. All lines in the file will be concatenated into a single query. If this flag is called multiple times, every file's search query will be combined with logical AND.
  -t, --template string          Message template file or URL
  -y, --yes                      Skips all prompts.
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

* [osdctl servicelog](osdctl_servicelog.md)	 - OCM/Hive Service log

