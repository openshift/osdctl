## osdctl completion

Output shell completion code for the specified shell (bash or zsh)

### Synopsis

Output shell completion code for the specified shell (bash or zsh)

```
osdctl completion SHELL
```

### Examples

```
  # Installing bash completion on macOS using homebrew
  ## If running Bash 3.2 included with macOS
  brew install bash-completion
  ## or, if running Bash 4.1+
  brew install bash-completion@2
  
  
  # Installing bash completion on Linux
  ## If bash-completion is not installed on Linux, please install the 'bash-completion' package
  ## via your distribution's package manager.
  ## Load the osdctl completion code for bash into the current shell
  source <(osdctl completion bash)
  ## Write bash completion code to a file and source if from .bash_profile
  osdctl completion bash > ~/.completion.bash.inc
  printf "
  # osdctl shell completion
  source '$HOME/.completion.bash.inc'
  " >> $HOME/.bash_profile
  source $HOME/.bash_profile
  
  
  # Load the osdctl completion code for zsh[1] into the current shell
  source <(osdctl completion zsh)
  # Set the osdctl completion code for zsh[1] to autoload on startup
  osdctl completion zsh > "${fpath[1]}/_osdctl"
```

### Options

```
  -h, --help   help for completion
```

### Options inherited from parent commands

```
      --cluster string             The name of the kubeconfig cluster to use
      --context string             The name of the kubeconfig context to use
      --insecure-skip-tls-verify   If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure
      --kubeconfig string          Path to the kubeconfig file to use for CLI requests.
      --request-timeout string     The length of time to wait before giving up on a single server request. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h). A value of zero means don't timeout requests. (default "0")
  -s, --server string              The address and port of the Kubernetes API server
```

### SEE ALSO

* [osdctl](osdctl.md)	 - OSD CLI

