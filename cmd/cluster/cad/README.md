# CAD (Configuration Anomaly Detection) Commands

Commands for running manual investigations on the Configuration Anomaly Detection (CAD) clusters and writing the results to a backplane report.

## Prerequisites

- Connected to environment of the target cluster: `ocm login --use-auth-code --url "<production|stage>"`
- The CAD clusters (both stage and prod) are always in production OCM

## Usage

```bash
osdctl cluster cad run \
  --cluster-id <cluster-id> \
  --investigation <investigation-name> \
  --environment <stage|production> \
  --reason "<JIRA-ticket or reason>"
```

### Flags

- `--cluster-id` / `-C`: Target cluster ID (internal or external)
- `--investigation` / `-i`: Investigation to run (see available investigations below)
- `--environment` / `-e`: Target cluster environment (`stage` or `production`). This is kept explicit, because the pipeline will silently fail if this parameter isn't correct
- `--reason`: Elevation reason for backplane access (e.g., `OHSS-1234` or `#ITN-2024-12345`)
- `--dry-run` / `-d`: Run the investigation with the dry-run flag. This will not create a report

### Available Investigations

- `chgm` - Change Management
- `cmbb` - Configuration Management Baseline Check
- `can-not-retrieve-updates` - Update Retrieval Issues
- `ai` - AI-based Analysis
- `cpd` - Control Plane Degradation
- `etcd-quota-low` - ETCD Quota Issues
- `insightsoperatordown` - Insights Operator Down
- `machine-health-check` - Machine Health Check
- `must-gather` - Must-Gather Collection
- `upgrade-config` - Upgrade Configuration Check
- `restart-controlplane` - Restart Control Plane

### Example

```bash
osdctl cluster cad run \
  --cluster-id 1a2b3c4d5e6f7g8h9i0j \
  --investigation chgm \
  --environment production \
  --reason "OHSS-12345"
```

## Debugging

To check the status of a PipelineRun after scheduling:

**1. Connect to production OCM**
```bash
ocm login --use-auth-code --url "production"
```

**2. Login to the CAD cluster**
- For stage: `ocm backplane login cads01ue1`
- For prod: `ocm backplane login cadp01ue1`

**3. Check PipelineRuns**
- For stage:
  ```bash
  ocm backplane elevate -n -- get pipelinerun -n configuration-anomaly-detection-stage
  ```
- For prod:
  ```bash
  ocm backplane elevate -n -- get pipelinerun -n configuration-anomaly-detection-production
  ```

## Architecture Notes

- **CAD Cluster IDs**: Hardcoded in app-interface
  - Stage: `2f9ghpikkv446iidcv7b92em2hgk13q9` (cads01ue1)
  - Prod: `2fbi9mjhqpobh20ot5d7e5eeq3a8gfhs` (cadp01ue1)
- **Namespaces**:
  - Stage: `configuration-anomaly-detection-stage`
  - Prod: `configuration-anomaly-detection-production`
- **Pipeline**: `cad-manual-investigation-pipeline` (Tekton)
- The command always connects to production OCM internally, regardless of user's current OCM context

## Viewing Reports

After the investigation completes (may take several minutes), view reports using:

```bash
osdctl cluster reports list -C <cluster-id> -l 1
```

**Note**: You need to be connected to the correct OCM environment for the target cluster to view its reports.