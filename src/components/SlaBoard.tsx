import React from 'react';
import { ExpectationOccurrence, Partner, FileContract } from '../types/financial';
import { assessOccurrenceSla } from '../scheduler/deadlineEngine';
import { Clock, ShieldAlert, CheckCircle2, AlertCircle, ArrowUpRight } from 'lucide-react';

interface SlaBoardProps {
  occurrences: ExpectationOccurrence[];
  contracts: FileContract[];
  partners: Partner[];
  onSelectOccurrence: (occurrence: ExpectationOccurrence) => void;
}

export const SlaBoard: React.FC<SlaBoardProps> = ({
  occurrences,
  contracts,
  partners,
  onSelectOccurrence
}) => {
  return (
    <div className="glass-panel" style={{ padding: '20px' }}>
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: '16px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Clock size={18} color="var(--accent-cyan)" />
          <h2 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
            Expected Delivery Windows & SLA Breach Radar
          </h2>
        </div>
        <span className="badge badge-neutral">
          {occurrences.length} Active Windows
        </span>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: '16px' }}>
        {occurrences.map(occ => {
          const contract = contracts.find(c => c.id === occ.contractId);
          const partner = partners.find(p => p.id === occ.partnerId);
          const assessment = assessOccurrenceSla(occ);

          return (
            <div 
              key={occ.id}
              onClick={() => onSelectOccurrence(occ)}
              style={{
                background: 'var(--bg-secondary)',
                border: `1px solid ${assessment.isBreached ? 'rgba(239, 68, 68, 0.4)' : 'var(--border-subtle)'}`,
                borderRadius: '8px',
                padding: '16px',
                cursor: 'pointer',
                transition: 'all 0.2s ease',
                position: 'relative',
                overflow: 'hidden'
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.borderColor = 'var(--border-active)';
                e.currentTarget.style.transform = 'translateY(-2px)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.borderColor = assessment.isBreached ? 'rgba(239, 68, 68, 0.4)' : 'var(--border-subtle)';
                e.currentTarget.style.transform = 'translateY(0)';
              }}
            >
              {/* Top Row: Partner & Status */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '8px' }}>
                <div>
                  <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                    {partner?.name || 'Counterparty'}
                  </span>
                  <h3 style={{ fontSize: '0.9375rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '2px' }}>
                    {contract?.name || occ.expectedDescription}
                  </h3>
                </div>
                <span className={`badge badge-${assessment.badgeVariant}`}>
                  {assessment.statusLabel}
                </span>
              </div>

              {/* Middle Row: Times & Breach Probability */}
              <div style={{
                background: 'var(--bg-primary)',
                padding: '10px 12px',
                borderRadius: '6px',
                margin: '12px 0',
                display: 'grid',
                gridTemplateColumns: '1fr 1fr',
                gap: '8px',
                fontSize: '0.8125rem'
              }}>
                <div>
                  <div style={{ color: 'var(--text-muted)', fontSize: '0.7rem' }}>SLA DEADLINE (UTC)</div>
                  <div className="font-mono" style={{ fontWeight: 600, color: 'var(--text-primary)', marginTop: '2px' }}>
                    {occ.dueAtUtc.substring(11, 19)}
                  </div>
                </div>
                <div>
                  <div style={{ color: 'var(--text-muted)', fontSize: '0.7rem' }}>GRACE EXPIRY</div>
                  <div className="font-mono" style={{ fontWeight: 600, color: assessment.isBreached ? 'var(--accent-crimson)' : 'var(--text-secondary)', marginTop: '2px' }}>
                    {occ.graceExpiresAtUtc.substring(11, 19)}
                  </div>
                </div>
              </div>

              {/* Bottom Progress & Risk Indicator */}
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: '0.75rem' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  {assessment.isBreached ? (
                    <ShieldAlert size={14} color="var(--accent-crimson)" />
                  ) : assessment.occurrence.status === 'VALID' ? (
                    <CheckCircle2 size={14} color="var(--accent-emerald)" />
                  ) : (
                    <AlertCircle size={14} color="var(--accent-amber)" />
                  )}
                  <span style={{ color: 'var(--text-secondary)' }}>
                    Breach Probability: <strong style={{ color: assessment.breachProbability > 0.5 ? 'var(--accent-crimson)' : 'var(--text-primary)' }}>{(assessment.breachProbability * 100).toFixed(0)}%</strong>
                  </span>
                </div>
                <span style={{ color: 'var(--accent-cyan)', display: 'flex', alignItems: 'center', gap: '2px' }}>
                  Inspect <ArrowUpRight size={12} />
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
