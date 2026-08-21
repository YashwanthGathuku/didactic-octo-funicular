# SentinelFlow Architecture Decision Records (ADRs)

This index records all major architectural and security decisions made in SentinelFlow. Each record outlines context, trade-offs considered, chosen solution, and permanent operational invariants.

---

## Index of Architectural Decisions

| ADR ID | Decision Title | Status | Primary Invariant |
|---|---|---|---|
| **[ADR-0001](#adr-0001-sqlite-and-postgresql-coexistence--migration)** | Dual Database Engine (SQLite Dev/Edge & PostgreSQL Production) | **ACCEPTED** | Repository layer abstract interface (`Repository`); strict `tenant_id` column indexing. |
| **[ADR-0002](#adr-0002-zero-copy-streaming-nacha-parser--file-identity)** | Zero-Copy Streaming NACHA Ingress Parser | **ACCEPTED** | Fixed 94-byte record validation; deterministic SHA-256 batch hash sum verification. |
| **[ADR-0003](#adr-0003-isolated-python-ai-evaluator-with-zero-write-policy)** | Isolated Python AI Incident Analyst (Zero-Write Policy) | **ACCEPTED** | AI operates in a sandboxed, read-only process; zero mutation or write authority over ledgers. |
| **[ADR-0004](#adr-0004-clean-room-sftp-ingress-and-agplv3-legal-isolation)** | Clean-Room SFTP Ingress & AGPLv3 Legal Boundary | **ACCEPTED** | Zero AGPL code in SentinelFlow binaries; communication strictly over HMAC-SHA256 HTTP webhooks. |
| **[ADR-0005](#adr-0005-outbound-only-mtls-edge-agent-architecture)** | Outbound-Only mTLS Edge Agent for Private Customer VPCs | **ACCEPTED** | No listening ports on customer networks; transport-derived mTLS verification (`r.TLS.VerifiedChains`). |
| **[ADR-0006](#adr-0006-dual-control-quarantine-release--cryptographic-proofs)** | Dual-Control Separation of Duties for High-Risk Releases | **ACCEPTED** | Two distinct human operators required; creator of quarantined batch cannot approve release. |
| **[ADR-0007](#adr-0007-immutable-linear-hash-chained-audit-ledger)** | Tamper-Evident Append-Only Audit Ledger | **ACCEPTED** | SHA-256 linear hash chaining; record updates or table truncations break signature chain. |
| **[ADR-0008](#adr-0008-multi-database-connector-platform-with-safe-templates)** | Multi-Database Connectors with Parameterized Templates | **ACCEPTED** | Zero arbitrary SQL execution from UI/AI; admin-approved templates with driver parameter binding. |
| **[ADR-0009](#adr-0009-google-adk-multi-agent-fleet-and-model-armor-orchestration)** | Google ADK Multi-Agent Fleet & Model Armor Orchestration | **ACCEPTED** | Orchestrated specialist fleet with least-privilege tool scopes; pre-screened via Google Cloud Model Armor. |

---

## Detailed Records

### ADR-0001: SQLite and PostgreSQL Coexistence & Migration
* **Context**: Development speed and lightweight edge deployments require zero-dependency local execution, while institutional high-concurrency production requires PostgreSQL with connection pooling.
* **Decision**: Maintain a shared repository interface (`gateway/internal/repository/repository.go`). Implement SQLite via `modernc.org/sqlite` (pure Go, CGO-free) and PostgreSQL via `jackc/pgx/v5`.
* **Consequences**: Local smoke tests and CI test suites run in under 40 seconds with zero external database dependencies. Production environments scale horizontally against managed PostgreSQL.

---

### ADR-0002: Zero-Copy Streaming NACHA Parser & File Identity
* **Context**: Multi-gigabyte NACHA batch files contain millions of payment entries. Reading whole files into memory causes garbage collection stalls and OOM crashes.
* **Decision**: Implemented a streaming record reader in `gateway/internal/nacha/parser.go` that reads exactly 94-byte chunks, validates batch headers, and computes running balance checksums in a single pass.
* **Consequences**: Ingestion memory footprint remains bounded at $< 16\text{MB}$ regardless of incoming file size.

---

### ADR-0003: Isolated Python AI Evaluator with Zero-Write Policy
* **Context**: LLMs provide valuable insights for triage of malformed files and payment anomalies, but cannot be trusted to execute financial transactions due to prompt injection and hallucination risks.
* **Decision**: Deployed the AI tier as a standalone Python container (`ai-tier/`) communicating strictly over internal HTTP. The AI tier has no database write credentials and no execution authority.
* **Consequences**: Even if an adversarial NACHA file compromises the LLM context, it cannot mutate records, approve quarantined batches, or pivot onto internal networks.

---

### ADR-0004: Clean-Room SFTP Ingress and AGPLv3 Legal Isolation
* **Context**: Many SFTP servers (such as SFTPGo) are licensed under AGPLv3. Static or dynamic linking in production would create legal ambiguity for enterprise deployments.
* **Decision**: SentinelFlow contains **zero SFTPGo code or libraries**. Ingestion is decoupled via generic HMAC-SHA256 HTTP webhooks and a local reconciliation scanner ([`docs/engineering/SFTPGO_LICENSE_AND_INTEGRATION_DECISION.md`](file:///c:/Users/Gathu/Projects/fintech/docs/engineering/SFTPGO_LICENSE_AND_INTEGRATION_DECISION.md)).
* **Consequences**: SentinelFlow binaries remain 100% clean under permissive open-source licenses (MIT/Apache-2.0).

---

### ADR-0005: Outbound-Only mTLS Edge Agent Architecture
* **Context**: Enterprise banks refuse inbound firewall openings or exposed VPN tunnels into their private core networks.
* **Decision**: The SentinelFlow Edge Agent initiates an outbound-only, TLS 1.2+ mTLS control stream to the Gateway API ([`docs/engineering/EDGE_AGENT_DESIGN.md`](file:///c:/Users/Gathu/Projects/fintech/docs/engineering/EDGE_AGENT_DESIGN.md)).
* **Consequences**: Edge agents operate securely behind enterprise NATs without requiring public IP addresses or inbound listening ports.

---

### ADR-0006: Dual-Control Quarantine Release & Cryptographic Proofs
* **Context**: An insider with administrative access could unilaterally approve a flagged or fraudulent payment batch.
* **Decision**: Enforce identity-bound dual-control approval with cryptographic artifact and policy integrity binding in `gateway/internal/review/release.go` and `review.go`. An approval requires two distinct authenticated identities. The operator who uploaded or quarantined an artifact cannot approve its release. State transitions are committed to the append-only linear hash chain ledger.
* **Consequences**: Eliminates single-operator fraud risk and complies with SOX, FFIEC, and Nacha Operating Rules.

---

### ADR-0007: Immutable Linear Hash-Chained Audit Ledger
* **Context**: Compliance regulations require non-repudiable audit logs that cannot be altered even by database administrators.
* **Decision**: Implement a linear hash chain in `gateway/internal/ledger/ledger.go` where `Record_N.Hash = SHA256(Record_{N-1}.Hash + Record_N.Data)`.
* **Consequences**: Any direct DB manipulation or deletion immediately invalidates subsequent chain verification hashes.

---

### ADR-0008: Multi-Database Connector Platform with Safe Templates
* **Context**: Users need to sync payment metadata from PostgreSQL, Oracle, Snowflake, and BigQuery without exposing databases to SQL injection.
* **Decision**: Ban arbitrary SQL execution. All queries must reference administrator-registered parameterized templates with strict capability bounds (`gateway/internal/connectors/query.go`).
* **Consequences**: Prevents SQL injection by construction; restricts queries to explicit schema allowlists.

---

### ADR-0009: Google ADK Multi-Agent Fleet and Model Armor Orchestration
* **Context**: Monolithic single-prompt AI analysts lack specialized domain depth, risk prompt injection across broad tool scopes, and cannot retain cross-session memory without security leakage.
* **Decision**: Deployed a hierarchical 6-agent fleet using the Google Agent Development Kit (ADK) and Gemini 2.5 Flash (`google-genai` SDK), gated by Google Cloud Model Armor for input/output screening, with persistent tenant-isolated Memory Bank storage.
* **Consequences**: Each agent operates within a declared least-privilege tool scope (e.g. ComplianceAgent reads rules only; RemediationAgent proposes derived artifacts without mutating originals). Model Armor blocks adversarial prompt injections and PII leakage.
