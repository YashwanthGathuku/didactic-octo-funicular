import React, { useState, useEffect } from 'react';
import { 
  Building2, 
  FileSpreadsheet, 
  X, 
  Plus, 
  CheckCircle2, 
  Clock
} from 'lucide-react';

interface ContractConfigModalProps {
  onClose: () => void;
}

export const ContractConfigModal: React.FC<ContractConfigModalProps> = ({ onClose }) => {
  const [partners, setPartners] = useState<any[]>([]);
  const [contracts, setContracts] = useState<any[]>([]);
  const [activeTab, setActiveTab] = useState<'PARTNERS' | 'CONTRACTS' | 'NEW_PARTNER'>('CONTRACTS');
  const [newPartnerName, setNewPartnerName] = useState<string>('');
  const [newPartnerRouting, setNewPartnerRouting] = useState<string>('');
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [successMsg, setSuccessMsg] = useState<string>('');

  const fetchConfig = async () => {
    try {
      const [pRes, cRes] = await Promise.all([
        fetch('http://localhost:8080/api/v1/partners').then(r => r.json()),
        fetch('http://localhost:8080/api/v1/contracts').then(r => r.json())
      ]);
      setPartners(pRes || []);
      setContracts(cRes || []);
    } catch (e) {
      console.warn('Config fetch notice:', e);
    }
  };

  useEffect(() => {
    fetchConfig();
  }, []);

  const handleCreatePartner = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newPartnerName || !newPartnerRouting) return;
    setIsSubmitting(true);
    try {
      const res = await fetch('http://localhost:8080/api/v1/partners', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newPartnerName, routingNumber: newPartnerRouting })
      });
      if (res.ok) {
        setSuccessMsg(`Partner ${newPartnerName} successfully registered in Go Gateway ledger!`);
        setNewPartnerName('');
        setNewPartnerRouting('');
        await fetchConfig();
        setTimeout(() => setSuccessMsg(''), 3000);
      }
    } catch (err: any) {
      alert(`Error creating partner: ${err.message}`);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(7, 11, 18, 0.85)',
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
        maxWidth: '850px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.7)',
        overflow: 'hidden'
      }}>
        {/* Header */}
        <div style={{
          padding: '16px 24px',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'rgba(14, 20, 34, 0.6)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div style={{
              width: '32px',
              height: '32px',
              borderRadius: '6px',
              background: 'rgba(2, 132, 199, 0.2)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <Building2 size={18} color="var(--accent-cyan)" />
            </div>
            <div>
              <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                Counterparty Partners & File Contracts Manager
              </h3>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Configure expected arrival windows, filename regex patterns, and SLA grace periods.
              </p>
            </div>
          </div>

          <button 
            className="btn btn-secondary" 
            onClick={onClose}
            style={{ padding: '6px', borderRadius: '50%' }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Tab Switcher */}
        <div style={{
          display: 'flex',
          borderBottom: '1px solid var(--border-subtle)',
          background: 'var(--bg-primary)',
          padding: '0 24px'
        }}>
          <button
            onClick={() => setActiveTab('CONTRACTS')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'CONTRACTS' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'CONTRACTS' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <FileSpreadsheet size={14} />
            <span>Active File Contracts ({contracts.length})</span>
          </button>

          <button
            onClick={() => setActiveTab('PARTNERS')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'PARTNERS' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'PARTNERS' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Building2 size={14} />
            <span>Authorized Counterparties ({partners.length})</span>
          </button>

          <button
            onClick={() => setActiveTab('NEW_PARTNER')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'NEW_PARTNER' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'NEW_PARTNER' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Plus size={14} />
            <span>Register New Counterparty</span>
          </button>
        </div>

        {/* Content Area */}
        <div style={{ padding: '24px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {successMsg && (
            <div style={{
              background: 'rgba(16, 185, 129, 0.15)',
              border: '1px solid rgba(16, 185, 129, 0.4)',
              borderRadius: '8px',
              padding: '12px 16px',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              color: 'var(--accent-emerald)',
              fontSize: '0.8125rem',
              fontWeight: 600
            }}>
              <CheckCircle2 size={16} />
              <span>{successMsg}</span>
            </div>
          )}

          {activeTab === 'CONTRACTS' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {contracts.map(c => (
                <div
                  key={c.id}
                  style={{
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '14px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between'
                  }}
                >
                  <div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                        {c.name}
                      </span>
                      <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>{c.direction}</span>
                      <span className="badge badge-neutral" style={{ fontSize: '0.65rem' }}>{c.timezone}</span>
                    </div>
                    <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                      Counterparty: <strong style={{ color: 'var(--text-secondary)' }}>{c.partnerName}</strong> | Pattern: <code>{c.filenamePattern}</code>
                    </div>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                    <div style={{ textAlign: 'right' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '4px', justifyContent: 'flex-end', fontSize: '0.75rem', color: 'var(--accent-amber)', fontWeight: 600 }}>
                        <Clock size={12} />
                        <span>Cutoff: {c.expectedTime}</span>
                      </div>
                      <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Grace: +{c.gracePeriodMinutes}m</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}

          {activeTab === 'PARTNERS' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
              {partners.map(p => (
                <div
                  key={p.id}
                  style={{
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '14px'
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                    <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                      {p.name}
                    </span>
                    <span className="badge badge-success" style={{ fontSize: '0.65rem' }}>ACTIVE</span>
                  </div>
                  <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                    Fed Routing (ABA): <strong className="font-mono" style={{ color: 'var(--accent-cyan)' }}>{p.routingNumber}</strong>
                  </div>
                  <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                    Registered: {p.createdAt?.substring(0, 10)}
                  </div>
                </div>
              ))}
            </div>
          )}

          {activeTab === 'NEW_PARTNER' && (
            <form onSubmit={handleCreatePartner} style={{ display: 'flex', flexDirection: 'column', gap: '14px', maxWidth: '500px' }}>
              <div>
                <label style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                  Institution / Counterparty Name:
                </label>
                <input
                  type="text"
                  placeholder="e.g. Citadel Securities Clearing LLC"
                  value={newPartnerName}
                  onChange={(e) => setNewPartnerName(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '6px',
                    color: 'var(--text-primary)',
                    fontSize: '0.8125rem'
                  }}
                  required
                />
              </div>

              <div>
                <label style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                  Federal Reserve ABA Routing Number (9 Digits):
                </label>
                <input
                  type="text"
                  placeholder="e.g. 021000021"
                  maxLength={9}
                  value={newPartnerRouting}
                  onChange={(e) => setNewPartnerRouting(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '6px',
                    color: 'var(--text-primary)',
                    fontFamily: 'var(--font-mono)',
                    fontSize: '0.8125rem'
                  }}
                  required
                />
              </div>

              <button
                type="submit"
                className="btn btn-primary"
                disabled={isSubmitting}
                style={{ marginTop: '8px', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px' }}
              >
                <Plus size={14} />
                <span>{isSubmitting ? 'Registering Counterparty...' : 'Register Counterparty in Ledger'}</span>
              </button>
            </form>
          )}
        </div>

        {/* Footer */}
        <div style={{
          padding: '14px 24px',
          borderTop: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          background: 'rgba(14, 20, 34, 0.6)'
        }}>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
