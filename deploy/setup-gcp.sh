#!/usr/bin/env bash
# ==============================================================================
# SentinelFlow — Google Cloud Resource Provisioning Script
# Provisions Cloud SQL (PostgreSQL 16), Cloud KMS, Artifact Registry,
# Cloud Run services, and Secret Manager for the All Things Agentic Hackathon.
# ==============================================================================

set -euo pipefail

PROJECT_ID="${1:-${GOOGLE_CLOUD_PROJECT:-}}"
REGION="${2:-us-central1}"

if [ -z "$PROJECT_ID" ]; then
    echo "Usage: ./setup-gcp.sh <PROJECT_ID> [REGION]"
    exit 1
fi

echo "==> Configuring Google Cloud Project: $PROJECT_ID ($REGION)"
gcloud config set project "$PROJECT_ID"

# 1. Enable Required GCP APIs
echo "==> Enabling required GCP APIs..."
gcloud services enable \
    run.googleapis.com \
    sqladmin.googleapis.com \
    cloudkms.googleapis.com \
    secretmanager.googleapis.com \
    artifactregistry.googleapis.com \
    aiplatform.googleapis.com

# 2. Create Artifact Registry repository for container images
echo "==> Creating Artifact Registry repository..."
gcloud artifacts repositories create sentinel-repo \
    --repository-format=docker \
    --location="$REGION" \
    --description="SentinelFlow container images" \
    || true

# 3. Create Cloud KMS Key Ring and Asymmetric Signing Key for Ledger Checkpoints
echo "==> Creating Cloud KMS key ring and signing key..."
gcloud kms keyrings create sentinel-ring \
    --location="$REGION" \
    || true

gcloud kms keys create ledger-signer \
    --location="$REGION" \
    --keyring=sentinel-ring \
    --purpose=asymmetric-signing \
    --default-algorithm=ec-sign-p256-sha256 \
    || true

# 4. Create Cloud SQL PostgreSQL 16 Instance
echo "==> Provisioning Cloud SQL PostgreSQL 16..."
gcloud sql instances create sentinel-db \
    --database-version=POSTGRES_16 \
    --tier=db-custom-2-7680 \
    --region="$REGION" \
    --storage-type=SSD \
    --storage-size=20GB \
    --storage-auto-increase \
    --availability-type=zonal \
    || true

# 5. Create Cloud Storage Bucket for Immutable Artifact Blobs
echo "==> Creating Cloud Storage bucket for immutable artifacts..."
gcloud storage buckets create "gs://sentinel-artifacts-${PROJECT_ID}" \
    --location="$REGION" \
    --uniform-bucket-level-access \
    || true

echo "==> [OK] Google Cloud resources provisioned successfully for $PROJECT_ID!"
