## osdctl rhobs alerts silences create

Create a silence at RHOBS cell level

### Synopsis

Create a silence at RHOBS cell level. The mandatory selector argument filters the alerts on which the silence will apply; Use ==, !=, =~ and !~ as an operator between a label key and its value; for instance: key1=value1,key2!=value2; Use a comma to separate the constraints if more than one or repeat this argument: key1==value1 key2!=value2; Special characters (like the ones used in operators) need to be back-slashed if present in a key or, more likely, in a value; same applies to the backslash character itself.

```
osdctl rhobs alerts silences create selector [flags]
```

### Options

```
      --author string           Name of the person creating the silence (default to the OS user name)
      --comment string          Some free text giving some context around why the silence is created - you can give JIRA or other references there
      --end-time time           Time at which the silence will expire - Mandatory unless --expire-after is set
      --expire-after duration   Duration (e.g. 24h, 30m) after which the silence will expire - exclusive with --start-time & --end-time
  -h, --help                    help for create
      --start-time time         Time at which the silence will start to take effect (defaults to now)
```

### Options inherited from parent commands

```
  -C, --cluster-id string     Name or Internal ID of the cluster (defaults to current cluster context)
      --hive-ocm-url string   OCM environment URL for hive operations - aliases: "production", "staging", "integration" (default "production")
  -S, --skip-version-check    skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl rhobs alerts silences](osdctl_rhobs_alerts_silences.md)	 - The alerts silences defined at RHOBS cell level

