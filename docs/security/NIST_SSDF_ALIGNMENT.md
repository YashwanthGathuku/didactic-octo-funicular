# NIST SP 800-218 (SSDF v1.1) Secure Software Development Alignment

> [!NOTE]
> **DISCLAIMER**: This document outlines SentinelFlow's implementation and control alignment against the **NIST Special Publication 800-218: Secure Software Development Framework (SSDF) Version 1.1**. This record reflects engineering practices and automated technical controls; it is not a formal government accreditation.

---

## 1. Executive Overview

The NIST SSDF defines actionable recommendations for mitigating software supply-chain risks, preventing vulnerabilities, and demonstrating software integrity. SentinelFlow maps its secure engineering lifecycle across all four SSDF practice groups:

```
  ┌───────────────────────────────┐        ┌───────────────────────────────┐
  │   PO: Prepare Organization    │        │    PS: Protect Software       │
  │   - Security & License Policy │        │    - Cosign OIDC Attestation  │
  │   - Branch Protection Rules   │        │    - Syft CycloneDX/SPDX SBOM │
  └───────────────┬───────────────┘        └───────────────┬───────────────┘
                  │                                        │
  ┌───────────────┴───────────────┐        ┌───────────────┴───────────────┐
  │   PW: Produce Well-Secured    │        │  RV: Respond to Vulnerabilities│
  │   - Clean-Room Architecture   │        │    - 24h Critical Triage SLA  │
  │   - Secret Hygiene Gates      │        │    - Govulncheck & Dependabot │
  └───────────────────────────────┘        └───────────────────────────────┘
```

---

## 2. Practice Group Alignments & Concrete Artifacts

### 2.1 Prepare the Organization (PO)

| Practice ID | Task Description | SentinelFlow Implementation Artifact | Status |
|---|---|---|---|
| **PO.1.1** | Identify and document security requirements for software development. | [`SECURITY.md`](file:///c:/Users/Gathu/Projects/fintech/SECURITY.md) and [`docs/security/DEPENDENCY_AND_VULNERABILITY_POLICY.md`](file:///c:/Users/Gathu/Projects/fintech/docs/security/DEPENDENCY_AND_VULNERABILITY_POLICY.md). | **PASS** |
| **PO.1.2** | Implement roles and responsibilities with separation of duties. | Dual-control quarantine release protocol in [`gateway/internal/review/dual_control.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/review/dual_control.go). | **PASS** |
| **PO.2.1** | Define and enforce open-source license governance allowlists. | Permissive license allowlist (MIT, Apache, BSD, ISC) in [`docs/security/DEPENDENCY_AND_VULNERABILITY_POLICY.md`](file:///c:/Users/Gathu/Projects/fintech/docs/security/DEPENDENCY_AND_VULNERABILITY_POLICY.md). | **PASS** |
| **PO.3.1** | Maintain automated toolchains for linting, security analysis, and testing. | Multi-stage CI pipeline in [`.github/workflows/ci.yml`](file:///c:/Users/Gathu/Projects/fintech/.github/workflows/ci.yml). | **PASS** |

---

### 2.2 Protect the Software (PS)

| Practice ID | Task Description | SentinelFlow Implementation Artifact | Status |
|---|---|---|---|
| **PS.1.1** | Store all code in version control with strict branch protection. | Branch protection rules defined in [`docs/engineering/RELEASE_CHECKLIST.md`](file:///c:/Users/Gathu/Projects/fintech/docs/engineering/RELEASE_CHECKLIST.md). | **PASS** |
| **PS.2.1** | Generate and publish Software Bill of Materials (SBOM). | Automated CycloneDX and SPDX SBOM generation via Syft in [`.github/workflows/release.yml`](file:///c:/Users/Gathu/Projects/fintech/.github/workflows/release.yml). | **PASS** |
| **PS.3.1** | Sign software release artifacts and record cryptographic provenance. | Keyless container image signing and attestation with Cosign in [`.github/workflows/release.yml`](file:///c:/Users/Gathu/Projects/fintech/.github/workflows/release.yml). | **PASS** |

---

### 2.3 Produce Well-Secured Software (PW)

| Practice ID | Task Description | SentinelFlow Implementation Artifact | Status |
|---|---|---|---|
| **PW.1.1** | Design software architecture using threat modeling and trust boundaries. | Threat model and trust boundary mapping in [`docs/security/THREAT_MODEL.md`](file:///c:/Users/Gathu/Projects/fintech/docs/security/THREAT_MODEL.md). | **PASS** |
| **PW.2.1** | Enforce least privilege and isolation across multi-tenant boundaries. | Tenant-scoped data store and row-level isolation in [`gateway/internal/auth/scope.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/auth/scope.go). | **PASS** |
| **PW.4.1** | Acquire and vet third-party components for security and license risk. | Zero AGPL copyleft clean-room boundary documented in [`docs/engineering/SFTPGO_LICENSE_AND_INTEGRATION_DECISION.md`](file:///c:/Users/Gathu/Projects/fintech/docs/engineering/SFTPGO_LICENSE_AND_INTEGRATION_DECISION.md). | **PASS** |
| **PW.5.1** | Prevent secret credential leakage in source, logs, and serialization. | Secret hygiene scanner [`gateway/secret_hygiene_test.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/secret_hygiene_test.go) and disclosure prevention in [`gateway/internal/connectors/disclosure_test.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/connectors/disclosure_test.go). | **PASS** |
| **PW.8.1** | Build reproducible container artifacts pinned by immutable digest. | Dockerfile pinned with base image digests in [`.github/workflows/release.yml`](file:///c:/Users/Gathu/Projects/fintech/.github/workflows/release.yml). | **PASS** |

---

### 2.4 Respond to Vulnerabilities (RV)

| Practice ID | Task Description | SentinelFlow Implementation Artifact | Status |
|---|---|---|---|
| **RV.1.1** | Maintain vulnerability reporting channels and coordinated disclosure policy. | Security contact (`security@sentinelflow.io`), PGP key, and SLA policy in [`SECURITY.md`](file:///c:/Users/Gathu/Projects/fintech/SECURITY.md). | **PASS** |
| **RV.1.2** | Continuously monitor third-party components for new CVEs. | Govulncheck and Trivy vulnerability scanning in [`.github/workflows/ci.yml`](file:///c:/Users/Gathu/Projects/fintech/.github/workflows/ci.yml). | **PASS** |
| **RV.3.1** | Remediate vulnerabilities according to documented severity SLAs. | SLA matrix (24h Critical, 7d High, 30d Medium) in [`docs/security/DEPENDENCY_AND_VULNERABILITY_POLICY.md`](file:///c:/Users/Gathu/Projects/fintech/docs/security/DEPENDENCY_AND_VULNERABILITY_POLICY.md). | **PASS** |
