# Third-Party Reference: Microsoft Data Formulator & Flint

## 1. Executive Summary

This document establishes the architectural evaluation, licensing boundaries, and adaptation strategy for concepts inspired by **Microsoft Data Formulator** and the **Flint** semantic visualization language in relation to the planned **SentinelFlow Lens** subsystem.

SentinelFlow is a fortified financial reliability system governed by strict zero-trust security and deterministic invariants. While open-source research from Microsoft Data Formulator provides valuable UX and concept design patterns for interactive data exploration, **no unconstrained third-party agent runtime, execution engine, or database connector will ever be directly imported into SentinelFlow**.

---

## 2. Repository & License Provenance

| Project Name | Repository Reference | Primary Authors | License | SentinelFlow Assessment |
|---|---|---|---|---|
| **Microsoft Data Formulator** | `microsoft/data-formulator` | Microsoft Research (MSR) | **MIT License** | Conceptual reference for Data Thread DAG, branching visual exploration UX, and lineage. |
| **Flint Visualization** | `microsoft/flint-chart` | Microsoft Research (MSR) | **MIT License** | Evaluated for future semantic visualization grammar (Vega-Lite / ECharts target). |

### License Compliance & Attribution Invariant
Both repositories are released under the permissive **MIT License**.
If any frontend components or visualization schemas derived from these repositories are introduced in future implementation phases:
1. The original copyright notice and MIT permission notice must be retained in all copies or substantial portions of the software.
2. Attribution must be preserved in `docs/third_party/NOTICES.md` and inline source headers.
3. Independent licensing verification must be repeated prior to importing any third-party code.

---

## 3. Adaptation Strategy: What SentinelFlow Adapts vs. Rejects

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SENTINELFLOW LENS ADAPTATION BOUNDARY                    │
├─────────────────────────────────────────────┬───────────────────────────────┤
│        ADAPTED CONCEPTUAL DESIGNS           │   STRICTLY REJECTED RUNTIMES  │
├─────────────────────────────────────────────┼───────────────────────────────┤
│ ✓ Data Thread / Branching Investigation DAG │ ✗ Unrestricted LLM-generated  │
│   (Immutable parent-child exploration tree) │   raw SQL execution           │
│                                             │                               │
│ ✓ Interactive Visual Exploration UX         │ ✗ Direct production database  │
│   (Shelf-based field binding, recommendations)│   credential binding          │
│                                             │                               │
│ ✓ Dataset Lineage & Transform Graphs        │ ✗ Unconstrained Python/code   │
│   (Derived data flow tracking)              │   execution sandboxes         │
│                                             │                               │
│ ✓ Chart Recommendation Paradigms            │ ✗ Third-party LLM agent       │
│   (Visual intent to Vega-Lite grammar)      │   authority / control planes  │
│                                             │                               │
│ ✓ Investigation Report Composition          │ ✗ Third-party authentication  │
│   (Exporting findings with evidence)        │   or tenant isolation schemes │
└─────────────────────────────────────────────┴───────────────────────────────┘
```

### 3.1 Concepts Adapted for SentinelFlow Lens
1. **Data Thread (Investigation Graph)**:
   - Exploration is represented as an append-only, branching Directed Acyclic Graph (DAG) of investigation nodes (`analytics_investigation_nodes`).
   - Every analytical step records its parent node, input dataset, transform intent, generated visualization, and user notes.
2. **Visual Exploration UX**:
   - Field-driven visualization authoring (binding metrics and dimensions to visual channels).
   - Context-aware chart recommendations based on data grain and temporal attributes.
3. **Dataset Lineage**:
   - Explicit lineage tracking from root curated views through intermediate filtered subsets.
4. **Report Composition**:
   - Bundling multiple investigation branches into an auditable incident post-mortem or compliance export.

### 3.2 Architectural Elements Explicitly Banned / Not Integrated
1. **No LLM-Generated Raw SQL**:
   - In Data Formulator, models generate direct SQL or pandas code. In SentinelFlow, models produce only structured `QueryIntent` (JSON AST). Raw SQL generation by LLMs is strictly prohibited.
2. **No Direct Production Database Access**:
   - The analytics tier never connects directly to operational financial OLTP tables. It operates exclusively against curated, read-only analytics views or ephemeral sandboxed DuckDB instances.
3. **No Unrestricted Code Execution**:
   - Arbitrary Python code execution environments are banned. All transformations execute via SentinelFlow's deterministic Safe Query Compiler.
4. **No Third-Party Control Plane or Auth**:
   - All analytics queries must pass through SentinelFlow's native Tenant Scope, JWT Principal verification, P03 Policy Engine, and P04 Tool Gateway.

---

## 4. Evaluation of Flint Semantic Visualization

**Flint** (`microsoft/flint-chart`) is a high-level declarative chart specification language designed for AI models. It separates semantic visualization intent (goals, encodings, chart types) from low-level rendering engines (Vega-Lite, ECharts, Chart.js).

### Evaluation for SentinelFlow Lens (Future Phase):
- **Benefits**:
  - Compact JSON representation well-suited for structured model outputs.
  - Cross-compiles deterministically to standard Vega-Lite specifications rendered in React.
- **Constraints**:
  - Must run entirely client-side or within a sandboxed Go/TypeScript compiler.
  - Must not make outbound network requests or load remote scripts.
- **Status**: Evaluated for post-hackathon visualization extensions (`PLANNED`).
