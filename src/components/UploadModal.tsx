import React, { useState, useRef } from 'react';
import { 
  UploadCloud, 
  X, 
  Play, 
  CheckCircle2, 
  FolderPlus, 
  Cpu, 
  ShieldAlert,
  Loader2
} from 'lucide-react';
import { PRESET_OPTIONS, GeneratorPresetKey } from '../mockData/generator';
import { ingestRawNacha, ApiIngestionResult } from '../services/api';

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
  const [ingestError, setIngestError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState<boolean>(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

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

  const handleIngestContent = async (filename: string, content: string) => {
    setIsProcessing(true);
    setIngestionResult(null);

    const result = await ingestRawNacha(filename, content);
    setIsProcessing(false);

    if (result.state === 'ok') {
      setIngestionResult(result.data);
      onFileIngested(result.data, content);
      setIngestError(null);
      return;
    }
    setIngestionResult(null);
    setIngestError(
      result.state === 'unavailable'
        ? `The gateway could not be reached: ${result.error}`
        : result.state === 'forbidden'
          ? 'Your account does not hold the permission to upload an artifact.'
          : result.state === 'unauthenticated'
            ? 'Your session is no longer valid. Sign in again and retry.'
            : `The gateway refused this file: ${result.error}`,
    );
  };

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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 p-4 backdrop-blur-sm">
      <div className="flex max-h-[90vh] w-full max-w-3xl flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl">
        {/* Modal Header */}
        <div className="flex items-center justify-between border-b border-slate-200 bg-slate-50 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600 border border-indigo-100 shadow-xs">
              <UploadCloud className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-slate-900">
                Direct Ingestion & Test Scenario Harness
              </h3>
              <p className="text-xs text-slate-500">
                Execute streaming validation, SHA-256 calculation, and deterministic quarantine routing.
              </p>
            </div>
          </div>

          <button 
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-200 hover:text-slate-700 transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Tab Navigation */}
        <div className="flex border-b border-slate-200 bg-slate-50/70 px-6">
          <button
            onClick={() => setActiveTab('PRESET')}
            className={`flex items-center gap-2 border-b-2 px-4 py-3 text-xs font-bold transition-all ${
              activeTab === 'PRESET'
                ? 'border-indigo-600 text-indigo-600'
                : 'border-transparent text-slate-500 hover:text-slate-900'
            }`}
          >
            <Cpu className="h-4 w-4" />
            <span>Preset Scenarios</span>
          </button>

          <button
            onClick={() => setActiveTab('DROP')}
            className={`flex items-center gap-2 border-b-2 px-4 py-3 text-xs font-bold transition-all ${
              activeTab === 'DROP'
                ? 'border-indigo-600 text-indigo-600'
                : 'border-transparent text-slate-500 hover:text-slate-900'
            }`}
          >
            <UploadCloud className="h-4 w-4" />
            <span>Upload NACHA File</span>
          </button>

          <button
            onClick={() => setActiveTab('SFTP_INBOX')}
            className={`flex items-center gap-2 border-b-2 px-4 py-3 text-xs font-bold transition-all ${
              activeTab === 'SFTP_INBOX'
                ? 'border-indigo-600 text-indigo-600'
                : 'border-transparent text-slate-500 hover:text-slate-900'
            }`}
          >
            <FolderPlus className="h-4 w-4" />
            <span>SFTP Ingress Daemon</span>
          </button>
        </div>

        {/* Modal Body */}
        <div className="flex-1 space-y-4 overflow-y-auto p-6">
          {activeTab === 'PRESET' && (
            <div className="space-y-4">
              <label className="text-xs font-bold uppercase tracking-wider text-slate-500">
                Select Test Scenario Preset
              </label>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {PRESET_OPTIONS.map(opt => {
                  const isSelected = selectedPreset === opt.key;
                  const isNominal = opt.category === 'NOMINAL';
                  const isAllowable = opt.category === 'CONTRACT_ALLOWABLE';

                  return (
                    <div
                      key={opt.key}
                      onClick={() => setSelectedPreset(opt.key)}
                      className={`cursor-pointer rounded-xl border p-3.5 transition-all ${
                        isSelected
                          ? 'border-indigo-600 bg-indigo-50/70 shadow-xs ring-1 ring-indigo-600'
                          : 'border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50'
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2 mb-1.5">
                        <span className={`text-xs font-bold ${isSelected ? 'text-indigo-900' : 'text-slate-900'}`}>
                          {opt.label}
                        </span>
                        <span className={`rounded-full px-2 py-0.5 text-[9px] font-bold uppercase tracking-wide ${
                          isNominal ? 'badge-emerald' : isAllowable ? 'badge-sky' : 'badge-rose'
                        }`}>
                          {opt.category}
                        </span>
                      </div>
                      <p className="text-xs text-slate-600 leading-snug">
                        {opt.description}
                      </p>
                    </div>
                  );
                })}
              </div>

              {/* Raw Record Preview */}
              <div className="space-y-1.5">
                <div className="flex items-center justify-between text-xs text-slate-600">
                  <span className="font-semibold">NACHA Payload Preview (94-char fixed-width alignment):</span>
                  <span className="font-mono text-indigo-600 font-bold text-xs">{previewFilename}</span>
                </div>
                <pre className="max-h-36 overflow-x-auto rounded-xl border border-slate-800 bg-slate-900 p-3 font-mono text-[11px] leading-relaxed text-slate-200">
                  {previewContent || 'Generating preset records...'}
                </pre>
              </div>
            </div>
          )}

          {activeTab === 'DROP' && (
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
              className={`flex cursor-pointer flex-col items-center justify-center rounded-2xl border-2 border-dashed p-10 text-center transition-all ${
                dragOver
                  ? 'border-indigo-600 bg-indigo-50/50'
                  : 'border-slate-300 bg-slate-50 hover:border-indigo-400 hover:bg-indigo-50/30'
              }`}
            >
              <input
                type="file"
                ref={fileInputRef}
                className="hidden"
                accept=".ach,.txt,.nacha"
                onChange={(e) => {
                  if (e.target.files && e.target.files[0]) {
                    handleFileDrop(e.target.files[0]);
                  }
                }}
              />
              <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-indigo-50 text-indigo-600 mb-3">
                <UploadCloud className="h-7 w-7" />
              </div>
              <p className="text-sm font-bold text-slate-900">Drop .ach / .txt NACHA file here</p>
              <p className="text-xs text-slate-500 mt-1">or click to browse your computer</p>
            </div>
          )}

          {activeTab === 'SFTP_INBOX' && (
            <div className="rounded-xl border border-slate-200 bg-slate-50 p-6 text-center space-y-2">
              <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600 mx-auto">
                <FolderPlus className="h-6 w-6" />
              </div>
              <h4 className="text-sm font-bold text-slate-900">SFTP Ingress Daemon</h4>
              <p className="text-xs text-slate-600 max-w-md mx-auto">
                SFTP uploads to the partner directory are ingested automatically via HMAC-SHA256 webhooks and reconciliation scanners.
              </p>
              <p className="font-mono text-xs font-semibold text-indigo-600 bg-white border border-slate-200 rounded-lg p-2 max-w-xs mx-auto">
                /app/inbox/partner-treasury/*.ach
              </p>
            </div>
          )}

          {/* Ingest Error / Success Message */}
          {ingestError && (
            <div className="flex items-start gap-2.5 rounded-xl border border-rose-200 bg-rose-50 p-3 text-xs text-rose-900">
              <ShieldAlert className="h-4 w-4 shrink-0 text-rose-600 mt-0.5" />
              <p>{ingestError}</p>
            </div>
          )}

          {ingestionResult && (
            <div className="flex items-start gap-2.5 rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-xs text-emerald-900">
              <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600 mt-0.5" />
              <div>
                <p className="font-bold">File successfully ingested (ID #{ingestionResult.fileId})</p>
                <p className="font-mono text-[11px] text-emerald-700 mt-0.5">SHA-256: {ingestionResult.hash}</p>
              </div>
            </div>
          )}
        </div>

        {/* Footer Actions */}
        <div className="flex items-center justify-between border-t border-slate-200 bg-slate-50 px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-slate-200 bg-white px-4 py-2 text-xs font-semibold text-slate-700 hover:bg-slate-100 shadow-2xs"
          >
            Close
          </button>

          {activeTab === 'PRESET' && (
            <button
              type="button"
              onClick={() => handleIngestContent(previewFilename, previewContent)}
              disabled={isProcessing || !previewContent}
              className="inline-flex items-center gap-2 rounded-lg bg-indigo-600 px-4 py-2 text-xs font-semibold text-white shadow-xs hover:bg-indigo-700 disabled:opacity-50 transition-colors"
            >
              {isProcessing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
              <span>{isProcessing ? 'Processing...' : 'Run Ingestion Scenario'}</span>
            </button>
          )}
        </div>
      </div>
    </div>
  );
};
