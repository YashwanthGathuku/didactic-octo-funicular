import React, { useEffect, useState } from 'react';
import { ShieldCheck, Clock, FileText, AlertTriangle, Activity, Building2, GitCompare } from 'lucide-react';

interface HeaderProps {
  onOpenAudit: () => void;
  onOpenUpload: () => void;
  onOpenContracts: () => void;
  onOpenDiff?: () => void;
  openIncidentsCount: number;
  quarantinedCount: number;
}

export const Header: React.FC<HeaderProps> = ({
  onOpenAudit,
  onOpenUpload,
  onOpenContracts,
  onOpenDiff,
  openIncidentsCount,
  quarantinedCount
}) => {
  const [timeUtc, setTimeUtc] = useState<string>('');

  useEffect(() => {
    const updateTime = () => {
      const now = new Date();
      setTimeUtc(now.toISOString().replace('T', ' ').substring(0, 19) + ' UTC');
    };
    updateTime();
    const interval = setInterval(updateTime, 1000);
    return () => clearInterval(interval);
  }, []);

  return (
    <header style={{
      background: 'var(--bg-secondary)',
      borderBottom: '1px solid var(--border-subtle)',
      padding: '12px 24px',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      position: 'sticky',
      top: 0,
      zIndex: 50
    }}>
      {/* Brand Identity */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
        <div style={{
          width: '36px',
          height: '36px',
          borderRadius: '8px',
          background: 'linear-gradient(135deg, #0284C7 0%, #0369A1 100%)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          boxShadow: '0 0 15px rgba(2, 132, 199, 0.4)'
        }}>
          <ShieldCheck size={22} color="#FFFFFF" />
        </div>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span style={{ fontSize: '1.125rem', fontWeight: 700, letterSpacing: '-0.02em', color: '#F8FAFC' }}>
              SENTINEL FLOW
            </span>
            <span className="badge badge-cyan">Pre-Ledger Gateway</span>
            <span className="badge badge-emerald" style={{ fontSize: '0.65rem', display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
              <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: '#10B981', display: 'inline-block' }} />
              Go + Moov ACH (:8080)
            </span>
            <span className="badge badge-emerald" style={{ fontSize: '0.65rem', display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
              <span style={{ width: '6px', height: '6px', borderRadius: '50%', background: '#10B981', display: 'inline-block' }} />
              Astra 2.0 AI (:8000)
            </span>
          </div>
          <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
            Financial File Reliability, NACHA Streaming Validator & Governed AI Triage
          </p>
        </div>
      </div>

      {/* Telemetry & Clocks */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '20px' }}>
        {/* Settlement Cutoff Indicators */}
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: '12px',
          padding: '6px 14px',
          background: 'var(--bg-primary)',
          borderRadius: '6px',
          border: '1px solid var(--border-subtle)',
          fontSize: '0.8125rem'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Clock size={14} color="var(--accent-cyan)" />
            <span style={{ color: 'var(--text-muted)' }}>Clock:</span>
            <span className="font-mono" style={{ color: 'var(--text-primary)', fontWeight: 600 }}>
              {timeUtc}
            </span>
          </div>
          <div style={{ height: '14px', width: '1px', background: 'var(--border-subtle)' }} />
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <Activity size={14} color="var(--accent-emerald)" />
            <span style={{ color: 'var(--text-muted)' }}>Next Clearing:</span>
            <span className="font-mono" style={{ color: 'var(--accent-amber)', fontWeight: 600 }}>
              16:45 FedACH Window
            </span>
          </div>
        </div>

        {/* Status Counters */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          {openIncidentsCount > 0 && (
            <div style={{
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              padding: '6px 12px',
              background: 'var(--accent-crimson-dim)',
              border: '1px solid rgba(239, 68, 68, 0.4)',
              borderRadius: '6px',
              color: 'var(--accent-crimson)',
              fontSize: '0.8125rem',
              fontWeight: 600
            }}>
              <AlertTriangle size={14} />
              <span>{openIncidentsCount} Open Incidents</span>
            </div>
          )}

          {quarantinedCount > 0 && (
            <div style={{
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              padding: '6px 12px',
              background: 'var(--accent-amber-dim)',
              border: '1px solid rgba(245, 158, 11, 0.4)',
              borderRadius: '6px',
              color: 'var(--accent-amber)',
              fontSize: '0.8125rem',
              fontWeight: 600
            }}>
              <span>{quarantinedCount} Quarantined</span>
            </div>
          )}

          <button 
            className="btn btn-secondary"
            onClick={onOpenContracts}
            style={{ padding: '6px 12px', fontSize: '0.8125rem', display: 'flex', alignItems: 'center', gap: '6px' }}
          >
            <Building2 size={14} color="var(--accent-cyan)" />
            <span>Contracts & Partners</span>
          </button>

          {onOpenDiff && (
            <button 
              className="btn btn-secondary"
              onClick={onOpenDiff}
              style={{ padding: '6px 12px', fontSize: '0.8125rem', display: 'flex', alignItems: 'center', gap: '6px', borderColor: 'rgba(6, 182, 212, 0.4)' }}
            >
              <GitCompare size={14} color="var(--accent-cyan)" />
              <span>Visual Redliner</span>
            </button>
          )}

          <button 
            className="btn btn-primary"
            onClick={onOpenUpload}
            style={{ padding: '6px 12px', fontSize: '0.8125rem', display: 'flex', alignItems: 'center', gap: '6px' }}
          >
            <Activity size={14} />
            <span>Ingest & Presets</span>
          </button>

          <button 
            className="btn btn-secondary"
            onClick={onOpenAudit}
            style={{ padding: '6px 12px', fontSize: '0.8125rem' }}
          >
            <FileText size={14} />
            <span>Audit Proof Ledger</span>
          </button>
        </div>
      </div>
    </header>
  );
};
