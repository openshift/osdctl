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

`verify-secrets` command verifies the IAM User Secret associated with Account Accout CR.

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
osdctl get aws-account -c <Claim Name> -n <Claim Namepace>
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
