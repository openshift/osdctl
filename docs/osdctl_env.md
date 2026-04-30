## osdctl env

Create an environment to interact with a cluster

### Synopsis


Creates an isolated environment where you can interact with a cluster.
The environment is set up in a dedicated folder in $HOME/.ocenv.
The $CLUSTERID variable will be populated with the external ID of the cluster you're logged in to.

*PS1*
osdctl env renders the required PS1 function 'kube_ps1' to use when logged in to a cluster.
You need to include it inside your .bashrc or .bash_profile by adding a snippet like the following:

export PS1='...other information for your PS1... $(kube_ps1) \$ '

e.g.

export PS1='\[\033[36m\]\u\[\033[m\]@\[\033[32m\]\h:\[\033[33;1m\]\w\[\033[m\]$(kube_ps1) \$ '

osdctl env creates a new ocm and kube config so you can log in to different environments at the same time.
When initialized, osdctl env will copy the ocm config you're currently using.

*Logging in to the cluster*

To log in to a cluster within the environment using backplane, osdctl creates the ocb command.
The ocb command is created in the bin directory in the environment folder and added to the PATH when inside the environment.


```
osdctl env [flags] [env-alias]
```

### Options

```
  -a, --api string                       OpenShift API URL for individual cluster login
      --as string                        Username to impersonate for the operation. User could be a regular user or a service account in a namespace.
      --cluster string                   The name of the kubeconfig cluster to use
  -C, --cluster-id string                Cluster ID
      --context string                   The name of the kubeconfig context to use
  -d, --delete                           Delete environment
  -k, --export-kubeconfig                Output export kubeconfig statement, to use environment outside of the env directory
  -h, --help                             help for env
      --insecure-skip-tls-verify         If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
  -K, --kubeconfig string                KUBECONFIG file to use in this env (will be copied to the environment dir)
  -l, --login-script string              OCM login script to execute in a loop in ocb every 30 seconds
  -o, --output string                    Valid formats are ['', 'json', 'yaml', 'env']
  -p, --password string                  Password for individual cluster login
      --request-timeout string           The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -r, --reset                            Reset environment
  -s, --server string                    The address and port of the Kubernetes API server
      --skip-aws-proxy-check aws_proxy   Don't use the configured aws_proxy value
  -t, --temp                             Delete environment on exit
  -u, --username string                  Username for individual cluster login
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl](osdctl.md)	 - OSD CLI

