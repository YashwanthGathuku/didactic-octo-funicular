/**
 * Artifacts & Ingress Validation Screen (Light Fintech Theme)
 * 
 * Clean, structured inspection pane for deterministic NACHA verification results.
 */

import React, { useCallback, useEffect, useState } from 'react';
import { FileCode, Search, CheckCircle2, ShieldAlert, Copy, Check, Layers } from 'lucide-react';
import { getArtifact, getArtifacts } from '../../api/endpoints';
import { usePagedList } from '../../state/usePagedList';
import type { ApiResult } from '../../api/client';
import type { ArtifactDetail, ArtifactStatus, ArtifactSummary, Severity, Finding } from '../../api/types';
import { EmptyState, LoadingState, PartialBanner, ResultState } from './states';
import { Timestamp } from './Timestamp';

const STATUSES: Array<ArtifactStatus | ''> = [
  '', 'RECEIVED', 'VALIDATING', 'VALIDATED', 'QUARANTINED', 'APPROVED', 'RELEASED', 'REJECTED',
];

const statusBadge: Record<ArtifactStatus, { cls: string }> = {
  RECEIVED: { cls: 'badge-slate' },
  VALIDATING: { cls: 'badge-sky' },
  VALIDATED: { cls: 'badge-emerald' },
  QUARANTINED: { cls: 'badge-amber' },
  APPROVED: { cls: 'badge-emerald' },
  RELEASED: { cls: 'badge-emerald' },
  REJECTED: { cls: 'badge-rose' },
};

const severityStyle: Record<Severity, string> = {
  BLOCKING: 'border-rose-200 bg-rose-50 text-rose-900',
  WARNING: 'border-amber-200 bg-amber-50 text-amber-900',
  INFO: 'border-slate-200 bg-slate-50 text-slate-900',
};

export const ArtifactsScreen: React.FC<{ initialStatus?: ArtifactStatus }> = ({ initialStatus }) => {
  const [status, setStatus] = useState<ArtifactStatus | ''>(initialStatus ?? '');
  const [filename, setFilename] = useState('');
  const [debounced, setDebounced] = useState('');
  const [selected, setSelected] = useState<number | null>(null);

  useEffect(() => {
    const t = setTimeout(() => setDebounced(filename.trim()), 250);
    return () => clearTimeout(t);
  }, [filename]);

  const fetchPage = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      getArtifacts({ status: status || undefined, filename: debounced || undefined, cursor, limit: 25 }, signal),
    [status, debounced],
  );
  const list = usePagedList<ArtifactSummary>(fetchPage, `artifacts:${status}:${debounced}`);

  return (
    <section aria-labelledby="artifacts-heading" className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)]">
      {/* Left Column: Artifact List */}
      <div className="space-y-3.5">
        <div>
          <h2 id="artifacts-heading" className="text-sm font-bold tracking-tight text-slate-900">
            Ingested Artifacts
          </h2>
          <p className="text-xs text-slate-500">
            Immutable files received via API and SFTP webhooks
          </p>
        </div>

        {/* Filter Toolbar */}
        <div className="flex flex-wrap gap-2">
          <label className="flex items-center gap-1.5 text-xs text-slate-600">
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value as ArtifactStatus | '')}
              className="rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-800 shadow-xs focus:border-indigo-500 focus:outline-none"
            >
              {STATUSES.map((s) => (
                <option key={s || 'all'} value={s}>{s ? s : 'All Statuses'}</option>
              ))}
            </select>
          </label>

          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-slate-400" aria-hidden />
            <input
              value={filename}
              onChange={(e) => setFilename(e.target.value)}
              placeholder="Search filename..."
              className="w-full rounded-lg border border-slate-200 bg-white pl-8 pr-3 py-1.5 text-xs text-slate-800 shadow-xs focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
          </div>
        </div>

        {list.partial && <PartialBanner reason={list.partial} />}
        {list.result && <ResultState result={list.result} onRetry={list.reload} needs="tenant:read" />}
        {list.loading && <LoadingState what="artifacts" />}

        {list.result?.state === 'ok' && list.items.length === 0 && (
          <EmptyState
            title="No matching artifacts"
            detail="The gateway answered; no ingested artifacts match this filter."
          />
        )}

        <ul className="space-y-2">
          {list.items.map((a) => {
            const isSelected = selected === a.id;
            const badge = statusBadge[a.status] || { cls: 'badge-slate' };
            return (
              <li key={a.id}>
                <button
                  type="button"
                  onClick={() => setSelected(a.id)}
                  aria-current={isSelected ? 'true' : undefined}
                  className={`w-full text-left p-3.5 rounded-xl border transition-all ${
                    isSelected
                      ? 'border-indigo-500 bg-indigo-50/50 shadow-xs'
                      : 'border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50/80 shadow-xs'
                  }`}
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <FileCode className="h-4 w-4 shrink-0 text-indigo-600" />
                        <span className="font-semibold text-xs text-slate-900 truncate">{a.filename}</span>
                      </div>
                      <div className="mt-1 flex items-center gap-3 text-[11px] text-slate-500">
                        <span>{(a.sizeBytes / 1024).toFixed(1)} KB</span>
                        <span>·</span>
                        <Timestamp iso={a.receivedAt} />
                      </div>
                    </div>
                    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wide ${badge.cls}`}>
                      {a.status}
                    </span>
                  </div>
                </button>
              </li>
            );
          })}
        </ul>
      </div>

      {/* Right Column: Artifact Inspector */}
      <div>
        {selected === null ? (
          <div className="fintech-card flex h-full min-h-[300px] flex-col items-center justify-center p-8 text-center text-slate-400 bg-white border border-slate-200">
            <Layers className="h-8 w-8 text-slate-400 mb-2" />
            <p className="text-xs font-semibold text-slate-700">Select an artifact to inspect verification details</p>
            <p className="text-[11px] text-slate-500 mt-1 max-w-xs">
              View deterministic NACHA checksums, ABA check digits, evaluated rules, and validation findings.
            </p>
          </div>
        ) : (
          <ArtifactDetailPane id={selected} />
        )}
      </div>
    </section>
  );
};

const ArtifactDetailPane: React.FC<{ id: number }> = ({ id }) => {
  const [detail, setDetail] = useState<ApiResult<ArtifactDetail> | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    const ctrl = new AbortController();
    getArtifact(id, ctrl.signal).then(setDetail);
    return () => ctrl.abort();
  }, [id]);

  if (!detail) return <LoadingState what="artifact details" />;
  if (detail.state !== 'ok') return <ResultState result={detail} needs="tenant:read" />;

  const a = detail.data;
  const badge = statusBadge[a.status] || { cls: 'badge-slate' };

  const copyHash = () => {
    navigator.clipboard.writeText(a.sha256);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="fintech-card p-5 space-y-4 bg-white border border-slate-200">
      {/* Header */}
      <div className="flex items-start justify-between gap-3 border-b border-slate-200 pb-3.5">
        <div>
          <div className="flex items-center gap-2">
            <FileCode className="h-5 w-5 text-indigo-600" />
            <h3 className="text-sm font-bold text-slate-900">{a.filename}</h3>
          </div>
          <p className="font-mono text-[11px] text-slate-500 mt-1">ID #{a.id} · Received <Timestamp iso={a.receivedAt} /></p>
        </div>
        <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wide ${badge.cls}`}>
          {a.status}
        </span>
      </div>

      {/* Content Hash & Metadata */}
      <div className="space-y-1.5">
        <div className="text-[11px] font-bold uppercase tracking-wider text-slate-600">Cryptographic Digest (SHA-256)</div>
        <div className="flex items-center justify-between gap-2 rounded-lg border border-slate-200 bg-slate-50 p-2 font-mono text-[11px] text-slate-700">
          <span className="truncate font-semibold">{a.sha256}</span>
          <button
            type="button"
            onClick={copyHash}
            className="inline-flex items-center gap-1 rounded border border-slate-200 bg-white px-2 py-1 text-[10px] text-slate-700 hover:bg-slate-100 shadow-2xs"
          >
            {copied ? <Check className="h-3 w-3 text-emerald-600" /> : <Copy className="h-3 w-3" />}
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
      </div>

      {/* Validation Run Details */}
      {a.validationRun === null ? (
        <div className="rounded-xl border border-amber-200 bg-amber-50 p-3.5 text-xs text-amber-900">
          <ShieldAlert className="h-4 w-4 text-amber-600 mb-1" />
          <span className="font-bold">Validation Pending:</span> This artifact has not yet completed deterministic validation.
        </div>
      ) : (
        <div className="space-y-3 pt-2">
          <div className="flex items-center justify-between text-xs">
            <span className="font-bold text-slate-700">Deterministic Validator Verdict</span>
            <span className="font-mono text-slate-500">Outcome: <strong className="text-slate-900">{a.validationRun.outcome}</strong></span>
          </div>

          {a.findings.length === 0 ? (
            <div className="flex items-center gap-2 rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-xs text-emerald-900 font-medium">
              <CheckCircle2 className="h-4 w-4 text-emerald-600 shrink-0" />
              All structural checks passed (ABA Mod-10 check digits, batch balance sums, fixed record widths).
            </div>
          ) : (
            <div className="space-y-2">
              <div className="text-[11px] font-bold uppercase tracking-wider text-slate-600">
                Rule Findings ({a.findings.length})
              </div>
              <ul className="space-y-1.5">
                {a.findings.map((f: Finding) => (
                  <li key={f.id} className={`rounded-lg border p-2.5 text-xs ${severityStyle[f.severity] || severityStyle.INFO}`}>
                    <div className="flex items-center justify-between font-bold">
                      <span>Rule: {f.code}</span>
                      <span className="text-[10px] uppercase font-bold tracking-wider">{f.severity}</span>
                    </div>
                    <p className="mt-1 text-slate-800">{f.description}</p>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
