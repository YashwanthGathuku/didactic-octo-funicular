# SentinelFlow P17 Proof Pack

This pack closes cloud proof without changing financial authority. Every command is dry-run by default unless `--execute` is explicit.

## Proof order

1. Run the local proof-pack gate.
2. Prepare bounded public/synthetic demo data.
3. Run Google Cloud preflight and enable only required APIs.
4. Prove one direct Gemini 3.5 Flash invocation.
5. Deploy the fixed ADK fleet to Agent Runtime and capture the reasoningEngine resource.
6. Inspect the server-returned Agent Identity and run the managed stream smoke.
7. Prove Memory Bank on that exact reasoningEngine with a synthetic advisory fact, then delete the fact.
8. Create a minimal Model Armor template and prove a direct Google-managed prompt-injection match.
9. Configure Agent Registry/Gateway in audit-only mode, inspect logs, then enforce only after expected traffic is understood.
10. Do not claim Runtime -> Gateway -> Go financial-tool authentication until the downstream Agent Gateway/DPoP contract is observed and verified. Never replace it with unsigned application headers.

## Local gate

```bash
bash scripts/verify_p17_proof_pack.sh
```

## IBM AML ACH subset

```bash
python scripts/prepare_ibm_aml_demo.py /path/to/HI-Small_Trans.csv \
  --output-dir demo/public/ibm-aml \
  --max-rows 100000
python scripts/verify_public_demo_data.py demo/public/ibm-aml
```

The Lens CSV intentionally excludes `Is Laundering`; labels are held in a separate ground-truth file for after-the-fact evaluation only.

## Moov ACH fixture staging

```bash
python scripts/prepare_moov_ach_demo.py /path/to/moov-ach/testdata \
  --output-dir demo/public/moov-ach \
  --source-commit YOUR_MOOV_COMMIT
```

Every non-empty staged record must be exactly 94 printable ASCII characters.

## Live Gemini proof

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/live_gemini_proof.py \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --location "$GOOGLE_CLOUD_LOCATION" \
  --execute | tee p17-gemini-proof.json
```

Expected terminal state: `PASS_LIVE`. The script records only latency, usage metadata, response length and a SHA-256 digest, not model content.

## Agent Runtime + Agent Identity

Follow `docs/deployment/P17_LIVE_PROOF_RUNBOOK.md` to create the staging bucket and deploy. Then set:

```bash
export SENTINEL_AGENT_RUNTIME_RESOURCE='projects/.../locations/.../reasoningEngines/...'
export SENTINEL_AGENT_RUNTIME_ENGINE_ID='...'
```

Inspect:

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/inspect_agent_runtime.py \
  --engine-id "$SENTINEL_AGENT_RUNTIME_ENGINE_ID" \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --location "$GOOGLE_CLOUD_LOCATION"
```

Smoke:

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/smoke_agent_runtime.py \
  --engine-id "$SENTINEL_AGENT_RUNTIME_ENGINE_ID" \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --location "$GOOGLE_CLOUD_LOCATION" \
  --execute | tee p17-runtime-proof.json
```

## Memory Bank proof

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/live_memory_bank_proof.py \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --location "$GOOGLE_CLOUD_LOCATION" \
  --engine-name "$SENTINEL_AGENT_RUNTIME_RESOURCE" \
  --execute | tee p17-memory-proof.json
```

By default the synthetic memory is deleted after successful retrieval.

## Model Armor proof

Create/reuse a minimal template:

```bash
bash deployment/gcp/setup_model_armor_demo.sh --execute
```

Then call Google Model Armor directly:

```bash
PYTHONPATH=ai-tier python ai-tier/runtime/live_model_armor_proof.py \
  --project "$GOOGLE_CLOUD_PROJECT" \
  --location "$GOOGLE_CLOUD_LOCATION" \
  --template-id "${SENTINEL_MODEL_ARMOR_TEMPLATE:-sentinelflow-p17-demo}" \
  --execute | tee p17-model-armor-proof.json
```

Expected terminal state: `PASS_LIVE` with `filter_match_state=MATCH_FOUND`. This direct REST proof deliberately bypasses SentinelFlow's local heuristic guard so it cannot be confused with a local-only detection.

## Agent Gateway caution

Agent Gateway/IAP authenticates and authorizes Agent Identity at the gateway and current platform documentation describes mTLS plus DPoP for the end-to-end path. Do not assume that a classic web-IAP `X-Goog-IAP-JWT-Assertion` header is delivered to an arbitrary downstream endpoint. Keep gateway authorization in audit-only mode first and treat the existing Go signed-IAP ingress adapter as `IMPLEMENTED` until its exact downstream attestation contract is proven live.

## Cleanup

Keep only resources needed for the final recorded demo. Delete temporary Model Armor templates after recording if no longer required:

```bash
bash deployment/gcp/setup_model_armor_demo.sh --execute --delete
```

Agent Runtime can be deleted using the Agent Platform SDK (`remote_agent.delete(force=True)`) after submission/demo evidence is captured.
