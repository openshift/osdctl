# osd-utils-cli

A toolbox for OSD!

## Overview

osd-utils-cli is a cli tool intended to eliminate toils for SREs when managing OSD related work.

Currently, it mainly supports related work for AWS, especially [aws-account-operator](https://github.com/openshift/aws-account-operator).

## Installation

### Build from source

``` bash
git clone https://github.com/openshift/osd-utils-cli.git
make build
```

Then you can find the `osd-utils-cli` binary file in the `./bin` directory.

### Download from release

TBD

## Usage

For the detailed usage of each command, please refer to [here](./docs/command).

### AWS account cr reset

`reset` command resets the Account CR status and cleans up related secrets.

``` bash
osd-utils-cli reset test-cr

Deleting secret test-cr-osdmanagedadminsre-secret
Deleting secret test-cr-secret
Deleting secret test-cr-sre-cli-credentials
Deleting secret test-cr-sre-console-url
```

### Aws account cr status patch

`set` command enables you to patch Account CR status directly.

```bash
# patch status directly using raw data, the default patch strategy is merge
osd-utils-cli set test-cr --patch='{"status":{"state": "Failed", "claimed": false}}'

# using flags
osd-utils-cli set test-cr --state=Creating
```
