## osdctl promote managedscripts

Promote https://github.com/openshift/managed-scripts

```
osdctl promote managedscripts [flags]
```

### Examples

```

		# Promote managed-scripts repo
		osdctl promote managedscripts --gitHash <git-hash>
```

### Options

```
      --appInterfaceDir string   location of app-interface checkout. Falls back to current working directory
  -g, --gitHash string           Git hash of the managed-scripts repo commit getting promoted
  -h, --help                     help for managedscripts
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl promote](osdctl_promote.md)	 - Utilities to promote services/operators

