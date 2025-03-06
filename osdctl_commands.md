# OSDCTL Command Reference

This document provides a comprehensive list of all available osdctl commands, organized by category.

## Promote Commands

* `saas` - Utilities to promote SaaS services/operators
* `dynatrace` - Utilities to promote dynatrace
* `package` - Utilities to promote package-operator services

## Servicelog Commands

* `list` - Get service logs for a given cluster identifier.
* `post` - Post a service log to a cluster or list of clusters

## Cluster Commands

* `from-infra-id` - Get cluster ID and external ID from a given infrastructure ID commonly used by Splunk
* `sre-operators` - SRE operator related utilities
* `detach-stuck-volume` - Detach openshift-monitoring namespace's volume from a cluster forcefully
* `health` - Describes health of cluster nodes and provides other cluster vitals.
* `hypershift-info` - Pull information about AWS objects from the cluster, the management cluster and the privatelink cluster
* `ssh` - utilities for accessing cluster via ssh
* `cpd` - Runs diagnostic for a Cluster Provisioning Delay (CPD)
* `support` - Cluster Support
* `validate-pull-secret` - Checks if the pull secret email matches the owner email
* `resize` - resize control-plane/infra nodes
* `etcd-health-check` - Checks the etcd components and member health
* `logging-check` - Shows the logging support status of a specified cluster
* `resync` - Force a resync of a cluster from Hive
* `check-banned-user` - Checks if the cluster owner is a banned user.
* `owner` - List the clusters owned by the user (can be specified to any user, not only yourself)
* `orgId` - Get the OCM org ID for a given cluster
* `etcd-member-replace` - Replaces an unhealthy etcd node
* `transfer-owner` - Transfer cluster ownership to a new user (to be done by Region Lead)
* `break-glass` - Emergency access to a cluster

## Alert Commands

* `silence` - add, expire and list silence associated with alerts

## Ssh Commands

* `key` - Retrieve a cluster's SSH key from Hive

## Break-Glass Commands

* `cleanup` - Drop emergency access to a cluster

## Hcp Commands

* `must-gather` - Create a must-gather for HCP cluster

## Cloudtrail Commands

* `write-events` - Prints cloudtrail write events to console with optional filtering
* `permission-denied-events` - Prints cloudtrail permission-denied events to console.

## Org Commands

* `aws-accounts` - get organization AWS Accounts
* `describe` - describe organization
* `context` - fetches information about the given organization
* `labels` - get organization labels
* `get` - get organization by users
* `current` - gets current organization
* `customers` - get paying/non-paying organizations
* `users` - get organization users
* `clusters` - get all active organization clusters

## Swarm Commands

* `secondary` - List unassigned JIRA issues based on criteria

## Cost Commands

* `reconcile` - Checks if there's a cost category for every OU. If an OU is missing a cost category, creates the cost category

## Dynatrace Commands

* `url` - Get the Dyntrace Tenant URL for a given MC or HCP cluster
* `gather-logs` - Gather all Pod logs and Application event from HCP
* `logs` - Fetch logs from Dynatrace

## Get Commands

* `secrets` - Get AWS Account CR related secrets
* `aws-account` - Get AWS Account ID
* `legal-entity` - Get AWS Account Legal Entity

## Mgmt Commands

* `iam` - Creates an IAM user in a given AWS account and prints out the credentials
* `assign` - Assign account to user
* `unassign` - Unassign account to user

## Clusterdeployment Commands

* `listresources` - List all resources on a hive cluster related to a given cluster

## Iampermissions Commands

* `diff` - Diff iam permissions for cluster operators between two versions
* `save` - Save iam permissions for use in mcc

## Support Commands

* `status` - Shows the support status of a specified cluster

## Jumphost Commands

* `delete` - Delete a jumphost created by `osdctl jumphost create`
* `create` - Create a jumphost for emergency SSH access to a cluster's VMs

## General Commands

* `osdctl` - OSD CLI
* `aao` - AWS Account Operator Debugging Utilities
* `servicelog` - OCM/Hive Service log
* `upgrade` - Upgrade osdctl
* `alert` - List alerts
* `env` - Create an environment to interact with a cluster
* `iampermissions` - STS/WIF utilities
* `promote` - Utilities to promote services/operators
* `jira` - Provides a set of commands for interacting with Jira
* `swarm` - Provides a set of commands for swarming activity
* `cluster` - Provides information for a specified cluster
* `jumphost` - 
* `version` - Display the version
* `network` - network related utilities
* `setup` - Setup the configuration
* `hcp` - 
* `mc` - 
* `cloudtrail` - AWS CloudTrail related utilities
* `org` - Provides information for a specified organization
* `cost` - Cost Management related utilities
* `hive` - hive related utilities

## Hive Commands

* `clustersync-failures` - List clustersync failures
* `clusterdeployment` - cluster deployment related utilities

## List Commands

* `account-claim` - List AWS Account Claim CR
* `account` - List AWS Account CR

## Aao Commands

* `pool` - Get the status of the AWS Account Operator AccountPool

## Resize Commands

* `control-plane` - Resize an OSD/ROSA cluster's control plane nodes
* `infra` - Resize an OSD/ROSA cluster's infra nodes

## Network Commands

* `packet-capture` - Start packet capture
* `verify-egress` - Verify an AWS OSD/ROSA cluster can reach all required external URLs necessary for full support.

## Silence Commands

* `expire` - Expire Silence for alert
* `add` - Add new silence for alert

## Jira Commands

* `quick-task` - creates a new ticket with the given name

## Account Commands

* `rotate-secret` - Rotate IAM credentials secret
* `clean-velero-snapshots` - Cleans up S3 buckets whose name start with managed-velero
* `verify-secrets` - Verify AWS Account CR IAM User credentials
* `reset` - Reset AWS Account CR
* `set` - Set AWS Account CR status
* `servicequotas` - Interact with AWS service-quotas
* `generate-secret` - Generates IAM credentials secret
* `mgmt` - AWS Account Management
* `console` - Generate an AWS console URL on the fly
* `cli` - Generate temporary AWS CLI credentials on demand

