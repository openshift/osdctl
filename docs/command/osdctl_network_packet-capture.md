## osdctl network packet-capture

Start packet capture

### Synopsis

Start packet capture

```
osdctl network packet-capture [flags]
```

### Options

```
  -d, --duration int              Duration (in seconds) of packet capture (default 60)
  -h, --help                      help for packet-capture
      --name string               Name of Daemonset (default "packet-capture")
  -n, --namespace string          Namespace to deploy Daemonset (default "default")
      --node-label-key string     Node label key (default "node-role.kubernetes.io/worker")
      --node-label-value string   Node label value
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

* [osdctl network](osdctl_network.md)	 - network related utilities

