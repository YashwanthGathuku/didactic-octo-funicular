# SentinelFlow Engineering & System Design Interview Study Guide

This guide maps real architectural components in SentinelFlow to classic technical interview questions in **System Design**, **Concurrency & Distributed Systems**, and **Fintech Security / Cryptography**.

---

## 1. System Design & Distributed Architecture

### Q1: "How would you design a high-throughput, fault-tolerant financial file ingestion system?"
* **Relevant SentinelFlow Module**: [`gateway/internal/nacha/`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/nacha/) and [`gateway/internal/jobs/`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/jobs/).
* **Key Design Concept**:
  - **Streaming Zero-Copy Parsing**: Rather than loading 1GB+ files into heap memory, use a fixed 94-byte chunk reader with running checksum accumulators.
  - **Asynchronous Decoupling**: Ingestion immediately stores the raw payload in S3/MinIO, records a durable job in the queue, and returns `202 Accepted` to the client.
  - **Deterministic Idempotency**: Deduplicate identical uploads using a composite SHA-256 key:
    $$\text{DedupeKey} = \text{SHA256}(\text{TenantID} \parallel \text{VirtualPath} \parallel \text{FileSHA256} \parallel \text{SizeBytes})$$

---

### Q2: "How do you build a tamper-evident audit ledger without blockchain overhead?"
* **Relevant SentinelFlow Module**: [`gateway/internal/ledger/ledger.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/ledger/ledger.go).
* **Key Design Concept**:
  - **Linear Cryptographic Hash Chaining**: Each audit entry $N$ contains `prev_hash` equal to the cryptographic digest of entry $N-1$:
    $$H_N = \text{SHA256}(H_{N-1} \parallel \text{Timestamp} \parallel \text{Actor} \parallel \text{PayloadHash})$$
  - **Verification Complexity**: Validating the chain is a fast $O(N)$ sequential scan. Any out-of-band modification by a rogue database admin breaks the hash chain at entry $N+1$.

---

## 2. Concurrency & Go Runtime Engineering

### Q3: "How do you prevent race conditions and duplicate job claims across distributed workers?"
* **Relevant SentinelFlow Module**: [`gateway/internal/jobs/worker.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/jobs/worker.go) and [`gateway/worker_test.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/worker_test.go).
* **Key Design Concept**:
  - **Atomic Lease Acquisition**: Workers claim jobs using optimistic locking or atomic database updates (`UPDATE jobs SET worker_id = $1, lease_expires_at = $2 WHERE id = $3 AND (lease_expires_at < NOW() OR worker_id IS NULL)`).
  - **Lock Contention Backoff**: Handles `SQLITE_BUSY` (or PostgreSQL serialization failures) using jittered exponential backoff rather than unbounded spinning.
  - **Single-Flight Enqueue**: `TestConcurrentEnqueueOfOneKeyProducesOneJob` tests and proves that 20 parallel goroutines enqueueing the same key produce exactly one scheduled job.

---

### Q4: "How do you coordinate graceful shutdown across dozens of long-running goroutines?"
* **Relevant SentinelFlow Module**: [`gateway/internal/sftp/reconcile.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/sftp/reconcile.go#L45-L75).
* **Key Design Concept**:
  - **Context Propagation & WaitGroups**: Root context passed to all worker goroutines. Upon `SIGINT`/`SIGTERM`, root context cancels, terminating `select { case <-ctx.Done(): }` loops, and `sync.WaitGroup.Wait()` ensures in-flight transactions complete before process exit.

---

## 3. Fintech Security & Applied Cryptography

### Q5: "How do you implement Separation of Duties (Dual-Control) for high-value financial movements?"
* **Relevant SentinelFlow Module**: [`gateway/internal/review/dual_control.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/review/dual_control.go).
* **Key Design Concept**:
  - **Cryptographic Non-Repudiation**: A single user account cannot initiate AND approve a transfer.
  - **Two-Person Rule Invariant**:
    $$\text{ValidateRelease}(p) \implies (\text{CreatorID} \neq \text{ApproverID}) \land \text{ValidRole}(\text{ApproverID}) \land (\text{Now} \le \text{Expiry})$$

---

### Q6: "How do you prevent credential leakage in distributed logging systems (Datadog/Elasticsearch)?"
* **Relevant SentinelFlow Module**: [`gateway/internal/secrets/store.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/secrets/store.go) and [`gateway/internal/connectors/disclosure_test.go`](file:///c:/Users/Gathu/Projects/fintech/gateway/internal/connectors/disclosure_test.go).
* **Key Design Concept**:
  - **Opaque Secret Encapsulation**: Passwords and tokens are wrapped in custom types implementing `fmt.Stringer`, `fmt.GoStringer`, and `json.Marshaler` returning `"[REDACTED]"`.
  - **Sanitized Error Categorization**: Database drivers returning connection errors have raw error strings mapped to abstract enums (`ErrorAuthentication`, `ErrorTimeout`) before bubbling up to logging layers.

---

### Q7: "How do you securely connect to on-premise customer networks without opening inbound firewall ports?"
* **Relevant SentinelFlow Module**: [`edge-agent/main.go`](file:///c:/Users/Gathu/Projects/fintech/edge-agent/main.go) and [`docs/engineering/EDGE_AGENT_DESIGN.md`](file:///c:/Users/Gathu/Projects/fintech/docs/engineering/EDGE_AGENT_DESIGN.md).
* **Key Design Concept**:
  - **Outbound-Only mTLS Stream**: The edge agent connects outbound over TLS 1.2+ to the central gateway, establishing a long-lived bidirectional gRPC/SSE stream to receive work without any inbound listening ports.
  - **Transport-Derived Identity**: The gateway extracts client identity directly from `r.TLS.VerifiedChains`, rejecting spoofed reverse-proxy headers (`X-Forwarded-Client-Cert`).
