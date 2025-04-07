# osdctl Commands

## Command Overview

- `aao` - AWS Account Operator Debugging Utilities
  - `pool` - Get the status of the AWS Account Operator AccountPool
- `account` - AWS Account related utilities
  - `clean-velero-snapshots` - Cleans up S3 buckets whose name start with managed-velero
  - `cli` - Generate temporary AWS CLI credentials on demand
  - `console` - Generate an AWS console URL on the fly
  - `generate-secret <IAM User name>` - Generates IAM credentials secret
  - `get` - Get resources
    - `account` - Get AWS Account CR
    - `account-claim` - Get AWS Account Claim CR
    - `aws-account` - Get AWS Account ID
    - `legal-entity` - Get AWS Account Legal Entity
    - `secrets` - Get AWS Account CR related secrets
  - `list` - List resources
    - `account` - List AWS Account CR
    - `account-claim` - List AWS Account Claim CR
  - `mgmt` - AWS Account Management
    - `assign` - Assign account to user
    - `iam` - Creates an IAM user in a given AWS account and prints out the credentials
    - `list` - List out accounts for username
    - `unassign` - Unassign account to user
  - `reset <account name>` - Reset AWS Account CR
  - `rotate-secret <aws-account-cr-name>` - Rotate IAM credentials secret
  - `servicequotas` - Interact with AWS service-quotas
    - `describe` - Describe AWS service-quotas
  - `set <account name>` - Set AWS Account CR status
  - `verify-secrets [<account name>]` - Verify AWS Account CR IAM User credentials
- `alert` - List alerts
  - `list --cluster-id <cluster-id> --level [warning, critical, firing, pending, all]` - List all alerts or based on severity
  - `silence` - add, expire and list silence associated with alerts
    - `add --cluster-id <cluster-identifier> [--all --duration --comment | --alertname --duration --comment]` - Add new silence for alert
    - `expire [--cluster-id <cluster-identifier>] [--all | --silence-id <silence-id>]` - Expire Silence for alert
    - `list --cluster-id <cluster-identifier>` - List all silences
    - `org <org-id> [--all --duration --comment | --alertname --duration --comment]` - Add new silence for alert for org
- `cloudtrail` - AWS CloudTrail related utilities
  - `permission-denied-events` - Prints cloudtrail permission-denied events to console.
  - `write-events` - Prints cloudtrail write events to console with optional filtering
- `cluster` - Provides information for a specified cluster
  - `break-glass --cluster-id <cluster-identifier>` - Emergency access to a cluster
    - `cleanup --cluster-id <cluster-identifier>` - Drop emergency access to a cluster
  - `check-banned-user --cluster-id <cluster-identifier>` - Checks if the cluster owner is a banned user.
  - `context --cluster-id <cluster-identifier>` - Shows the context of a specified cluster
  - `cpd` - Runs diagnostic for a Cluster Provisioning Delay (CPD)
  - `detach-stuck-volume --cluster-id <cluster-identifier>` - Detach openshift-monitoring namespace's volume from a cluster forcefully
  - `etcd-health-check --cluster-id <cluster-id> --reason <reason for escalation>` - Checks the etcd components and member health
  - `etcd-member-replace --cluster-id <cluster-identifier>` - Replaces an unhealthy etcd node
  - `from-infra-id` - Get cluster ID and external ID from a given infrastructure ID commonly used by Splunk
  - `health` - Describes health of cluster nodes and provides other cluster vitals.
  - `hypershift-info` - Pull information about AWS objects from the cluster, the management cluster and the privatelink cluster
  - `logging-check --cluster-id <cluster-identifier>` - Shows the logging support status of a specified cluster
  - `orgId --cluster-id <cluster-identifier` - Get the OCM org ID for a given cluster
  - `owner` - List the clusters owned by the user (can be specified to any user, not only yourself)
  - `resize` - resize control-plane/infra nodes
    - `control-plane` - Resize an OSD/ROSA cluster's control plane nodes
    - `infra` - Resize an OSD/ROSA cluster's infra nodes
  - `resync` - Force a resync of a cluster from Hive
  - `sre-operators` - SRE operator related utilities
    - `describe` - Describe SRE operators
    - `list` - List the current and latest version of SRE operators
  - `ssh` - utilities for accessing cluster via ssh
    - `key --reason $reason [--cluster-id $CLUSTER_ID]` - Retrieve a cluster's SSH key from Hive
  - `support` - Cluster Support
    - `delete --cluster-id <cluster-identifier>` - Delete specified limited support reason for a given cluster
    - `post --cluster-id <cluster-identifier>` - Send limited support reason to a given cluster
    - `status --cluster-id <cluster-identifier>` - Shows the support status of a specified cluster
  - `transfer-owner` - Transfer cluster ownership to a new user (to be done by Region Lead)
  - `validate-pull-secret --cluster-id <cluster-identifier>` - Checks if the pull secret email matches the owner email
  - `validate-pull-secret-ext [CLUSTER_ID]` - Extended checks to confirm pull-secret data is synced with current OCM data
- `cost` - Cost Management related utilities
  - `create` - Create a cost category for the given OU
  - `get` - Get total cost of a given OU
  - `list` - List the cost of each Account/OU under given OU
  - `reconcile` - Checks if there's a cost category for every OU. If an OU is missing a cost category, creates the cost category
- `dynatrace` - Dynatrace related utilities
  - `gather-logs --cluster-id <cluster-identifier>` - Gather all Pod logs and Application event from HCP
  - `logs --cluster-id <cluster-identifier>` - Fetch logs from Dynatrace
  - `url --cluster-id <cluster-identifier>` - Get the Dynatrace Tenant URL for a given MC or HCP cluster
- `env [flags] [env-alias]` - Create an environment to interact with a cluster
- `hcp` - 
  - `must-gather --cluster-id <cluster-identifier>` - Create a must-gather for HCP cluster
- `hive` - hive related utilities
  - `clusterdeployment` - cluster deployment related utilities
    - `list` - List cluster deployment crs
    - `listresources` - List all resources on a hive cluster related to a given cluster
  - `clustersync-failures [flags]` - List clustersync failures
- `iampermissions` - STS/WIF utilities
  - `diff` - Diff iam permissions for cluster operators between two versions
  - `get` - Get OCP CredentialsRequests
  - `save` - Save iam permissions for use in mcc
- `jira` - Provides a set of commands for interacting with Jira
  - `quick-task <title>` - creates a new ticket with the given name
- `jumphost` - 
  - `create` - Create a jumphost for emergency SSH access to a cluster's VMs
  - `delete` - Delete a jumphost created by `osdctl jumphost create`
- `mc` - 
  - `list` - List ROSA HCP Management Clusters
- `network` - network related utilities
  - `packet-capture` - Start packet capture
  - `verify-egress` - Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.
- `org` - Provides information for a specified organization
  - `aws-accounts` - get organization AWS Accounts
  - `clusters` - get all active organization clusters
  - `context orgId` - fetches information about the given organization
  - `current` - gets current organization
  - `customers` - get paying/non-paying organizations
  - `describe` - describe organization
  - `get` - get organization by users
  - `labels` - get organization labels
  - `users` - get organization users
- `promote` - Utilities to promote services/operators
  - `dynatrace` - Utilities to promote dynatrace
  - `package` - Utilities to promote package-operator services
  - `saas` - Utilities to promote SaaS services/operators
- `servicelog` - OCM/Hive Service log
  - `list --cluster-id <cluster-identifier> [flags] [options]` - Get service logs for a given cluster identifier.
  - `post --cluster-id <cluster-identifier>` - Post a service log to a cluster or list of clusters
- `setup` - Setup the configuration
- `swarm` - Provides a set of commands for swarming activity
  - `secondary` - List unassigned JIRA issues based on criteria
- `upgrade` - Upgrade osdctl
- `version` - Display the version

## Command Details

### osdctl

CLI tool to provide OSD related utilities

```
osdctl [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for osdctl
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl aao

AWS Account Operator Debugging Utilities

```
osdctl aao [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for aao
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl aao pool

Get the status of the AWS Account Operator AccountPool

```
osdctl aao pool [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for pool
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account

AWS Account related utilities

```
osdctl account [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for account
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account clean-velero-snapshots

Cleans up S3 buckets whose name start with managed-velero

```
osdctl account clean-velero-snapshots [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -a, --aws-access-key-id string         AWS Access Key ID
  -c, --aws-config string                specify AWS config file path
  -p, --aws-profile string               specify AWS profile
  -r, --aws-region string                specify AWS region (default "us-east-1")
  -x, --aws-secret-access-key string     AWS Secret Access Key
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for clean-velero-snapshots
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account cli

Generate temporary AWS CLI credentials on demand

```
osdctl account cli [flags]
```

#### Flags

```
  -i, --accountId string                 AWS Account ID
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for cli
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Output type
  -p, --profile string                   AWS Profile
  -r, --region string                    Region
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --verbose                          Verbose output
```

### osdctl account console

Generate an AWS console URL on the fly

```
osdctl account console [flags]
```

#### Flags

```
  -i, --accountId string                 AWS Account ID
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -d, --duration int32                   The duration of the console session. Default value is 3600 seconds(1 hour) (default 3600)
  -h, --help                             help for console
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --launch                           Launch web browser directly
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --profile string                   AWS Profile
  -r, --region string                    Region
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --verbose                          Verbose output
```

### osdctl account generate-secret

When logged into a hive shard, this generates a new IAM credential secret for a given IAM user

```
osdctl account generate-secret <IAM User name> [flags]
```

#### Flags

```
  -i, --account-id string                AWS Account ID
  -a, --account-name string              AWS Account CR name
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -p, --aws-profile string               specify AWS profile
      --ccs                              Only generate specific secret for osdCcsAdmin. Requires Account CR name
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for generate-secret
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --quiet                            Suppress logged output
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
      --secret-name string               Specify name of the generated secret
      --secret-namespace string          Specify namespace of the generated secret (default "aws-account-operator")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account get

Get resources

```
osdctl account get [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for get
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account get account

Get AWS Account CR

```
osdctl account get account [flags]
```

#### Flags

```
  -c, --account-claim string             Account Claim CR name
  -n, --account-claim-ns string          Account Claim CR namespace
  -i, --account-id string                AWS account ID
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for account
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --show-managed-fields              If true, keep the managedFields when printing objects in JSON or YAML format.
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --template string                  Template string or path to template file to use when --output=jsonpath, --output=jsonpath-file.
```

### osdctl account get account-claim

Get AWS Account Claim CR

```
osdctl account get account-claim [flags]
```

#### Flags

```
  -a, --account string                   Account CR Name
  -i, --account-id string                AWS account ID
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for account-claim
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --show-managed-fields              If true, keep the managedFields when printing objects in JSON or YAML format.
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --template string                  Template string or path to template file to use when --output=jsonpath, --output=jsonpath-file.
```

### osdctl account get aws-account

Get AWS Account ID

```
osdctl account get aws-account [flags]
```

#### Flags

```
  -a, --account string                   Account CR Name
  -c, --account-claim string             Account Claim CR Name
  -n, --account-claim-ns string          Account Claim CR Namespace
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for aws-account
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account get legal-entity

Get AWS Account Legal Entity

```
osdctl account get legal-entity [flags]
```

#### Flags

```
  -i, --account-id string                AWS account ID
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for legal-entity
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account get secrets

Get AWS Account CR related secrets

```
osdctl account get secrets [flags]
```

#### Flags

```
  -i, --account-id string                AWS account ID
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for secrets
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account list

List resources

```
osdctl account list [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account list account

List AWS Account CR

```
osdctl account list account [flags]
```

#### Flags

```
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -c, --claim string                     Filter account CRs by claimed or not. Supported values are true, false. Otherwise it lists all accounts
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for account
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -r, --reuse string                     Filter account CRs by reused or not. Supported values are true, false. Otherwise it lists all accounts
  -s, --server string                    The address and port of the Kubernetes API server
      --show-managed-fields              If true, keep the managedFields when printing objects in JSON or YAML format.
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --state string                     Account cr state. The default value is all to display all the crs (default "all")
      --template string                  Template string or path to template file to use when --output=jsonpath, --output=jsonpath-file.
```

### osdctl account list account-claim

List AWS Account Claim CR

```
osdctl account list account-claim [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for account-claim
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --state string                     Account cr state. If not specified, it will list all crs by default.
```

### osdctl account mgmt

AWS Account Management

```
osdctl account mgmt [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for mgmt
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account mgmt assign

Assign account to user

```
osdctl account mgmt assign [flags]
```

#### Flags

```
  -i, --account-id string                (optional) Specific AWS account ID to assign
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for assign
  -I, --iam-user                         (optional) Create an AWS IAM user and Access Key
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --payer-account string             Payer account type
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --show-managed-fields              If true, keep the managedFields when printing objects in JSON or YAML format.
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --template string                  Template string or path to template file to use when --output=jsonpath, --output=jsonpath-file.
  -u, --username string                  LDAP username
```

### osdctl account mgmt iam

Creates an IAM user in a given AWS account and prints out the credentials

```
osdctl account mgmt iam [flags]
```

#### Flags

```
  -i, --accountId string                 AWS account ID to run this against
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for iam
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --profile string                   AWS Profile
  -r, --region string                    AWS Region
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -R, --rotate                           Rotate an IAM user's credentials and print the output
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -u, --user string                      Kerberos username to run this for
```

### osdctl account mgmt list

List out accounts for username

```
osdctl account mgmt list [flags]
```

#### Flags

```
  -i, --account-id string                Account ID
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --payer-account string             Payer account type
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --show-managed-fields              If true, keep the managedFields when printing objects in JSON or YAML format.
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --template string                  Template string or path to template file to use when --output=jsonpath, --output=jsonpath-file.
  -u, --user string                      LDAP username
```

### osdctl account mgmt unassign

Unassign account to user

```
osdctl account mgmt unassign [flags]
```

#### Flags

```
  -i, --account-id string                Account ID
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for unassign
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --payer-account string             Payer account type
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --show-managed-fields              If true, keep the managedFields when printing objects in JSON or YAML format.
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --template string                  Template string or path to template file to use when --output=jsonpath, --output=jsonpath-file.
  -u, --username string                  LDAP username
```

### osdctl account reset

Reset AWS Account CR

```
osdctl account reset <account name> [flags]
```

#### Flags

```
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for reset
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
      --reset-legalentity                This will wipe the legalEntity, claimLink and reused fields, allowing accounts to be used for different Legal Entities.
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account rotate-secret

When logged into a hive shard, this rotates IAM credential secrets for a given `account` CR.

```
osdctl account rotate-secret <aws-account-cr-name> [flags]
```

#### Flags

```
      --admin-username osdManagedAdmin*   The admin username to use for generating access keys. Must be in the format of osdManagedAdmin*. If not specified, this is inferred from the account CR.
      --as string                         Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -p, --aws-profile string                specify AWS profile
      --ccs                               Also rotates osdCcsAdmin credential. Use caution.
      --cluster string                    The name of the kubeconfig cluster to use
      --context string                    The name of the kubeconfig context to use
  -h, --help                              help for rotate-secret
      --insecure-skip-tls-verify          If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                 Path to the kubeconfig file to use for CLI requests.
  -o, --output string                     Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                     The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)
      --request-timeout string            The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                     The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy    Don't use the configured aws_proxy value
  -S, --skip-version-check                skip checking to see if this is the most recent release
```

### osdctl account servicequotas

Interact with AWS service-quotas

```
osdctl account servicequotas [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for servicequotas
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl account servicequotas describe

Describe AWS service-quotas

```
osdctl account servicequotas describe [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --clusterID string                 Cluster ID
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for describe
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --profile string                   AWS Profile
  -q, --quota-code string                Query for QuotaCode (default "L-1216C47A")
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --service-code string              Query for ServiceCode (default "ec2")
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --verbose                          Verbose output
```

### osdctl account set

Set AWS Account CR status

```
osdctl account set <account name> [flags]
```

#### Flags

```
  -a, --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for set
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --patch string                     the raw payload used to patch the account status
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -r, --rotate-credentials               set status.rotateCredentials in the specified account
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --state string                     set status.state field in the specified account
  -t, --type string                      The type of patch being provided; one of [merge json]. The strategic patch is not supported. (default "merge")
```

### osdctl account verify-secrets

Verify AWS Account CR IAM User credentials

```
osdctl account verify-secrets [<account name>] [flags]
```

#### Flags

```
      --account-namespace string         The namespace to keep AWS accounts. The default value is aws-account-operator. (default "aws-account-operator")
  -A, --all                              Verify all Account CRs
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for verify-secrets
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --verbose                          Verbose output
```

### osdctl alert

List alerts

```
osdctl alert [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for alert
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl alert list

Checks the alerts for the cluster and print the list based on severity

```
osdctl alert list --cluster-id <cluster-id> --level [warning, critical, firing, pending, all] [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Provide the internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -l, --level string                     Alert level [warning, critical, firing, pending, all] (default "all")
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl alert silence

add, expire and list silence associated with alerts

```
osdctl alert silence [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for silence
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl alert silence add

add new silence for specfic or all alert with comment and duration of alert

```
osdctl alert silence add --cluster-id <cluster-identifier> [--all --duration --comment | --alertname --duration --comment] [flags]
```

#### Flags

```
      --alertname strings                alertname (comma-separated)
  -a, --all                              Adding silences for all alert
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Provide the internal ID of the cluster
  -c, --comment string                   add comment about silence (default "Adding silence using the osdctl alert command")
      --context string                   The name of the kubeconfig context to use
  -d, --duration string                  Adding duration for silence as 15 days (default "15d")
  -h, --help                             help for add
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl alert silence expire

expire all silence or based on silenceid

```
osdctl alert silence expire [--cluster-id <cluster-identifier>] [--all | --silence-id <silence-id>] [flags]
```

#### Flags

```
  -a, --all                              clear all silences
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Provide the internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for expire
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --silence-id strings               silence id (comma-separated)
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl alert silence list

print the list of silences

```
osdctl alert silence list --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Provide the internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl alert silence org

add new silence for specfic or all alerts with comment and duration of alert for an organization. OHSS required for org-wide silence

```
osdctl alert silence org <org-id> [--all --duration --comment | --alertname --duration --comment] [flags]
```

#### Flags

```
      --alertname strings                alertname (comma-separated)
  -a, --all                              add silences for all alert
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --comment string                   add comment about silence. OHSS required for org-wide silence
      --context string                   The name of the kubeconfig context to use
  -d, --duration string                  add duration for silence (default "15d")
  -h, --help                             help for org
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cloudtrail

AWS CloudTrail related utilities

```
osdctl cloudtrail [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for cloudtrail
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cloudtrail permission-denied-events

Prints cloudtrail permission-denied events to console.

```
osdctl cloudtrail permission-denied-events [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                Cluster ID
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for permission-denied-events
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -r, --raw-event                        Prints the cloudtrail events to the console in raw json format
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --since string                     Specifies that only events that occur within the specified time are returned.Defaults to 5m. Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". (default "5m")
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -u, --url                              Generates Url link to cloud console cloudtrail event
```

### osdctl cloudtrail write-events

Prints cloudtrail write events to console with optional filtering

```
osdctl cloudtrail write-events [flags]
```

#### Flags

```
  -A, --all                              Prints all cloudtrail write events without filtering
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                Cluster ID
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for write-events
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -r, --raw-event                        Prints the cloudtrail events to the console in raw json format
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --since string                     Specifies that only events that occur within the specified time are returned.Defaults to 1h.Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". (default "1h")
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -u, --url                              Generates Url link to cloud console cloudtrail event
```

### osdctl cluster

Provides information for a specified cluster

```
osdctl cluster [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for cluster
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster break-glass

Obtain emergency credentials to access the given cluster. You must be logged into the cluster's hive shard

```
osdctl cluster break-glass --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Provide the internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for break-glass
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster break-glass cleanup

Relinquish emergency access from the given cluster. If the cluster is PrivateLink, it deletes
all jump pods in the cluster's namespace (because of this, you must be logged into the hive shard
when dropping access for PrivateLink clusters). For non-PrivateLink clusters, the $KUBECONFIG
environment variable is unset, if applicable.

```
osdctl cluster break-glass cleanup --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                [Mandatory] Provide the Internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for cleanup
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    [Mandatory for PrivateLink clusters] The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster check-banned-user

Checks if the cluster owner is a banned user.

```
osdctl cluster check-banned-user --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                Provide internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for check-banned-user
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster context

Shows the context of a specified cluster

```
osdctl cluster context --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                Provide internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -d, --days int                         Command will display X days of Error SLs sent to the cluster. Days is set to 30 by default (default 30)
      --full                             Run full suite of checks.
  -h, --help                             help for context
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --jiratoken jira_token             Pass in the Jira access token directly. If not passed in, by default will read jira_token from ~/.config/osdctl.
                                         Jira access tokens can be registered by visiting https://issues.redhat.com//secure/ViewProfile.jspa?selectedTab=com.atlassian.pats.pats-plugin:jira-user-personal-access-tokens
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --oauthtoken pd_oauth_token        Pass in PD oauthtoken directly. If not passed in, by default will read pd_oauth_token from ~/.config/osdctl.
                                         PD OAuth tokens can be generated by visiting https://martindstone.github.io/PDOAuth/
  -o, --output string                    Valid formats are ['long', 'short', 'json']. Output is set to 'long' by default (default "long")
      --pages int                        Command will display X pages of Cloud Trail logs for the cluster. Pages is set to 40 by default (default 40)
  -p, --profile string                   AWS Profile
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -t, --team-ids team_ids                Pass in PD team IDs directly to filter the PD Alerts by team. Can also be defined as team_ids in ~/.config/osdctl
                                         Will show all PD Alerts for all PD service IDs if none is defined
      --usertoken pd_user_token          Pass in PD usertoken directly. If not passed in, by default will read pd_user_token from ~/config/osdctl
      --verbose                          Verbose output
```

### osdctl cluster cpd


Helps investigate OSD/ROSA cluster provisioning delays (CPD) or failures

  This command only supports AWS at the moment and will:
	
  * Check the cluster's dnszone.hive.openshift.io custom resource
  * Check whether a known OCM error code and message has been shared with the customer already
  * Check that the cluster's VPC and/or subnet route table(s) contain a route for 0.0.0.0/0 if it's BYOVPC


```
osdctl cluster cpd [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                The internal (OCM) Cluster ID
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for cpd
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --profile string                   AWS profile name
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster detach-stuck-volume

Detach openshift-monitoring namespace's volume from a cluster forcefully

```
osdctl cluster detach-stuck-volume --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Provide internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for detach-stuck-volume
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster etcd-health-check

Checks etcd component health status for member replacement

```
osdctl cluster etcd-health-check --cluster-id <cluster-id> --reason <reason for escalation> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Provide the internal Cluster ID or name to perform health check on
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for etcd-health-check
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    Specify a reason for privilege escalation
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster etcd-member-replace

Replaces an unhealthy ectd node using the member id provided

```
osdctl cluster etcd-member-replace --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Provide internal Cluster ID
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for etcd-member-replace
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --node string                      Node ID (required)
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster from-infra-id

Get cluster ID and external ID from a given infrastructure ID commonly used by Splunk

```
osdctl cluster from-infra-id [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for from-infra-id
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster health

Describes health of cluster nodes and provides other cluster vitals.

```
osdctl cluster health [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                Internal Cluster ID
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for health
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --profile string                   AWS Profile
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --verbose                          Verbose output
```

### osdctl cluster hypershift-info

This command aggregates AWS objects from the cluster, management cluster and privatelink for hypershift cluster.
It attempts to render the relationships as graphviz if that output format is chosen or will simply print the output as tables.

```
osdctl cluster hypershift-info [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                Provide internal ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for hypershift-info
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    output format ['table', 'graphviz'] (default "graphviz")
  -l, --privatelinkaccount string        Privatelink account ID
  -p, --profile string                   AWS Profile
  -r, --region string                    AWS Region
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --verbose                          Verbose output
```

### osdctl cluster logging-check

Shows the logging support status of a specified cluster

```
osdctl cluster logging-check --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                The internal ID of the cluster to check (required)
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for logging-check
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --verbose                          Verbose output
```

### osdctl cluster orgId

Get the OCM org ID for a given cluster

```
osdctl cluster orgId --cluster-id <cluster-identifier [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                The internal ID of the cluster to check (required)
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for orgId
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster owner

List the clusters owned by the user (can be specified to any user, not only yourself)

```
osdctl cluster owner [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for owner
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -u, --user-id string                   user to check the cluster owner on
```

### osdctl cluster resize

resize control-plane/infra nodes

```
osdctl cluster resize [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for resize
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster resize control-plane

Resize an OSD/ROSA cluster's control plane nodes

  Requires previous login to the api server via "ocm backplane login".
  The user will be prompted to send a service log after initiating the resize. The resize process runs asynchronously,
  and this command exits immediately after sending the service log. Any issues with the resize will be reported via PagerDuty.

```
osdctl cluster resize control-plane [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                The internal ID of the cluster to perform actions on
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for control-plane
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --machine-type string              The target AWS machine type to resize to (e.g. m5.2xlarge)
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster resize infra

Resize an OSD/ROSA cluster's infra nodes

  This command automates most of the "machinepool dance" to safely resize infra nodes for production classic OSD/ROSA 
  clusters. This DOES NOT work in non-production due to environmental differences.

  Remember to follow the SOP for preparation and follow up steps:

    https://github.com/openshift/ops-sop/blob/master/v4/howto/resize-infras-workers.md


```
osdctl cluster resize infra [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                OCM internal/external cluster id or cluster name to resize infra nodes for.
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for infra
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --instance-type string             (optional) Override for an AWS or GCP instance type to resize the infra nodes to, by default supported instance types are automatically selected.
      --justification string             The justification behind resize
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --ohss string                      OHSS ticket tracking this infra node resize
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster resync

Force a resync of a cluster from Hive

  Normally, clusters are periodically synced by Hive every two hours at minimum. This command deletes a cluster's
  clustersync from its managing Hive cluster, causing the clustersync to be recreated in most circumstances and forcing
  a resync of all SyncSets and SelectorSyncSets. The command will also wait for the clustersync to report its status
  again (Success or Failure) before exiting.


```
osdctl cluster resync [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                OCM internal/external cluster id or cluster name to delete the clustersync for.
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for resync
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster sre-operators

SRE operator related utilities

```
osdctl cluster sre-operators [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for sre-operators
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster sre-operators describe


  Helps obtain various health information about a specified SRE operator within a cluster,
  including CSV, Subscription, OperatorGroup, Deployment, and Pod health statuses.

  A git_access token is required to fetch the latest version of the operators, and can be 
  set within the config file using the 'osdctl setup' command.

  The command creates a Kubernetes client to access the current cluster context, and GitLab/GitHub
  clients to fetch the latest versions of each operator from its respective repository.
	

```
osdctl cluster sre-operators describe [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for describe
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster sre-operators list


	Lists the current version, channel, and status of SRE operators running in the current 
	cluster context, and by default fetches the latest version from the operators' repositories.
	
	A git_access token is required to fetch the latest version of the operators, and can be 
	set within the config file using the 'osdctl setup' command.
	
	The command creates a Kubernetes client to access the current cluster context, and GitLab/GitHub
	clients to fetch the latest versions of each operator from its respective repository.
	

```
osdctl cluster sre-operators list [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --no-commit                        Excluse commit shas and repository URL from the output
      --no-headers                       Exclude headers from the output
      --operator string                  Filter to only show the specified operator.
      --outdated                         Filter to only show operators running outdated versions
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --short                            Exclude fetching the latest version from repositories for faster output
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster ssh

utilities for accessing cluster via ssh

```
osdctl cluster ssh [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for ssh
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster ssh key

Retrieve a cluster's SSH key from Hive. If a cluster-id is provided, then the key retrieved will be for that cluster. If no cluster-id is provided, then the key for the cluster backplane is currently logged into will be used instead. This command should only be used as a last resort, when all other means of accessing a node are lost.

```
osdctl cluster ssh key --reason $reason [--cluster-id $CLUSTER_ID] [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Cluster identifier (internal ID, UUID, name, etc) to retrieve the SSH key for. If not specified, the current cluster will be used.
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for key
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    Provide a reason for accessing the clusters SSH key, used for backplane. Eg: 'OHSS-XXXX', or '#ITN-2024-XXXXX
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -y, --yes                              Skip any confirmation prompts and print the key automatically. Useful for redirects and scripting.
```

### osdctl cluster support

Cluster Support

```
osdctl cluster support [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for support
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster support delete

Delete specified limited support reason for a given cluster

```
osdctl cluster support delete --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --all                                Remove all limited support reasons
      --as string                          Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                     The name of the kubeconfig cluster to use
  -c, --cluster-id string                  Internal cluster ID (required)
      --context string                     The name of the kubeconfig context to use
  -d, --dry-run                            Dry-run - print the limited support reason about to be sent but don't send it.
  -h, --help                               help for delete
      --insecure-skip-tls-verify           If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                  Path to the kubeconfig file to use for CLI requests.
  -i, --limited-support-reason-id string   Limited support reason ID
  -o, --output string                      Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string             The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                      The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy     Don't use the configured aws_proxy value
  -S, --skip-version-check                 skip checking to see if this is the most recent release
      --verbose                            Verbose output
```

### osdctl cluster support post

Sends limited support reason to a given cluster, along with an internal service log detailing why the cluster was placed into limited support.
The caller will be prompted to continue before sending the limited support reason.

```
osdctl cluster support post --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                Intenal Cluster ID (required)
      --context string                   The name of the kubeconfig context to use
      --evidence string                  (optional) The reasoning that led to the decision to place the cluster in limited support. Can also be a link to a Jira case. Used for internal service log only.
  -h, --help                             help for post
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --misconfiguration cloud           The type of misconfiguration responsible for the cluster being placed into limited support. Valid values are cloud or `cluster`.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --param stringArray                Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.
      --problem string                   Complete sentence(s) describing the problem responsible for the cluster being placed into limited support. Will form the limited support message with the contents of --resolution appended
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
      --resolution string                Complete sentence(s) describing the steps for the customer to take to resolve the issue and move out of limited support. Will form the limited support message with the contents of --problem prepended
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -t, --template string                  Message template file or URL
```

### osdctl cluster support status

Shows the support status of a specified cluster

```
osdctl cluster support status --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                Cluster ID for which to get support status
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for status
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --verbose                          Verbose output
```

### osdctl cluster transfer-owner

Transfer cluster ownership to a new user (to be done by Region Lead)

```
osdctl cluster transfer-owner [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                The Internal Cluster ID/External Cluster ID/ Cluster Name
      --context string                   The name of the kubeconfig context to use
  -d, --dry-run                          Dry-run - show all changes but do not apply them
  -h, --help                             help for transfer-owner
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --new-owner string                 The new owners username to transfer the cluster to
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster validate-pull-secret

Checks if the pull secret email matches the owner email.

This command will automatically login to the cluster to check the current pull-secret defined in 'openshift-config/pull-secret'


```
osdctl cluster validate-pull-secret --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                The internal ID of the cluster to check (required)
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for validate-pull-secret
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command to be run (usually an OHSS or PD ticket), mandatory when using elevate
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster validate-pull-secret-ext


	Attempts to validate if a cluster's pull-secret auth values are in sync with the account's email, 
	registry_credential, and access token data stored in OCM.  
	If this is being executed against a cluster which is not owned by the current OCM account, 
	Region Lead permissions are required to view and validate the OCM AccessToken. 


```
osdctl cluster validate-pull-secret-ext [CLUSTER_ID] [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for validate-pull-secret-ext
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -l, --log-level string                 debug, info, warn, error. (default=info) (default "info")
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    Mandatory reason for this command to be run (usually includes an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-access-token                Exclude OCM AccessToken checks against cluster secret
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
      --skip-registry-creds              Exclude OCM Registry Credentials checks against cluster secret
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cluster validate-pull-secret-ext


	Attempts to validate if a cluster's pull-secret auth values are in sync with the account's email, 
	registry_credential, and access token data stored in OCM.  
	If this is being executed against a cluster which is not owned by the current OCM account, 
	Region Lead permissions are required to view and validate the OCM AccessToken. 


```
osdctl cluster validate-pull-secret-ext [CLUSTER_ID] [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for validate-pull-secret-ext
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -l, --log-level string                 debug, info, warn, error. (default=info) (default "info")
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    Mandatory reason for this command to be run (usually includes an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-access-token                Exclude OCM AccessToken checks against cluster secret
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
      --skip-registry-creds              Exclude OCM Registry Credentials checks against cluster secret
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cost

The cost command allows for cost management on the AWS platform (other 
platforms may be added in the future)

```
osdctl cost [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -a, --aws-access-key-id string         AWS Access Key ID
  -c, --aws-config string                specify AWS config file path
  -p, --aws-profile string               specify AWS profile
  -g, --aws-region string                specify AWS region (default "us-east-1")
  -x, --aws-secret-access-key string     AWS Secret Access Key
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for cost
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cost create

Create a cost category for the given OU

```
osdctl cost create [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -a, --aws-access-key-id string         AWS Access Key ID
  -c, --aws-config string                specify AWS config file path
  -p, --aws-profile string               specify AWS profile
  -g, --aws-region string                specify AWS region (default "us-east-1")
  -x, --aws-secret-access-key string     AWS Secret Access Key
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for create
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --ou string                        get OU ID
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl cost get

Get total cost of a given OU

```
osdctl cost get [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -a, --aws-access-key-id string         AWS Access Key ID
  -c, --aws-config string                specify AWS config file path
  -p, --aws-profile string               specify AWS profile
  -g, --aws-region string                specify AWS region (default "us-east-1")
  -x, --aws-secret-access-key string     AWS Secret Access Key
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --csv                              output result as csv
      --end string                       set end date range
  -h, --help                             help for get
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --ou string                        set OU ID
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -r, --recursive                        recurse through OUs
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --start string                     set start date range
      --sum                              Hide sum rows (default true)
  -t, --time string                      set time. One of 'LM', 'MTD', 'YTD', '3M', '6M', '1Y'
```

### osdctl cost list

List the cost of each Account/OU under given OU

```
osdctl cost list [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -a, --aws-access-key-id string         AWS Access Key ID
  -c, --aws-config string                specify AWS config file path
  -p, --aws-profile string               specify AWS profile
  -g, --aws-region string                specify AWS region (default "us-east-1")
  -x, --aws-secret-access-key string     AWS Secret Access Key
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --csv                              output result as csv
      --end string                       set end date range
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --level string                     Cost cummulation level: possible options: ou, account (default "ou")
      --ou stringArray                   get OU ID
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --start string                     set start date range
      --sum                              Hide sum rows (default true)
  -t, --time string                      set time. One of 'LM', 'MTD', 'YTD', '3M', '6M', '1Y'
```

### osdctl cost reconcile

Checks if there's a cost category for every OU. If an OU is missing a cost category, creates the cost category

```
osdctl cost reconcile [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -a, --aws-access-key-id string         AWS Access Key ID
  -c, --aws-config string                specify AWS config file path
  -p, --aws-profile string               specify AWS profile
  -g, --aws-region string                specify AWS region (default "us-east-1")
  -x, --aws-secret-access-key string     AWS Secret Access Key
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for reconcile
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --ou string                        get OU ID
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl dynatrace

Dynatrace related utilities

```
osdctl dynatrace [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for dynatrace
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl dynatrace gather-logs

Gathers pods logs and evnets of a given HCP from Dynatrace.

  This command fetches the logs from the HCP namespace, the hypershift namespace and cert-manager related namespaces.
  Logs will be dumped to a directory with prefix hcp-must-gather.
		

```
osdctl dynatrace gather-logs --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Internal ID of the HCP cluster to gather logs from (required)
      --context string                   The name of the kubeconfig context to use
      --dest-dir string                  Destination directory for the logs dump, defaults to the local directory.
  -h, --help                             help for gather-logs
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --since int                        Number of hours (integer) since which to pull logs and events (default 10)
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --sort string                      Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc' (default "asc")
      --tail int                         Last 'n' logs and events to fetch. By default it will pull everything
```

### osdctl dynatrace logs


  Fetch logs of current cluster context (by default) from Dynatrace and display the logs like oc logs.

  This command also prints the Dynatrace URL and the corresponding DQL in the output.



```
osdctl dynatrace logs --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Name or Internal ID of the cluster (defaults to current cluster context)
      --console                          Print the url to the dynatrace web console instead of outputting the logs
      --container strings                Container name(s) (comma-separated)
      --contains string                  Include logs which contain a phrase
      --context string                   The name of the kubeconfig context to use
      --dry-run                          Only builds the query without fetching any logs from the tenant
  -h, --help                             help for logs
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -n, --namespace strings                Namespace(s) (comma-separated)
      --node strings                     Node name(s) (comma-separated)
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --since int                        Number of hours (integer) since which to search (defaults to 1 hour) (default 1)
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --sort string                      Sort the results by timestamp in either ascending or descending order. Accepted values are 'asc' and 'desc'. Defaults to 'asc' (default "asc")
      --status strings                   Status(Info/Warn/Error) (comma-separated)
      --tail int                         Last 'n' logs to fetch (defaults to 100) (default 1000)
```

### osdctl dynatrace url

Get the Dynatrace Tenant URL for a given MC or HCP cluster

```
osdctl dynatrace url --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                ID of the cluster
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for url
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl env


Creates an isolated environment where you can interact with a cluster.
The environment is set up in a dedicated folder in $HOME/.ocenv.
The $CLUSTERID variable will be populated with the external ID of the cluster you're logged in to.

*PS1*
osdctl env renders the required PS1 function 'kube_ps1' to use when logged in to a cluster.
You need to include it inside your .bashrc or .bash_profile by adding a snippet like the following:

export PS1='...other information for your PS1... $(kube_ps1) \$ '

e.g.

export PS1='\[\033[36m\]\u\[\033[m\]@\[\033[32m\]\h:\[\033[33;1m\]\w\[\033[m\]$(kube_ps1) \$ '

osdctl env creates a new ocm and kube config so you can log in to different environments at the same time.
When initialized, osdctl env will copy the ocm config you're currently using.

*Logging in to the cluster*

To log in to a cluster within the environment using backplane, osdctl creates the ocb command.
The ocb command is created in the bin directory in the environment folder and added to the PATH when inside the environment.


```
osdctl env [flags] [env-alias]
```

#### Flags

```
  -a, --api string                       OpenShift API URL for individual cluster login
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --cluster-id string                Cluster ID
      --context string                   The name of the kubeconfig context to use
  -d, --delete                           Delete environment
  -k, --export-kubeconfig                Output export kubeconfig statement, to use environment outside of the env directory
  -h, --help                             help for env
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
  -K, --kubeconfig string                KUBECONFIG file to use in this env (will be copied to the environment dir)
  -l, --login-script string              OCM login script to execute in a loop in ocb every 30 seconds
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --password string                  Password for individual cluster login
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -r, --reset                            Reset environment
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -t, --temp                             Delete environment on exit
  -u, --username string                  Username for individual cluster login
```

### osdctl hcp

```
osdctl hcp [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for hcp
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl hcp must-gather

Create a must-gather for an HCP cluster with optional gather targets

```
osdctl hcp must-gather --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --acm_image string                 Overrides the acm must-gather image being used for acm mc, sc as well as hcp must-gathers. (default "quay.io/stolostron/must-gather:2.11.4-SNAPSHOT-2024-12-02-15-19-44")
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Internal ID of the cluster to gather data from
      --context string                   The name of the kubeconfig context to use
      --gather string                    Comma-separated list of gather targets (available: sc, sc_acm, mc, hcp). (default "hcp")
  -h, --help                             help for must-gather
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation (e.g., OHSS ticket or PD incident).
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl hive

hive related utilities

```
osdctl hive [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for hive
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl hive clusterdeployment

cluster deployment related utilities

```
osdctl hive clusterdeployment [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for clusterdeployment
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl hive clusterdeployment list

List cluster deployment crs

```
osdctl hive clusterdeployment list [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl hive clusterdeployment listresources

List all resources on a hive cluster related to a given cluster

```
osdctl hive clusterdeployment listresources [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                Cluster ID
      --context string                   The name of the kubeconfig context to use
  -e, --external                         only list external resources (i.e. exclude resources in cluster namespace)
  -h, --help                             help for listresources
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl hive clustersync-failures


  Helps investigate ClusterSyncs in a failure state on OSD/ROSA hive shards.

  This command by default will list ClusterSyncs that are in a failure state
  for clusters that are not in limited support or hibernating.

  Error messages are include in all output format except the text format.


```
osdctl hive clustersync-failures [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                Internal ID to list failing syncsets and relative errors for a specific cluster.
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for clustersync-failures
  -i, --hibernating                      Include hibernating clusters.
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -l, --limited-support                  Include clusters in limited support.
      --no-headers                       Don't print headers when output format is set to text.
      --order string                     Set the sorting order. Options: asc, desc. (default "asc")
  -o, --output string                    Set the output format. Options: yaml, json, csv, text. (default "text")
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --sort-by string                   Sort the output by a specified field. Options: name, timestamp, failingsyncsets. (default "timestamp")
      --syncsets                         Include failing syncsets. (default true)
```

### osdctl iampermissions

STS/WIF utilities

```
osdctl iampermissions [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -c, --cloud CloudSpec                  cloud for which the policies should be retrieved. supported values: [aws, sts, gcp, wif] (default aws)
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for iampermissions
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl iampermissions diff

Diff iam permissions for cluster operators between two versions

```
osdctl iampermissions diff [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -b, --base-version string              
  -c, --cloud CloudSpec                  cloud for which the policies should be retrieved. supported values: [aws, sts, gcp, wif] (default aws)
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for diff
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -t, --target-version string            
```

### osdctl iampermissions get

Get OCP CredentialsRequests

```
osdctl iampermissions get [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -c, --cloud CloudSpec                  cloud for which the policies should be retrieved. supported values: [aws, sts, gcp, wif] (default aws)
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for get
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -r, --release-version string           
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl iampermissions save

Save iam permissions for use in mcc

```
osdctl iampermissions save [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -c, --cloud CloudSpec                  cloud for which the policies should be retrieved. supported values: [aws, sts, gcp, wif] (default aws)
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -d, --dir string                       Folder where the policy files should be written
  -f, --force                            Overwrite existing files
  -h, --help                             help for save
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -r, --release-version string           ocp version for which the policies should be downloaded
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl jira

Provides a set of commands for interacting with Jira

```
osdctl jira [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for jira
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl jira quick-task

Creates a new ticket with the given name and a label specified by "jira_team_label" from the osdctl config. The flags "jira_board_id" and "jira_team" are also required for running this command.
The ticket will be assigned to the caller and added to their team's current sprint as an OSD Task.
A link to the created ticket will be printed to the console.

```
osdctl jira quick-task <title> [flags]
```

#### Flags

```
      --add-to-sprint                    whether or not to add the created Jira task to the SRE's current sprint.
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for quick-task
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl jumphost

```
osdctl jumphost [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for jumphost
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl jumphost create

Create a jumphost for emergency SSH access to a cluster's VMs'

  This command automates the process of creating a jumphost in order to gain SSH
  access to a cluster's EC2 instances and should generally only be used as a last
  resort when the cluster's API server is otherwise inaccessible. It requires valid
  AWS credentials to be already set and a subnet ID in the associated AWS account.
  The provided subnet ID must be a public subnet.

  When the cluster's API server is accessible, prefer "oc debug node".

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
osdctl jumphost create [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for create
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --subnet-id string                 public subnet id to create a jumphost in
```

### osdctl jumphost delete

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

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for delete
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --subnet-id string                 subnet id to search for and delete a jumphost in
```

### osdctl mc

```
osdctl mc [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for mc
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl mc list

List ROSA HCP Management Clusters.

```
osdctl mc list [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --output string                    Output format. Supported output formats include: table, text, json, yaml (default "table")
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl network

network related utilities

```
osdctl network [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for network
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl network packet-capture

Start packet capture

```
osdctl network packet-capture [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -d, --duration int                     Duration (in seconds) of packet capture (default 60)
  -h, --help                             help for packet-capture
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --name string                      Name of Daemonset (default "sre-packet-capture")
  -n, --namespace string                 Namespace to deploy Daemonset (default "default")
      --node-label-key string            Node label key (default "node-role.kubernetes.io/worker")
      --node-label-value string          Node label value
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --reason string                    The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --single-pod                       toggle deployment as single pod (default: deploy a daemonset)
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl network verify-egress

Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.

  This command is an opinionated wrapper around running https://github.com/openshift/osd-network-verifier for SREs.
  Given an OCM cluster name or id, this command will attempt to automatically detect the security group, subnet, and
  cluster-wide proxy configuration to run osd-network-verifier's egress verification. The purpose of this check is to
  verify whether a ROSA cluster's VPC allows for all required external URLs are reachable. The exact cause can vary and
  typically requires a customer to remediate the issue themselves.

  The osd-network-verifier launches a probe, an instance in a given subnet, and checks egress to external required URL's. Since October 2022, the probe is an instance without a public IP address. For this reason, the probe's requests will fail for subnets that don't have a NAT gateway. The osdctl network verify-egress command will always fail and give a false negative for public subnets (in non-privatelink clusters), since they have an internet gateway and no NAT gateway.

  Docs: https://docs.openshift.com/rosa/rosa_install_access_delete_clusters/rosa_getting_started_iam/rosa-aws-prereqs.html#osd-aws-privatelink-firewall-prerequisites_prerequisites

```
osdctl network verify-egress [flags]
```

#### Flags

```
  -A, --all-subnets                      (optional) an option for AWS Privatelink clusters to run osd-network-verifier against all subnets listed by ocm.
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cacert string                    (optional) path to a file containing the additional CA trust bundle. Typically set so that the verifier can use a configured cluster-wide proxy.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                (optional) OCM internal/external cluster id to run osd-network-verifier against.
      --context string                   The name of the kubeconfig context to use
      --cpu-arch string                  (optional) compute instance CPU architecture. E.g., 'x86' or 'arm' (default "x86")
      --debug                            (optional) if provided, enable additional debug-level logging
      --egress-timeout duration          (optional) timeout for individual egress verification requests (default 5s)
      --gcp-project-id string            (optional) the GCP project ID to run verification for
  -h, --help                             help for verify-egress
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --no-tls                           (optional) if provided, ignore all ssl certificate validations on client-side.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --platform string                  (optional) override for cloud platform/product. E.g., 'aws-classic' (OSD/ROSA Classic), 'aws-hcp' (ROSA HCP), or 'aws-hcp-zeroegress'
      --probe string                     (optional) select the probe to be used for egress testing. Either 'curl' (default) or 'legacy' (default "curl")
      --region string                    (optional) AWS region
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
      --security-group string            (optional) security group ID override for osd-network-verifier, required if not specifying --cluster-id
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
      --subnet-id stringArray            (optional) private subnet ID override, required if not specifying --cluster-id and can be specified multiple times to run against multiple subnets
      --version                          When present, prints out the version of osd-network-verifier being used
      --vpc string                       (optional) VPC name for cases where it can't be fetched from OCM
```

### osdctl org

Provides information for a specified organization

```
osdctl org [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for org
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl org aws-accounts

get organization AWS Accounts

```
osdctl org aws-accounts [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -p, --aws-profile string               specify AWS profile
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for aws-accounts
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
      --ou-id string                     specify organization unit id
  -o, --output string                    valid output formats are ['', 'json']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl org clusters

By default, returns all active clusters for a given organization. The organization can either be specified with an argument
passed in, or by providing both the --aws-profile and --aws-account-id flags. You can request all clusters regardless of status by providing the --all flag.

```
osdctl org clusters [flags]
```

#### Flags

```
  -A, --all                              get all clusters regardless of status
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
  -a, --aws-account-id string            specify AWS Account Id
  -p, --aws-profile string               specify AWS profile
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for clusters
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    valid output formats are ['', 'json']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl org context

Fetches information about the given organization. This data is presented as a table where each row includes the name, version, ID, cloud provider, and plan for the cluster.
Rows will also include the number of recent service logs, active PD Alerts, Jira Issues, and limited support status for that specific cluster.

```
osdctl org context orgId [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for context
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    output format for the results. only supported value currently is 'json'
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl org current

gets current organization

```
osdctl org current [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for current
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    valid output formats are ['', 'json']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl org customers

get paying/non-paying organizations

```
osdctl org customers [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for customers
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    valid output formats are ['', 'json']
      --paying                           get organization based on paying status (default true)
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl org describe

describe organization

```
osdctl org describe [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for describe
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    valid output formats are ['', 'json']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl org get

get organization by users

```
osdctl org get [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --ebs-id string                    search organization by ebs account id 
  -h, --help                             help for get
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    valid output formats are ['', 'json']
      --part-match                       Part matching user name
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -u, --user string                      search organization by user name 
```

### osdctl org labels

get organization labels

```
osdctl org labels [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for labels
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    valid output formats are ['', 'json']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl org users

get organization users

```
osdctl org users [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for users
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    valid output formats are ['', 'json']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl promote

Utilities to promote services/operators

```
osdctl promote [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for promote
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl promote dynatrace

Utilities to promote dynatrace

```
osdctl promote dynatrace [flags]
```

#### Flags

```
      --appInterfaceDir pwd              location of app-interfache checkout. Falls back to pwd and /home/slamba/git/app-interface
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -c, --component string                 Dynatrace component getting promoted
      --context string                   The name of the kubeconfig context to use
      --dynatraceConfigDir pwd           location of dynatrace-config checkout. Falls back to pwd and /home/slamba/git/dynatrace-config
  -g, --gitHash string                   Git hash of the SaaS service/operator commit getting promoted
  -h, --help                             help for dynatrace
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -l, --list                             List all SaaS services/operators
  -m, --module string                    module to promote
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -t, --terraform                        deploy dynatrace-config terraform job
```

### osdctl promote package

Utilities to promote package-operator services

```
osdctl promote package [flags]
```

#### Flags

```
      --appInterfaceDir pwd              location of app-interfache checkout. Falls back to pwd and /home/slamba/git/app-interface
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
      --hcp                              The service being promoted conforms to the HyperShift progressive delivery definition
  -h, --help                             help for package
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
  -n, --serviceName string               Service getting promoted
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -t, --tag string                       Package tag being promoted to
```

### osdctl promote saas

Utilities to promote SaaS services/operators

```
osdctl promote saas [flags]
```

#### Flags

```
      --appInterfaceDir pwd              location of app-interfache checkout. Falls back to pwd and /home/slamba/git/app-interface
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -g, --gitHash string                   Git hash of the SaaS service/operator commit getting promoted
      --hcp                              HCP service/operator getting promoted
  -h, --help                             help for saas
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -l, --list                             List all SaaS services/operators
  -n, --namespaceRef string              SaaS target namespace reference name
      --osd                              OSD service/operator getting promoted
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --serviceName string               SaaS service/operator getting promoted
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl servicelog

OCM/Hive Service log

```
osdctl servicelog [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for servicelog
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl servicelog list

Get service logs for a given cluster identifier.

# To return just service logs created by SREs
osdctl servicelog list --cluster-id=my-cluster-id

# To return all service logs, including those by automated systems
osdctl servicelog list --cluster-id=my-cluster-id --all-messages

# To return all service logs, as well as internal service logs
osdctl servicelog list --cluster-id=my-cluster-id --all-messages --internal


```
osdctl servicelog list --cluster-id <cluster-identifier> [flags] [options]
```

#### Flags

```
  -A, --all-messages                     Toggle if we should see all of the messages or only SRE-P specific ones
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --cluster-id string                Internal Cluster identifier (required)
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for list
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
  -i, --internal                         Toggle if we should see internal messages
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl servicelog post

Post a service log to a cluster or list of clusters

  Docs: https://docs.openshift.com/rosa/logging/sd-accessing-the-service-logs.html

```
osdctl servicelog post --cluster-id <cluster-identifier> [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                Internal ID of the cluster to post the service log to
  -c, --clusters-file string             Read a list of clusters to post the servicelog to. the format of the file is: {"clusters":["$CLUSTERID"]}
      --context string                   The name of the kubeconfig context to use
  -d, --dry-run                          Dry-run - print the service log about to be sent but don't send it.
  -h, --help                             help for post
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
  -i, --internal                         Internal only service log. Use MESSAGE for template parameter (eg. -p MESSAGE='My super secret message').
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -r, --override Info                    Specify a key-value pair (eg. -r FOO=BAR) to replace a JSON key in the document, only supports string fields, specifying -r without -t or -i will use a default template with severity Info and internal_only=True unless these are also overridden.
  -p, --param stringArray                Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.
  -q, --query stringArray                Specify a search query (eg. -q "name like foo") for a bulk-post to matching clusters.
  -f, --query-file stringArray           File containing search queries to apply. All lines in the file will be concatenated into a single query. If this flag is called multiple times, every file's search query will be combined with logical AND.
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
  -t, --template string                  Message template file or URL
  -y, --yes                              Skips all prompts.
```

### osdctl setup

Setup the configuration

```
osdctl setup [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for setup
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl swarm

Provides a set of commands for swarming activity

```
osdctl swarm [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for swarm
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl swarm secondary

Lists unassigned Jira issues from the 'OHSS' project
		for the following Products
		- OpenShift Dedicated
		- Openshift Online Pro
		- OpenShift Online Starter
		- Red Hat OpenShift Service on AWS
		- HyperShift Preview
		- Empty 'Products' field in Jira
		with the 'Summary' field  of the new ticket not matching the following
		- Compliance Alert
		and the 'Work Type' is not one of the RFE or Change Request 

```
osdctl swarm secondary [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for secondary
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl upgrade

Fetch latest osdctl from GitHub and replace the running binary

```
osdctl upgrade [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for upgrade
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

### osdctl version

Display version of osdctl

```
osdctl version [flags]
```

#### Flags

```
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
      --context string                   The name of the kubeconfig context to use
  -h, --help                             help for version
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string                Path to the kubeconfig file to use for CLI requests.
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -S, --skip-version-check               skip checking to see if this is the most recent release
```

