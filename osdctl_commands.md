# OSDCTL Command Reference

This document provides a comprehensive list of all available osdctl commands, organized by category.

## Iampermissions Commands

* `save` - Save iam permissions for use in mcc
* `diff` - Diff iam permissions for cluster operators between two versions

## Org Commands

* `labels` - get organization labels
* `describe` - describe organization
* `get` - get organization by users
* `users` - get organization users
* `context` - fetches information about the given organization
* `current` - gets current organization
* `aws-accounts` - get organization AWS Accounts
* `customers` - get paying/non-paying organizations
* `clusters` - get all active organization clusters

## Cloudtrail Commands

* `permission-denied-events` - Prints cloudtrail permission-denied events to console.
* `write-events` - Prints cloudtrail write events to console with optional filtering

## Dynatrace Commands

* `logs` - Fetch logs from Dynatrace
* `gather-logs` - Gather all Pod logs and Application event from HCP
* `url` - Get the Dyntrace Tenant URL for a given MC or HCP cluster

## Ssh Commands

* `key` - Retrieve a cluster's SSH key from Hive

## Hcp Commands

* `must-gather` - Create a must-gather for HCP cluster

## Cluster Commands

* `validate-pull-secret` - Checks if the pull secret email matches the owner email
* `orgId` - Get the OCM org ID for a given cluster
* `etcd-member-replace` - Replaces an unhealthy etcd node
* `detach-stuck-volume` - Detach openshift-monitoring namespace's volume from a cluster forcefully
* `ssh` - utilities for accessing cluster via ssh
* `cpd` - Runs diagnostic for a Cluster Provisioning Delay (CPD)
* `support` - Cluster Support
* `sre-operators` - SRE operator related utilities
* `etcd-health-check` - Checks the etcd components and member health
* `transfer-owner` - Transfer cluster ownership to a new user (to be done by Region Lead)
* `from-infra-id` - Get cluster ID and external ID from a given infrastructure ID commonly used by Splunk
* `check-banned-user` - Checks if the cluster owner is a banned user.
* `resync` - Force a resync of a cluster from Hive
* `break-glass` - Emergency access to a cluster
* `resize` - resize control-plane/infra nodes
* `hypershift-info` - Pull information about AWS objects from the cluster, the management cluster and the privatelink cluster
* `owner` - List the clusters owned by the user (can be specified to any user, not only yourself)
* `health` - Describes health of cluster nodes and provides other cluster vitals.
* `logging-check` - Shows the logging support status of a specified cluster

## General Commands

* `cost` - Cost Management related utilities
* `cluster` - Provides information for a specified cluster
* `jumphost` - 
* `servicelog` - OCM/Hive Service log
* `version` - Display the version
* `promote` - Utilities to promote services/operators
* `hcp` - 
* `network` - network related utilities
* `upgrade` - Upgrade osdctl
* `cloudtrail` - AWS CloudTrail related utilities
* `iampermissions` - STS/WIF utilities
* `mc` - 
* `setup` - Setup the configuration
* `dynatrace` - Dynatrace related utilities
* `osdctl` - OSD CLI
* `swarm` - Provides a set of commands for swarming activity
* `hive` - hive related utilities
* `env` - Create an environment to interact with a cluster
* `alert` - List alerts
* `jira` - Provides a set of commands for interacting with Jira
* `aao` - AWS Account Operator Debugging Utilities
* `org` - Provides information for a specified organization

## Resize Commands

* `infra` - Resize an OSD/ROSA cluster's infra nodes
* `control-plane` - Resize an OSD/ROSA cluster's control plane nodes

## Promote Commands

* `package` - Utilities to promote package-operator services
* `saas` - Utilities to promote SaaS services/operators

## Clusterdeployment Commands

* `listresources` - List all resources on a hive cluster related to a given cluster

## Alert Commands

* `silence` - add, expire and list silence associated with alerts

## Mgmt Commands

* `assign` - Assign account to user
* `unassign` - Unassign account to user
* `iam` - Creates an IAM user in a given AWS account and prints out the credentials

## Account Commands

* `cli` - Generate temporary AWS CLI credentials on demand
* `console` - Generate an AWS console URL on the fly
* `set` - Set AWS Account CR status
* `servicequotas` - Interact with AWS service-quotas
* `rotate-secret` - Rotate IAM credentials secret
* `generate-secret` - Generates IAM credentials secret
* `verify-secrets` - Verify AWS Account CR IAM User credentials
* `reset` - Reset AWS Account CR
* `mgmt` - AWS Account Management
* `clean-velero-snapshots` - Cleans up S3 buckets whose name start with managed-velero

## Break-Glass Commands

* `cleanup` - Drop emergency access to a cluster

## Aao Commands

* `pool` - Get the status of the AWS Account Operator AccountPool

## Support Commands

* `status` - Shows the support status of a specified cluster

## Servicelog Commands

* `post` - Post a service log to a cluster or list of clusters
* `list` - Get service logs for a given cluster identifier.

## Cost Commands

* `reconcile` - Checks if there's a cost category for every OU. If an OU is missing a cost category, creates the cost category

## Jira Commands

* `quick-task` - creates a new ticket with the given name

## Swarm Commands

* `secondary` - List unassigned JIRA issues based on criteria

## Get Commands

* `legal-entity` - Get AWS Account Legal Entity
* `aws-account` - Get AWS Account ID
* `secrets` - Get AWS Account CR related secrets

## Jumphost Commands

* `create` - Create a jumphost for emergency SSH access to a cluster's VMs
* `delete` - Delete a jumphost created by `osdctl jumphost create`

## List Commands

* `account` - List AWS Account CR
* `account-claim` - List AWS Account Claim CR

## Silence Commands

* `expire` - Expire Silence for alert
* `add` - Add new silence for alert

## Hive Commands

* `clusterdeployment` - cluster deployment related utilities
* `clustersync-failures` - List clustersync failures

## Network Commands

* `packet-capture` - Start packet capture
* `verify-egress` - Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.

