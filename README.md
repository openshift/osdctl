# osdctl

A toolbox for OSD!

## Overview

osdctl is a cli tool intended to eliminate toils for SREs when managing OSD related work.

Currently, it mainly supports related work for AWS, especially [aws-account-operator](https://github.com/openshift/aws-account-operator).

## Installation

### Build from source

#### Requirements

- Go >= 1.13
- make
- [goreleaser](https://github.com/goreleaser)

```shell
# Goreleaser is required for builds,
# but can be downloaded with the `make download-goreleaser` target

git clone https://github.com/openshift/osdctl.git
make download-goreleaser # only needs to be done once
make build
```

Then you can find the `osdctl` binary file in the `./dist` subdirectory matching your architecture.

### Download from release

Release are available on Github

### Creating a release

Repository owners can create a new `osdctl` release with the `make release` target. An API token with `repo` permissions is required. [See: https://goreleaser.com/environment/#api-tokens](https://goreleaser.com/environment/#api-tokens)

The goreleaser config (`.goreleaser.yaml`) will look for the token in `~/.config/goreleaser/token`.

Goreleaser uses the latest Git tag from the repository to create a release. To make a new release, create a new Git tag:

```shell
# Creating a new osdctl Github release

# Create a git tag to be the basis of the release
git tag -a vX.Y.Z -m "new release message"
git push origin vX.Y.Z

# Create the release
make release
```

## Run tests

``` bash
make test
```

## Usage

For the detailed usage of each command, please refer to [here](./docs/command).

### AWS Account CR reset

`reset` command resets the Account CR status and cleans up related secrets.

``` bash
osdctl account reset test-cr
Reset account test-cr? (Y/N) y

Deleting secret test-cr-secret
Deleting secret test-cr-sre-cli-credentials
Deleting secret test-cr-sre-console-url
```

You can skip the prompt by adding a flag `-y`, but it is not recommended.

```bash
osdctl account reset test-cr -y
```

### AWS Account CR status patch

`set` command enables you to patch Account CR status directly.

There are two ways of status patching:

1. Using flags.

``` bash
osdctl account set test-cr --state=Creating -r=true
```

2. Using raw data. For patch strategy, only `merge` and `json` are supported. The default is `merge`.

```bash
osdctl account set test-cr --patch='{"status":{"state": "Failed", "claimed": false}}'
```

### AWS Account CR list

`list account` command lists the Account CRs in the cluster. You can use flags to filter the status.

```bash
osdctl account list account --state=Creating

Name                State               AWS ACCOUNT ID      Last Probe Time                 Last Transition Time            Message
test-cr             Creating            181787396432        2020-06-18 10:38:40 -0400 EDT   2020-06-18 10:38:40 -0400 EDT   AWS account already created

# filter accounts by reused or claimed status
osdctl account list --reuse=true --claim=false

# custom output using jsonpath
osdctl account list -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.awsAccountID}{"\t"}{.status.state}{"\n"}{end}'
test-cr             Creating            111111111111        2020-06-18 10:38:40 -0400 EDT   2020-06-18 10:38:40 -0400 EDT   AWS account already created
```

### AWS Account Claim CR list

`list account-claim` command lists the Account Claim CRs in the cluster. You can use flags to filter the status.

```bash
osdctl account list account-claim --state=Ready
```

### AWS Account Mgmt Assign

`assign` command assigns a developer account to a user

```bash
osdctl account mgmt assign -u <LDAP username> -p <profile name>
```

### AWS Account Mgmt list

`list` command lists the owner of an AWS account given an account id, or the account id(s) given an LDAP username. 
If neither an account id or username is provided, it lists all the accounts from the developers OU

```bash
# list LDAP username
osdctl account mgmt list -i 111111111111 -p <profile name>

# list account(s) from user
osdctl account mgmt list -u <LDAP username> -p <profile name>

# list all accounts in the developer OU
osdctl account mgmt list -p <profile name>
```

### AWS Account Mgmt Unassign

`unassign` command takes either an LDAP username or account ID and removes any IAM users, Roles, and Policies

```bash
# unassigns all accounts from user
osdctl account mgmt unassign -u <LDAP username> -p <profile name>

# cleans up specified account along with its user
osdctl account mgmt unassign -i <account ID> -p <profile name>
```

### AWS Account Console URL generate

`console` command generates an AWS console URL for the specified Account CR or AWS Account ID.

```bash
# generate console URL via Account CR name
osdctl account console -a test-cr

# generate console URL via AWS Account ID
osdctl account console -i 1111111111

# The --launch flag will open the url in the browser
osdctl account console -i 1111111111 --launch
```

### Cleanup Velero managed snapshots

`clean-velero-snapshots` command cleans up the Velero managed buckets for the specified Account.

```bash
# clean up by providing the credentials via flags
osdctl account clean-velero-snapshots -a <AWS ACCESS KEY ID> -x <AWS SECRET ACCESS KEY>

# if flags are not provided, it will get credentials from credentials file,
# we also support specifying profile and config file path
osdctl account clean-velero-snapshots -p <profile name> -c <config file path>
```

### AWS Account IAM User Credentials validation

`verify-secrets` command verifies the IAM User Secret associated with Account Account CR.

```bash
# no argument, verify all account secrets
osdctl account verify-secrets

# specify the Account CR name, then only verify the IAM User Secret for that Account.
osdctl account verify-secrets <Account CR Name>
```

### Match AWS Account with AWS Account Operator related resources

1. Get AWS Account Operator related resources

```bash
# Get Account Name by AWS Account ID, output to json
osdctl account get account -i <Account ID> -o json

# Get Account Claim CR by Account CR Name
osdctl account get account-claim -a <Account CR Name>

# Get Account Claim CR by AWS Account ID, output to yaml
osdctl account get account-claim -i <Account ID> -o yaml

# Get Legal Entity information by AWS Account ID
osdctl account get legal-entity -i <Account ID>

# Get Secrets information by AWS Account ID
osdctl account get secrets -i <Account ID>

test-cr-secret
```

2. Get AWS Account ID

```bash
# Get AWS Account ID by Account CR Name
osdctl get aws-account -a <Account CR Name>

# Get AWS Account ID by Account Claim CR Name and Namespace
osdctl get aws-account -c <Claim Name> -n <Claim Namespace>
```

### Rotate AWS IAM Credentials

`rotate-secret` command rotates the credentials for one IAM User, it will print out the generated secret by default.

```bash
# specify by Account ID
osdctl account rotate-secret <IAM Username> -i 1111111111

# specify by Account CR Name
osdctl account rotate-secret <IAM Username> -a test-cr

# output the new secret to a path
osdctl account rotate-secret <IAM Username> -a test-cr --output=/test/secret --secret-name=secret
```

### AWS Account Operator metrics display

```bash
osdctl metrics

aws_account_operator_pool_size_vs_unclaimed{name="aws-account-operator"} => 893.000000
aws_account_operator_total_account_crs{name="aws-account-operator"} => 2173.000000
aws_account_operator_total_accounts_crs_claimed{name="aws-account-operator"} => 436.000000
......
```

### Get cluster policy and policy-diff

`policy` command saves the crs files in /tmp/crs- directory for given `x.y.z` release version. `policy-diff` command, in addition, compares the files of directories and outputs the diff.

```bash
osdctl sts policy <OCP version>
osdctl sts policy-diff <old version> <new version>
```

### Hive ClusterDeployment CR list

```bash
osdctl clusterdeployment list
```

### AWS Account Federated Role Apply

```bash
# apply via URL
osdctl federatedrole apply -u <URL>

# apply via local file
osdctl federatedrole apply -f <yaml file>
```

### Send a servicelog to a cluster

#### List servicelogs

```bash
# list current servicelogs
CLUSTERID= # can be internal/external/name, but should be unique enough
osdctl servicelog list ${CLUSTERID}

# show all servicelogs (not only ones sent by SREP)
CLUSTERID= # can be internal/external/name, but should be unique enough
osdctl servicelog list ${CLUSTERID} --all-messages
```

#### Post servicelogs

```bash
CLUSTER_ID= # the unique cluster name, or internal, external id for a cluster
TEMPLATE= # file or url in which the template exists in
osdctl servicelog post ${CLUSTER_ID} --template=${TEMPLATE} --dry-run

QUERIES_HERE= # queries that can be run on ocm's `clusters` resource
TEMPLATE= # file or url in which the template exists in
osdctl servicelog post --template=${TEMPLATE} --query=${QUERIES_HERE} --dry-run

QUERIES_HERE= # queries that can be run on ocm's `clusters` resource
# to test the queries you can run:
# ocm list clusters --parameter search="${QUERIES_HERE}"
cat << EOF > query_file.txt
${QUERIES_HERE}
EOF
TEMPLATE= # file or url in which the template exists in
osdctl servicelog post --template=${TEMPLATE} --query-file=query_file.txt --dry-run

CLUSTER_ID= # the unique cluster name, or internal, external id for a cluster
ANOTHER_CLUSTER_ID= # similar, but shows how to  have multiple clusters as input
# clusters_list.json will have the custom list of clusters to iterate on
cat << EOF > clusters_list.json
{
  "clusters": [
    "${CLUSTER_ID}",
    "${ANOTHER_CLUSTER_ID}"
  ]
}
EOF
# post servicelog to a custom set of clusters
# EXTERNAL_ID is inferred here from the `--clusters-file`
TEMPLATE= # file or url in which the template exists in
osdctl servicelog post --clusters-file=clusters_list.json --template=${TEMPLATE} --dry-run
```

### Cluster environments

`osdctl env` can be used to log in to several OpenShift clusters at the same time.
Each cluster is referred to by a user-defined alias.
`osdctl env` creates a directory in `~/ocenv/` for each cluster named by the alias.
It contains an `.ocenv` that will set `$KUBECONFIG` and `$OCM_CONFIG` when the environment is started.

You can run `osdctl env my-cluster` to create a new environment or switch between environments.
Each environment will use a separate `$KUBECONFIG` so you can easily switch between them.
`my-cluster` in this case is an alias that you can use to identify you cluster later.

Optionally, you can run `osdctl env -c my-cluster-id my-cluster` to set the `$CLUSTERID` variable in the environment.
This is useful to log in to OSD clusters.
When using `ocm` you can use the shorthands `ocl` to log in to the cluster, `oct` to create a tunnel when inside the environment, and `ocb` to log in with the backplane plugin.

You can leave an environment by pressing `ctrl+D`.


### OCM Environment Auto-detection

You can let osdctl detect the OCM environment and select a login script based on the environment you're currently logged in.
This will spare you from having to pass a script with the `-l` argument each time you log in.
To use this feature, provide your login scripts in the config file `~/.osdctl.yaml` like in the following example:

```
loginScripts:
  https://api.stage.openshift.com: ocm-stage-login
  https://api.openshift.com: ocm-prod-login
  https://api.integration.openshift.com: ocm-int-login
```

#### Example workflows

##### Use backplane to log in to OSD cluster and come back later

```
$ osdctl env -l prod-login.sh -c hf203489-23fsdf-23rsdf my-cluster
$ ocb # login to the cluster
$ exit # tunnel and login loop will be closed on exit
...
$ osdctl env my-cluster # no need to setup and remember everything again
$ ocb # login to the cluster
$ exit
```

##### Create a temporary environment for a quick investigation

```
$ osdctl env -l prod-login.sh -t -c hf203489-23fsdf-23rsdf
$ ocb # login to the cluster
$ oc get pods .... # investigate
$ exit # tunnel and login loop will be closed on exit, environment will be cleaned up.
```

##### Use KUBECONFIG outside of the env

```
$ osdctl env -l prod-login.sh -t -c hf203489-23fsdf-23rsdf my-cluster
$ ocb # login to the cluster
... in some other shell ...
$ `osdctl env -k my-cluster` # use KUBECONFIG from environment
$ oc get pods ...
```

##### Logging in to Individual Clusters

`osdctl env` supports creating environments for non-ocm-managed clusters as well.
You can either provide an API URL or an existing KUBECONFIG.

###### With Username and Password

Set username, API url, and (optionally) password

```
$ osdctl env -u myuser -p topsecret -a https://api.mycluster.com:6443 mycluster
```

Careful: The password will be stored in clear text if you pass it.
In most cases it will be better to read it from STDIN on login.

log in with `ocl`

```
$ ocl
```

###### With existing KUBECONFIG

Log in with a kubeconfig that exists in the filesystem:

```
$ osdctl env --kubeconfig ~/kube/config mycluster
```

Log in with a kubeconfig from clipboard (linux with xclip):

```
$ osdctl env --kubeconfig <(xclip -o) mycluster
```

Log in with a kubeconfig from clipboard (Mac):

```
$ osdctl env --kubeconfig <(pbpaste) mycluster
```

### Network Utilities
#### OSD network verifier
1. Egress test - [SOP](https://github.com/openshift/ops-sop/blob/master/v4/knowledge_base/osd-network-verifier.md)
```
$ osdctl network verify-egress --subnet-id=$(SUBNET-ID) --region=$(REGION)
```