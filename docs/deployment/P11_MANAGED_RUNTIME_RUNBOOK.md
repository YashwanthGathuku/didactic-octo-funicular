# SentinelFlow P11 — Managed Agent Runtime Runbook

This operational runbook provides step-by-step instructions for deploying, verifying, and troubleshooting the SentinelFlow ADK fleet on Google Agent Runtime.

---

## 1. Pre-Flight Verification

1. **Verify Google Cloud Authentication & ADC**:
   ```bash
   gcloud auth list
   gcloud config get-value project  # Expected: telos-agent
   ```
2. **Verify Required APIs**:
   ```bash
   gcloud services list --enabled --filter="name:(aiplatform OR networkservices OR iap OR cloudtrace)"
   ```

---

## 2. ADK Fleet Packaging & Local Smoke Test

1. **Verify Local AI Fleet Tests**:
   ```bash
   pytest ai-tier/tests/ -v
   ```
2. **Run Full Adversarial Evaluation Suite (145 Scenarios)**:
   ```bash
   python ai-tier/evals/runner.py
   ```
   *Expected Output*: `145 / 145 passed (100.0%)`.

---

## 3. Deployment via `agents-cli`

> [!IMPORTANT]
> **Human Approval Required**: Never execute deployment without explicit confirmation and active cloud billing credits.

```bash
# 1. Enhance scaffolding if needed
agents-cli scaffold enhance ai-tier --deployment-target agent_runtime --agent-gateway

# 2. Deploy ADK container to Google Agent Runtime
agents-cli deploy \
  --project=telos-agent \
  --region=us-central1 \
  --service-name=sentinelflow-ai-tier \
  --agent-identity \
  --agent-gateway-egress="projects/telos-agent/locations/us-central1/agentGateways/sentinelflow-agent-gateway-dev" \
  --update-env-vars="LOGS_BUCKET_NAME=telos-agent-sentinelflow-logs,OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=NO_CONTENT,ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS=false" \
  --cpu=1 \
  --memory=4Gi \
  --concurrency=8
```

---

## 4. Post-Deployment Health & Connectivity Proof

1. **Health Endpoint**:
   ```bash
   curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
     https://us-central1-aiplatform.googleapis.com/reasoningEngines/v1/projects/telos-agent/locations/us-central1/reasoningEngines/sentinelflow-ai-tier/api/health
   ```
2. **Agent Registry Verification**:
   ```bash
   # Query registered SentinelFlow Agent in Registry
   gcloud beta network-services agent-registries describe sentinelflow-registry --location=us-central1
   ```
3. **Trace Explorer**:
   Navigate to **Cloud Console -> Trace -> Trace explorer** to observe metadata-only spans without raw financial payloads.
