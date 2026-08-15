import React, { useState, useEffect } from 'react';
import { DomainEvent } from '../types/financial';
import { TamperEvidentEventStore } from '../audit/hashChain';
import { X, ShieldCheck, Download, CheckCircle } from 'lucide-react';

interface AuditLedgerModalProps {
  eventStore: TamperEvidentEventStore;
  onClose: () => void;
}

export const AuditLedgerModal: React.FC<AuditLedgerModalProps> = ({
  eventStore,
  onClose
}) => {
  const [events, setEvents] = useState<DomainEvent[]>([]);
  const [integrity, setIntegrity] = useState<{ isValid: boolean; totalEvents: number; error?: string }>({
    isValid: true,
    totalEvents: 0
  });

  useEffect(() => {
    const loadAndVerify = async () => {
      const allEvents = eventStore.getEvents();
      setEvents(allEvents);
      const res = await eventStore.verifyIntegrity();
      setIntegrity(res);
    };
    loadAndVerify();
  }, [eventStore]);

  const handleExportJson = () => {
    const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(events, null, 2));
    const downloadAnchor = document.createElement('a');
    downloadAnchor.setAttribute("href", dataStr);
    downloadAnchor.setAttribute("download", `sentinel-flow-audit-ledger-${Date.now()}.json`);
    document.body.appendChild(downloadAnchor);
    downloadAnchor.click();
    downloadAnchor.remove();
  };

  const handleExportCompliancePackage = async () => {
    try {
      const pkg = await fetch('http://localhost:8080/api/v1/compliance/export').then(r => r.json());
      const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(pkg, null, 2));
      const downloadAnchor = document.createElement('a');
      downloadAnchor.setAttribute("href", dataStr);
      downloadAnchor.setAttribute("download", `SEC_17a4_COMPLIANCE_PROOF_PACKAGE_${Date.now()}.json`);
      document.body.appendChild(downloadAnchor);
      downloadAnchor.click();
      downloadAnchor.remove();
    } catch {
      handleExportJson();
    }
  };

  return (
    <div style={{
      position: 'fixed',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      background: 'rgba(5, 8, 14, 0.85)',
      backdropFilter: 'blur(8px)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 110,
      padding: '24px'
    }}>
      <div style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-medium)',
        borderRadius: '12px',
        width: '100%',
        maxWidth: '920px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 20px 40px rgba(0, 0, 0, 0.8)',
        overflow: 'hidden'
      }}>
        {/* Header */}
        <div style={{
          padding: '16px 24px',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'var(--bg-card)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{
              width: '32px',
              height: '32px',
              borderRadius: '6px',
              background: 'var(--accent-emerald-dim)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: '1px solid rgba(16, 185, 129, 0.4)'
            }}>
              <ShieldCheck size={18} color="var(--accent-emerald)" />
            </div>
            <div>
              <h2 style={{ fontSize: '1.0625rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                Append-Only Audit Ledger
              </h2>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Application hash chain — linear SHA-256 predecessor links, verified by recomputation
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <button 
              className="btn btn-primary"
              onClick={handleExportCompliancePackage}
              style={{ fontSize: '0.8125rem', padding: '6px 12px', display: 'flex', alignItems: 'center', gap: '6px', background: 'var(--accent-emerald)', borderColor: 'var(--accent-emerald)', color: '#000', fontWeight: 700 }}
            >
              <Download size={14} />
              <span>Download Evidence Export</span>
            </button>
            <button 
              className="btn btn-secondary"
              onClick={handleExportJson}
              style={{ fontSize: '0.8125rem', padding: '6px 12px' }}
            >
              <Download size={14} />
              <span>Export Raw Ledger (JSON)</span>
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

        {/* Verification Status Banner */}
        <div style={{
          padding: '12px 24px',
          background: integrity.isValid ? 'var(--accent-emerald-dim)' : 'var(--accent-crimson-dim)',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: '0.8125rem'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {integrity.isValid ? (
              <>
                <CheckCircle size={16} color="var(--accent-emerald)" />
                <span style={{ color: 'var(--accent-emerald)', fontWeight: 600 }}>
                  Cryptographic Chain Verified ({events.length} blocks without tampering)
                </span>
              </>
            ) : (
              <span style={{ color: 'var(--accent-crimson)', fontWeight: 600 }}>
                {integrity.error}
              </span>
            )}
          </div>
          <span className="font-mono" style={{ color: 'var(--text-secondary)', fontSize: '0.75rem' }}>
            Algorithm: SHA-256 linear hash chain (not a Merkle tree)
          </span>
        </div>

        {/* Events List */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {events.length === 0 ? (
            <div style={{ textAlign: 'center', color: 'var(--text-muted)', padding: '40px' }}>
              No domain events recorded in session yet.
            </div>
          ) : (
            events.map((evt, idx) => (
              <div 
                key={evt.id}
                style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '8px',
                  padding: '14px 16px',
                  position: 'relative'
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '8px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <span className="badge badge-cyan">#{idx + 1} {evt.eventType}</span>
                    <span className="badge badge-neutral">{evt.aggregateType}:{evt.aggregateId}</span>
                  </div>
                  <span className="font-mono" style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                    {evt.timestampUtc}
                  </span>
                </div>

                {/* Hashes Row */}
                <div style={{
                  background: 'var(--bg-secondary)',
                  padding: '8px 12px',
                  borderRadius: '4px',
                  fontSize: '0.7rem',
                  fontFamily: 'var(--font-mono)',
                  marginBottom: '8px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '4px'
                }}>
                  <div style={{ display: 'flex', gap: '8px' }}>
                    <span style={{ color: 'var(--text-dim)', width: '90px' }}>PREV HASH:</span>
                    <span style={{ color: 'var(--text-muted)' }}>{evt.previousHash}</span>
                  </div>
                  <div style={{ display: 'flex', gap: '8px' }}>
                    <span style={{ color: 'var(--accent-cyan)', width: '90px', fontWeight: 600 }}>BLOCK HASH:</span>
                    <span style={{ color: 'var(--accent-cyan)' }}>{evt.currentHash}</span>
                  </div>
                </div>

                {/* Payload snippet */}
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                  <span style={{ color: 'var(--text-muted)' }}>Actor: </span>
                  <strong>{evt.actor}</strong>
                  <span style={{ margin: '0 8px', color: 'var(--border-subtle)' }}>|</span>
                  <span style={{ color: 'var(--text-muted)' }}>Correlation ID: </span>
                  <span className="font-mono">{evt.correlationId}</span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
