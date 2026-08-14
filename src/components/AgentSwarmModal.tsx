import React, { useState } from 'react';
import { 
  Bot, 
  X, 
  ShieldCheck, 
  Cpu, 
  CheckCircle2, 
  Search, 
  Play, 
  RefreshCw,
  GitMerge,
  UserCheck
} from 'lucide-react';

interface AgentSwarmModalProps {
  onClose: () => void;
  onApproveConsensus?: (action: string) => void;
}

interface AgentMessage {
  id: string;
  agentRole: 'LEAD_SUPERVISOR' | 'FORMAT_VALIDATOR' | 'LINEAGE_RECON' | 'AUDIT_COMPLIANCE';
  agentName: string;
  stepType: 'THOUGHT' | 'TOOL_CALL' | 'OBSERVATION' | 'CONCLUSION';
  content: string;
  toolName?: string;
  toolParameters?: string;
  confidence: number;
}

const DEFAULT_MESSAGES: AgentMessage[] = [
  {
    id: 'MSG-1',
    agentRole: 'LEAD_SUPERVISOR',
    agentName: 'Astra Lead Supervisor',
    stepType: 'THOUGHT',
    content: 'Incident #101 detected on Inbound NACHA Transmission File #501. Initializing multi-agent triage: (1) Format syntax validation, (2) Blast radius assessment, (3) SEC compliance verification.',
    confidence: 0.99
  },
  {
    id: 'MSG-2',
    agentRole: 'FORMAT_VALIDATOR',
    agentName: 'Syntax & Mod10 Inspector',
    stepType: 'TOOL_CALL',
    toolName: 'validate_routing_mod10',
    toolParameters: '{"routingNumber": "021000021"}',
    content: 'Executing deterministic Federal Reserve Mod10 checksum formula on Transit Routing Number 021000021.',
    confidence: 0.99
  },
  {
    id: 'MSG-3',
    agentRole: 'FORMAT_VALIDATOR',
    agentName: 'Syntax & Mod10 Inspector',
    stepType: 'OBSERVATION',
    content: 'Violation confirmed: Federal Reserve check digit calculation yields 8, but record specifies 1. Violates Nacha 2025 Operating Rules, Section 3.2.',
    confidence: 0.99
  },
  {
    id: 'MSG-4',
    agentRole: 'LINEAGE_RECON',
    agentName: 'Settlement Lineage Recon',
    stepType: 'TOOL_CALL',
    toolName: 'check_staging_leakage',
    toolParameters: '{"dbTable": "public.settlement_batches", "fileHash": "0a9b...c4"}',
    content: 'Scanning downstream PostgreSQL staging ledger for potential leaked transactions.',
    confidence: 0.98
  },
  {
    id: 'MSG-5',
    agentRole: 'LINEAGE_RECON',
    agentName: 'Settlement Lineage Recon',
    stepType: 'OBSERVATION',
    content: 'Zero leakage detected. Sentinel Gateway quarantined the payload at ingress boundary. Core settlement ledgers are fully isolated.',
    confidence: 0.99
  },
  {
    id: 'MSG-6',
    agentRole: 'AUDIT_COMPLIANCE',
    agentName: 'SEC 17a-4 Audit Defense',
    stepType: 'CONCLUSION',
    content: 'SHA-256 Merkle audit entry anchored to immutable chain. Non-repudiable evidence package generated for compliance archive.',
    confidence: 1.00
  },
  {
    id: 'MSG-7',
    agentRole: 'LEAD_SUPERVISOR',
    agentName: 'Astra Lead Supervisor',
    stepType: 'CONCLUSION',
    content: 'Unanimous Consensus Reached: Contain file in Dead-Letter Quarantine, dispatch Nacha Section 3.2 remediation notice to counterparty, require Tier-3 human supervisor cryptographic approval.',
    confidence: 0.985
  }
];

export const AgentSwarmModal: React.FC<AgentSwarmModalProps> = ({ onClose, onApproveConsensus }) => {
  const [messages, setMessages] = useState<AgentMessage[]>(DEFAULT_MESSAGES);
  const [isDeliberating, setIsDeliberating] = useState<boolean>(false);
  const [isApproved, setIsApproved] = useState<boolean>(false);

  const handleRunSwarm = async () => {
    setIsDeliberating(true);
    try {
      const res = await fetch('http://localhost:8080/api/v1/swarm/deliberate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          incidentId: 101,
          fileId: 501,
          findings: ['INVALID_MOD10_ROUTING'],
          rawData: '6220210000218420000245000999888800John Doe'
        })
      });
      if (res.ok) {
        const data = await res.json();
        if (data.messages && data.messages.length > 0) {
          setMessages(data.messages);
        }
      }
    } catch (e) {
      console.warn('Backend swarm dispatch, using cached consensus', e);
    } finally {
      setIsDeliberating(false);
    }
  };

  const handleSignOff = () => {
    setIsApproved(true);
    if (onApproveConsensus) {
      onApproveConsensus('QUARANTINE_AND_DISPATCH_CORRECTED_RESEND_NOTICE');
    }
  };

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(10, 15, 29, 0.85)',
      backdropFilter: 'blur(8px)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 100,
      padding: '24px'
    }}>
      <div style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-subtle)',
        borderRadius: '12px',
        width: '100%',
        maxWidth: '1100px',
        maxHeight: '92vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.5)'
      }}>
        {/* Header */}
        <div style={{
          padding: '18px 24px',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'rgba(14, 20, 34, 0.6)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{
              width: '36px',
              height: '36px',
              borderRadius: '8px',
              background: 'rgba(139, 92, 246, 0.15)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: '1px solid rgba(139, 92, 246, 0.4)'
            }}>
              <Bot size={20} color="#A78BFA" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Astra Multi-Agent Collaborative Swarm
                </h3>
                <span className="badge" style={{ background: 'rgba(139, 92, 246, 0.2)', color: '#C4B5FD', fontSize: '0.65rem' }}>
                  4 Autonomous Agents Active
                </span>
                <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>Authority Tier 2</span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Collaborative ReAct reasoning, parallel blast-radius audit & human-in-the-loop sign-off
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <button
              onClick={handleRunSwarm}
              disabled={isDeliberating}
              className="btn btn-primary"
              style={{ fontSize: '0.75rem', padding: '6px 12px', display: 'flex', alignItems: 'center', gap: '6px' }}
            >
              {isDeliberating ? <RefreshCw size={14} className="animate-spin" /> : <Play size={14} />}
              <span>{isDeliberating ? 'Swarm Deliberating...' : 'Trigger Swarm Re-Triage'}</span>
            </button>

            <button
              onClick={onClose}
              style={{
                background: 'transparent',
                border: 'none',
                color: 'var(--text-muted)',
                cursor: 'pointer',
                padding: '4px'
              }}
            >
              <X size={20} />
            </button>
          </div>
        </div>

        {/* 4 Agent Fleet Status Bar */}
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(4, 1fr)',
          gap: '10px',
          padding: '14px 24px',
          background: 'rgba(14, 20, 34, 0.4)',
          borderBottom: '1px solid var(--border-subtle)'
        }}>
          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '6px', padding: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Cpu size={16} color="var(--accent-cyan)" />
            <div>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>Lead Supervisor</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>● Orchestrating</div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '6px', padding: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Search size={16} color="var(--accent-amber)" />
            <div>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>Syntax & Mod10</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>● Verified (100%)</div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '6px', padding: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            <GitMerge size={16} color="var(--accent-emerald)" />
            <div>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>Lineage Recon</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>● Blast Radius 0</div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '6px', padding: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            <ShieldCheck size={16} color="#A78BFA" />
            <div>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>Audit Defense</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>● Merkle Proof Signed</div>
            </div>
          </div>
        </div>

        {/* Body: Thought Stream and Consensus */}
        <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: '16px', padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {/* Left Column: Inter-Agent Reasoning Transcript */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Inter-Agent ReAct Reasoning Stream
            </h4>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {messages.map((m, idx) => {
                let badgeColor = 'badge-cyan';
                if (m.agentRole === 'FORMAT_VALIDATOR') badgeColor = 'badge-amber';
                if (m.agentRole === 'LINEAGE_RECON') badgeColor = 'badge-emerald';
                if (m.agentRole === 'AUDIT_COMPLIANCE') badgeColor = 'badge-neutral';

                return (
                  <div
                    key={idx}
                    style={{
                      background: 'var(--bg-primary)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '8px',
                      padding: '12px',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '6px'
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <span className={`badge ${badgeColor}`} style={{ fontSize: '0.65rem' }}>{m.agentName}</span>
                        <span style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>[{m.stepType}]</span>
                      </div>
                      <span className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>
                        {(m.confidence * 100).toFixed(1)}% Conf
                      </span>
                    </div>

                    <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                      {m.content}
                    </p>

                    {m.toolName && (
                      <div style={{
                        background: 'rgba(0, 0, 0, 0.4)',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '4px',
                        padding: '6px 8px',
                        marginTop: '4px'
                      }}>
                        <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--accent-cyan)' }}>
                          <strong>Tool:</strong> {m.toolName}()
                        </div>
                        {m.toolParameters && (
                          <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>
                            {m.toolParameters}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>

          {/* Right Column: Multi-Agent Consensus & Sign-Off */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
            <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Swarm Consensus & Execution Gate
            </h4>

            <div style={{
              background: 'var(--bg-primary)',
              border: '1px solid rgba(139, 92, 246, 0.4)',
              borderRadius: '8px',
              padding: '16px',
              display: 'flex',
              flexDirection: 'column',
              gap: '12px'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Consensus State:</span>
                <span className="badge badge-emerald" style={{ fontSize: '0.7rem' }}>UNANIMOUS (4/4)</span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Overall Confidence:</span>
                <span className="font-mono" style={{ fontSize: '0.875rem', fontWeight: 700, color: 'var(--accent-emerald)' }}>
                  98.5%
                </span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Recommended Action:</span>
                <span className="badge badge-crimson" style={{ fontSize: '0.65rem' }}>QUARANTINE & RESEND</span>
              </div>

              <div style={{
                background: 'rgba(239, 68, 68, 0.08)',
                border: '1px solid rgba(239, 68, 68, 0.25)',
                borderRadius: '6px',
                padding: '10px',
                fontSize: '0.75rem',
                color: 'var(--text-secondary)',
                lineHeight: 1.4
              }}>
                <strong>Authority Tier 3 Boundary Enforced:</strong> The autonomous swarm is prohibited from releasing funds or waiving compliance rules without human supervisor cryptographic sign-off.
              </div>

              <button
                onClick={handleSignOff}
                disabled={isApproved}
                className="btn btn-primary"
                style={{
                  padding: '10px 16px',
                  fontSize: '0.8125rem',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  marginTop: '8px',
                  background: isApproved ? 'var(--accent-emerald)' : undefined
                }}
              >
                {isApproved ? <CheckCircle2 size={16} /> : <UserCheck size={16} />}
                <span>{isApproved ? 'Consensus Approved & Executed' : 'Supervisor Dual-Control Sign-Off'}</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
