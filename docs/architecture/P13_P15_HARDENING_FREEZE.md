# P13–P15 — Production Hardening & Red-Team Freeze

## Status

**IMPLEMENTED — local regression execution required before the capability matrix may call the new controls TESTED.**

This compressed phase intentionally adds no new agent intelligence. It closes operational failure modes around the already-governed agent fleet while preserving SentinelFlow's core authority boundary:

> **Managed/AI execution may stop or degrade; financial truth must not move with it.**

## Authority invariants

```text
KillSwitch != BusinessPolicy
BudgetExhausted != FinancialDecision
AgentUnavailable != FinancialControlFailure
ObservabilityFailure != EvidenceMutation
ManagedRetry != DuplicateBusinessExecution
```

The existing Go Tool Gateway, Policy Engine, workflow state machine, deterministic validators, verification service, M1 memory, human review and release gates remain authoritative.

## Agent kill switches

`gateway/internal/executioncontrol/controller.go` implements process-level emergency controls with four scopes:

- `GLOBAL`
- `TENANT`
- `WORKFLOW`
- `AGENT`

Every switch carries a monotonically increasing `generation`. A stale update cannot overwrite a newer stop instruction. Optional expiry allows a temporary operational stop without silently deleting history.

The control is deliberately agent-specific: human/service recovery paths are not disabled by an *agent* kill switch. Their normal authentication, RBAC and policy checks remain mandatory.

## Execution budgets

The control plane supports bounded:

- logical tool calls per workflow;
- concurrent agent executions per workflow;
- workflow execution duration.

`gateway/internal/toolgateway/context.go` also carries the trusted budget snapshot into each tool request. Because the fields are server-generated and included in the canonical context hash, a model cannot increase its own budget without changing the protected execution context.

### Tool Gateway pre-execution rule

For an `AGENT` caller:

```text
Permit =
    !AgentExecutionDisabled
    ∧ ToolCallOrdinal <= MaxToolCalls
    ∧ Now < WorkflowStartedAt + MaxWorkflowDuration
```

A failed term stops execution before tool lookup, policy evaluation, idempotency mutation or side effect.

## Process-level vs durable truth

The new `executioncontrol.Controller` is intentionally documented as **process-level operational control**. Its counters are not financial persistence and are not marketed as a distributed exactly-once budget service.

Durable safety remains provided by the existing workflow repository, Tool Gateway database idempotency, candidate derivation idempotency and transaction/outbox boundaries.

For requests crossing processes, the authoritative Go orchestrator should inject the trusted budget/kill-switch snapshot into `TrustedExecutionContext`; the Tool Gateway then rechecks that snapshot immediately before execution.

## Failure isolation

| Failure | Required behavior |
|---|---|
| Global/tenant/workflow/agent kill | Agent execution fails closed; operator recovery remains possible |
| Tool-call budget exhausted | No further agent tool execution |
| Workflow deadline exceeded | Agent execution stops; durable financial state remains unchanged |
| Agent Runtime unavailable | Go deterministic pipeline remains internally consistent |
| Agent Gateway unavailable | Managed agents cannot reach tools |
| Model/Model Armor unavailable | Model-dependent stage becomes unavailable; deterministic controls continue |
| Telemetry unavailable | No authorization is granted and no evidence is rewritten |

## Verification

Run the repository-wide freeze gate:

```bash
bash scripts/verify_submission_freeze.sh
```

The gate performs no deployment and no production mutation. It verifies the P11.5 platform truth gate, P12.5 return-risk truth gate, execution-control/Tool-Gateway race tests, full Python tests/evals, frontend tests/build, secret/model-version checks and generated-document consistency.

## Deferred beyond the submission freeze

The following are intentionally not added during P13–P15:

- dynamic agents;
- new databases;
- distributed budget service;
- predictive ML;
- SentinelFlow Lens;
- additional payment rails;
- autonomous release;
- infrastructure-admin agent tools.

The submission phase prioritizes proof of the existing governed architecture over feature count.
