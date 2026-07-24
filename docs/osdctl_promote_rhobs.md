## osdctl promote rhobs

Promote RHOBS configuration to production

```
osdctl promote rhobs [flags]
```

### Examples

```

		# List all RHOBS services
		osdctl promote rhobs --list

		# Promote all RHOBS services to the latest rhobs/configuration main
		osdctl promote rhobs --latest

		# Promote all RHOBS services to a specific git hash
		osdctl promote rhobs --gitHash <git-hash>

		# Promote a single RHOBS service
		osdctl promote rhobs --serviceId saas-hcp-rules --gitHash <git-hash>
```

### Options

```
      --appInterfaceDir string   Location of app-interface checkout
      --configRepoDir string     Location of rhobs/configuration checkout (auto-detected from ~/src/)
  -g, --gitHash string           Git hash of rhobs/configuration to promote to (required for bulk promotion; defaults to HEAD for --serviceId)
  -h, --help                     help for rhobs
      --latest                   Promote all services to the latest rhobs/configuration origin/main HEAD
  -l, --list                     List all RHOBS SaaS file names
      --serviceId string         Name of the SaaS file (without extension)
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl promote](osdctl_promote.md)	 - Utilities to promote services/operators

