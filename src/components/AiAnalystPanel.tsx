import React, { useState } from 'react';
import { AgentRun, Incident, Approval } from '../types/financial';
import { Bot, Shield, CheckCircle2, AlertOctagon, Send, Lock, Copy, Check } from 'lucide-react';

interface AiAnalystPanelProps {
  agentRun: AgentRun;
  incident: Incident;
  onApproveAction: (actionType: Approval['actionType'], reason: string) => void;
  onClose: () => void;
}

export const AiAnalystPanel: React.FC<AiAnalystPanelProps> = ({
  agentRun,
  incident,
  onApproveAction,
  onClose
}) => {
  const [approvalReason, setApprovalReason] = useState<string>('');
  const [copiedDraft, setCopiedDraft] = useState<boolean>(false);
  const [submittedAction, setSubmittedAction] = useState<boolean>(false);

  const handleCopyDraft = () => {
    if (agentRun.draftExternalPartnerNotice) {
      navigator.clipboard.writeText(agentRun.draftExternalPartnerNotice);
      setCopiedDraft(true);
      setTimeout(() => setCopiedDraft(false), 2000);
    }
  };

  const handleExecuteApproval = (actionType: Approval['actionType']) => {
    onApproveAction(actionType, approvalReason || 'Operator confirmed pre-flight exception and authorized workflow transition.');
    setSubmittedAction(true);
  };

  return (
    <div className="glass-panel" style={{ padding: '24px', border: '1px solid rgba(99, 102, 241, 0.3)' }}>
      {/* Top Banner: Model & Agent ID */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingBottom: '16px',
        borderBottom: '1px solid var(--border-subtle)',
        marginBottom: '20px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <div style={{
            width: '36px',
            height: '36px',
            borderRadius: '8px',
            background: 'var(--accent-violet-dim)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            border: '1px solid rgba(99, 102, 241, 0.4)'
          }}>
            <Bot size={20} color="var(--accent-violet)" />
          </div>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                {agentRun.agentVersion}
              </h3>
              <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>
                Constrained RRR-Model
              </span>
            </div>
            <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
              Execution Target: <strong style={{ color: 'var(--text-secondary)' }}>{agentRun.modelIdentifier}</strong> | Run ID: {agentRun.id}
            </p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <span className="badge badge-neutral" style={{ fontSize: '0.7rem' }}>
            Latency: {agentRun.metrics.durationMs}ms
          </span>
          <span className="badge badge-neutral" style={{ fontSize: '0.7rem' }}>
            Tokens: {agentRun.metrics.inputTokens + agentRun.metrics.outputTokens}
          </span>
        </div>
      </div>

      {/* Safety Notice: Zero Autonomous Execution */}
      <div style={{
        background: 'rgba(99, 102, 241, 0.08)',
        border: '1px solid rgba(99, 102, 241, 0.25)',
        borderRadius: '6px',
        padding: '10px 14px',
        marginBottom: '20px',
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        fontSize: '0.8125rem',
        color: '#C7D2FE'
      }}>
        <Shield size={16} color="var(--accent-violet)" />
        <span>
          <strong>Constrained Agent Safety Guarantee:</strong> This AI agent holds read-only tools and cannot directly alter financial records or bypass dual-control release policies without signed human supervisor authorization.
        </span>
      </div>

      {/* Findings Summary */}
      <div style={{ marginBottom: '20px' }}>
        <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>
          1. Evidence Synthesis & Citations
        </h4>
        <div style={{
          background: 'var(--bg-primary)',
          border: '1px solid var(--border-subtle)',
          borderRadius: '6px',
          padding: '14px',
          fontSize: '0.875rem',
          color: 'var(--text-secondary)',
          lineHeight: 1.6
        }}>
          {agentRun.findingsSummary}

          <div style={{ marginTop: '12px', display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
            {agentRun.citedFindingCodes.map((code, idx) => (
              <span key={idx} className="badge badge-danger" style={{ fontSize: '0.7rem' }}>
                Cited Finding: {code}
              </span>
            ))}
            {agentRun.citedRunbookSections.map((rb, idx) => (
              <span key={idx} className="badge badge-cyan" style={{ fontSize: '0.7rem' }}>
                {rb}
              </span>
            ))}
          </div>
        </div>
      </div>

      {/* Hypotheses */}
      <div style={{ marginBottom: '20px' }}>
        <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>
          2. Root-Cause Hypotheses
        </h4>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          {agentRun.hypotheses.map((hypo, idx) => (
            <div 
              key={idx}
              style={{
                background: 'var(--bg-secondary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                padding: '12px 14px'
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                <span style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Hypothesis #{idx + 1}: {hypo.hypothesis}
                </span>
                <span className={`badge ${hypo.confidence === 'HIGH' ? 'badge-warning' : 'badge-neutral'}`}>
                  Confidence: {hypo.confidence}
                </span>
              </div>
              <ul style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', paddingLeft: '20px', marginTop: '4px' }}>
                {hypo.supportingEvidence.map((ev, i) => (
                  <li key={i}>{ev}</li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </div>

      {/* Action Plan & Human Approval Gate */}
      <div style={{ marginBottom: '20px' }}>
        <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>
          3. Proposed Remediation Plan & Authority Tiers
        </h4>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          {agentRun.proposedActionPlan.map((action, idx) => (
            <div 
              key={idx}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '10px 14px',
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                fontSize: '0.8125rem'
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                <span className="badge badge-neutral">Step {action.step}</span>
                <span style={{ color: 'var(--text-primary)' }}>{action.action}</span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span className="badge badge-cyan">Tier {action.authorityTier}</span>
                {action.requiresHumanApproval ? (
                  <span className="badge badge-danger">Requires Approval</span>
                ) : (
                  <span className="badge badge-success">Automated / Read-Only</span>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* External Partner Notification Draft */}
      {agentRun.draftExternalPartnerNotice && (
        <div style={{ marginBottom: '20px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
            <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              4. Drafted Counterparty SLA Notification
            </h4>
            <button 
              className="btn btn-secondary"
              onClick={handleCopyDraft}
              style={{ fontSize: '0.75rem', padding: '4px 10px' }}
            >
              {copiedDraft ? <Check size={12} color="var(--accent-emerald)" /> : <Copy size={12} />}
              <span>{copiedDraft ? 'Copied to Clipboard' : 'Copy Draft Notice'}</span>
            </button>
          </div>
          <div className="log-viewer" style={{ whiteSpace: 'pre-wrap', maxHeight: '180px' }}>
            {agentRun.draftExternalPartnerNotice}
          </div>
        </div>
      )}

      {/* Human Supervisor Approval Box */}
      <div style={{
        background: 'var(--bg-card)',
        border: '1px solid var(--border-medium)',
        borderRadius: '8px',
        padding: '16px',
        marginTop: '12px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
          <Lock size={16} color="var(--accent-amber)" />
          <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
            Human Supervisor Sign-Off & Dual-Control Gate
          </h4>
        </div>

        {submittedAction ? (
          <div style={{
            padding: '12px',
            background: 'var(--accent-emerald-dim)',
            border: '1px solid rgba(16, 185, 129, 0.4)',
            borderRadius: '6px',
            color: 'var(--accent-emerald)',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            fontSize: '0.875rem'
          }}>
            <CheckCircle2 size={18} />
            <span>Action authorized and committed to tamper-evident audit ledger.</span>
          </div>
        ) : (
          <div>
            <input 
              type="text"
              placeholder="Enter a justification for this decision (required, recorded in the audit ledger)..."
              value={approvalReason}
              onChange={(e) => setApprovalReason(e.target.value)}
              style={{
                width: '100%',
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                padding: '10px 14px',
                color: 'var(--text-primary)',
                fontSize: '0.8125rem',
                marginBottom: '12px',
                outline: 'none'
              }}
            />

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
              <button 
                className="btn btn-secondary"
                onClick={onClose}
                style={{ fontSize: '0.8125rem' }}
              >
                Close Panel
              </button>
              {incident.type === 'MISSING_FILE_DEADLINE' ? (
                <button 
                  className="btn btn-primary"
                  onClick={() => handleExecuteApproval('WAIVE_MISSING_FILE')}
                  style={{ fontSize: '0.8125rem' }}
                >
                  <Send size={14} />
                  <span>Authorize Temporary SLA Extension</span>
                </button>
              ) : (
                <button 
                  className="btn btn-danger"
                  onClick={() => handleExecuteApproval('EXCEPTIONAL_RELEASE')}
                  style={{ fontSize: '0.8125rem' }}
                >
                  <AlertOctagon size={14} />
                  <span>Authorize Exceptional Release</span>
                </button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
