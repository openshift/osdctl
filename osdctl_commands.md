# OSDCTL Command Reference

This document provides a comprehensive list of all available osdctl commands, organized by category.

## Hcp Commands

* `must-gather` - Create a must-gather for HCP cluster

## Iampermissions Commands

* `save` - Save iam permissions for use in mcc
* `diff` - Diff iam permissions for cluster operators between two versions

## Silence Commands

* `add` - Add new silence for alert
* `expire` - Expire Silence for alert

## Network Commands

* `verify-egress` - Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.
* `packet-capture` - Start packet capture

## Clusterdeployment Commands

* `listresources` - List all resources on a hive cluster related to a given cluster

## Alert Commands

* `silence` - add, expire and list silence associated with alerts

## Promote Commands

* `dynatrace` - Utilities to promote dynatrace
* `saas` - Utilities to promote SaaS services/operators
* `package` - Utilities to promote package-operator services

## Cloudtrail Commands

* `write-events` - Prints cloudtrail write events to console with optional filtering
* `permission-denied-events` - Prints cloudtrail permission-denied events to console.

## Account Commands

* `cli` - Generate temporary AWS CLI credentials on demand
* `verify-secrets` - Verify AWS Account CR IAM User credentials
* `generate-secret` - Generates IAM credentials secret
* `set` - Set AWS Account CR status
* `servicequotas` - Interact with AWS service-quotas
* `reset` - Reset AWS Account CR
* `rotate-secret` - Rotate IAM credentials secret
* `clean-velero-snapshots` - Cleans up S3 buckets whose name start with managed-velero
* `console` - Generate an AWS console URL on the fly
* `mgmt` - AWS Account Management

## Cost Commands

* `reconcile` - Checks if there's a cost category for every OU. If an OU is missing a cost category, creates the cost category

## List Commands

* `account-claim` - List AWS Account Claim CR
* `account` - List AWS Account CR

## Cluster Commands

* `health` - Describes health of cluster nodes and provides other cluster vitals.
* `logging-check` - Shows the logging support status of a specified cluster
* `check-banned-user` - Checks if the cluster owner is a banned user.
* `resize` - resize control-plane/infra nodes
* `from-infra-id` - Get cluster ID and external ID from a given infrastructure ID commonly used by Splunk
* `detach-stuck-volume` - Detach openshift-monitoring namespace's volume from a cluster forcefully
* `resync` - Force a resync of a cluster from Hive
* `validate-pull-secret` - Checks if the pull secret email matches the owner email
* `owner` - List the clusters owned by the user (can be specified to any user, not only yourself)
* `ssh` - utilities for accessing cluster via ssh
* `etcd-health-check` - Checks the etcd components and member health
* `cpd` - Runs diagnostic for a Cluster Provisioning Delay (CPD)
* `etcd-member-replace` - Replaces an unhealthy etcd node
* `orgId` - Get the OCM org ID for a given cluster
* `hypershift-info` - Pull information about AWS objects from the cluster, the management cluster and the privatelink cluster
* `transfer-owner` - Transfer cluster ownership to a new user (to be done by Region Lead)
* `break-glass` - Emergency access to a cluster
* `support` - Cluster Support
* `sre-operators` - SRE operator related utilities

## Break-Glass Commands

* `cleanup` - Drop emergency access to a cluster

## Jumphost Commands

* `create` - Create a jumphost for emergency SSH access to a cluster's VMs
* `delete` - Delete a jumphost created by `osdctl jumphost create`

## Hive Commands

* `clustersync-failures` - List clustersync failures
* `clusterdeployment` - cluster deployment related utilities

## General Commands

* `aao` - AWS Account Operator Debugging Utilities
* `iampermissions` - STS/WIF utilities
* `env` - Create an environment to interact with a cluster
* `org` - Provides information for a specified organization
* `cost` - Cost Management related utilities
* `hcp` - 
* `mc` - 
* `cloudtrail` - AWS CloudTrail related utilities
* `osdctl` - OSD CLI
* `jumphost` - 
* `cluster` - Provides information for a specified cluster
* `promote` - Utilities to promote services/operators
* `alert` - List alerts
* `upgrade` - Upgrade osdctl
* `hive` - hive related utilities
* `setup` - Setup the configuration
* `swarm` - Provides a set of commands for swarming activity
* `version` - Display the version
* `network` - network related utilities
* `jira` - Provides a set of commands for interacting with Jira
* `servicelog` - OCM/Hive Service log

## Resize Commands

* `control-plane` - Resize an OSD/ROSA cluster's control plane nodes
* `infra` - Resize an OSD/ROSA cluster's infra nodes

## Servicelog Commands

* `post` - Post a service log to a cluster or list of clusters
* `list` - Get service logs for a given cluster identifier.

## Jira Commands

* `quick-task` - creates a new ticket with the given name

## Mgmt Commands

* `unassign` - Unassign account to user
* `assign` - Assign account to user
* `iam` - Creates an IAM user in a given AWS account and prints out the credentials

## Ssh Commands

* `key` - Retrieve a cluster's SSH key from Hive

## Swarm Commands

* `secondary` - List unassigned JIRA issues based on criteria

## Dynatrace Commands

* `logs` - Fetch logs from Dynatrace
* `gather-logs` - Gather all Pod logs and Application event from HCP
* `url` - Get the Dyntrace Tenant URL for a given MC or HCP cluster

## Aao Commands

* `pool` - Get the status of the AWS Account Operator AccountPool

## Get Commands

* `secrets` - Get AWS Account CR related secrets
* `legal-entity` - Get AWS Account Legal Entity
* `aws-account` - Get AWS Account ID

## Support Commands

* `status` - Shows the support status of a specified cluster

## Org Commands

* `aws-accounts` - get organization AWS Accounts
* `current` - gets current organization
* `customers` - get paying/non-paying organizations
* `users` - get organization users
* `labels` - get organization labels
* `context` - fetches information about the given organization
* `describe` - describe organization
* `get` - get organization by users
* `clusters` - get all active organization clusters

