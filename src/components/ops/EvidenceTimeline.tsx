/**
 * The evidence timeline.
 *
 * Two endpoints back this screen and they answer different questions. The paged
 * timeline says what happened; the whole-chain read says whether the record is
 * intact. They are separate because a page of a hash chain proves nothing about
 * the chain -- verification needs every link -- and a paged view that implied
 * verification would be the most consequential kind of false comfort in the
 * product.
 *
 * The chain status is therefore rendered as its own statement, from its own
 * request, and it is never inferred from a page loading successfully.
 */

import React, { useCallback, useEffect, useState } from 'react';
import { Link2, ShieldCheck, ShieldX } from 'lucide-react';
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
    <section aria-labelledby="evidence-heading" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="evidence-heading" className="text-sm font-semibold uppercase tracking-wide text-slate-300">
          Evidence timeline
        </h2>
        <label className="flex items-center gap-2 text-xs text-slate-400">
          <span className="sr-only">Filter by event type</span>
          <input
            value={eventType}
            onChange={(e) => setEventType(e.target.value)}
            placeholder="event type (exact)"
            className="rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-200"
          />
        </label>
      </div>

      {/* Chain integrity, stated separately from the list. */}
      <div className="rounded border border-slate-800 bg-slate-900/50 px-3 py-2">
        {chain === null ? (
          <p className="text-[11px] text-slate-400">Verifying the chain…</p>
        ) : chain.state !== 'ok' ? (
          <p className="flex items-center gap-1.5 text-[11px] text-amber-300" role="alert">
            <ShieldX className="h-3.5 w-3.5" aria-hidden />
            The chain could not be verified ({chain.error}). The entries below may still be
            listed; whether the record is intact is currently unknown.
          </p>
        ) : chain.data.isChainValid ? (
          <p className="flex items-center gap-1.5 text-[11px] text-emerald-300">
            <ShieldCheck className="h-3.5 w-3.5" aria-hidden />
            Chain verified over {chain.data.totalEvents.toLocaleString()} events. Head{' '}
            <span className="font-mono">{chain.data.lastEventHash.slice(0, 16)}…</span>
          </p>
        ) : (
          <p className="flex items-center gap-1.5 text-[11px] text-rose-300" role="alert">
            <ShieldX className="h-3.5 w-3.5" aria-hidden />
            The chain does not verify. A link is broken or a record was altered. Treat every
            entry below as unproven and escalate.
          </p>
        )}
      </div>

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="evidence:read" />}
      {list.loading && <LoadingState what="the evidence timeline" />}

      {list.result?.state === 'ok' && list.items.length === 0 && (
        <EmptyState
          title="No evidence entries match"
          detail="The gateway answered. Either nothing has happened yet, or nothing matches this event type."
        />
      )}

      <ol className="space-y-1.5">
        {list.items.map((e) => (
          <li key={e.id} className="rounded border border-slate-800 bg-slate-900/40 px-3 py-2">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <span className="font-mono text-xs text-slate-200">{e.eventType}</span>
              <span className="text-[10px] text-slate-500">
                #{e.sequenceNo} · <Timestamp iso={e.createdAt} />
              </span>
            </div>
            <p className="mt-0.5 text-[11px] text-slate-400">
              by {e.actor}
              {e.objectType ? ` · ${e.objectType} ${e.objectId ?? ''}` : ''}
              {e.correlationId ? ` · ${e.correlationId}` : ''}
            </p>
            <p className="mt-1 flex items-center gap-1 font-mono text-[10px] text-slate-600">
              <Link2 className="h-3 w-3" aria-hidden />
              {e.previousHash.slice(0, 12)}… → {e.currentHash.slice(0, 12)}…
            </p>
            <details className="mt-1">
              <summary className="cursor-pointer text-[10px] text-slate-500 hover:text-slate-300">
                payload
              </summary>
              {/* The canonical JSON that was hashed, passed through rather than
                  re-encoded: re-encoding would change the bytes the hash was
                  taken over and show a record that differs from the signed one. */}
              <pre className="mt-1 overflow-x-auto rounded bg-slate-950 p-2 text-[10px] text-slate-400">
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
          className="rounded border border-slate-700 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-800 disabled:opacity-50"
        >
          {list.loadingMore ? 'Loading…' : 'Load more'}
        </button>
      )}
    </section>
  );
};
