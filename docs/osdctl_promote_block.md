## osdctl promote block

Add a blocked version to a component in app.yaml

### Synopsis

Add a SHA commit hash to the blockedVersions list for a code component
in the application's app.yaml file. This prevents the specified version
from being promoted through progressive delivery.

The command locates the app.yaml through the SaaS service file, finds
the specified component by name, and appends the git hash to its
codeComponents[].blockedVersions array. If the blockedVersions field
does not yet exist, it will be created.

Duplicate entries are rejected with an error.

```
osdctl promote block [flags]
```

### Examples

```

		# List all services and their components
		osdctl promote block --list

		# Block a specific version for a single component
		osdctl promote block --serviceId <service> --component <component-name> --gitHash <sha>

		# Block a specific version for all components of a service
		osdctl promote block --serviceId <service> --all --gitHash <sha>

		# With explicit app-interface path
		osdctl promote block --serviceId <service> --component <component-name> --gitHash <sha> --appInterfaceDir /path/to/app-interface
```

### Options

```
  -a, --all                      Block the version for all components of the service (mutually exclusive with --component)
      --appInterfaceDir string   Location of app-interface checkout. Falls back to the current working directory
  -c, --component string         Name of the code component in app.yaml
  -g, --gitHash string           SHA commit hash to add to blockedVersions
  -h, --help                     help for block
  -l, --list                     List all services and their components
      --serviceId string         Name of the SaaS service file (without extension)
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl promote](osdctl_promote.md)	 - Utilities to promote services/operators

