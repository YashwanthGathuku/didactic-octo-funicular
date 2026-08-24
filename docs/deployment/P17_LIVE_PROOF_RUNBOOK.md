# P17 Live Google Managed-Platform Proof Runbook

P17 is deployment/proof work, not a transfer of financial authority to Google-managed infrastructure.

## Invariants

- `AgentRuntime != WorkflowAuthority`
- `AgentIdentity != ToolAuthorization`
- `AgentGatewayAllow != ToolExecutable`
- `RegistryPresence != SentinelFlowRosterAllows`
- `ManagedSessionID != WorkflowID`
- `ModelArmorPass != Authorization`
- `MemoryRecall != Evidence`

The Go control plane remains authoritative for workflow state, tenant scope, Tool Gateway, policy, candidate derivation, deterministic validation/verification, review, release, evidence and M1 memory.

## Proof-state vocabulary

Use only:

- `PASS_LIVE`: a real Google managed call/resource was observed successfully.
- `TESTED`: deterministic/local integration tests ran successfully.
- `IMPLEMENTED`: code/config exists but no live proof exists.
- `NOT_RUN`: intentionally not executed.
- `FAIL`: live/local proof attempted and failed.

Never upgrade a managed service from `NOT_RUN`/`IMPLEMENTED` because a mock or local simulation passed.

## 0. Local freeze first

```bash
bash scripts/verify_submission_freeze.sh
```

Do not create cloud resources while the local freeze gate is red.

## 1. Read-only Google Cloud preflight

```bash
export GOOGLE_CLOUD_PROJECT=telos-agent
export GOOGLE_CLOUD_LOCATION=us-central1
bash deployment/gcp/p17_preflight.sh
```

The preflight checks gcloud auth, ADC, active project, billing visibility and required APIs. It does not enable APIs or create resources.

If ADC is missing, complete the interactive Google flow yourself:

```bash
gcloud auth application-default login
gcloud auth application-default set-quota-project "$GOOGLE_CLOUD_PROJECT"
```

Never paste an OAuth token, ADC file, private key or password into documentation or chat.

## 2. Prepare one development staging bucket

Use one existing dedicated development bucket where possible. The deployment script requires an explicit `gs://...` value and does not create buckets implicitly.

```bash
export SENTINEL_AGENT_RUNTIME_STAGING_BUCKET=gs://YOUR_P17_DEV_BUCKET
```

Keep the live fixture synthetic and tiny.

## 3. Deploy the real ADK fleet to Agent Runtime

Dry-run first:

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/deploy_agent_runtime.py \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --location "$GOOGLE_CLOUD_LOCATION" \
  --staging-bucket "$SENTINEL_AGENT_RUNTIME_STAGING_BUCKET"
```

Only after the plan is correct:

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/deploy_agent_runtime.py \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --location "$GOOGLE_CLOUD_LOCATION" \
  --staging-bucket "$SENTINEL_AGENT_RUNTIME_STAGING_BUCKET" \
  --execute
```

Capture the returned reasoning-engine resource ID. Do not call this `PASS_LIVE` until the create succeeds and the resource can be read back.

```bash
export SENTINEL_RUNTIME_ENGINE_ID=YOUR_ENGINE_ID
PYTHONPATH=ai-tier python ai-tier/runtime/inspect_agent_runtime.py \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --location "$GOOGLE_CLOUD_LOCATION" \
  --engine-id "$SENTINEL_RUNTIME_ENGINE_ID"
```

Required proof:

- resource exists in the intended project/region;
- `spec.effectiveIdentity` is a system Agent Identity (`agents....system.id.goog/.../reasoningEngines/...`), not a service account;
- model/config remains SentinelFlow's intended Gemini 3.5 path.

Set the IAM-member form only from the server-returned identity:

```bash
export SENTINEL_AGENT_IDENTITY_PRINCIPAL="principal://<effectiveIdentity>"
```

Never manufacture this value from agent name or project name.

## 4. Prove basic Agent Runtime execution

Dry-run:

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/smoke_agent_runtime.py \
  --engine-id "$SENTINEL_RUNTIME_ENGINE_ID"
```

One live synthetic call:

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/smoke_agent_runtime.py \
  --engine-id "$SENTINEL_RUNTIME_ENGINE_ID" \
  --execute
```

A successful real `streamQuery` may upgrade `LIVE_AGENT_RUNTIME` to `PASS_LIVE`. `managed_multi_agent_observed=true` only if streamed events actually contain a specialist author; do not infer multi-agent proof merely from the deployed topology.

The smoke script intentionally records authors/counts and latency, not model text.

## 5. Make the Go agent endpoint cloud-reachable

Expose only the authenticated managed-agent endpoint:

`POST /api/v1/internal/agent-tools`

The Go process must be configured with:

```text
SENTINEL_MANAGED_AGENT_INGRESS=true
GOOGLE_CLOUD_PROJECT=<project>
SENTINEL_IAP_EXPECTED_AUDIENCE=<actual IAP audience>
SENTINEL_AGENT_RUNTIME_SUBJECT=<server-observed agents.* identity>
```

The managed route verifies a signed IAP assertion, maps the real workload identity to the fixed SentinelFlow roster, derives tenant/artifact state from the durable workflow row, and invokes the normal Tool Gateway + deterministic Policy Engine.

Do not expose a direct database, raw object store, release route or arbitrary backend endpoint to the agent.

## 6. Create Agent Registry endpoints + Agent-to-Anywhere Gateway in audit-only mode

The exact Go destination must be:

```bash
export SENTINEL_GO_AGENT_ENDPOINT=https://YOUR_AUTHENTICATED_GO_HOST/api/v1/internal/agent-tools
```

Review the plan:

```bash
bash deployment/gcp/setup_agent_gateway.sh
```

Then create/reuse the development resources in **IAP DRY_RUN/AUDIT-ONLY** mode:

```bash
bash deployment/gcp/setup_agent_gateway.sh --execute --enable-apis
```

The script registers the exact business destination and the Google API hostnames needed by Runtime/telemetry. Base-host registrations are intentionally treated as development proof, not a final least-privilege production policy; after dry-run logs show the actual request URIs, narrow the allowlist where practical.

Capture:

```text
SENTINEL_AGENT_GATEWAY_RESOURCE=projects/.../locations/.../agentGateways/...
```

## 7. Bind the existing Runtime to the Gateway

Dry-run:

```bash
bash deployment/gcp/bind_runtime_gateway.sh
```

Live:

```bash
bash deployment/gcp/bind_runtime_gateway.sh --execute
```

The script reads `spec.effectiveIdentity` first and refuses a non-Agent-Identity Runtime. It then patches only `spec.deploymentSpec.agentGatewayConfig` and reads the resource back to prove the requested binding.

## 8. Read-only Runtime -> Gateway -> Go Tool Gateway proof

Seed/use a synthetic durable SentinelFlow workflow and incident. The live proof must use only read-only business capabilities such as:

```text
incident.get
workflow.get
validation.findings.list_redacted
artifact.metadata.get
```

The expected authority path is:

```text
Agent Runtime
  -> system Agent Identity
  -> Agent-to-Anywhere Gateway
  -> IAP
  -> signed assertion at Go ingress
  -> fixed SentinelFlow roster
  -> workflow-derived tenant scope
  -> SentinelFlow Tool Gateway
  -> deterministic Policy Engine
  -> typed read-only result
```

The managed Go ingress deliberately refuses `remediation.candidate.create` in P17. Candidate derivation remains exercised through SentinelFlow's established Go-controlled workflow path until a separate cloud mutation proof is justified.

A successful network request alone is **not** the proof. Record the returned Tool Gateway `invocation_id`, `manifest_hash`, `policy_decision_hash`, `policy_bundle_hash`, `output_hash`, and `tenant_scope_source=GO_WORKFLOW_REPOSITORY`.

## 9. Negative proof

With Gateway still audit-only, prove application-side and Tool Gateway denial using synthetic requests. Then use Google Gateway audit logs to confirm what would be denied by IAP.

At minimum demonstrate:

- unregistered destination -> local/Gateway deny evidence;
- valid managed identity + tool absent from agent manifest -> SentinelFlow deny;
- model-supplied tenant mismatch -> Go 403;
- candidate/release/approval capability -> unavailable/denied;
- memory or Model Armor result -> cannot authorize business execution.

Do not probe third-party systems.

## 10. Switch IAP from audit-only to enforcement only after evidence is clean

Re-run the gateway setup with enforcement only after the expected allow/deny graph is established:

```bash
bash deployment/gcp/setup_agent_gateway.sh --execute --enforce
```

Then repeat one read-only positive call and one controlled negative call.

Only the actual managed Gateway/IAP result may upgrade `LIVE_AGENT_GATEWAY` / `LIVE_GATEWAY_DEFAULT_DENY` to `PASS_LIVE`.

## 11. Observability proof

Agent Runtime tracing/metrics are managed infrastructure; SentinelFlow also preserves Go-side audit/evidence.

Capture safe metadata only:

- Runtime resource ID;
- trace ID;
- agent/stage name;
- Tool Gateway invocation ID;
- latency/result category;
- policy/manifest/output hashes.

Do not enable raw prompt/response capture. `ai-tier/observability/telemetry.py` forces no-content capture and sanitizes control characters, NACHA-shaped records, account/routing patterns and credential-shaped values.

## 12. Memory Bank

Memory Bank remains a separate capability. If P10.6 live proof has not run, keep it `NOT_RUN` even when Agent Runtime is live. Never treat Runtime deployment as Memory Bank proof.

## 13. Demo proof statuses

After real evidence exists, supply only the states actually proven to the UI build, for example:

```bash
export VITE_SENTINEL_AGENT_RUNTIME_PROOF=PASS_LIVE
export VITE_SENTINEL_AGENT_IDENTITY_PROOF=PASS_LIVE
export VITE_SENTINEL_AGENT_GATEWAY_PROOF=PASS_LIVE
export VITE_SENTINEL_AGENT_REGISTRY_PROOF=PASS_LIVE
export VITE_SENTINEL_OBSERVABILITY_PROOF=PASS_LIVE
```

Leave Memory Bank, Gateway Model Armor, or any other managed service at `NOT_RUN`/`IMPLEMENTED` unless its own live test succeeded.

## Stop condition

P17 is complete when:

1. local freeze is green;
2. real Agent Runtime create/read/query is proven;
3. server-returned Agent Identity is proven;
4. Runtime is registered/bound to Agent Gateway;
5. one read-only Runtime -> Gateway -> Go Tool Gateway call succeeds with deterministic policy evidence;
6. one controlled denial succeeds;
7. managed traces/metrics are visible without raw financial payload logging;
8. capability matrix distinguishes every `PASS_LIVE`, `TESTED`, `IMPLEMENTED`, and `NOT_RUN` claim.

Do not add new domain features during P17.
