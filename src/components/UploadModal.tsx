import React, { useState, useRef } from 'react';
import { 
  UploadCloud, 
  FileCode, 
  X, 
  Play, 
  CheckCircle2, 
  AlertOctagon, 
  FolderPlus, 
  Cpu, 
  ArrowRight,
  ShieldAlert
} from 'lucide-react';
import { PRESET_OPTIONS, GeneratorPresetKey } from '../mockData/generator';
import { SentinelApi, ApiIngestionResult } from '../services/api';

interface UploadModalProps {
  onClose: () => void;
  onFileIngested: (result: ApiIngestionResult, rawContent: string) => void;
}

export const UploadModal: React.FC<UploadModalProps> = ({ onClose, onFileIngested }) => {
  const [activeTab, setActiveTab] = useState<'DROP' | 'PRESET' | 'SFTP_INBOX'>('PRESET');
  const [selectedPreset, setSelectedPreset] = useState<GeneratorPresetKey>('BALANCED_PPD_PAYROLL');
  const [previewContent, setPreviewContent] = useState<string>('');
  const [previewFilename, setPreviewFilename] = useState<string>('');
  const [isProcessing, setIsProcessing] = useState<boolean>(false);
  const [ingestionResult, setIngestionResult] = useState<ApiIngestionResult | null>(null);
  const [dragOver, setDragOver] = useState<boolean>(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Load preset preview on mount or preset change
  React.useEffect(() => {
    const fetchPreset = async () => {
      try {
        const res = await fetch(`http://localhost:8080/api/v1/generator/sample?preset=${selectedPreset}`);
        if (res.ok) {
          const data = await res.json();
          setPreviewContent(data.content || '');
          setPreviewFilename(data.filename || `SAMPLE_${selectedPreset}.ach`);
        }
      } catch (e) {
        console.warn('Preset fetch failed:', e);
      }
    };
    fetchPreset();
  }, [selectedPreset]);

  // Handle Ingest
  const handleIngestContent = async (filename: string, content: string) => {
    setIsProcessing(true);
    setIngestionResult(null);

    try {
      const result = await SentinelApi.ingestRawNacha(filename, content);
      setIngestionResult(result);
      onFileIngested(result, content);
    } catch (err: any) {
      alert(`Ingestion error: ${err.message}`);
    } finally {
      setIsProcessing(false);
    }
  };

  // Handle Local File Selection / Drop
  const handleFileDrop = (file: File) => {
    const reader = new FileReader();
    reader.onload = async (e) => {
      const text = e.target?.result as string;
      setPreviewContent(text);
      setPreviewFilename(file.name);
      await handleIngestContent(file.name, text);
    };
    reader.readAsText(file);
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
              <UploadCloud size={18} color="var(--accent-cyan)" />
            </div>
            <div>
              <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                Direct Transmission Ingestion & Preset Harness
              </h3>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Test live Go streaming validation, SHA-256 calculation, and automatic AI quarantine routing.
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
            onClick={() => setActiveTab('PRESET')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'PRESET' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'PRESET' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Cpu size={14} />
            <span>Preset Scenario Generator</span>
          </button>

          <button
            onClick={() => setActiveTab('DROP')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'DROP' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'DROP' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <UploadCloud size={14} />
            <span>Drag & Drop Local NACHA File</span>
          </button>

          <button
            onClick={() => setActiveTab('SFTP_INBOX')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'SFTP_INBOX' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'SFTP_INBOX' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <FolderPlus size={14} />
            <span>Live SFTP Inbox Drop Daemon</span>
          </button>
        </div>

        {/* Tab Content */}
        <div style={{ padding: '24px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {activeTab === 'PRESET' && (
            <div>
              <label style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '8px' }}>
                Select Test Scenario Preset:
              </label>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px', marginBottom: '16px' }}>
                {PRESET_OPTIONS.map(opt => {
                  const isSelected = selectedPreset === opt.key;
                  const isNominal = opt.category === 'NOMINAL';
                  const isAllowable = opt.category === 'CONTRACT_ALLOWABLE';

                  return (
                    <div
                      key={opt.key}
                      onClick={() => setSelectedPreset(opt.key)}
                      style={{
                        padding: '12px',
                        background: isSelected ? 'rgba(2, 132, 199, 0.15)' : 'var(--bg-primary)',
                        border: `1px solid ${isSelected ? 'var(--accent-cyan)' : 'var(--border-subtle)'}`,
                        borderRadius: '8px',
                        cursor: 'pointer',
                        transition: 'all 0.15s ease'
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '4px' }}>
                        <span style={{ fontSize: '0.8125rem', fontWeight: 600, color: isSelected ? 'var(--accent-cyan)' : 'var(--text-primary)' }}>
                          {opt.label}
                        </span>
                        <span className={`badge ${isNominal ? 'badge-success' : isAllowable ? 'badge-cyan' : 'badge-danger'}`} style={{ fontSize: '0.65rem' }}>
                          {opt.category}
                        </span>
                      </div>
                      <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)', lineHeight: 1.4 }}>
                        {opt.description}
                      </p>
                    </div>
                  );
                })}
              </div>

              {/* Raw Record Preview */}
              <div>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                  <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                    Generated NACHA Payload Preview (94-char fixed-width alignment):
                  </span>
                  <span className="font-mono" style={{ fontSize: '0.7rem', color: 'var(--accent-cyan)' }}>
                    {previewFilename}
                  </span>
                </div>
                <pre style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  padding: '12px',
                  borderRadius: '6px',
                  fontFamily: 'var(--font-mono)',
                  fontSize: '0.72rem',
                  lineHeight: 1.4,
                  overflowX: 'auto',
                  maxHeight: '150px',
                  color: 'var(--text-secondary)'
                }}>
                  {previewContent || 'Generating preset records...'}
                </pre>
              </div>
            </div>
          )}

          {activeTab === 'DROP' && (
            <div>
              <div
                onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
                onDragLeave={() => setDragOver(false)}
                onDrop={(e) => {
                  e.preventDefault();
                  setDragOver(false);
                  if (e.dataTransfer.files && e.dataTransfer.files[0]) {
                    handleFileDrop(e.dataTransfer.files[0]);
                  }
                }}
                onClick={() => fileInputRef.current?.click()}
                style={{
                  border: `2px dashed ${dragOver ? 'var(--accent-cyan)' : 'var(--border-subtle)'}`,
                  background: dragOver ? 'rgba(2, 132, 199, 0.1)' : 'var(--bg-primary)',
                  borderRadius: '10px',
                  padding: '40px 20px',
                  textAlign: 'center',
                  cursor: 'pointer',
                  transition: 'all 0.2s ease'
                }}
              >
                <input
                  type="file"
                  ref={fileInputRef}
                  style={{ display: 'none' }}
                  accept=".ach,.txt,.csv,.dat"
                  onChange={(e) => {
                    if (e.target.files && e.target.files[0]) {
                      handleFileDrop(e.target.files[0]);
                    }
                  }}
                />
                <FileCode size={36} color="var(--accent-cyan)" style={{ margin: '0 auto 12px auto' }} />
                <h4 style={{ fontSize: '0.9375rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Click to select or drag & drop financial file here
                </h4>
                <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                  Supports NACHA ACH (.ach, .txt), ISO 20022 XML, and BAI2 files up to 20MB.
                </p>
              </div>
            </div>
          )}

          {activeTab === 'SFTP_INBOX' && (
            <div style={{
              background: 'var(--bg-primary)',
              border: '1px solid var(--border-subtle)',
              borderRadius: '8px',
              padding: '16px'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '10px' }}>
                <FolderPlus size={18} color="var(--accent-emerald)" />
                <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Active SFTP Inbox Poller Daemon
                </h4>
                <span className="badge badge-emerald">ACTIVE POLLING</span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                The Go Gateway is currently listening for real SFTP file drops in directory:
              </p>
              <div style={{
                background: 'var(--bg-secondary)',
                border: '1px solid var(--border-subtle)',
                padding: '8px 12px',
                borderRadius: '6px',
                fontFamily: 'var(--font-mono)',
                fontSize: '0.8rem',
                color: 'var(--accent-cyan)',
                margin: '8px 0 12px 0'
              }}>
                valiant-davinci/gateway/inbox/
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                When any <code>.ach</code> or <code>.txt</code> file is copied into this folder, the Go worker will automatically calculate its SHA-256 digest in real-time, validate all 94-char records, and route it to <code>/processed</code> (if valid) or <code>/quarantine</code> (if malformed).
              </p>
            </div>
          )}

          {/* Real-time Ingestion Result Banner */}
          {ingestionResult && (
            <div style={{
              background: ingestionResult.status === 'VALIDATED' ? 'rgba(16, 185, 129, 0.15)' : 'rgba(239, 68, 68, 0.15)',
              border: `1px solid ${ingestionResult.status === 'VALIDATED' ? 'rgba(16, 185, 129, 0.4)' : 'rgba(239, 68, 68, 0.4)'}`,
              borderRadius: '8px',
              padding: '14px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                {ingestionResult.status === 'VALIDATED' ? (
                  <CheckCircle2 size={20} color="var(--accent-emerald)" />
                ) : (
                  <AlertOctagon size={20} color="var(--accent-crimson)" />
                )}
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                      Ingestion Complete: {ingestionResult.status}
                    </span>
                    <span className="badge badge-neutral" style={{ fontSize: '0.65rem' }}>
                      {ingestionResult.sizeBytes} bytes
                    </span>
                  </div>
                  <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '2px' }}>
                    SHA-256: <code>{ingestionResult.hash.substring(0, 20)}...</code> | Findings: {ingestionResult.findings.length}
                  </p>
                </div>
              </div>

              {ingestionResult.status === 'QUARANTINED' && (
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <ShieldAlert size={16} color="var(--accent-crimson)" />
                  <span style={{ fontSize: '0.75rem', color: 'var(--accent-crimson)', fontWeight: 600 }}>
                    AI Incident Auto-Triggered
                  </span>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer Actions */}
        <div style={{
          padding: '16px 24px',
          borderTop: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          gap: '12px',
          background: 'rgba(14, 20, 34, 0.6)'
        }}>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>

          {activeTab === 'PRESET' && (
            <button
              className="btn btn-primary"
              disabled={isProcessing}
              onClick={() => handleIngestContent(previewFilename, previewContent)}
              style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
            >
              <Play size={14} />
              <span>{isProcessing ? 'Streaming to Go Gateway...' : 'Ingest Scenario into Live Gateway'}</span>
              <ArrowRight size={14} />
            </button>
          )}
        </div>
      </div>
    </div>
  );
};
