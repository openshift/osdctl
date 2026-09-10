# Post-Quantum Cryptography (PQC) Readiness Assessment — Osdctl

**Jira:** ROSAENG-61476
**Parent Epic:** HCMSEC-3301
**Risk Level:** HIGH
**Date:** 2026-09-10

## Executive Summary

Osdctl has direct SSH key management in production CLI commands. It retrieves
private SSH keys from Hive Kubernetes secrets and creates/deletes EC2 key pairs
for emergency jumphost access. These are active production paths that handle
cryptographic key material. No direct `crypto/*` standard library imports exist
in osdctl's own source code — all cryptographic operations are delegated to the
AWS EC2 API (for key pair lifecycle) and the Kubernetes API (for secret
retrieval). PQC migration for osdctl is therefore primarily blocked on upstream
providers (AWS, OpenSSH, OpenShift/Hive).

No code changes are required at this time. This document inventories all
cryptographic paths and maps the upstream dependencies that must be resolved
before PQC migration can proceed.

## Crypto Path Inventory

### 1. EC2 Key Pair Creation (Ed25519) — ACTIVE, PRODUCTION

**File:** `cmd/jumphost/create.go` (lines 108-148)

The `createKeyPair()` function creates an EC2 key pair via the AWS API with
`KeyType: types.KeyTypeEd25519` and `KeyFormat: types.KeyFormatPem`. The
private key material is saved to a temporary `.pem` file with `0400`
permissions for SSH access to the jumphost.

- **Algorithm:** Ed25519 (not PQC-safe)
- **PQC impact:** Ed25519 is vulnerable to quantum attack. Must be replaced
  with a PQC-compatible key type when AWS EC2 supports one.
- **Blocked on:** AWS EC2 API adding PQC key pair types.
- **Migration action:** Update `KeyType` from `types.KeyTypeEd25519` to a
  PQC-compatible type once AWS makes one available. The change is a
  single-line constant swap in `create.go:113`.

### 2. EC2 Key Pair Lifecycle Management — ACTIVE, PRODUCTION

**File:** `cmd/jumphost/cmd.go` (lines 47-65)

The `jumphostAWSClient` interface defines `CreateKeyPair`, `DeleteKeyPair`,
and `DescribeKeyPairs` methods for full EC2 SSH key pair lifecycle management.

**File:** `cmd/jumphost/delete.go` (lines 94-117)

The `deleteKeyPair()` function searches for and deletes EC2 key pairs by tag
filter during jumphost cleanup.

- **PQC impact:** These functions are key-type-agnostic — they operate on
  whatever key type was created. No changes needed here when the key type
  in `createKeyPair()` is updated.

### 3. SSH Security Group Ingress (Port 22) — ACTIVE, PRODUCTION

**File:** `cmd/jumphost/create.go` (lines 293-319)

The `allowJumphostSshFromIp()` function creates a security group ingress rule
allowing TCP port 22 (SSH) from the user's public IP to the jumphost instance.

**File:** `cmd/jumphost/create.go` (lines 258-269)

The `assembleNextSteps()` function outputs `ssh -i <keyfile> ec2-user@<ip>`
connection instructions.

- **PQC impact:** The SSH protocol itself needs PQC-safe key exchange (ML-KEM)
  once OpenSSH implementations support it. The security group rule (port 22)
  is transport-layer and does not need changes.
- **Blocked on:** OpenSSH adding ML-KEM key exchange support. The Amazon
  Linux 2023 AMI used for jumphosts (`findLatestJumphostAMI`) will inherit
  PQC-safe SSH once AWS updates the OS packages.

### 4. Hive SSH Private Key Retrieval — ACTIVE, PRODUCTION

**File:** `cmd/cluster/ssh/key.go` (lines 84-185)

The `PrintKey()` function retrieves a cluster's private SSH key from Hive
Kubernetes secrets (secret name `ssh`, data key `ssh-privatekey`) and prints
it to stdout. This is direct SSH private key retrieval for last-resort cluster
node access.

**File:** `cmd/cluster/ssh/ssh.go` (lines 1-13)

SSH subcommand entrypoint registering `osdctl cluster ssh key`.

- **Algorithm:** Determined by Hive/OpenShift installer at cluster creation
  time (typically RSA).
- **PQC impact:** RSA is quantum-vulnerable. The key algorithm must migrate
  to a PQC-safe type.
- **Blocked on:** OpenShift/Hive installer generating PQC-safe SSH keys at
  cluster install time.
- **Migration action:** No changes needed in osdctl — this code retrieves
  whatever key Hive stores. Once Hive generates PQC keys, osdctl will
  retrieve them transparently.

### 5. SSH Bastion Instructions (Informational) — NOT ACTIVE

**File:** `cmd/cluster/access/access.go` (line 381)

Prints `ssh bastion` as a next-step instruction for accessing private API
clusters. This is informational text only — no SSH key material is handled.

- **PQC impact:** None. No cryptographic operations occur.

### 6. Network Verification NoTls Flag — TOOL, NOT PQC-RELATED

**File:** `cmd/network/verification.go` (lines 84-85, 197)

The `NoTls` boolean flag allows ignoring SSL certificate validation on the
client-side for egress verification. The TLS handling itself is delegated to
the `osd-network-verifier` library via the `proxy.ProxyConfig` struct.

- **PQC impact:** Not directly PQC-related. The flag controls client-side
  validation behavior, not cipher suite selection. TLS 1.3 and PQC key
  exchange would be handled by the upstream `osd-network-verifier` library
  and Go's `crypto/tls` package.
- **Note:** This flag is used for debugging/testing egress verification
  only, not in production data paths.

### 7. HCP Certificate Status Monitoring — READ-ONLY TOOL

**File:** `cmd/hcp/status/parser.go` (lines 312-357)

The `parseCertificate()` function parses and displays ingress certificate
status (`NotAfter`, `RenewalTime`, `DNSNames`) from standalone certificate
resources in the OCM live resources map (keys with a `certificate-` prefix).
This is read-only monitoring — it displays certificate metadata but does not
validate signatures or handle key material.

- **PQC impact:** When CAs begin issuing certificates with PQC signature
  algorithms (ML-DSA), this parsing code must handle new algorithm OIDs in
  the certificate fields. However, since osdctl only reads string fields
  (`NotAfter`, `RenewalTime`, `DNSNames`) from JSON — not parsing X.509
  DER/PEM structures — no changes are needed.

## Crypto Library Dependencies

| Library | Version | Source | Context |
|---------|---------|--------|---------|
| AWS SDK Go v2 (`aws-sdk-go-v2`) | via `go.mod` | Direct | EC2 key pair API calls |
| `golang.org/x/crypto` | v0.53.0 | Indirect | Transitive dependency from upstream packages |
| `golang.org/x/net` | v0.56.0 | Indirect | Transitive networking (HTTP/2 support) |

**Note:** No direct imports of `crypto/tls`, `crypto/rsa`, `crypto/ecdsa`,
`crypto/x509`, or `crypto/ed25519` exist in osdctl's non-vendor, non-test
source code. All cryptographic operations are delegated to external services.

## Upstream Dependency Map

| Dependency | Tracking | Status | Osdctl Impact |
|------------|----------|--------|---------------|
| AWS EC2 PQC key pair types | AWS roadmap | Not yet available | `cmd/jumphost/create.go` — update `KeyType` constant |
| OpenSSH ML-KEM key exchange | OpenSSH project | Not yet available | Jumphost SSH connections — transparent once AMI updated |
| OpenShift/Hive PQC SSH keys | HCMSEC-3301 | Phase 1-2 (2026) | `cmd/cluster/ssh/key.go` — transparent, no code change |
| `golang.org/x/crypto` PQC | Go project | Tracking ML-KEM | Indirect — inherited transitively |
| `osd-network-verifier` TLS | openshift/osd-network-verifier | Active | TLS 1.3 + ML-KEM handled upstream |

## Migration Readiness Assessment

### Actionable (when upstream is ready)

1. **EC2 key pair type migration** (`cmd/jumphost/create.go:113`):
   Single-line change from `types.KeyTypeEd25519` to a PQC type once AWS
   supports it. Low complexity, low risk.

### Blocked on Upstream

2. **Hive SSH key algorithm**: Depends on OpenShift/Hive installer changes.
   Osdctl code is algorithm-agnostic (retrieves opaque key bytes). No osdctl
   changes needed.

3. **SSH protocol key exchange**: Depends on OpenSSH ML-KEM support. Osdctl
   does not implement SSH — it invokes the system `ssh` binary. No osdctl
   changes needed.

4. **TLS configuration**: Osdctl delegates TLS to Go's standard library and
   upstream libraries (`osd-network-verifier`). TLS 1.3 with ML-KEM hybrid
   key exchange will be inherited when Go and upstream libraries add support.

### Not Applicable

5. **Certificate parsing** (`cmd/hcp/status/parser.go`): Read-only JSON field
   extraction. No cryptographic validation occurs.

6. **SSH bastion instructions** (`cmd/cluster/access/access.go`): Informational
   text only.

## Recommendation: SSM Session Manager as SSH Alternative

The jumphost workflow (`cmd/jumphost/`) could be partially replaced by AWS
Systems Manager (SSM) Session Manager, which:

- Eliminates the need for SSH key pairs entirely
- Removes the requirement for port 22 security group ingress
- Uses IAM-based authentication instead of SSH keys
- Supports PQC-safe TLS for the SSM channel (when AWS enables it)

**Trade-off:** SSM requires the SSM agent on target instances and appropriate
IAM roles. This may not be available on all ROSA cluster nodes. Evaluating SSM
as an alternative should be considered as part of Phase 3 (PQC Testing).

## Timeline Alignment with HCMSEC-3301

| Phase | Timeline | Osdctl Actions |
|-------|----------|----------------|
| Phase 1-2: Discovery & Dependency Mapping | Q1 2026 | This assessment (complete) |
| Phase 3: PQC Testing | Q2-Q3 2026 | Test with PQC key types when AWS EC2 supports them; evaluate SSM alternative |
| Phase 4: ML-KEM Enablement | Q4 2026 | Update `KeyType` in jumphost code; verify TLS 1.3 + ML-KEM in dependencies |

## Conclusion

Osdctl's PQC exposure is limited to two active production paths:

1. **EC2 Ed25519 key pair creation** — a single-line constant change when AWS
   supports PQC key types
2. **Hive SSH key retrieval** — algorithm-agnostic code that requires no changes

Both paths are blocked on upstream providers. No immediate code changes are
required. This assessment should be revisited during Phase 3 when upstream
PQC support becomes available for testing.
