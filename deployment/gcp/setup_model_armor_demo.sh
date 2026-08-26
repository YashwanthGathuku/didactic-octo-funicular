#!/usr/bin/env bash
set -euo pipefail

# Creates one minimal P17 Model Armor template only when --execute is passed.
# Prompt/response operation logging is intentionally NOT enabled.

EXECUTE=0
DELETE=0
for arg in "$@"; do
  case "$arg" in
    --execute) EXECUTE=1 ;;
    --delete) DELETE=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: setup_model_armor_demo.sh [--execute] [--delete]

Environment:
  GOOGLE_CLOUD_PROJECT                 required for live execution
  GOOGLE_CLOUD_LOCATION                default us-central1
  SENTINEL_MODEL_ARMOR_TEMPLATE        default sentinelflow-p17-demo

Dry-run is default. --delete requires --execute.
EOF
      exit 0
      ;;
    *) echo "Unknown arg: $arg" >&2; exit 2 ;;
  esac
done

PROJECT="${GOOGLE_CLOUD_PROJECT:-}"
LOCATION="${GOOGLE_CLOUD_LOCATION:-us-central1}"
TEMPLATE="${SENTINEL_MODEL_ARMOR_TEMPLATE:-sentinelflow-p17-demo}"

cat <<EOF
Model Armor P17 plan
  project:   ${PROJECT:-<set GOOGLE_CLOUD_PROJECT>}
  location:  ${LOCATION}
  template:  ${TEMPLATE}
  operation: $([[ "$DELETE" -eq 1 ]] && echo DELETE || echo CREATE_OR_REUSE)
  execute:   $([[ "$EXECUTE" -eq 1 ]] && echo YES || echo NO)
  filters:   prompt-injection/jailbreak = enabled (HIGH)
  logging:   sanitize payload logging NOT enabled
EOF

if [[ "$EXECUTE" -ne 1 ]]; then
  echo "DRY_RUN: no Model Armor resource changed."
  exit 0
fi
[[ -n "$PROJECT" ]] || { echo "ERROR: GOOGLE_CLOUD_PROJECT required" >&2; exit 1; }
command -v gcloud >/dev/null || { echo "ERROR: gcloud not installed" >&2; exit 1; }

gcloud services enable modelarmor.googleapis.com --project="$PROJECT"
gcloud config set api_endpoint_overrides/modelarmor "https://modelarmor.${LOCATION}.rep.googleapis.com/" >/dev/null

if [[ "$DELETE" -eq 1 ]]; then
  gcloud model-armor templates delete "$TEMPLATE" --project="$PROJECT" --location="$LOCATION" --quiet
  echo "DELETED: ${TEMPLATE}"
  exit 0
fi

if gcloud model-armor templates describe "$TEMPLATE" --project="$PROJECT" --location="$LOCATION" >/dev/null 2>&1; then
  echo "REUSED: ${TEMPLATE}"
else
  gcloud model-armor templates create "$TEMPLATE" \
    --project="$PROJECT" \
    --location="$LOCATION" \
    --pi-and-jailbreak-filter-settings-enforcement=enabled \
    --pi-and-jailbreak-filter-settings-confidence-level=HIGH
  echo "CREATED: ${TEMPLATE}"
fi
