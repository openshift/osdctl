## osdctl jira quick-task

creates a new ticket with the given name

### Synopsis

Creates a new ticket with the given name and a label specified by "jira_team_label" from the osdctl config. The flags "jira_board_id" and "jira_team" are also required for running this command.
The ticket will be assigned to the caller and added to their team's current sprint as an OSD Task.
A link to the created ticket will be printed to the console.

```
osdctl jira quick-task <title> [flags]
```

### Examples

```
#Create a new Jira issue
osdctl jira quick-task "Update command to take new flag"

#Create a new Jira issue and add to the caller's current sprint
osdctl jira quick-task "Update command to take new flag" --add-to-sprint

```

### Options

```
      --add-to-sprint   whether or not to add the created Jira task to the SRE's current sprint.
  -h, --help            help for quick-task
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

* [osdctl jira](osdctl_jira.md)	 - Provides a set of commands for interacting with Jira

