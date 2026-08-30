-- 019_agent_workflow_trigger_idempotency.sql -- PostgreSQL dialect
-- Agent Workflow Trigger Idempotency & Binding Provenance.

ALTER TABLE agent_workflows ADD COLUMN IF NOT EXISTS trigger_event_id TEXT;
ALTER TABLE agent_workflows ADD COLUMN IF NOT EXISTS policy_bundle_hash TEXT;
ALTER TABLE agent_workflows ADD COLUMN IF NOT EXISTS authorized_evidence_set_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_workflows_trigger_uniq
    ON agent_workflows(tenant_id, trigger_event_id, workflow_type)
    WHERE trigger_event_id IS NOT NULL AND trigger_event_id != '';

CREATE INDEX IF NOT EXISTS idx_agent_workflows_trigger_lookup
    ON agent_workflows(tenant_id, trigger_event_id);
