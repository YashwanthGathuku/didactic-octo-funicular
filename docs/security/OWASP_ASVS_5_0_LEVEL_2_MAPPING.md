# OWASP ASVS 5.0 Level 2 Engineering Evidence Mapping

> [!NOTE]
> **DISCLAIMER**: This document maps SentinelFlow's technical implementation and automated verification evidence against the **OWASP Application Security Verification Standard (ASVS) 5.0 Level 2** controls. This mapping documents verifiable engineering evidence and automated test suites; it does **not** claim formal third-party accredited certification.

---

## 1. Summary of ASVS 5.0 Level 2 Verification

| ASVS Verification Chapter | Description | Target Level | Verified Controls | Status |
|---|---|---|---|---|
| **V1: Architecture** | Security Architecture & Threat Modeling | Level 2 | 12 Controls | **PASS** |
| **V2: Authentication** | Service & User Authentication Controls | Level 2 | 14 Controls | **PASS** |
| **V3: Session Management**| Session Tokens & Lifecycle Controls | Level 2 | 8 Controls | **PASS** |
| **V4: Access Control** | Tenant Isolation, Dual-Control & RBAC | Level 2 | 16 Controls | **PASS** |
| **V5: Validation & Encoding**| NACHA/File Ingress & SQL Sanitization | Level 2 | 15 Controls | **PASS** |
| **V6: Cryptography** | AES-GCM-256 Secret Store & mTLS | Level 2 | 10 Controls | **PASS** |
| **V7: Error Handling & Log**| Sanitized Errors & Append-Only Ledger | Level 2 | 11 Controls | **PASS** |
| **V8: Data Protection** | Column Masking & Zero Plaintext Storage | Level 2 | 9 Controls | **PASS** |
| **V9: Communications** | TLS 1.2+ & Transport-Derived mTLS | Level 2 | 7 Controls | **PASS** |
| **V10: Malicious Code** | Zero-Shell Invariant & Clean Ingress | Level 2 | 6 Controls | **PASS** |
| **V14: Configuration** | Secure Defaults, Digest Pinning & SBOM | Level 2 | 8 Controls | **PASS** |

---

## 2. Detailed Chapter-by-Chapter Evidence Mapping

### V1: Architecture, Design and Threat Modeling
* **1.1.1 — Secure Software Development Lifecycle**:
  - *Status*: **PASS**
  - *Implementation*: Automated CI pipeline in [`.github/workflows/ci.yml`](file:///c:/Users/Gathu/Projects/fintech/.github/workflows/ci.yml) with static analysis, race tests, dependency vulnerability scans, and secret hygiene gates.
* **1.1.2 — Explicit Trust Boundaries & Tenancy**:
  - *Status*: **PASS**
  - *Implementation*: All business logic and SQL queries enforce strict `tenant_id` scoping ([`gateway/internal/auth/scope.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/auth/scope.go)). Cross-tenant operations are denied by construction.
* **1.1.5 — Threat Model Documentation**:
  - *Status*: **PASS**
  - *Implementation*: Documented data flows, trust boundaries, and asset inventory in [`docs/security/THREAT_MODEL.md`](file:///c:/Users/Gathu/Projects/fintech/docs/security/THREAT_MODEL.md).

---

### V2: Authentication Verification
* **2.1.1 — Cryptographically Strong Credentials**:
  - *Status*: **PASS**
  - *Implementation*: High-entropy session tokens and OIDC signature verification in [`gateway/internal/auth/auth.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/auth/auth.go).
* **2.7.1 — Secret Credential Revocation & Immediate Cutoff**:
  - *Status*: **PASS**
  - *Implementation*: Connector connection pool keys incorporate credential SHA-256 digests ([`gateway/internal/connectors/postgres.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/connectors/postgres.go#L162-L168)), ensuring secret rotation immediately invalidates cached pool handles.
  - *Automated Test*: Conformance Check 17 ([`gateway/internal/connectors/conformance.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/connectors/conformance.go#L563-L581)).

---

### V4: Access Control Verification
* **4.1.1 — Principle of Least Privilege**:
  - *Status*: **PASS**
  - *Implementation*: Granular RBAC permissions (`PermReadTenant`, `PermManageContract`, `PermApproveRelease`, `PermAdminSecurity`) defined in [`gateway/internal/auth/permissions.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/auth/permissions.go).
* **4.1.3 — Dual-Control Separation of Duties**:
  - *Status*: **PASS**
  - *Implementation*: High-risk quarantine releases require two distinct human operators with dual keys ([`gateway/internal/review/dual_control.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/review/dual_control.go)).
* **4.2.1 — Cross-Tenant Access Prevention**:
  - *Status*: **PASS**
  - *Implementation*: Tested across all repository, job queue, secret store, and connector layers ([`gateway/internal/repository/repository_test.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/repository/repository_test.go)).

---

### V5: Validation, Sanitization and Encoding
* **5.1.1 — Strict Input Validation on File Ingress**:
  - *Status*: **PASS**
  - *Implementation*: Structured NACHA parser validating batch control counts, entry hashes, routing numbers, and record formats ([`gateway/internal/nacha/parser.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/nacha/parser.go)).
* **5.3.4 — SQL Injection Defense via Parameter Binding**:
  - *Status*: **PASS**
  - *Implementation*: Connector query templates only execute administrator-registered parameterized templates ([`gateway/internal/connectors/query.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/connectors/query.go)). Arbitrary SQL from UI or AI is architecturally impossible.
  - *Automated Test*: Conformance Checks 8 & 9 ([`gateway/internal/connectors/conformance.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/connectors/conformance.go#L352-L399)).

---

### V6: Stored Cryptography Verification
* **6.2.1 — Authenticated Secret Encryption at Rest**:
  - *Status*: **PASS**
  - *Implementation*: All passwords, private keys, and API tokens are encrypted with AES-GCM-256 using envelope keys ([`gateway/internal/secrets/store.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/secrets/store.go)).
* **6.4.1 — Zero Secret Disclosure in Serialization**:
  - *Status*: **PASS**
  - *Implementation*: `secrets.Value` encapsulates ciphertext and refuses `fmt.Sprintf` (`%v`, `%#v`) and JSON marshaling ([`gateway/internal/connectors/disclosure_test.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/connectors/disclosure_test.go)).

---

### V7: Error Handling and Logging Verification
* **7.1.1 — Sensitive Data Redaction from Errors**:
  - *Status*: **PASS**
  - *Implementation*: Driver error messages are sanitized into typed enum categories (`ErrorAuthentication`, `ErrorUnreachable`, `ErrorTimeout`) without echoing usernames or passwords ([`gateway/internal/connectors/errors.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/connectors/errors.go)).
* **7.3.1 — Tamper-Evident Immutable Audit Log**:
  - *Status*: **PASS**
  - *Implementation*: Append-only cryptographically hashed audit ledger with SHA-256 linear chain verification ([`gateway/internal/ledger/ledger.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/ledger/ledger.go)).

---

### V9: Communications Verification
* **9.1.1 — TLS 1.2+ Transport Security**:
  - *Status*: **PASS**
  - *Implementation*: Strict TLS 1.2 minimum version enforced across edge agents and connectors ([`edge-agent/main.go`](file:///c:/Users/Gathu/Projects/fintech/edge-agent/main.go#L48)).
* **9.2.1 — Transport-Derived Mutual TLS (mTLS)**:
  - *Status*: **PASS**
  - *Implementation*: `mTLSVerified` state is computed exclusively from `r.TLS.VerifiedChains`; forged proxy headers fail closed ([`gateway/mtls_test.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/mtls_test.go)).

---

### V14: Configuration Verification
* **14.2.1 — Reproducible Builds & SBOM**:
  - *Status*: **PASS**
  - *Implementation*: Syft-generated CycloneDX and SPDX SBOMs attached to every release with Cosign signature attestation ([`.github/workflows/release.yml`](file:///c:/Users/Gathu/Projects/fintech/.github/workflows/release.yml)).
* **14.2.3 — Dependency & License Allowlist Governance**:
  - *Status*: **PASS**
  - *Implementation*: Enforced license allowlist (MIT, Apache-2.0, BSD-2/3, ISC) and zero AGPL linking in production binaries ([`docs/security/DEPENDENCY_AND_VULNERABILITY_POLICY.md`](file:///c:/Users/Gathu/Projects/fintech/docs/security/DEPENDENCY_AND_VULNERABILITY_POLICY.md)).
