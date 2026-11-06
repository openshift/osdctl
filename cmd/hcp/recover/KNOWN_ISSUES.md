# dns-recover: Known Issues and Post-Migration Recovery Notes

## Etcd TLS Certificate Mismatch After Cross-MC Migration

**Discovered:** 2026-11-03 during rosa-286 migration testing
**Cluster:** 2tr5c0gjqmvh6b84bm5q113g943pe79o (tfilko-test-private-2)
**Migration:** hs-mc-s73b6gqkg -> hs-mc-s739up51g

### Symptom

After OADP restore on the destination MC, etcd peers reject connections with TLS errors:

```
rejected connection on peer endpoint
error: tls: "10.128.34.67" does not match any of DNSNames
  ["*.etcd-discovery.ocm-int-...-j7q1h1i1j4u4o2e.svc",
   "*.etcd-discovery.ocm-int-...-j7q1h1i1j4u4o2e.svc.cluster.local",
   "127.0.0.1", "::1"]
```

The etcd peer TLS certificates were backed up from the source MC and restored via
Velero. The certificates only contain DNS wildcard SANs for the etcd-discovery
service, not IP SANs. When etcd pods get new IPs on the destination MC, peer
connections via IP fail TLS verification.

### Impact

- kube-apiserver cannot connect to etcd (`dial tcp: lookup etcd-client...svc: operation was canceled`)
- Hosted cluster API is unreachable (both via VPC endpoint and internally from MC)
- Backplane login returns 406 (can reach API but can't authenticate)
- dns-recover completes successfully but cluster remains inaccessible

### Root Cause

The OADP/Velero backup includes etcd TLS secrets (`etcd-peer-tls`, `etcd-server-tls`,
`etcd-client-tls`, `etcd-signer`) from the source MC. These certs are valid for the
source MC's pod network, but not the destination MC's. The control-plane-operator
does not automatically detect and regenerate stale etcd certificates after restore.

### Attempted Remediation

1. Delete etcd TLS secrets and restart CPO to trigger regeneration
2. Scale etcd to 0, delete all etcd TLS + signer secrets, restart CPO, wait, scale back to 3

Neither approach reliably regenerated consistent certs across all 3 etcd members.
The CPO may need the HostedControlPlane to be in a specific state to trigger cert
rotation, or the etcd-operator sidecar handles cert management differently.

### Recommended Fix (migration script)

The migration script should include a post-restore step that:

1. Scales etcd to 0
2. Deletes etcd TLS secrets (`etcd-peer-tls`, `etcd-server-tls`, `etcd-client-tls`)
3. Deletes etcd signer secrets (`etcd-signer`, `etcd-metrics-signer`)
4. Triggers CPO reconciliation (delete CPO pod or annotate HostedControlPlane)
5. Waits for CPO to regenerate all etcd TLS secrets
6. Scales etcd back to 3

This should happen **after** the OADP restore (Step 5) and **before** unpausing
reconciliation (Step 7).

## Ordering Requirements for dns-recover

### Correct Workflow

The intended order for HCP migration DNS recovery:

1. **Complete source MC teardown** (Step 9) - stops ExternalDNS on source from recreating records
2. **Run `release-dns`** - deletes stale Route 53 records pointing to old VPC endpoint
3. **Run `dns-recover`** - deletes and recreates AWSEndpointService/private-router on destination MC
4. **Wait for ExternalDNS** - destination MC's ExternalDNS creates new records pointing to new VPC endpoint
5. **Verify connectivity** - backplane login, nginx healthcheck

### What Goes Wrong Without Correct Ordering

- Running dns-recover **before** source teardown: ExternalDNS on source MC recreates
  old DNS records, competing with the destination MC's ExternalDNS
- Running dns-recover **before** release-dns: Old TXT ownership records prevent
  destination ExternalDNS from creating new records (owner ID mismatch)
- Not waiting for ExternalDNS: DNS resolves to NXDOMAIN until ExternalDNS reconciles (~1-2 min)

## Additional Steps for dns-recover (Future Work)

The following checks could be added to dns-recover to improve the recovery workflow:

1. **Etcd health check** - verify etcd cluster is healthy before attempting DNS recovery
2. **kube-apiserver reachability** - test if API is reachable from within the MC after recovery
3. **ExternalDNS reconciliation wait** - poll Route 53 until new records appear after recovery
4. **Source MC teardown verification** - check that the source MC no longer has the HCP namespace
