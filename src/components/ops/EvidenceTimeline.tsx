/**
 * The Evidence Timeline (Light Fintech Theme)
 * 
 * Verifiable, append-only cryptographic hash chain viewer.
 */

import React, { useCallback, useEffect, useState } from 'react';
import { Link2, ShieldCheck, ShieldX, ScrollText } from 'lucide-react';
import { getEvidence, verifyLedger } from '../../api/endpoints';
import { usePagedList } from '../../state/usePagedList';
import type { ApiResult } from '../../api/client';
import type { EvidenceEntry } from '../../api/types';
import { EmptyState, LoadingState, PartialBanner, ResultState } from './states';
import { Timestamp } from './Timestamp';

type ChainResult = ApiResult<{ totalEvents: number; isChainValid: boolean; lastEventHash: string }>;

export const EvidenceTimeline: React.FC = () => {
  const [eventType, setEventType] = useState('');
  const [chain, setChain] = useState<ChainResult | null>(null);

  const loadChain = useCallback(() => {
    setChain(null);
    void verifyLedger().then(setChain);
  }, []);
  useEffect(loadChain, [loadChain]);

  const fetchPage = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      getEvidence({ eventType: eventType || undefined, cursor, limit: 50 }, signal),
    [eventType],
  );
  const list = usePagedList<EvidenceEntry>(fetchPage, `evidence:${eventType}`);

  return (
    <section aria-labelledby="evidence-heading" className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 id="evidence-heading" className="text-sm font-bold tracking-tight text-slate-900 flex items-center gap-2">
            <ScrollText className="h-4 w-4 text-indigo-600" />
            Cryptographic Audit Evidence
          </h2>
          <p className="text-xs text-slate-500">
            Immutable SHA-256 linear hash-chained audit ledger
          </p>
        </div>

        <label className="flex items-center gap-2 text-xs text-slate-600">
          <span className="font-semibold">Filter Event:</span>
          <input
            value={eventType}
            onChange={(e) => setEventType(e.target.value)}
            placeholder="e.g. ARTIFACT_RECEIVED"
            className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs text-slate-800 shadow-xs focus:border-indigo-500 focus:outline-none"
          />
        </label>
      </div>

      {/* Chain Integrity Status Card */}
      <div className="rounded-xl border p-4 shadow-xs transition-all">
        {chain === null ? (
          <p className="text-xs font-semibold text-slate-500">Verifying cryptographic hash chain integrity…</p>
        ) : chain.state !== 'ok' ? (
          <div className="flex items-center gap-2 text-xs text-amber-900 bg-amber-50 border border-amber-200 p-3 rounded-lg" role="alert">
            <ShieldX className="h-4 w-4 text-amber-600 shrink-0" aria-hidden />
            <span>The chain could not be verified ({chain.error}). Integrity is currently unconfirmed.</span>
          </div>
        ) : chain.data.isChainValid ? (
          <div className="flex items-center justify-between gap-2 text-xs text-emerald-900 bg-emerald-50 border border-emerald-200 p-3 rounded-lg">
            <div className="flex items-center gap-2">
              <ShieldCheck className="h-4 w-4 text-emerald-600 shrink-0" aria-hidden />
              <span className="font-semibold">Chain 100% Intact & Verified over {chain.data.totalEvents.toLocaleString()} historical events.</span>
            </div>
            <span className="font-mono text-[11px] text-emerald-800 bg-emerald-100/60 px-2 py-0.5 rounded border border-emerald-300">
              Head: {chain.data.lastEventHash.slice(0, 16)}…
            </span>
          </div>
        ) : (
          <div className="flex items-center gap-2 text-xs text-rose-900 bg-rose-50 border border-rose-200 p-3 rounded-lg" role="alert">
            <ShieldX className="h-4 w-4 text-rose-600 shrink-0" aria-hidden />
            <span className="font-bold">Chain Verification Failed: A historical record has been modified or forked!</span>
          </div>
        )}
      </div>

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="evidence:read" />}
      {list.loading && <LoadingState what="the evidence timeline" />}

      {list.result?.state === 'ok' && list.items.length === 0 && (
        <EmptyState
          title="No audit records match"
          detail="The gateway answered. No ledger entries match this event filter."
        />
      )}

      <ol className="space-y-2">
        {list.items.map((e) => (
          <li key={e.id} className="fintech-card p-3.5 bg-white border border-slate-200">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <span className="font-mono text-xs font-bold text-slate-900">{e.eventType}</span>
              <span className="font-mono text-[11px] text-slate-500">
                Sequence #{e.sequenceNo} · <Timestamp iso={e.createdAt} />
              </span>
            </div>
            <p className="mt-0.5 text-xs text-slate-600">
              Actor: <strong className="text-slate-800">{e.actor}</strong>
              {e.objectType ? ` · Target: ${e.objectType} ${e.objectId ?? ''}` : ''}
              {e.correlationId ? ` · Ref: ${e.correlationId}` : ''}
            </p>
            <p className="mt-1.5 flex items-center gap-1.5 font-mono text-[11px] text-slate-500 bg-slate-50 p-1.5 rounded border border-slate-100">
              <Link2 className="h-3.5 w-3.5 text-indigo-600" aria-hidden />
              <span>{e.previousHash.slice(0, 16)}…</span>
              <span>→</span>
              <span className="font-bold text-slate-800">{e.currentHash.slice(0, 16)}…</span>
            </p>
            <details className="mt-2">
              <summary className="cursor-pointer text-[11px] font-semibold text-indigo-600 hover:text-indigo-800">
                View Canonical Hashed Payload
              </summary>
              <pre className="mt-1.5 overflow-x-auto rounded-xl border border-slate-800 bg-slate-900 p-3 font-mono text-[11px] text-slate-200">
                {JSON.stringify(e.payload, null, 2)}
              </pre>
            </details>
          </li>
        ))}
      </ol>

      {list.hasMore && (
        <button
          type="button"
          onClick={list.loadMore}
          disabled={list.loadingMore}
          className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 shadow-2xs"
        >
          {list.loadingMore ? 'Loading…' : 'Load more'}
        </button>
      )}
    </section>
  );
};
