## osdctl account aws-creds rotate

Rotate AWS IAM credentials for a cluster

### Synopsis

Rotates AWS IAM credentials for osdManagedAdmin and/or osdCcsAdmin users.
Runs a diagnostic snapshot first, then performs the rotation with
interactive confirmation.

Use --refresh-secrets to only delete and recreate CredentialRequest secrets
without rotating AWS keys or modifying Hive secrets. This is useful when
CCO needs to re-provision secrets with existing credentials.

AWS credentials are obtained via backplane by default, falling back to the
default AWS credential chain (env vars, ~/.aws/config). Use --aws-profile
to specify a named profile, or --aws-use-env to skip backplane and use
environment credentials directly (e.g. after rh-aws-saml-login).

Pre-flight checks (IAM permissions, secret existence) block rotation by
default. Use --force to allow proceeding past errors with explicit YES
confirmation — only when you are certain the errors are benign.

```
osdctl account aws-creds rotate -C <cluster-id> --reason <reason> [flags]
```

### Examples

```
  # Rotate osdManagedAdmin credentials
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin

  # Rotate osdCcsAdmin credentials (CCS clusters only)
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --ccs-admin

  # Rotate both
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin --ccs-admin

  # Only refresh CredentialRequest secrets (no key rotation)
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --refresh-secrets

  # Dry-run: preview what would happen
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin --dry-run

  # Using rh-aws-saml-login credentials (no backplane)
  kinit $USER@IPA.REDHAT.COM
  eval $(rh-aws-saml-login --output env rhcontrol)
  export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin --aws-use-env

  # With staging cluster and production hive
  osdctl account aws-creds rotate -C $CLUSTER_ID --reason "$JIRA_TICKET" --managed-admin --hive-ocm-url production
```

### Options

```
      --admin-username string   Override the osdManagedAdmin IAM username. Only needed if auto-detection fails (e.g. custom or legacy username)
  -p, --aws-profile string      AWS profile for role chaining. If omitted, tries backplane then falls back to default AWS credential chain
      --aws-use-env             Use AWS credentials from environment variables (e.g. after rh-aws-saml-login), skipping backplane
      --ccs-admin               Rotate osdCcsAdmin credentials (CCS clusters only)
  -C, --cluster-id string       (Required) OCM internal or external cluster ID
      --dry-run                 Preview rotation actions without making changes
      --force                   Allow proceeding past pre-flight errors with YES confirmation. Use only when certain the errors are benign (e.g., known SCP restrictions that won't affect rotation)
  -h, --help                    help for rotate
      --hive-ocm-url string     OCM environment for Hive operations (aliases: production, staging, integration)
  -l, --log-level string        Log level: debug, info, warn, error (default "info")
      --managed-admin           Rotate osdManagedAdmin credentials
  -r, --reason string           (Required) Elevation reason, usually a Jira ticket ID
      --refresh-secrets         Only delete and recreate CredentialRequest secrets (no key rotation)
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

* [osdctl account aws-creds](osdctl_account_aws-creds.md)	 - Diagnose and manage AWS IAM credentials for a cluster

