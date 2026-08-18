/**
 * The expected-feed status board.
 *
 * What is expected, when it is due in the partner's own zone, and whether it
 * arrived. Every field comes from the server; nothing here computes a deadline,
 * a breach, or a countdown. The board this replaces rendered BreachRiskPct and
 * CountdownMinutes assigned by an if-statement on status.
 */

import React, { useCallback, useMemo, useState } from 'react';
import { AlertOctagon, Check, Clock, HelpCircle, Pause } from 'lucide-react';
import { getBoard } from '../../api/endpoints';
import { usePagedList } from '../../state/usePagedList';
import type { Expectation, ExpectationStatus } from '../../api/types';
import { EmptyState, LoadingState, PartialBanner, ResultState } from './states';
import { Timestamp } from './Timestamp';

const STATUSES: Array<ExpectationStatus | ''> = [
  '',
  'PENDING',
  'DUE',
  'OVERDUE',
  'BREACHED',
  'ARRIVED',
  'WAIVED',
];

const statusStyle: Record<ExpectationStatus, { cls: string; Icon: React.ElementType }> = {
  PENDING: { cls: 'text-slate-300 border-slate-600', Icon: Clock },
  DUE: { cls: 'text-sky-300 border-sky-700', Icon: Clock },
  OVERDUE: { cls: 'text-amber-300 border-amber-700', Icon: AlertOctagon },
  BREACHED: { cls: 'text-rose-300 border-rose-700', Icon: AlertOctagon },
  ARRIVED: { cls: 'text-emerald-300 border-emerald-700', Icon: Check },
  WAIVED: { cls: 'text-slate-400 border-slate-700', Icon: Pause },
};

export const FeedBoard: React.FC = () => {
  const [status, setStatus] = useState<ExpectationStatus | ''>('');

  const fetchPage = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      getBoard({ status: status || undefined, cursor, limit: 50 }, signal),
    [status],
  );

  const list = usePagedList<Expectation>(fetchPage, `board:${status}`);
  const rows = useMemo(() => list.items, [list.items]);

  return (
    <section aria-labelledby="board-heading" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="board-heading" className="text-sm font-semibold uppercase tracking-wide text-slate-300">
          Expected feeds
        </h2>
        <label className="flex items-center gap-2 text-xs text-slate-400">
          Status
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value as ExpectationStatus | '')}
            className="rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-200"
          >
            {STATUSES.map((s) => (
              <option key={s || 'all'} value={s}>
                {s || 'All'}
              </option>
            ))}
          </select>
        </label>
      </div>

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="tenant:read" />}

      {list.loading && <LoadingState what="the feed board" />}

      {list.result?.state === 'ok' && rows.length === 0 && (
        <EmptyState
          title={status ? `No feeds are ${status.toLowerCase()}` : 'No feeds are expected'}
          detail={
            status
              ? 'The gateway answered; nothing matches this filter.'
              : 'Nothing has been materialized for the current horizon. If that is unexpected, check the scheduler on the health screen.'
          }
        />
      )}

      {rows.length > 0 && (
        <div className="overflow-x-auto rounded border border-slate-800">
          <table className="min-w-full text-left text-xs">
            <caption className="sr-only">
              Expected feeds, their contracted deadlines in the partner&apos;s time zone, and their
              current status.
            </caption>
            <thead className="bg-slate-900/80 text-slate-400">
              <tr>
                <th scope="col" className="px-3 py-2 font-medium">Partner / feed</th>
                <th scope="col" className="px-3 py-2 font-medium">Business date</th>
                <th scope="col" className="px-3 py-2 font-medium">Due</th>
                <th scope="col" className="px-3 py-2 font-medium">Breaches at</th>
                <th scope="col" className="px-3 py-2 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800">
              {rows.map((e) => {
                const s = statusStyle[e.status] ?? statusStyle.PENDING;
                return (
                  <tr key={e.id} className="hover:bg-slate-900/50">
                    <td className="px-3 py-2">
                      <div className="font-medium text-slate-200">{e.partnerName}</div>
                      <div className="text-[11px] text-slate-500">
                        {e.contractName}
                        {e.feedId ? ` · ${e.feedId}` : ''}
                      </div>
                      <div className="font-mono text-[10px] text-slate-600">{e.filenamePattern}</div>
                    </td>
                    <td className="px-3 py-2 tabular-nums text-slate-300">{e.businessDate ?? '—'}</td>
                    <td className="px-3 py-2 text-slate-300">
                      <Timestamp iso={e.deliveryEnd} zone={e.timezone} localLabel={e.dueLocal} />
                      {e.scheduleNote && (
                        /* Why this deadline is not simply the contracted time:
                           a calendar adjustment, a collision, or a DST
                           transition. Hiding it would make a correct deadline
                           look like a mistake. */
                        <div className="mt-0.5 flex items-start gap-1 text-[10px] text-amber-300/80">
                          <HelpCircle className="mt-px h-3 w-3 shrink-0" aria-hidden />
                          <span>{e.scheduleNote}</span>
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-2 text-slate-400">
                      <Timestamp iso={e.breachesAt} zone={e.timezone} />
                    </td>
                    <td className="px-3 py-2">
                      <span
                        className={`inline-flex items-center gap-1 rounded border px-2 py-0.5 text-[11px] font-medium ${s.cls}`}
                      >
                        <s.Icon className="h-3 w-3" aria-hidden />
                        {e.status}
                      </span>
                      {e.reviewRequired && (
                        <div className="mt-1 text-[10px] text-amber-300">Needs review</div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

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
