# SentinelFlow Production Release & Branch Protection Checklist

## 1. Branch Protection Requirements (`main` & `release/*`)

To maintain supply-chain integrity, the GitHub repository configuration enforces the following branch protection rules:

* [x] **Require a pull request before merging** (Direct pushes to `main` and `release/*` are blocked).
* [x] **Require approvals**: Minimum 2 senior code owner approvals required.
* [x] **Dismiss stale pull request approvals when new commits are pushed**.
* [x] **Require status checks to pass before merging**:
  - `lint-and-format` (Go fmt, golangci-lint, TypeScript, Ruff)
  - `test-backend` (`go test -race ./...`)
  - `test-frontend` (`vitest run`, `vite build`)
  - `test-ai-tier` (`python ai-tier/evals/runner.py`)
  - `test-migrations` (`go test -run "TestMigration"`)
  - `security-and-license-scan` (`govulncheck`, `trivy`, `secret hygiene`)
* [x] **Require branches to be up to date before merging**.
* [x] **Require cryptographically signed commits (GPG / SSH)**.
* [x] **Include administrators** (Enforce protections for repository owners).

---

## 2. Pre-Release Verification Checklist

Before tagging any official release (e.g. `v1.2.0`), release managers must verify:

### 2.1 Code & Test Integrity
- [ ] **Clean Test Suite**: All 15 gateway packages pass cleanly with race detection (`go test -race ./...`).
- [ ] **Clean Conformance Artifacts**: Verified connector conformance records are attached for any selectable connectors.
- [ ] **AI Safety Evals**: Adversarial dataset evaluation runs at 100% pass rate with zero jailbreaks.
- [ ] **Database Migrations**: Upgrade and clean-install smoke tests execute with zero constraint violations.

### 2.2 Security & Supply-Chain
- [ ] **Secret Hygiene Scan**: `TestNoLiteralSecretsInSourceOrConfig` passes with zero findings.
- [ ] **Vulnerability Clearance**: Govulncheck and Trivy report zero CRITICAL or HIGH unresolved vulnerabilities.
- [ ] **License Audit**: Zero unapproved copyleft (AGPL/GPL) dependencies present in release binaries.
- [ ] **Base Image Digests**: Base image references in `Dockerfile` are pinned to immutable SHA-256 digests.

### 2.3 Release Tagging & Attestation
- [ ] **Annotated Git Tag**: Create cryptographically signed git tag:
  ```bash
  git tag -s -m "Release v1.2.0: Institutional ACH & SFTP Ingress" v1.2.0
  git push origin v1.2.0
  ```
- [ ] **SBOM Verification**: Verify `sbom.cyclonedx.json` and `sbom.spdx.json` are attached to the release.
- [ ] **Cosign Signature**: Verify container image signature:
  ```bash
  cosign verify --certificate-identity-regexp ".*" --certificate-oidc-issuer "https://token.actions.githubusercontent.com" ghcr.io/yashwanthgathuku/didactic-octo-funicular:v1.2.0
  ```
