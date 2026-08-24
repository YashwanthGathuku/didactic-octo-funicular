/**
 * Evidence timeline for SentinelFlow's tamper-evident append-only SHA-256 chain.
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
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 id="evidence-heading" className="flex items-center gap-2 text-sm font-semibold text-slate-950">
            <ScrollText className="h-4 w-4 text-slate-500" aria-hidden />
            Tamper-evident audit evidence
          </h2>
          <p className="mt-0.5 text-xs text-slate-500">
            Append-only linear SHA-256 chain with deterministic sequence, predecessor, payload, and record-hash verification.
          </p>
        </div>

        <label className="flex items-center gap-2 text-xs font-medium text-slate-600">
          <span>Event type</span>
          <input
            value={eventType}
            onChange={(e) => setEventType(e.target.value)}
            placeholder="ARTIFACT_RECEIVED"
            className="rounded-md border border-slate-300 bg-white px-2.5 py-1.5 font-mono text-xs text-slate-800 shadow-xs focus:border-slate-500"
          />
        </label>
      </div>

      <div className="surface-panel overflow-hidden" aria-label="Evidence-chain integrity">
        {chain === null ? (
          <div className="px-4 py-3 text-xs font-medium text-slate-500">Verifying evidence-chain integrity…</div>
        ) : chain.state !== 'ok' ? (
          <div className="flex items-start gap-2 border-l-2 border-amber-500 bg-amber-50 px-4 py-3 text-xs text-amber-900" role="alert">
            <ShieldX className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" aria-hidden />
            <div>
              <p className="font-semibold">Integrity is unconfirmed.</p>
              <p className="mt-0.5">The verifier could not complete: {chain.error}</p>
            </div>
          </div>
        ) : chain.data.isChainValid ? (
          <div className="grid gap-3 px-4 py-3 sm:grid-cols-[1fr_auto] sm:items-center">
            <div className="flex items-start gap-2 text-xs text-slate-700">
              <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-emerald-700" aria-hidden />
              <div>
                <p className="font-semibold text-slate-900">Chain verified across {chain.data.totalEvents.toLocaleString()} events.</p>
                <p className="mt-0.5 text-slate-500">The current sequence and hash links are internally consistent at verification time.</p>
              </div>
            </div>
            <code className="rounded-md border border-slate-200 bg-slate-50 px-2 py-1 font-mono text-[10px] text-slate-600">
              head/{chain.data.lastEventHash.slice(0, 16)}…
            </code>
          </div>
        ) : (
          <div className="flex items-start gap-2 border-l-2 border-rose-600 bg-rose-50 px-4 py-3 text-xs text-rose-900" role="alert">
            <ShieldX className="mt-0.5 h-4 w-4 shrink-0 text-rose-700" aria-hidden />
            <div>
              <p className="font-semibold">Evidence-chain integrity mismatch detected.</p>
              <p className="mt-0.5">Sequence, predecessor linkage, payload hash, or record hash failed deterministic verification.</p>
            </div>
          </div>
        )}
      </div>

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="evidence:read" />}
      {list.loading && <LoadingState what="the evidence timeline" />}

      {list.result?.state === 'ok' && list.items.length === 0 && (
        <EmptyState
          title="No audit records match"
          detail="The gateway answered. No evidence entries match this event filter."
        />
      )}

      <ol className="surface-panel divide-y divide-slate-100 overflow-hidden">
        {list.items.map((e) => (
          <li key={e.id} className="px-4 py-3.5 transition-colors hover:bg-slate-50/70">
            <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
              <div className="min-w-0">
                <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                  <span className="font-mono text-xs font-semibold text-slate-950">{e.eventType}</span>
                  <span className="font-mono text-[10px] text-slate-400">sequence/{e.sequenceNo}</span>
                </div>
                <p className="mt-1 text-xs text-slate-600">
                  Actor <strong className="font-semibold text-slate-800">{e.actor}</strong>
                  {e.objectType ? ` · ${e.objectType} ${e.objectId ?? ''}` : ''}
                  {e.correlationId ? ` · ref ${e.correlationId}` : ''}
                </p>
                <div className="mt-2 flex min-w-0 items-center gap-1.5 overflow-x-auto rounded-md border border-slate-200 bg-slate-50 px-2 py-1.5 font-mono text-[10px] text-slate-500">
                  <Link2 className="h-3.5 w-3.5 shrink-0 text-slate-400" aria-hidden />
                  <span className="shrink-0">{e.previousHash.slice(0, 16)}…</span>
                  <span className="shrink-0 text-slate-300">→</span>
                  <span className="shrink-0 font-semibold text-slate-800">{e.currentHash.slice(0, 16)}…</span>
                </div>
                <details className="mt-2">
                  <summary className="cursor-pointer text-[11px] font-medium text-slate-600 hover:text-slate-950">
                    Inspect canonical hashed payload
                  </summary>
                  <pre className="mt-2 overflow-x-auto rounded-md border border-slate-800 bg-slate-950 p-3 font-mono text-[11px] leading-5 text-slate-200">
                    {JSON.stringify(e.payload, null, 2)}
                  </pre>
                </details>
              </div>
              <Timestamp iso={e.createdAt} />
            </div>
          </li>
        ))}
      </ol>

      {list.hasMore && (
        <button
          type="button"
          onClick={list.loadMore}
          disabled={list.loadingMore}
          className="secondary-action"
        >
          {list.loadingMore ? 'Loading…' : 'Load more evidence'}
        </button>
      )}
    </section>
  );
};
