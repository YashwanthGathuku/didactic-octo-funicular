#!/usr/bin/env bash
set -euo pipefail

# Bind an EXISTING Agent Runtime reasoning engine to an EXISTING
# Agent-to-Anywhere Gateway. Dry-run by default. The Runtime must already use
# identity_type=AGENT_IDENTITY; this script refuses service-account-backed
# engines rather than manufacturing a principal locally.

EXECUTE=0
for arg in "$@"; do
  case "$arg" in
    --execute) EXECUTE=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: bind_runtime_gateway.sh [--execute]

Required environment for --execute:
  GOOGLE_CLOUD_PROJECT
  GOOGLE_CLOUD_LOCATION
  SENTINEL_RUNTIME_ENGINE_ID
  SENTINEL_AGENT_GATEWAY_RESOURCE

Without --execute, prints the exact binding plan and performs no remote calls.
EOF
      exit 0
      ;;
    *) echo "Unknown argument: $arg" >&2; exit 2 ;;
  esac
done

PROJECT_ID="${GOOGLE_CLOUD_PROJECT:-telos-agent}"
REGION="${GOOGLE_CLOUD_LOCATION:-us-central1}"
ENGINE_ID="${SENTINEL_RUNTIME_ENGINE_ID:-}"
GATEWAY_RESOURCE="${SENTINEL_AGENT_GATEWAY_RESOURCE:-}"

EXPECTED_GATEWAY_PREFIX="projects/${PROJECT_ID}/locations/${REGION}/agentGateways/"
if [[ -n "$GATEWAY_RESOURCE" && "$GATEWAY_RESOURCE" != ${EXPECTED_GATEWAY_PREFIX}* ]]; then
  echo "ERROR: gateway must be in the same project/region for this hackathon profile" >&2
  exit 1
fi

cat <<EOF
SentinelFlow Runtime -> Agent Gateway binding plan
  project:  ${PROJECT_ID}
  region:   ${REGION}
  engine:   ${ENGINE_ID:-<set SENTINEL_RUNTIME_ENGINE_ID>}
  gateway:  ${GATEWAY_RESOURCE:-<set SENTINEL_AGENT_GATEWAY_RESOURCE>}
  execute:  $([[ "$EXECUTE" -eq 1 ]] && echo YES || echo NO)
EOF

if [[ "$EXECUTE" -ne 1 ]]; then
  echo "DRY_RUN: no Runtime resource was changed."
  exit 0
fi

[[ -n "$ENGINE_ID" ]] || { echo "ERROR: SENTINEL_RUNTIME_ENGINE_ID required" >&2; exit 1; }
[[ -n "$GATEWAY_RESOURCE" ]] || { echo "ERROR: SENTINEL_AGENT_GATEWAY_RESOURCE required" >&2; exit 1; }
command -v gcloud >/dev/null 2>&1 || { echo "ERROR: gcloud not installed" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "ERROR: curl not installed" >&2; exit 1; }
command -v python >/dev/null 2>&1 || { echo "ERROR: python not installed" >&2; exit 1; }

ACTIVE_PROJECT="$(gcloud config get-value project 2>/dev/null || true)"
[[ "$ACTIVE_PROJECT" == "$PROJECT_ID" ]] || {
  echo "ERROR: active project '$ACTIVE_PROJECT' != '$PROJECT_ID'" >&2
  exit 1
}

# Token value is held only in-process and is never printed.
ACCESS_TOKEN="$(gcloud auth print-access-token)"
[[ -n "$ACCESS_TOKEN" ]] || { echo "ERROR: could not obtain gcloud access token" >&2; exit 1; }

RESOURCE_URL="https://${REGION}-aiplatform.googleapis.com/v1/projects/${PROJECT_ID}/locations/${REGION}/reasoningEngines/${ENGINE_ID}"
PATCH_URL="${RESOURCE_URL}?updateMask=spec.deploymentSpec.agentGatewayConfig"
PAYLOAD="$(cat <<EOF
{
  "spec": {
    "deploymentSpec": {
      "agentGatewayConfig": {
        "agentToAnywhereConfig": {
          "agentGateway": "${GATEWAY_RESOURCE}"
        }
      }
    }
  }
}
EOF
)"

# Google documents an Agent Identity-backed Runtime effectiveIdentity in the
# form agents.global.org-...system.id.goog/resources/aiplatform/projects/...
# while a non-Agent-Identity Runtime reports a service agent/account instead.
BEFORE="$(curl --fail --silent --show-error \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  "$RESOURCE_URL")"
EFFECTIVE_IDENTITY="$(printf '%s' "$BEFORE" | python -c 'import json,sys; d=json.load(sys.stdin); print(d.get("spec",{}).get("effectiveIdentity", ""))')"
if [[ ! "$EFFECTIVE_IDENTITY" =~ ^agents\.[A-Za-z0-9._-]+\.system\.id\.goog/resources/aiplatform/projects/[^/]+/locations/[^/]+/reasoningEngines/[^/]+$ ]]; then
  echo "ERROR: Runtime effectiveIdentity is not a system-attested Agent Identity." >&2
  echo "Redeploy the reasoning engine with identity_type=AGENT_IDENTITY." >&2
  exit 1
fi

echo "Verified system-attested Runtime Agent Identity is present (principal intentionally not echoed)."

curl --fail --silent --show-error -X PATCH \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d "$PAYLOAD" \
  "$PATCH_URL" >/dev/null

AFTER="$(curl --fail --silent --show-error \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  "$RESOURCE_URL")"
BOUND_GATEWAY="$(printf '%s' "$AFTER" | python -c 'import json,sys; d=json.load(sys.stdin); print(d.get("spec",{}).get("deploymentSpec",{}).get("agentGatewayConfig",{}).get("agentToAnywhereConfig",{}).get("agentGateway", ""))')"

if [[ "$BOUND_GATEWAY" != "$GATEWAY_RESOURCE" ]]; then
  echo "ERROR: Runtime GET did not return the expected gateway binding" >&2
  exit 1
fi

echo "PASS_LIVE: Agent Runtime is bound to the requested Agent-to-Anywhere Gateway."
