# SentinelFlow Lens: Governed Financial Operations Intelligence

## 1. Status

> **Lens Lite status:** `IMPLEMENTED — LOCAL VERIFICATION REQUIRED`  
> **Authority:** advisory/read-only for financial truth  
> **Verification command:** `bash scripts/verify_lens_lite.sh`

SentinelFlow Lens is the intelligence layer beside the deterministic financial control plane. It turns tenant-scoped operational records into bounded visual investigations while preserving SentinelFlow's central authority rule:

> **Analytics may explain financial truth; analytics cannot create financial truth.**

Lens Lite is inspired by the branching analytical workflow of Microsoft Data Formulator, but it is not a fork and it is not a general-purpose BI authoring system.

## 2. What is implemented in Lens Lite

The current implementation contains:

- five fixed semantic datasets: `ach_return_intelligence`, `incident_trends`, `validation_findings`, `file_operations`, and `agent_operations`;
- a strongly typed `QueryIntent` contract with no raw-SQL field;
- a Go allowlisted semantic compiler that injects authenticated tenant scope server-side;
- mandatory bounded time ranges (maximum 90 days), allowlisted dimensions/metrics/filter operators, and maximum 500 result rows;
- parameterized SQLite execution over existing SentinelFlow operational tables plus the dedicated `lens_return_events` table;
- SHA-256 query and result digests;
- conservative provenance: synthetic observations cannot become authoritative evidence;
- append-only Investigation Threads whose nodes are server-reexecuted before query/result hashes are persisted;
- a three-pane React investigation workbench with thread, analytical canvas, and provenance surfaces;
- `lens.query` as a read-only `ANALYTICS_QUERY` Tool Gateway capability for governed agent/future MCP invocation;
- a deterministic synthetic ACH-return demo fixture explicitly labeled `SYNTHETIC_DEMO` and `verified=0`.

Lens Lite does **not** currently implement arbitrary SQL, arbitrary joins, a general report designer, DuckDB, Flint/Vega-Lite runtime integration, natural-language-to-intent generation, deployed MCP transport, or a new financial execution authority.

## 3. Authority model

The implemented paths are deliberately different for humans and managed agents:

```text
Human operator
    ↓ authenticated SentinelFlow HTTP/RBAC
Lens semantic endpoint
    ↓
Go semantic compiler
    ↓
tenant-scoped parameterized read

Managed agent / future MCP adapter
    ↓ cryptographically verified managed ingress
SentinelFlow Tool Gateway
    ↓ ANALYTICS_QUERY + policy
lens.query
    ↓
Go semantic compiler
    ↓
tenant-scoped parameterized read
```

In both paths, the model or client supplies a typed intent, not executable database text.

### Permanent invariants

\[
\boxed{LensResult \neq VerificationResult}
\]

\[
\boxed{Insight \neq PolicyDecision}
\]

\[
\boxed{MemoryRecall \neq Evidence}
\]

\[
\boxed{MCPAccess \neq FinancialAuthority}
\]

Lens cannot approve a release, unquarantine an artifact, mutate an original payment file, bypass deterministic validation, or satisfy dual control.

## 4. QueryIntent contract

Example:

```json
{
  "schema_version": "1.0",
  "dataset_id": "ach_return_intelligence",
  "time_range": {
    "start": "2026-08-01T00:00:00Z",
    "end": "2026-08-26T00:00:00Z"
  },
  "metrics": ["return_count", "associated_amount_cents"],
  "dimensions": ["day", "return_code"],
  "filters": [
    {
      "field": "return_code",
      "op": "IN",
      "values": ["R10", "R11"]
    }
  ],
  "order_by": [
    {"field": "day", "direction": "ASC"}
  ],
  "limit": 300
}
```

Unknown fields, unknown datasets, duplicate selected fields, invalid filter operators, time ranges over 90 days, and row limits above 500 fail closed.

## 5. Dataset registry

| Dataset | Current source | Purpose | Evidence behavior |
|---|---|---|---|
| `ach_return_intelligence` | `lens_return_events` | ACH return trends, partner and reason-code concentration | Only verified `CURATED_IMPORT` rows linked to tenant-bound incidents may emit incident evidence refs |
| `incident_trends` | `incidents` | Operational incident patterns | System-record provenance; no automatic claim-level evidence refs |
| `validation_findings` | `validation_findings` | Deterministic finding concentration | Deterministic-finding provenance; no raw financial payload |
| `file_operations` | `file_instances` | Artifact state and ingestion-volume metadata | Metadata only |
| `agent_operations` | `agent_runs` | Agent status, latency and token telemetry | No private chain-of-thought |

The registry contains static field expressions owned by SentinelFlow. A user cannot supply table names, column expressions, SQL fragments, or credentials.

## 6. Synthetic-data boundary

Migration `023_lens_lite.sql` enforces:

```text
source_type ∈ {SYNTHETIC_DEMO, CURATED_IMPORT}
verified ∈ {0,1}
SYNTHETIC_DEMO ⇒ verified = 0
```

A tenant-bound composite foreign key also prevents a Lens observation in one tenant from linking to an incident in another tenant.

Therefore:

\[
\boxed{SyntheticDemo \neq Evidence}
\]

The demo generator at `scripts/generate_lens_demo_data.py` creates a deterministic 45-day return history with a controlled R11 concentration for fictional partner `TEST-PAYROLL-17`. It is useful for visualization and demo rehearsal, not for proving real financial outcomes.

## 7. Investigation Thread

Lens persists investigations in:

- `lens_investigations`
- `lens_investigation_nodes`

Nodes are append-only through database triggers. When a node is added, the server **reexecutes the semantic query** and derives the result hash and evidence refs itself; browser-supplied hashes or evidence references are never authoritative inputs.

Each node stores:

- parent relationship;
- question;
- semantic QueryIntent;
- query hash;
- result hash;
- chart specification;
- server-derived evidence refs;
- actor and timestamp.

Raw result rows are intentionally not persisted in the investigation journal in Lens Lite.

## 8. Tool Gateway integration

`lens.query` is registered as:

```text
Tool ID:              lens.query
Capability:           ANALYTICS_QUERY
Side effect:          READ_ONLY
Policy action:        QUERY_ANALYTICS
Autonomy:             A1-compatible
Raw SQL:              not in contract
Financial mutation:   impossible through the tool
```

The fixed managed roster grants `lens.query` only to the Incident Commander and Return Risk specialist. Agent Identity is still only authentication; Tool Gateway capability + policy remain required.

## 9. UI

The Lens Lite workspace follows SentinelFlow's financial control-room visual language rather than a generic dashboard or chatbot. It has three principal surfaces:

```text
Investigation Thread | Analytical Canvas | Provenance
```

The first demo questions are deterministic presets such as:

- Why did ACH return activity change this month?
- Which partners are driving ACH returns?
- Where are deterministic blocking findings clustering?
- How is governed agent runtime latency trending?

Natural-language planning is intentionally deferred until it can be routed through the existing guarded model boundary without weakening the semantic contract.

## 10. Local verification

Run from repository root:

```bash
bash scripts/verify_lens_lite.sh
```

The gate verifies synthetic-data provenance, Lens Go tests, whole-gateway compilation, the no-raw-SQL contract, frontend tests/build, documentation synchronization, and then reruns the original 12-stage submission freeze.

Lens Lite should move from `IMPLEMENTED` to `TESTED` in the capability matrix only after this command succeeds on the project toolchain.

## 11. Post-hackathon roadmap

These are **not hackathon Lens Lite claims**:

- guarded natural-language → QueryIntent planning;
- Flint/Vega-Lite or equivalent richer semantic visualization compilation;
- governed MCP transport adapters for databases/FinOps/observability systems;
- curated warehouse views and optional analytical replicas;
- richer comparisons, joins, cohorts, anomaly scoring and saved reports;
- investigation-to-authoritative-incident handoff with full multi-agent workflow initiation;
- exportable evidence packages.

These extensions must preserve the same authority boundary rather than turning Lens into an unrestricted database copilot.
