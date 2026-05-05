## osdctl promote saas

Utilities to promote SaaS services/operators

```
osdctl promote saas [flags]
```

### Examples

```

		# List all SaaS services/operators
		osdctl promote saas --list

		# Promote a SaaS service/operator
		osdctl promote saas --serviceId <service> --gitHash <git-hash>
```

### Options

```
      --appInterfaceDir string   Location of app-interface checkout. Falls back to the current working directory
  -g, --gitHash string           Git hash of the repo described by the SaaS file to promote to
  -h, --help                     help for saas
      --hotfix                   Add gitHash to hotfixVersions in app.yml to bypass progressive delivery (requires --gitHash)
  -l, --list                     List all SaaS file names (without the extension)
  -n, --namespaceRef string      SaaS target namespace reference name
      --serviceId string         Name of the SaaS file (without the extension)
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl promote](osdctl_promote.md)	 - Utilities to promote services/operators

