## osdctl account aws-creds snapshot

Show a read-only credential status report for a cluster

### Synopsis

Produces a diagnostic report of AWS IAM credentials including:
  - IAM access keys and which Hive secrets reference them
  - CredentialRequest secrets and whether they need refresh
  - IAM permission simulation (SCP/policy restriction detection)

Use --cr-secrets to show only the CredentialRequest secrets table.

This is a read-only operation — no credentials are modified.

AWS credentials are obtained via backplane by default, falling back to the
default AWS credential chain (env vars, ~/.aws/config). Use --aws-profile
to specify a named profile, or --aws-use-env to skip backplane and use
environment credentials directly (e.g. after rh-aws-saml-login).

```
osdctl account aws-creds snapshot -C <cluster-id> --reason <reason> [flags]
```

### Examples

```
  # Full credential status report (uses backplane)
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET"

  # Only show CredentialRequest secret status
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET" --cr-secrets

  # Using rh-aws-saml-login credentials (no backplane)
  kinit $USER@IPA.REDHAT.COM
  eval $(rh-aws-saml-login --output env rhcontrol)
  export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET" --aws-use-env

  # With staging cluster and production hive
  osdctl account aws-creds snapshot -C $CLUSTER_ID --reason "$JIRA_TICKET" --hive-ocm-url production
```

### Options

```
      --admin-username string   Override the osdManagedAdmin IAM username. Only needed if auto-detection fails (e.g. custom or legacy username)
  -p, --aws-profile string      AWS profile for role chaining. If omitted, tries backplane then falls back to default AWS credential chain
      --aws-use-env             Use AWS credentials from environment variables (e.g. after rh-aws-saml-login), skipping backplane
  -C, --cluster-id string       (Required) OCM internal or external cluster ID
      --cr-secrets              Only show CredentialRequest secrets status
  -h, --help                    help for snapshot
      --hive-ocm-url string     OCM environment for Hive operations (aliases: production, staging, integration)
  -l, --log-level string        Log level: debug, info, warn, error (default "info")
  -r, --reason string           (Required) Elevation reason, usually a Jira ticket ID
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

