# SentinelFlow P11.5: Google Agent Platform network-governance infrastructure.
#
# IMPORTANT TRUTH BOUNDARY
# ------------------------
# Agent Runtime itself is created by ai-tier/runtime/deploy_agent_runtime.py
# because the Runtime deployment must first produce Google's system-attested
# Agent Identity principal.  This Terraform file then grants that *observed*
# principal the narrow Agent Gateway IAP egress role.  It never invents a
# service account and calls it Agent Identity.

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
  description = "Google Cloud project containing the P11 development resources."
}

variable "region" {
  type        = string
  default     = "us-central1"
  description = "Regional location shared by Agent Runtime, Registry and Gateway."
}

variable "agent_identity_principal" {
  type        = string
  default     = ""
  description = "Output-only principal://agents... identity observed from the real Agent Runtime deployment. Leave empty until Runtime exists."

  validation {
    condition = (
      var.agent_identity_principal == "" ||
      startswith(var.agent_identity_principal, "principal://agents.")
    )
    error_message = "agent_identity_principal must be empty or a Google system-attested principal://agents... identity."
  }
}

variable "enable_gateway" {
  type        = bool
  default     = false
  description = "Explicit cost/safety gate. Set true only for the live P11/P17 proof run."
}

# -----------------------------------------------------------------------------
# Agent Gateway
# -----------------------------------------------------------------------------
# A single regional development gateway.  The application still maintains its
# own exact-endpoint deny policy, and the Go Tool Gateway remains the business
# capability authority after network access is granted.
resource "google_network_services_agent_gateway" "sentinelflow_gateway" {
  count                = var.enable_gateway ? 1 : 0
  provider             = google-beta
  name                 = "sentinelflow-agent-gateway-dev"
  location             = var.region
  project              = var.project_id
  description          = "SentinelFlow P11 development Agent Gateway"
  governed_access_path = "AGENT_TO_ANYWHERE"

  labels = {
    application = "sentinelflow"
    environment = "p11-dev"
    hackathon   = "all-things-agentic"
    managed-by  = "sentinelflow-development"
  }
}

# -----------------------------------------------------------------------------
# Narrow egress authorization for the REAL Agent Identity
# -----------------------------------------------------------------------------
# This resource is intentionally absent until the runtime's output-only
# principal has been observed and supplied.  A generic service account is not a
# substitute for Agent Identity.
resource "google_project_iam_member" "agent_iap_egress" {
  count   = var.enable_gateway && var.agent_identity_principal != "" ? 1 : 0
  project = var.project_id
  role    = "roles/iap.egressor"
  member  = var.agent_identity_principal
}

output "agent_gateway_name" {
  description = "Created gateway resource name, or null when the explicit live gate is disabled."
  value = var.enable_gateway ? google_network_services_agent_gateway.sentinelflow_gateway[0].name : null
}

output "agent_identity_iap_binding_enabled" {
  value = var.enable_gateway && var.agent_identity_principal != ""
}
