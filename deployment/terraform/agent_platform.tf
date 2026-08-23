# SentinelFlow Phase P11: Google Agent Platform Infrastructure Definition
# Manages Agent Runtime, Agent Identity, Agent Gateway (Default Deny), and Observability.

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 6.0"
    }
  }
}

variable "project_id" {
  type        = string
  default     = "telos-agent"
  description = "GCP Project ID"
}

variable "region" {
  type        = string
  default     = "us-central1"
  description = "GCP Region for Agent Platform"
}

# ------------------------------------------------------------------------------
# 1. Agent Identity Service Accounts
# ------------------------------------------------------------------------------

resource "google_service_account" "agent_runtime_sa" {
  account_id   = "sentinelflow-runtime-sa"
  display_name = "SentinelFlow Agent Runtime Workload Identity"
  project      = var.project_id
}

# Least-Privilege IAM: Cloud Trace Agent + Vertex AI Invoker (No Cloud Admin or Storage Admin)
resource "google_project_iam_member" "runtime_trace_agent" {
  project = var.project_id
  role    = "roles/cloudtrace.agent"
  member  = "serviceAccount:${google_service_account.agent_runtime_sa.email}"
}

resource "google_project_iam_member" "runtime_logging_writer" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.agent_runtime_sa.email}"
}

# ------------------------------------------------------------------------------
# 2. Agent Gateway (Default-Deny Egress Governance)
# ------------------------------------------------------------------------------

resource "google_network_services_agent_gateway" "sentinelflow_gateway" {
  provider             = google-beta
  name                 = "sentinelflow-agent-gateway-dev"
  location             = var.region
  project              = var.project_id
  description          = "SentinelFlow Default-Deny Agent Gateway governing outbound AI tier communications"
  governed_access_path = "AGENT_TO_ANYWHERE"

  labels = {
    application = "sentinelflow"
    environment = "p11-dev"
    hackathon   = "all-things-agentic"
    managed-by  = "sentinelflow-development"
  }
}

# ------------------------------------------------------------------------------
# 3. IAP Egress Authorization for Registered Go Endpoint
# ------------------------------------------------------------------------------

resource "google_project_iam_member" "agent_iap_egress" {
  project = var.project_id
  role    = "roles/iap.egressor"
  member  = "serviceAccount:${google_service_account.agent_runtime_sa.email}"
}
