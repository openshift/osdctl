## osdctl rhobs hcp-dashboard

Get the HCP dashboard URL for a given HCP cluster

### Synopsis

Get the HCP dashboard URL for a given HCP cluster. The dashboard name is optional and defaults to the hosted cluster dashboard. Allowed values for the dashboard name are: hosted-cluster, management-cluster, kube-apis-slo, clusters-creation-slo, control-planes-upgrade-slo, nodepools-upgrade-slo, nodepools-slo, counters. The URL of the RHOBS cell(s) to use can be specified with the --rhobs-cell option, but it is usually more convenient to specify the cluster with the --cluster-id option and let the command figure out the RHOBS cell(s) to use. Note that the --rhobs-cell option is not working with all dashboards and cannot be used together with the --cluster-id option.

```
osdctl rhobs hcp-dashboard [dashboard-name] [flags]
```

### Options

```
  -b, --browser             Open in the URL in the default browser
  -h, --help                help for hcp-dashboard
  -c, --rhobs-cell string   RHOBS cell URL - for instance: https://us-east-1-0.rhobs.api.stage.openshift.com - use a comma to separate the RHOBS cell to use for metrics from the logs RHOBS cell if they are different - this option is not working with all dashboards - exclusive with --cluster-id
```

### Options inherited from parent commands

```
  -C, --cluster-id string     Name or Internal ID of the cluster (defaults to current cluster context)
      --hive-ocm-url string   OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
  -S, --skip-version-check    skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl rhobs](osdctl_rhobs.md)	 - RHOBS.next related utilities

