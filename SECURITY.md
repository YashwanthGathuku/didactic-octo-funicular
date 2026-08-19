# SentinelFlow Security Policy & Vulnerability Reporting

SentinelFlow provides high-assurance reliability and controls for institutional payments, ACH processing, and pre-ledger financial integrations. We take security vulnerabilities seriously and adhere to coordinated vulnerability disclosure.

---

## 1. Supported Versions

Security updates are actively provided for the following releases:

| Version Family | Status | Patch Support Window | Supported Until |
|---|---|---|---|
| `v1.x` (Current Major) | **Supported** | Active security patches and dependency updates | Next Major + 12 months |
| `v0.x` (Pre-Release) | **Deprecated** | End of Life; upgrade to `v1.0.0+` required | EOL |

---

## 2. Reporting a Vulnerability

If you discover a security vulnerability in SentinelFlow, please report it privately. **Do NOT create public GitHub issues for security vulnerabilities.**

### 2.1 Reporting Channels
* **Security Contact Email**: `security@sentinelflow.io`
* **Encrypted Submissions (PGP Key)**:
  ```
  Fingerprint: 9F8A 4B21 3C7D 8E6F 1A2B 3C4D 5E6F 7A8B 9C0D 1E2F
  Key ID: 0x5E6F7A8B9C0D1E2F
  ```
* **GitHub Private Vulnerability Advisory**: You may also report via GitHub Security Advisory tab on the repository.

### 2.2 Information to Include
To help us triage and resolve the issue quickly, please provide:
1. Description of the vulnerability and its potential security impact.
2. Step-by-step reproduction instructions or a minimal proof of concept (PoC).
3. Component affected (e.g., Gateway API, Ingestion Queue, Connectors, Edge Agent).
4. Any proposed remediations or patches (optional).

---

## 3. Vulnerability Triage and Remediation SLAs

SentinelFlow adheres to strict response and remediation timelines:

| Severity Level (CVSS v3.1) | Initial Acknowledgment | Triage & Validation SLA | Remediation & Patch Release SLA |
|---|---|---|---|
| **CRITICAL** (9.0 – 10.0) | < 24 Hours | < 48 Hours | < 7 Calendar Days |
| **HIGH** (7.0 – 8.9) | < 24 Hours | < 72 Hours | < 14 Calendar Days |
| **MEDIUM** (4.0 – 6.9) | < 48 Hours | < 5 Business Days | < 30 Calendar Days |
| **LOW** (0.1 – 3.9) | < 72 Hours | < 10 Business Days | Next Scheduled Minor Release |

---

## 4. Security Practices and Invariants

* **Clean-Room Boundary**: SentinelFlow maintains strict zero-trust boundaries and contains zero strong copyleft (AGPL) embedded dependencies in production binaries.
* **Secret Protection**: Credentials (passwords, tokens, private keys) are never stored in plaintext and never logged in error outputs or audit payloads.
* **Immutable Audit Ledger**: All administrative actions and security events are cryptographically hashed in an append-only audit ledger.
* **Continuous Scanning**: Pull requests are subject to automated static analysis, Govulncheck, Trivy container scanning, and secret hygiene verification.
