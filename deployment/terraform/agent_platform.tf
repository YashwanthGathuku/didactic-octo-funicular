# SentinelFlow P11.5/P17 — Agent Platform configuration metadata only.
#
# Agent Gateway's current Google-documented setup is intentionally performed by
# deployment/gcp/setup_agent_gateway.sh using `gcloud network-services
# agent-gateways import` plus Agent Registry/IAP resources.  Keeping an
# unverified Terraform Agent Gateway schema here would create a misleading
# second source of truth during the hackathon freeze.
#
# This Terraform module therefore has NO billable resources. It only validates
# the stable deployment inputs that the live scripts consume.

terraform {
  required_version = ">= 1.5.0"
}

variable "project_id" {
  type        = string
  default     = "telos-agent"
  description = "Google Cloud project containing SentinelFlow development resources."

  validation {
    condition     = length(trimspace(var.project_id)) > 0
    error_message = "project_id must not be empty."
  }
}

variable "region" {
  type        = string
  default     = "us-central1"
  description = "Region shared by Agent Runtime, regional Agent Registry, and Agent Gateway."

  validation {
    condition     = length(trimspace(var.region)) > 0
    error_message = "region must not be empty."
  }
}

variable "agent_identity_principal" {
  type        = string
  default     = ""
  description = "System-attested principal://agents... value observed from a real Agent Runtime resource."

  validation {
    condition = (
      var.agent_identity_principal == "" ||
      startswith(var.agent_identity_principal, "principal://agents.")
    )
    error_message = "agent_identity_principal must be empty or a system-attested principal://agents... identity."
  }
}

locals {
  regional_registry = "//agentregistry.googleapis.com/projects/${var.project_id}/locations/${var.region}"
  gateway_name      = "sentinelflow-agent-gateway-dev"
}

output "project_id" {
  value = var.project_id
}

output "region" {
  value = var.region
}

output "regional_agent_registry" {
  value = local.regional_registry
}

output "planned_agent_gateway_name" {
  value = local.gateway_name
}

output "agent_identity_principal_supplied" {
  value = var.agent_identity_principal != ""
}

output "live_setup_command" {
  value = "bash deployment/gcp/setup_agent_gateway.sh"
}
