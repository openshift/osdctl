# AGENTS.md

## Repository Overview

**osdctl** — cobra-based Go CLI for day-to-day OSD/ROSA SRE operations.  
Module: `github.com/openshift/osdctl` (single `go.mod` at the repo root, Go 1.25).  
Binary is built with **goreleaser**; the resulting binary lands in `./dist/`.

## Build / Test / Lint

```bash
make download-goreleaser   # one-time: fetches goreleaser into ./bin/
make build                 # goreleaser build --snapshot (produces ./dist/<arch>/osdctl)
make test                  # go test ./... -covermode=atomic -coverpkg=./...
make lint                  # golangci-lint run  (config: .golangci.yaml)
make fmt                   # gofmt + exits non-zero on diff
make mod                   # go mod tidy + exits non-zero on diff
make mockgen               # regenerates mocks under pkg/provider/aws/mock/
make all                   # format + mod + build + test + lint + verify-docs
```

Key linters enabled: `errcheck`, `gosec`, `govet`, `ineffassign`, `misspell`, `staticcheck`, `unused`.  
Lint only reports issues introduced after commit `8d912e3` (`new-from-rev` in `.golangci.yaml`).

## Architecture

```
cmd/          # one sub-directory per top-level cobra subcommand
  account/    # AWS Account Operator CRs (list, get, reset, rotate-secret, …)
  alerts/     # alert listing and silence management
  ci/         # CI pipeline helpers
  cloudtrail/ # CloudTrail error/event surfacing
  cluster/    # broad cluster ops (break-glass, etcd, health, SSH, resize, …)
  cost/       # AWS cost reporting and reconciliation
  dynatrace/  # Dynatrace log/dashboard/metrics access
  env/        # multi-cluster kubeconfig environment manager
  evidence/   # feature-testing evidence collection
  hcp/        # HyperShift / ROSA HCP specific operations
  hive/       # Hive ClusterDeployment / ClusterSync helpers
  iampermissions/ # IAM policy diff and snapshot
  jira/       # Jira ticket helpers
  jumphost/   # jumphost create/delete
  mc/         # management-cluster listing
  network/    # network verification (AWS + GCP) and packet capture
  org/        # OCM organisation queries
  promote/    # promotion workflows (Dynatrace, RHOBS, managed-scripts, …)
  rhobs/      # RHOBS metrics/logs/alerts + MCP server
  servicelog/ # OCM service-log list and post
  setup/      # interactive config-file initialisation
  swarm/      # Jira swarm / on-call secondary tooling
  common/     # shared helpers used across cmd/ packages

pkg/          # per-domain library packages consumed by cmd/
  backplane/  # backplane session and cluster-access helpers
  controller/ # generic controller/reconciler utilities
  docgen/     # cobra doc generation (used by make generate-docs)
  envConfig/  # config-file read/write (~/.config/osdctl)
  graphviz/   # dot-file rendering helpers
  infra/      # infrastructure-level utilities
  k8s/        # Kubernetes client helpers
  osdCloud/   # OSD cloud-provider abstractions
  osdctlCommand/ # cobra root and shared command setup
  osdctlConfig/  # structured config types
  policies/   # IAM policy helpers
  printer/    # table/JSON output formatting
  promote/    # promote-workflow library logic
  provider/   # cloud-provider interface + AWS implementation (with mocks)
  utils/      # miscellaneous shared utilities
```

Entry point: `cmd/cmd.go` registers all sub-commands onto the cobra root.

## Key Dependencies

| Dependency                                      | Purpose                                                                                                            |
|-------------------------------------------------|--------------------------------------------------------------------------------------------------------------------|
| `github.com/aws/aws-sdk-go-v2` (+ sub-modules)  | AWS API calls: config, credentials, CloudTrail, CostExplorer, EC2, ELB, IAM, Orgs, Route53, S3, ServiceQuotas, STS |
| `github.com/openshift/backplane-cli`            | Backplane cluster access; managed by Dependabot                                                                    |
| `github.com/openshift/osd-network-verifier`     | Egress network verification; managed by Dependabot                                                                 |
| `github.com/openshift-online/ocm-sdk-go`        | OCM API (clusters, service logs, organisations)                                                                    |
| `github.com/openshift/aws-account-operator/api` | Account / AccountClaim CRD types                                                                                   |
| `github.com/openshift/hive/apis`                | Hive CRD types                                                                                                     |
| `github.com/openshift/hypershift/api`           | HyperShift / ROSA HCP API types                                                                                    |
| `github.com/spf13/cobra`                        | CLI framework                                                                                                      |
| `github.com/modelcontextprotocol/go-sdk`        | MCP server (RHOBS sub-command)                                                                                     |
| `go.uber.org/mock/mockgen`                      | Mock generation for `pkg/provider/aws`                                                                             |
| `github.com/onsi/ginkgo` + `gomega`             | BDD-style tests                                                                                                    |

## Releases

Releases are fully automated — **do not push version tags or run `goreleaser` manually** unless the automation is unavailable:

1. Bump `VERSION` (plain `MAJOR.MINOR.PATCH`) on a branch and open a PR.  
   Shortcut: `make new-release RELEASE_VERSION=x.y.z`
2. On merge to `master`, the `release-on-version-bump` workflow tags `vX.Y.Z`, publishes the GitHub release via goreleaser, and fires the Fedora COPR webhook.
3. Pushing a `v*` tag by hand is still supported as a fallback and triggers the separate `release` and `trigger_copr` workflows.

## Working Rules

- **Dependabot** opens weekly `gomod` PRs for `osd-network-verifier` and `backplane-cli` only; the `dependabot-auto-merge` workflow auto-merges patch/minor updates. **Do not hand-bump these two dependencies** — let the bot manage them.
- All other dependencies are managed via MintMaker / manual PRs; do not bump them without a concrete reason.
- Keep changes **minimal and focused**; avoid unrelated refactors in the same PR.
- **Add unit tests** alongside every new function; existing test files (`*_test.go`) in the same package are the right place.
- Comments should explain **why**, not what — the code itself shows what.
- Run `make fmt mod` before committing to keep diffs clean for CI.
- Generated files (`pkg/provider/aws/mock/`, `docs/`) must be regenerated and committed when the source they reflect changes (`make mockgen`, `make generate-docs`).
