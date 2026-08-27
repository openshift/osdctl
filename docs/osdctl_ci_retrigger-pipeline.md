## osdctl ci retrigger-pipeline

Retrigger a failed SAPM Tekton PipelineRun on appsrep09ue1

### Synopsis

Retrigger a failed SAPM Tekton PipelineRun on appsrep09ue1.

Use this when a PipelineRun fails for infrastructure reasons unrelated to the operator code
or e2e tests (e.g. network blip, transient API failure). A successful retrigger publishes
to the SAPM success channel, unblocking downstream promotions.

To find the failed PipelineRun name:
  ocm backplane login --multi 29nmp5rhf8rgclg4a02lju4eld79js9e
  kubectl get pipelineruns -n <operator>-pipelines --sort-by=.metadata.creationTimestamp

Common namespaces: certman-operator-pipelines, cloud-ingress-operator-pipelines,
  rbac-permissions-operator-pipelines, ocm-agent-operator-pipelines

```
osdctl ci retrigger-pipeline [flags]
```

### Examples

```
  # Retrigger a failed certman int PipelineRun
  osdctl ci retrigger-pipeline -n certman-operator-pipelines \
    -r saas-co-prow-e2e-osd-integration-hivei01ue1-abc12 \
    --reason ROSAENG-62330
```

### Options

```
  -h, --help               help for retrigger-pipeline
  -n, --namespace string   Namespace containing the failed PipelineRun (e.g. certman-operator-pipelines)
      --reason string      Backplane elevation reason (Jira key, PD URL, or ITN key)
  -r, --run string         Name of the failed PipelineRun
```

### Options inherited from parent commands

```
  -S, --skip-version-check   skip checking to see if this is the most recent release
```

### SEE ALSO

* [osdctl ci](osdctl_ci.md)	 - Commands for managing CI pipelines and jobs

