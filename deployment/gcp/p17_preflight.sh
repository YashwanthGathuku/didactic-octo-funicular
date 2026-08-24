#!/usr/bin/env bash
set -euo pipefail

# SentinelFlow P17 zero-mutation Google Cloud preflight.
# This script performs read-only configuration/auth/API checks. It never prints
# OAuth tokens and never creates/enables/deletes resources.

PROJECT_ID="${GOOGLE_CLOUD_PROJECT:-telos-agent}"
REGION="${GOOGLE_CLOUD_LOCATION:-us-central1}"

required_apis=(
  aiplatform.googleapis.com
  agentregistry.googleapis.com
  networkservices.googleapis.com
  networksecurity.googleapis.com
  serviceextensions.googleapis.com
  iap.googleapis.com
  cloudtrace.googleapis.com
  logging.googleapis.com
)

status=0
pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1" >&2; status=1; }
info() { printf 'INFO  %s\n' "$1"; }

printf '== SentinelFlow P17 Google Cloud preflight ==\n'
printf 'project=%s region=%s\n\n' "$PROJECT_ID" "$REGION"

if ! command -v gcloud >/dev/null 2>&1; then
  fail "gcloud CLI is not installed"
  exit 1
fi

active_account="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null | head -n1 || true)"
if [[ -n "$active_account" ]]; then
  pass "gcloud authenticated (${active_account})"
else
  fail "gcloud has no active account"
fi

active_project="$(gcloud config get-value project 2>/dev/null || true)"
if [[ "$active_project" == "$PROJECT_ID" ]]; then
  pass "active gcloud project matches ${PROJECT_ID}"
else
  fail "active gcloud project '${active_project}' does not match '${PROJECT_ID}'"
fi

# ADC token is redirected and never displayed.
if gcloud auth application-default print-access-token >/dev/null 2>&1; then
  pass "Application Default Credentials are valid"
else
  fail "Application Default Credentials unavailable; run: gcloud auth application-default login"
fi

quota_project="$(gcloud auth application-default print-quota-project 2>/dev/null || true)"
if [[ -n "$quota_project" ]]; then
  info "ADC quota project=${quota_project}"
else
  info "ADC quota project not reported"
fi

billing_enabled="$(gcloud beta billing projects describe "$PROJECT_ID" --format='value(billingEnabled)' 2>/dev/null || true)"
case "${billing_enabled,,}" in
  true) pass "Cloud Billing is enabled for ${PROJECT_ID}" ;;
  false) fail "Cloud Billing is disabled for ${PROJECT_ID}" ;;
  *) info "Billing status could not be read with current permissions" ;;
esac

enabled_services="$(gcloud services list --enabled --project="$PROJECT_ID" --format='value(config.name)' 2>/dev/null || true)"
for api in "${required_apis[@]}"; do
  if grep -Fxq "$api" <<<"$enabled_services"; then
    pass "API enabled: $api"
  else
    fail "API not enabled: $api"
  fi
done

if [[ -n "${SENTINEL_AGENT_RUNTIME_STAGING_BUCKET:-}" ]]; then
  if [[ "$SENTINEL_AGENT_RUNTIME_STAGING_BUCKET" == gs://* ]]; then
    pass "Agent Runtime staging bucket configured"
  else
    fail "SENTINEL_AGENT_RUNTIME_STAGING_BUCKET must start with gs://"
  fi
else
  info "SENTINEL_AGENT_RUNTIME_STAGING_BUCKET not set yet"
fi

if [[ -n "${SENTINEL_GO_AGENT_ENDPOINT:-}" ]]; then
  if [[ "$SENTINEL_GO_AGENT_ENDPOINT" == https://*/api/v1/internal/agent-tools ]]; then
    pass "Go managed agent endpoint has canonical HTTPS path"
  else
    fail "SENTINEL_GO_AGENT_ENDPOINT must be https://.../api/v1/internal/agent-tools"
  fi
else
  info "SENTINEL_GO_AGENT_ENDPOINT not set yet"
fi

printf '\n'
if [[ "$status" -eq 0 ]]; then
  echo "P17 PREFLIGHT PASS: read-only cloud prerequisites are satisfied."
else
  echo "P17 PREFLIGHT FAIL: resolve the failures above before creating managed resources." >&2
fi
exit "$status"
