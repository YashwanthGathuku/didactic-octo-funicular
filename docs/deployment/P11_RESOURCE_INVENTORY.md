# SentinelFlow Phase P11 — Cloud Resource Inventory

This document records the declarative inventory of Google Agent Platform resources configured for SentinelFlow Phase P11 development and proof.

---

## 1. Resource Inventory Table

| Resource Type | Resource Name / ID | Project / Location | Role in SentinelFlow | Cost Guardrail |
|---|---|---|---|---|
| **Agent Runtime Deployment** | `sentinelflow-ai-tier` | `telos-agent` / `us-central1` | Hosting container for 6 fixed ADK agents | Min instances = 0, scale-to-zero |
| **Agent Identity Service Account** | `sentinelflow-runtime-sa@telos-agent.iam.gserviceaccount.com` | `telos-agent` / global | SPIFFE principal attestation | Least-privilege (`cloudtrace.agent`, `logging.logWriter`) |
| **Agent Registry v1** | `projects/telos-agent/locations/us-central1/agentRegistries/sentinelflow-registry` | `telos-agent` / `us-central1` | Declarative inventory of fleet & approved Go endpoint | Single regional instance |
| **Agent Gateway** | `sentinelflow-agent-gateway-dev` | `telos-agent` / `us-central1` | Egress network proxy with default-deny routing | Single regional instance |
| **IAP Egress Binding** | `roles/iap.egressor` on `/internal/agent-tools` | `telos-agent` / `us-central1` | Ingress token authorization to Go Tool Gateway | Narrow destination scope |
| **Cloud Trace** | OpenTelemetry Trace Exporter | `telos-agent` / `us-central1` | Distributed telemetry with privacy sanitization | `NO_CONTENT` in spans |

---

## 2. Resource Tags & Labels

All P11 cloud resources carry uniform metadata labels:
- `application`: `sentinelflow`
- `environment`: `p11-dev`
- `hackathon`: `all-things-agentic`
- `managed-by`: `sentinelflow-development`

No user PII, account numbers, or secrets are ever embedded in resource labels.
