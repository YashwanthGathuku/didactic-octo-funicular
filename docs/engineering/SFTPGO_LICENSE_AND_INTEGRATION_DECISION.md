# Clean-Room SFTP Ingress Architecture & Legal Integration Decision

## 1. Legal Analysis & Clean-Room Licensing Decision

### 1.1 AGPLv3 Evaluation & Corporate Policy
- **Software Evaluated**: SFTPGo (Open Source Community Edition, licensed under GNU Affero General Public License v3).
- **Core Invariant**: SentinelFlow is an institutional payments and custody gateway. **SentinelFlow contains ZERO SFTPGo source code, imports, or linked libraries.**
- **Network Boundary vs. Derivative Works**:
  - The Free Software Foundation (FSF) and standard copyright law establish that separate user-space processes communicating over generic network protocols (HTTP/JSON webhooks, POSIX filesystems, AWS S3 APIs) do **not** create a combined or derivative work.
  - However, because AGPLv3 Section 13 ("Remote Network Interaction") introduces legal ambiguity for commercial multi-tenant SaaS platforms, institutional policy requires formal separation.
- **Decision & Standing Legal Notice**:
  > [!IMPORTANT]
  > **LEGAL REVIEW IS REQUIRED FOR ANY COMMERCIAL DEPLOYMENT.**
  > SentinelFlow's core engine is 100% clean-room and vendor-agnostic. In enterprise deployments, organizations may choose between:
  > 1. Running unmodified SFTPGo Community Edition in an isolated sidecar container over standard HTTP webhooks.
  > 2. Procuring an **SFTPGo Enterprise Commercial License** for commercial warranty and indemnification.
  > 3. Using fully proprietary or cloud-native SFTP solutions (e.g. AWS Transfer Family, Azure Blob SFTP, or OpenSSH).

---

## 2. Ingress Spike & Protocol Invariants

### 2.1 The "In-Flight File" Ingestion Trap
Traditional filesystem watchers (`inotify` / `fsnotify` on `IN_CREATE` or `IN_MODIFY`) trigger immediately when a counterparty begins streaming bytes over an SSH/SFTP channel. Reading an in-flight file results in zero-byte files, truncated record parsing, or corrupted balances.

### 2.2 Finalized Event Mechanics
A file must only be ingested when the transfer channel has completed and the file descriptor is closed cleanly (`SSH_FXP_CLOSE` with `SSH_FX_OK`).

```
 [ Client SFTP Connection ]
           │
           ▼
 1. SSH_FXP_OPEN (write to temporary path: .upload_tmp.xxxx)
 2. SSH_FXP_WRITE (streaming chunk transfers)
 3. SSH_FXP_CLOSE (transfer complete)
           │
           ▼
 [ Atomic Rename to Target Path: /inbound/ach/payroll_2026.ach ]
           │
           ▼
 [ Compute SHA-256 Checksum & Exact File Size ]
           │
           ▼
 [ Emit Authenticated Finalized Webhook: action="upload", status=1 ]
```

### 2.3 Exact Finalized Event Schema
```json
{
  "event_id": "EVT-SFTP-99482710",
  "action": "upload",
  "status": 1,
  "timestamp": 1771448400000,
  "username": "meridian_treasury_svc",
  "ip_address": "198.51.100.42",
  "virtual_path": "/inbound/ach/payroll_2026_08_18.ach",
  "fs_path": "/data/sftp/meridian_treasury_svc/inbound/ach/payroll_2026_08_18.ach",
  "file_size": 24576,
  "sha256_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "elapsed_ms": 142
}
```

---

## 3. Mathematical & Algorithmic Specifications

### 3.1 Webhook Authentication & Integrity
The ingress endpoint validates incoming webhook authenticity using HMAC-SHA256:
$$\text{Signature} = \text{Hex}(\text{HMAC-SHA256}(K_{\text{tenant\_secret}}, T \parallel "\n" \parallel \text{PayloadBody}))$$

Where:
- $K_{\text{tenant\_secret}}$ is the secret key held in `internal/secrets`.
- $T$ is the `X-Sentinel-Timestamp` header. Signatures with $|T - \text{Now}| > 300\text{s}$ are rejected to prevent replay attacks.

### 3.2 Idempotency Deduplication Key
$$\text{DedupeKey} = \text{Hex}(\text{SHA-256}(\text{TenantID} \parallel ":" \parallel \text{VirtualPath} \parallel ":" \parallel \text{SHA256Hash} \parallel ":" \parallel \text{SizeBytes}))$$

If a webhook is delivered multiple times (at-least-once transport), duplicate arrivals resolve to the existing `file_instance_id` and `job_id` without creating orphan execution jobs.

### 3.3 Lost Webhook Reconciliation Algorithm
If a network partition causes a webhook delivery to be lost, the periodic reconciliation scanner recovers un-ingested files:

$$\text{Scan Condition}: \forall f \in \text{StorageInventory}, \text{Age}(f) \ge \Delta_{\text{settle}} \land \text{Extension}(f) \neq \text{".tmp"}$$

$$\text{Missing Check}: \nexists \text{ row in } \text{file\_instances WHERE } \text{tenant\_id} = T \land \text{sha256\_hash} = \text{Hash}(f)$$

When a missing file is discovered, the scanner computes its cryptographic hash, generates an idempotent synthetic event with actor attribution `SFTP_RECONCILIATION_SCANNER`, and enqueues it for deterministic ingestion.
