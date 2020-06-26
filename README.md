# osdctl

A toolbox for OSD!

## Overview

osdctl is a cli tool intended to eliminate toils for SREs when managing OSD related work.

Currently, it mainly supports related work for AWS, especially [aws-account-operator](https://github.com/openshift/aws-account-operator).

## Installation

### Build from source

``` bash
git clone https://github.com/openshift/osd-utils-cli.git
make build
```

Then you can find the `osdctl` binary file in the `./bin` directory.

### Download from release

TBD

## Run tests

``` bash
make test
```

## Usage

For the detailed usage of each command, please refer to [here](./docs/command).

### AWS Account CR reset

`reset` command resets the Account CR status and cleans up related secrets.

``` bash
osdctl reset test-cr

Deleting secret test-cr-osdmanagedadminsre-secret
Deleting secret test-cr-secret
Deleting secret test-cr-sre-cli-credentials
Deleting secret test-cr-sre-console-url
```

### AWS Account CR status patch

`set` command enables you to patch Account CR status directly. 

There are two ways of status patching:

1. Using flags.

``` bash
osdctl set test-cr --state=Creating -r=true
```

2. Using raw data. For patch strategy, only `merge` and `json` are supported. The default is `merge`. 

```bash
osdctl set test-cr --patch='{"status":{"state": "Failed", "claimed": false}}'
```

### AWS Account CR list

`list account` command lists the Account CRs in the cluster. You can use flags to filter the status.

```bash
osdctl list account --state=Creating

Name                State               AWS ACCOUNT ID      Last Probe Time                 Last Transition Time            Message
test-cr             Creating            181787396432        2020-06-18 10:38:40 -0400 EDT   2020-06-18 10:38:40 -0400 EDT   AWS account already created
```

### AWS Account Console URL generate

`console` command generates an AWS console URL for the specified Account CR or AWS Account ID.

```bash
# generate console URL via Account CR name
osdctl console -a test-cr

# generate console URL via AWS Account ID
osdctl console -i 1111111111
```

### AWS Account Operator metrics display

```bash
osdctl metrics

aws_account_operator_pool_size_vs_unclaimed{name="aws-account-operator"} => 893.000000
aws_account_operator_total_account_crs{name="aws-account-operator"} => 2173.000000
aws_account_operator_total_accounts_crs_claimed{name="aws-account-operator"} => 436.000000
......
```

### Cleanup Velero managed snapshots

`clean-velero-snapshots` command cleans up the Velero managed buckets for the specified Account.

```bash
# clean up by providing the credentials via flags
osdctl clean-velero-snapshots -a <AWS ACCESS KEY ID> -x <AWS SECRET ACCESS KEY>

# if flags are not provided, it will get credentials from credentials file,
# we also support specifying profile and config file path
osdctl clean-velero-snapshots -p <profile name> -c <config file path>
```
