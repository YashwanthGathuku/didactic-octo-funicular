# Private-Network Edge Agent Architecture & Zero-Trust Design

## 1. Architectural Principles & Security Boundary

SentinelFlow's Private Edge Agent enables institutional customers and financial counterparties to integrate on-premise core banking databases, internal SFTP drop zones, and localized file systems into the SentinelFlow gateway **without opening inbound firewall ports**.

```
  Customer Private VPC / On-Prem Network             Internet / Transport           SentinelFlow Cloud Gateway
 ┌──────────────────────────────────────────┐                                  ┌────────────────────────────────┐
 │ [Core Banking DB]     [Local SFTP Vault] │                                  │                                │
 │         │                     │          │                                  │                                │
 │         ▼                     ▼          │                                  │                                │
 │   ┌─────────────────────────────────┐    │  Outbound TLS 1.3 Control Stream │   ┌────────────────────────┐   │
 │   │     SentinelFlow Edge Agent     │════╪══════════════════════════════════╪══>│ Gateway Reverse Ingress│   │
 │   │  - Zero Inbound Listening Ports │    │   (Mutual X.509 mTLS Verified)   │   │  (mTLS Auth Enforced)  │   │
 │   │  - Local Spool Buffer (50 MB)   │    │                                  │   └────────────────────────┘   │
 │   │  - Strict Zero-Shell Invariant  │    │                                  │                                │
 │   └─────────────────────────────────┘    │                                  │                                │
 └──────────────────────────────────────────┘                                  └────────────────────────────────┘
```

---

## 2. Zero-Trust Identity & Mutual TLS (mTLS) Protocol

### 2.1 Outbound-Only Control Plane Connection
- The Edge Agent establishes an outbound HTTPS/TLS 1.3 long-polling or reverse-stream control connection to the Gateway.
- The Agent opens **zero inbound TCP/UDP listening ports** in the customer environment.

### 2.2 Mutual X.509 Certificate Authentication
- **Client Certificate**: Each agent instance is provisioned with a cryptographic private key generated locally on the device (ECDSA P-256 or Ed25519) and an X.509 client certificate $C_{\text{agent}}$.
- **Tenant & Connector Binding**: The certificate's Subject Common Name and SAN encode the tenant ID and connector enrollment ID:
  $$\text{Subject: } \text{CN}=\text{AGENT-01}, \text{OU}=\text{TENANT-CORP}, \text{O}=\text{SentinelFlow Edge}$$
- **Transport State Verification**:
  $$\text{mTLSVerified} \iff (\text{len}(r.\text{TLS}.\text{VerifiedChains}) > 0) \land (\text{Cert.NotAfter} > \text{Now}()) \land (\text{Issuer} == \text{SentinelFlow\_CA})$$
  The `mTLSVerified` flag is derived **exclusively** from verified Go runtime `crypto/tls` state; headers such as `X-Forwarded-Client-Cert` or `X-Client-Verified` are ignored and discarded.

### 2.3 Short-Lived Certificate Rotation & Revocation
- Agent client certificates carry a short validity window ($T_{\text{valid}} = 30\text{ days}$).
- Automated background rotation executes at $T_{\text{rotate}} = 0.5 \times T_{\text{valid}}$ (15 days) using an automated CSR exchange.
- Gateway maintains an active Certificate Revocation List (CRL) and checks OCSP stapling on every handshake.

---

## 3. Cryptographic Governance & Zero-Shell Policy

### 3.1 Signed Configuration & Update Provenance
- All remote polling rules, expectation templates, and connector configs are cryptographically signed by the tenant's security administrator before transmission:
  $$\text{Verify}(\text{ConfigBody}, \text{AdminPublicKey}, \text{Ed25519Signature}) == \text{VALID}$$
- The edge agent verifies the signature locally before applying any configuration change.

### 3.2 Strict Zero-Shell Invariant
- **No Arbitrary Shell**: The agent binary has **zero shell execution packages** (no `os/exec`, `bash`, `sh`, `cmd.exe`, or `powershell`).
- **No Remote Code Execution**: Commands cannot be passed from the gateway. The agent only understands typed task descriptors:
  1. `ACTION_POLL_DIRECTORY`: Lists files in allowlisted path.
  2. `ACTION_STREAM_ARTIFACT`: Streams file bytes over encrypted mTLS tunnel.
  3. `ACTION_EXECUTE_TEMPLATE`: Executes pre-registered SQL template against local database.

### 3.3 Strict Host and Resource Allowlisting
- The agent configuration explicitly bounds accessible paths and database connections:
  - Allowed File Prefixes: `["/var/sftp/inbound/ach/", "D:\\Treasury\\Inbound\\"]`
  - Allowed Database Schemas: `["settlement", "reporting"]`
- Any request attempting to traverse outside allowlisted boundaries (`../`, `C:\Windows`, `pg_catalog`, `/etc/passwd`) is rejected locally by the agent.

---

## 4. Local Metadata Redaction & Bounded Spooling

### 4.1 Bounded Local Spool During Gateway Outages
If the network connection between the customer VPC and SentinelFlow Gateway is interrupted:
- The edge agent records observed file arrivals in a bounded local ring buffer / append-only spool on disk.
- Spool Capacity Ceiling: $M_{\text{spool}} = 50\text{ MB}$.
- If the spool reaches 100% capacity, the agent applies backpressure, halts new file ingestion, and sounds a local operational alert rather than corrupting memory or dropping records.

### 4.2 Local Redaction at the Edge
- Routing transit numbers and account numbers are sanitized and hashed at the edge before telemetry metadata leaves the customer's VPC.
