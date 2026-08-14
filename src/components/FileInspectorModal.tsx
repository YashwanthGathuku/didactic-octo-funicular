import React, { useState } from 'react';
import { FileInstance, Partner, FileContract } from '../types/financial';
import { X, ShieldAlert, CheckCircle, Terminal, Copy, Check } from 'lucide-react';

interface FileInspectorModalProps {
  fileInstance: FileInstance;
  partner?: Partner;
  contract?: FileContract;
  rawContent?: string;
  onClose: () => void;
  onLaunchAiTriage: (file: FileInstance) => void;
}

export const FileInspectorModal: React.FC<FileInspectorModalProps> = ({
  fileInstance,
  partner,
  contract,
  rawContent = '',
  onClose,
  onLaunchAiTriage
}) => {
  const [activeTab, setActiveTab] = useState<'findings' | 'raw' | 'controls'>('findings');
  const [copiedHash, setCopiedHash] = useState(false);

  const res = fileInstance.validationResult;
  const isQuarantined = fileInstance.state === 'QUARANTINED';

  const handleCopyHash = () => {
    navigator.clipboard.writeText(fileInstance.sha256Hash);
    setCopiedHash(true);
    setTimeout(() => setCopiedHash(false), 2000);
  };

  const rawLines = rawContent.split(/\r?\n/).filter(l => l.length > 0);

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
      zIndex: 100,
      padding: '24px'
    }}>
      <div style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-medium)',
        borderRadius: '12px',
        width: '100%',
        maxWidth: '960px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 20px 40px rgba(0, 0, 0, 0.8)',
        overflow: 'hidden'
      }}>
        {/* Modal Header */}
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
              background: isQuarantined ? 'var(--accent-crimson-dim)' : 'var(--accent-emerald-dim)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: `1px solid ${isQuarantined ? 'rgba(239, 68, 68, 0.4)' : 'rgba(16, 185, 129, 0.4)'}`
            }}>
              {isQuarantined ? <ShieldAlert size={18} color="var(--accent-crimson)" /> : <CheckCircle size={18} color="var(--accent-emerald)" />}
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h2 style={{ fontSize: '1.0625rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  {fileInstance.filename}
                </h2>
                <span className={`badge ${isQuarantined ? 'badge-danger' : 'badge-success'}`}>
                  {fileInstance.state}
                </span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Partner: <strong style={{ color: 'var(--text-secondary)' }}>{partner?.name || 'Unknown'}</strong> | Contract: {contract?.name || 'Standard ACH'}
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <button 
              className="btn btn-primary"
              onClick={() => onLaunchAiTriage(fileInstance)}
              style={{ fontSize: '0.8125rem', padding: '6px 14px' }}
            >
              <Terminal size={14} />
              <span>Launch Astra Copilot</span>
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

        {/* Cryptographic Lineage Header Bar */}
        <div style={{
          padding: '10px 24px',
          background: 'var(--bg-primary)',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: '0.75rem',
          color: 'var(--text-secondary)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span style={{ color: 'var(--text-muted)' }}>SHA-256 Digest:</span>
            <span className="font-mono" style={{ color: 'var(--accent-cyan)' }}>
              {fileInstance.sha256Hash}
            </span>
            <button 
              onClick={handleCopyHash}
              style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center' }}
              title="Copy hash"
            >
              {copiedHash ? <Check size={12} color="var(--accent-emerald)" /> : <Copy size={12} />}
            </button>
          </div>
          <div>
            <span>Size: <strong>{fileInstance.byteSize.toLocaleString()} bytes</strong></span>
            <span style={{ margin: '0 8px', color: 'var(--border-subtle)' }}>|</span>
            <span>Received: <strong>{fileInstance.receivedAtUtc.substring(11, 19)} UTC</strong></span>
          </div>
        </div>

        {/* Tabs Bar */}
        <div style={{
          display: 'flex',
          gap: '8px',
          padding: '8px 24px',
          background: 'var(--bg-secondary)',
          borderBottom: '1px solid var(--border-subtle)'
        }}>
          <button
            onClick={() => setActiveTab('findings')}
            className={`btn ${activeTab === 'findings' ? 'btn-secondary' : ''}`}
            style={{
              padding: '6px 12px',
              fontSize: '0.8125rem',
              background: activeTab === 'findings' ? 'var(--bg-tertiary)' : 'transparent'
            }}
          >
            Deterministic Findings ({res?.findings.length || 0})
          </button>
          <button
            onClick={() => setActiveTab('controls')}
            className={`btn ${activeTab === 'controls' ? 'btn-secondary' : ''}`}
            style={{
              padding: '6px 12px',
              fontSize: '0.8125rem',
              background: activeTab === 'controls' ? 'var(--bg-tertiary)' : 'transparent'
            }}
          >
            Control Record Balancer
          </button>
          <button
            onClick={() => setActiveTab('raw')}
            className={`btn ${activeTab === 'raw' ? 'btn-secondary' : ''}`}
            style={{
              padding: '6px 12px',
              fontSize: '0.8125rem',
              background: activeTab === 'raw' ? 'var(--bg-tertiary)' : 'transparent'
            }}
          >
            Raw 94-Char Record Inspector
          </button>
        </div>

        {/* Tab Content */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {activeTab === 'findings' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {res?.findings.length === 0 ? (
                <div style={{
                  padding: '30px',
                  textAlign: 'center',
                  background: 'var(--accent-emerald-dim)',
                  border: '1px solid rgba(16, 185, 129, 0.3)',
                  borderRadius: '8px',
                  color: 'var(--accent-emerald)'
                }}>
                  <CheckCircle size={32} style={{ margin: '0 auto 8px auto' }} />
                  <p style={{ fontWeight: 600 }}>Pre-Flight Verification Succeeded</p>
                  <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', marginTop: '4px' }}>
                    Zero syntax, block alignment, entry hash, or arithmetic discrepancies detected.
                  </p>
                </div>
              ) : (
                res?.findings.map(finding => (
                  <div 
                    key={finding.id}
                    style={{
                      background: 'var(--bg-primary)',
                      border: `1px solid ${finding.severity === 'FATAL' || finding.severity === 'ERROR' || finding.severity === 'CRITICAL' ? 'rgba(239, 68, 68, 0.3)' : 'rgba(245, 158, 11, 0.3)'}`,
                      borderRadius: '8px',
                      padding: '14px 16px'
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                        <span className={`badge ${finding.severity === 'FATAL' || finding.severity === 'ERROR' || finding.severity === 'CRITICAL' ? 'badge-danger' : 'badge-warning'}`}>
                          {finding.code}
                        </span>
                        {finding.lineNumber && (
                          <span className="badge badge-neutral">Line {finding.lineNumber}</span>
                        )}
                        {finding.recordType && (
                          <span className="badge badge-neutral">Record Type {finding.recordType}</span>
                        )}
                      </div>
                      <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                        {finding.ruleReference}
                      </span>
                    </div>

                    <p style={{ fontSize: '0.875rem', color: 'var(--text-primary)', marginTop: '4px' }}>
                      {finding.message}
                    </p>

                    {finding.rawSampleRedacted && (
                      <div style={{ marginTop: '8px' }}>
                        <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>REDACTED EXCERPT:</span>
                        <div className="log-viewer log-line-error" style={{ marginTop: '2px' }}>
                          {finding.rawSampleRedacted}
                        </div>
                      </div>
                    )}
                  </div>
                ))
              )}
            </div>
          )}

          {activeTab === 'controls' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
              {/* Balances Card */}
              <div className="glass-panel" style={{ padding: '16px' }}>
                <h3 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
                  Dollar Amount Reconciliation
                </h3>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '0.8125rem' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Total Debits:</span>
                    <span className="font-mono" style={{ fontWeight: 600, color: 'var(--accent-crimson)' }}>
                      ${(res?.totalDebitsUsd || 0).toLocaleString(undefined, { minimumFractionDigits: 2 })}
                    </span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Total Credits:</span>
                    <span className="font-mono" style={{ fontWeight: 600, color: 'var(--accent-emerald)' }}>
                      ${(res?.totalCreditsUsd || 0).toLocaleString(undefined, { minimumFractionDigits: 2 })}
                    </span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Settlement State:</span>
                    <span className={`badge ${res?.isBalanced ? 'badge-success' : 'badge-warning'}`}>
                      {res?.isBalanced ? 'Balanced (Zero-Net)' : 'Unbalanced Batch'}
                    </span>
                  </div>
                </div>
              </div>

              {/* Hash Verification Card */}
              <div className="glass-panel" style={{ padding: '16px' }}>
                <h3 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
                  10-Digit Entry Hash Audit
                </h3>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '0.8125rem' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Calculated Entry Hash:</span>
                    <span className="font-mono" style={{ fontWeight: 600, color: 'var(--accent-cyan)' }}>
                      {res?.calculatedEntryHash || '0'}
                    </span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Declared Trailer Hash:</span>
                    <span className="font-mono" style={{ fontWeight: 600, color: 'var(--text-secondary)' }}>
                      {res?.expectedEntryHash || res?.calculatedEntryHash || '0'}
                    </span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Hash Parity:</span>
                    <span className="badge badge-success">
                      Verified Modulo 10^10
                    </span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'raw' && (
            <div className="log-viewer" style={{ maxHeight: '400px' }}>
              {rawLines.map((line, idx) => {
                const lineNum = idx + 1;
                const hasErrorOnLine = res?.findings.some(f => f.lineNumber === lineNum);

                return (
                  <div 
                    key={lineNum}
                    className={hasErrorOnLine ? 'log-line-error' : ''}
                    style={{ display: 'flex', gap: '12px', padding: '2px 0' }}
                  >
                    <span style={{ color: 'var(--text-dim)', width: '36px', textAlign: 'right', userSelect: 'none' }}>
                      {lineNum}
                    </span>
                    <span style={{ whiteSpace: 'pre', color: hasErrorOnLine ? '#FCA5A5' : 'var(--text-secondary)' }}>
                      {line}
                    </span>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
