#!/usr/bin/env bash
# ==============================================================================
# SentinelFlow — Cloud Run Production Deployment Script
# Builds and deploys Gateway, AI Tier, and Operations UI to Google Cloud Run.
# ==============================================================================

set -eu
set -o pipefail 2>/dev/null || true

PROJECT_ID="${1:-${GOOGLE_CLOUD_PROJECT:-}}"
REGION="${2:-${GOOGLE_CLOUD_LOCATION:-us-central1}}"

if [ -z "$PROJECT_ID" ]; then
    echo "Usage: ./deploy/deploy-cloudrun.sh <PROJECT_ID> [REGION]"
    echo "  Example: ./deploy/deploy-cloudrun.sh my-gcp-project us-central1"
    exit 1
fi

echo "============================================================"
echo " SentinelFlow Google Cloud Run Deployment"
echo " Project: $PROJECT_ID"
echo " Region:  $REGION"
echo "============================================================"

gcloud config set project "$PROJECT_ID"

# 1. Ensure Artifact Registry repository exists
echo "[1/5] Ensuring Artifact Registry repository exists..."
gcloud artifacts repositories create sentinel-repo \
    --repository-format=docker \
    --location="$REGION" \
    --description="SentinelFlow container repository" \
    2>/dev/null || true

REPO="us-central1-docker.pkg.dev/${PROJECT_ID}/sentinel-repo"

# 2. Build Container Images via Cloud Build
echo "[2/5] Building container images with Cloud Build..."
echo "  ==> Building AI Tier image..."
gcloud builds submit --tag "${REPO}/ai-tier:latest" ./ai-tier

echo "  ==> Building Gateway image..."
gcloud builds submit --tag "${REPO}/gateway:latest" ./gateway

echo "  ==> Building UI image..."
gcloud builds submit --tag "${REPO}/ui:latest" .

# 3. Deploy AI Tier on Cloud Run
echo "[3/5] Deploying sentinel-ai-tier on Cloud Run..."
gcloud run deploy sentinel-ai-tier \
    --image "${REPO}/ai-tier:latest" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --port 8000 \
    --cpu 2 \
    --memory 1Gi \
    --set-env-vars "GOOGLE_CLOUD_PROJECT=${PROJECT_ID},SENTINEL_GEMINI_MODEL=gemini-3.5-flash"

AI_TIER_URL=$(gcloud run services describe sentinel-ai-tier --region "$REGION" --format="value(status.url)")
echo "  [OK] AI Tier URL: $AI_TIER_URL"

# 4. Deploy Gateway on Cloud Run
echo "[4/5] Deploying sentinel-gateway on Cloud Run..."
gcloud run deploy sentinel-gateway \
    --image "${REPO}/gateway:latest" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --port 8080 \
    --cpu 1 \
    --memory 512Mi \
    --set-env-vars "PORT=8080,PROFILE=local-demo,AI_TIER_URL=${AI_TIER_URL},GOOGLE_CLOUD_PROJECT=${PROJECT_ID}"

GATEWAY_URL=$(gcloud run services describe sentinel-gateway --region "$REGION" --format="value(status.url)")
echo "  [OK] Gateway URL: $GATEWAY_URL"

# 5. Deploy UI on Cloud Run
echo "[5/5] Deploying sentinel-ui on Cloud Run..."
gcloud run deploy sentinel-ui \
    --image "${REPO}/ui:latest" \
    --region "$REGION" \
    --platform managed \
    --allow-unauthenticated \
    --port 3000 \
    --cpu 1 \
    --memory 256Mi

UI_URL=$(gcloud run services describe sentinel-ui --region "$REGION" --format="value(status.url)")

echo "============================================================"
echo " SentinelFlow Successfully Deployed to Google Cloud Run!"
echo " Operations Cockpit UI:  $UI_URL"
echo " Gateway Control Plane:  $GATEWAY_URL"
echo " Gemini AI Specialist:   $AI_TIER_URL"
echo "============================================================"
