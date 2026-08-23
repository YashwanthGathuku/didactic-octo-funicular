#!/usr/bin/env bash
set -euo pipefail

# SentinelFlow P17 live Agent Gateway/Registry/IAP bootstrap.
#
# DRY-RUN IS THE DEFAULT. Nothing is created unless --execute is provided.
# The script follows Google's current documented Agent-to-Anywhere flow:
#   regional Agent Registry -> registered destinations -> Agent Gateway ->
#   IAP request authorization extension/policy -> roles/iap.egressor bindings.
#
# It deliberately does not create Agent Runtime. Create Runtime separately with
# ai-tier/runtime/deploy_agent_runtime.py so the system-attested Agent Identity
# principal exists before endpoint IAM bindings are applied.

EXECUTE=0
ENFORCE=0
ENABLE_APIS=0

for arg in "$@"; do
  case "$arg" in
    --execute) EXECUTE=1 ;;
    --enforce) ENFORCE=1 ;;
    --enable-apis) ENABLE_APIS=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: setup_agent_gateway.sh [--execute] [--enforce] [--enable-apis]

Required environment for live execution:
  GOOGLE_CLOUD_PROJECT
  GOOGLE_CLOUD_LOCATION               (default: us-central1)
  SENTINEL_GO_AGENT_ENDPOINT          full HTTPS URL to /internal/agent-tools
  SENTINEL_AGENT_IDENTITY_PRINCIPAL   system-attested principal://agents... identity

Optional:
  SENTINEL_AGENT_GATEWAY_NAME         default sentinelflow-agent-gateway-dev

Default behavior is DRY-RUN/AUDIT-ONLY. --enforce removes IAP's DRY_RUN metadata.
EOF
      exit 0
      ;;
    *) echo "Unknown argument: $arg" >&2; exit 2 ;;
  esac
done

PROJECT_ID="${GOOGLE_CLOUD_PROJECT:-telos-agent}"
REGION="${GOOGLE_CLOUD_LOCATION:-us-central1}"
GATEWAY_NAME="${SENTINEL_AGENT_GATEWAY_NAME:-sentinelflow-agent-gateway-dev}"
GO_ENDPOINT="${SENTINEL_GO_AGENT_ENDPOINT:-}"
AGENT_PRINCIPAL="${SENTINEL_AGENT_IDENTITY_PRINCIPAL:-}"
REGISTRY_PATH="//agentregistry.googleapis.com/projects/${PROJECT_ID}/locations/${REGION}"
IAP_EXT_NAME="sentinelflow-iap-egress"
IAP_POLICY_NAME="sentinelflow-iap-egress-policy"

if [[ -z "$PROJECT_ID" || -z "$REGION" ]]; then
  echo "ERROR: project and region are required" >&2
  exit 1
fi

if [[ "$EXECUTE" -eq 1 ]]; then
  if [[ -z "$GO_ENDPOINT" ]]; then
    echo "ERROR: SENTINEL_GO_AGENT_ENDPOINT is required for --execute" >&2
    exit 1
  fi
  if [[ "$GO_ENDPOINT" != https://* ]]; then
    echo "ERROR: SENTINEL_GO_AGENT_ENDPOINT must use HTTPS" >&2
    exit 1
  fi
  if [[ -z "$AGENT_PRINCIPAL" || "$AGENT_PRINCIPAL" != principal://agents.* ]]; then
    echo "ERROR: --execute requires the real system-attested principal://agents... value" >&2
    exit 1
  fi
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

GATEWAY_YAML="$TMP_DIR/agent-gateway.yaml"
IAP_EXT_YAML="$TMP_DIR/iap-extension.yaml"
IAP_POLICY_YAML="$TMP_DIR/iap-policy.yaml"

cat >"$GATEWAY_YAML" <<EOF
name: ${GATEWAY_NAME}
protocols:
  - MCP
googleManaged:
  governedAccessPath: AGENT_TO_ANYWHERE
registries:
  - ${REGISTRY_PATH}
EOF

if [[ "$ENFORCE" -eq 1 ]]; then
  cat >"$IAP_EXT_YAML" <<EOF
name: ${IAP_EXT_NAME}
service: iap.googleapis.com
failOpen: true
timeout: 1s
metadata:
  iapPolicyVersion: "V1"
EOF
else
  cat >"$IAP_EXT_YAML" <<EOF
name: ${IAP_EXT_NAME}
service: iap.googleapis.com
failOpen: true
timeout: 1s
metadata:
  iapPolicyVersion: "V1"
  iamEnforcementMode: "DRY_RUN"
EOF
fi

cat >"$IAP_POLICY_YAML" <<EOF
name: ${IAP_POLICY_NAME}
target:
  resources:
    - "projects/${PROJECT_ID}/locations/${REGION}/agentGateways/${GATEWAY_NAME}"
policyProfile: REQUEST_AUTHZ
action: CUSTOM
customProvider:
  authzExtension:
    resources:
      - "projects/${PROJECT_ID}/locations/${REGION}/authzExtensions/${IAP_EXT_NAME}"
EOF

print_plan() {
  cat <<EOF
SentinelFlow Agent Gateway plan
  project:          ${PROJECT_ID}
  region:           ${REGION}
  registry:         ${REGISTRY_PATH}
  gateway:          ${GATEWAY_NAME}
  Go endpoint:      ${GO_ENDPOINT:-<set SENTINEL_GO_AGENT_ENDPOINT>}
  Agent Identity:   ${AGENT_PRINCIPAL:+SET (not printed)}${AGENT_PRINCIPAL:-<set after Runtime creation>}
  IAP mode:         $([[ "$ENFORCE" -eq 1 ]] && echo ENFORCE || echo DRY_RUN)
  execute:          $([[ "$EXECUTE" -eq 1 ]] && echo YES || echo NO)

Resources/actions:
  1. Register the exact Go agent-tools endpoint.
  2. Register essential Agent Platform/telemetry endpoints needed by Runtime.
  3. Import one regional Agent-to-Anywhere gateway bound to the regional registry.
  4. Import one IAP REQUEST_AUTHZ extension/policy.
  5. Grant roles/iap.egressor to the real Agent Identity on each registered endpoint.
EOF
}

print_plan

if [[ "$EXECUTE" -ne 1 ]]; then
  echo
  echo "DRY_RUN: no Google Cloud resources were changed."
  exit 0
fi

command -v gcloud >/dev/null 2>&1 || { echo "ERROR: gcloud not installed" >&2; exit 1; }

ACTIVE_ACCOUNT="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' | head -n1)"
[[ -n "$ACTIVE_ACCOUNT" ]] || { echo "ERROR: gcloud has no active account" >&2; exit 1; }
ACTIVE_PROJECT="$(gcloud config get-value project 2>/dev/null || true)"
if [[ "$ACTIVE_PROJECT" != "$PROJECT_ID" ]]; then
  echo "ERROR: active gcloud project '$ACTIVE_PROJECT' != requested '$PROJECT_ID'" >&2
  exit 1
fi

echo "Authenticated gcloud account found; credentials/tokens will not be printed."

if [[ "$ENABLE_APIS" -eq 1 ]]; then
  echo "Enabling only P17-required APIs..."
  gcloud services enable \
    aiplatform.googleapis.com \
    agentregistry.googleapis.com \
    networkservices.googleapis.com \
    networksecurity.googleapis.com \
    serviceextensions.googleapis.com \
    iap.googleapis.com \
    telemetry.googleapis.com \
    cloudtrace.googleapis.com \
    logging.googleapis.com \
    --project="$PROJECT_ID"
fi

# Returns registryResource for an existing/new endpoint. Creation is performed
# only when describe cannot resolve the named development service.
ensure_endpoint() {
  local name="$1"
  local display="$2"
  local url="$3"
  local protocol="${4:-http-json}"

  local existing
  existing="$(gcloud agent-registry services describe "$name" \
    --project="$PROJECT_ID" --location="$REGION" \
    --format='value(registryResource)' 2>/dev/null || true)"
  if [[ -n "$existing" ]]; then
    echo "$existing"
    return 0
  fi

  gcloud agent-registry services create "$name" \
    --project="$PROJECT_ID" \
    --location="$REGION" \
    --display-name="$display" \
    --endpoint-spec-type=no-spec \
    --interfaces="url=${url},protocolBinding=${protocol}" \
    --format='value(registryResource)'
}

# Exact business endpoint plus minimal platform endpoints required by the
# managed runtime. Keep this list small; add a hostname only after dry-run logs
# prove the SDK genuinely uses it.
declare -A ENDPOINTS
ENDPOINTS[sentinelflow-go-tools]="$GO_ENDPOINT"
ENDPOINTS[sentinelflow-agent-registry]="https://agentregistry.googleapis.com/"
ENDPOINTS[sentinelflow-telemetry]="https://telemetry.googleapis.com/"
ENDPOINTS[sentinelflow-logging]="https://logging.googleapis.com/"
ENDPOINTS[sentinelflow-aiplatform-regional]="https://${REGION}-aiplatform.googleapis.com/"
ENDPOINTS[sentinelflow-aiplatform-mtls]="https://${REGION}-aiplatform.mtls.googleapis.com/"
ENDPOINTS[sentinelflow-aiplatform-rep]="https://aiplatform.${REGION}.rep.googleapis.com/"

ENDPOINT_IDS=()
for name in "${!ENDPOINTS[@]}"; do
  echo "Registering/confirming endpoint: $name"
  registry_resource="$(ensure_endpoint "$name" "$name" "${ENDPOINTS[$name]}" http-json)"
  if [[ -z "$registry_resource" ]]; then
    echo "ERROR: endpoint registration produced no registryResource for $name" >&2
    exit 1
  fi
  ENDPOINT_IDS+=("$registry_resource")
done

echo "Importing Agent Gateway..."
gcloud network-services agent-gateways import "$GATEWAY_NAME" \
  --source="$GATEWAY_YAML" \
  --location="$REGION" \
  --project="$PROJECT_ID"

echo "Importing IAP authorization extension..."
gcloud beta service-extensions authz-extensions import "$IAP_EXT_NAME" \
  --source="$IAP_EXT_YAML" \
  --location="$REGION" \
  --project="$PROJECT_ID"

echo "Importing Agent Gateway authorization policy..."
gcloud beta network-security authz-policies import "$IAP_POLICY_NAME" \
  --source="$IAP_POLICY_YAML" \
  --location="$REGION" \
  --project="$PROJECT_ID"

echo "Granting endpoint-scoped IAP egress permission to Agent Identity..."
for endpoint_id in "${ENDPOINT_IDS[@]}"; do
  gcloud iap web add-iam-policy-binding \
    --resource-type=agent-registry \
    --endpoint="$endpoint_id" \
    --region="$REGION" \
    --project="$PROJECT_ID" \
    --member="$AGENT_PRINCIPAL" \
    --role=roles/iap.egressor >/dev/null
  echo "  bound endpoint: $endpoint_id"
done

echo
printf 'AGENT_GATEWAY_RESOURCE=projects/%s/locations/%s/agentGateways/%s\n' "$PROJECT_ID" "$REGION" "$GATEWAY_NAME"
echo "IAP_MODE=$([[ "$ENFORCE" -eq 1 ]] && echo ENFORCE || echo DRY_RUN)"
echo "P17 Agent Gateway bootstrap completed. Run managed smoke tests before switching from DRY_RUN to enforcement."
