-- Migration 019: Agent Workflow Trigger Idempotency & Binding Provenance
-- Extends agent_workflows with trigger_event_id and unique constraint for durable restart idempotency.

ALTER TABLE agent_workflows ADD COLUMN trigger_event_id TEXT;
ALTER TABLE agent_workflows ADD COLUMN policy_bundle_hash TEXT;
ALTER TABLE agent_workflows ADD COLUMN authorized_evidence_set_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_workflows_trigger_uniq 
    ON agent_workflows(tenant_id, trigger_event_id, workflow_type) 
    WHERE trigger_event_id IS NOT NULL AND trigger_event_id != '';

CREATE INDEX IF NOT EXISTS idx_agent_workflows_trigger_lookup 
    ON agent_workflows(tenant_id, trigger_event_id);
