/**
 * Expected-feed status board.
 *
 * Tracks contracted feed expectations, deadlines, and arrival statuses. Empty
 * state copy stays operational: it gives the operator a real next action rather
 * than turning the product surface into marketing feature cards.
 */

import React, { useCallback, useMemo, useState } from 'react';
import { Activity, AlertOctagon, CalendarClock, Check, Clock, Filter, Pause } from 'lucide-react';
import { getBoard } from '../../api/endpoints';
import { usePagedList } from '../../state/usePagedList';
import type { Expectation, ExpectationStatus } from '../../api/types';
import { LoadingState, PartialBanner, ResultState } from './states';
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

const statusBadge: Record<ExpectationStatus, { cls: string; Icon: React.ElementType }> = {
  PENDING: { cls: 'badge-slate', Icon: Clock },
  DUE: { cls: 'badge-sky', Icon: Clock },
  OVERDUE: { cls: 'badge-amber', Icon: AlertOctagon },
  BREACHED: { cls: 'badge-rose', Icon: AlertOctagon },
  ARRIVED: { cls: 'badge-emerald', Icon: Check },
  WAIVED: { cls: 'badge-slate', Icon: Pause },
};

interface FeedBoardProps {
  onNavigateToUpload?: () => void;
}

export const FeedBoard: React.FC<FeedBoardProps> = ({ onNavigateToUpload }) => {
  const [status, setStatus] = useState<ExpectationStatus | ''>('');

  const fetchPage = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      getBoard({ status: status || undefined, cursor, limit: 50 }, signal),
    [status],
  );

  const list = usePagedList<Expectation>(fetchPage, `board:${status}`);
  const rows = useMemo(() => list.items, [list.items]);

  return (
    <section aria-labelledby="board-heading" className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 id="board-heading" className="flex items-center gap-2 text-sm font-semibold text-slate-900">
            <CalendarClock className="h-4 w-4 text-slate-500" aria-hidden />
            Scheduled feed expectations
          </h2>
          <p className="mt-0.5 text-xs text-slate-500">Contract windows, partner-local deadlines, and materialized arrival state.</p>
        </div>

        <label className="flex items-center gap-2 text-xs font-medium text-slate-600">
          <Filter className="h-3.5 w-3.5 text-slate-400" aria-hidden />
          <span className="sr-only sm:not-sr-only">Status</span>
          <select
            value={status}
            onChange={(event) => setStatus(event.target.value as ExpectationStatus | '')}
            className="rounded-md border border-slate-300 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-800 shadow-xs focus:border-slate-500"
          >
            {STATUSES.map((item) => (
              <option key={item || 'all'} value={item}>
                {item || 'All statuses'}
              </option>
            ))}
          </select>
        </label>
      </div>

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="tenant:read" />}
      {list.loading && <LoadingState what="the feed board" />}

      {list.result?.state === 'ok' && rows.length === 0 && (
        <div className="surface-panel overflow-hidden">
          <div className="grid gap-0 lg:grid-cols-[minmax(0,1fr)_340px]">
            <div className="p-6 sm:p-8">
              <div className="flex h-9 w-9 items-center justify-center rounded-md border border-slate-200 bg-slate-50 text-slate-600">
                <CalendarClock className="h-[18px] w-[18px]" aria-hidden />
              </div>
              <h3 className="mt-4 text-base font-semibold text-slate-950">
                {status ? `No feeds with status ${status}` : 'No feeds currently expected'}
              </h3>
              <p className="mt-1.5 max-w-xl text-sm leading-6 text-slate-600">
                {status
                  ? 'The gateway answered successfully, but no materialized expectations match this status filter.'
                  : 'No feed expectations are materialized in the current rolling window. You can still ingest a payment file to exercise deterministic validation and the governed remediation path.'}
              </p>
              {!status && onNavigateToUpload && (
                <button type="button" onClick={onNavigateToUpload} className="primary-action mt-5">
                  <Activity className="h-3.5 w-3.5" aria-hidden />
                  Ingest a file
                </button>
              )}
            </div>

            <aside className="border-t border-slate-200 bg-slate-50/70 p-5 lg:border-l lg:border-t-0" aria-label="Control path summary">
              <p className="text-xs font-semibold text-slate-800">What happens after ingest</p>
              <ol className="mt-3 space-y-3 text-xs leading-5 text-slate-600">
                <li><strong className="font-semibold text-slate-800">1 · Validate.</strong> Go establishes file and control-total truth before agent reasoning.</li>
                <li><strong className="font-semibold text-slate-800">2 · Investigate.</strong> The bounded agent fleet may explain findings and propose allowlisted remediation intent.</li>
                <li><strong className="font-semibold text-slate-800">3 · Verify.</strong> Derived candidates are independently re-read and deterministically validated.</li>
                <li><strong className="font-semibold text-slate-800">4 · Authorize.</strong> Release remains behind identity-bound dual control and a tamper-evident append-only evidence chain.</li>
              </ol>
            </aside>
          </div>
        </div>
      )}

      {rows.length > 0 && (
        <div className="surface-panel overflow-hidden">
          <div className="overflow-x-auto">
            <table className="min-w-full text-left text-xs">
              <caption className="sr-only">Expected feeds, deadlines in partner timezone, and arrival statuses.</caption>
              <thead className="border-b border-slate-200 bg-slate-50 text-[11px] font-semibold text-slate-600">
                <tr>
                  <th scope="col" className="px-4 py-3">Partner / feed contract</th>
                  <th scope="col" className="px-4 py-3">Business date</th>
                  <th scope="col" className="px-4 py-3">Due deadline</th>
                  <th scope="col" className="px-4 py-3">Breach threshold</th>
                  <th scope="col" className="px-4 py-3">Arrival status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 text-slate-700">
                {rows.map((row) => {
                  const badge = statusBadge[row.status] || { cls: 'badge-slate', Icon: Clock };
                  const StatusIcon = badge.Icon;
                  return (
                    <tr key={row.id} className="transition-colors hover:bg-slate-50/80">
                      <td className="px-4 py-3.5">
                        <div className="font-semibold text-slate-900">{row.partnerName}</div>
                        <div className="font-mono text-[11px] text-slate-500">{row.contractName}</div>
                      </td>
                      <td className="px-4 py-3.5 font-mono text-slate-700">{row.businessDate ?? '—'}</td>
                      <td className="px-4 py-3.5 text-slate-600"><Timestamp iso={row.deliveryEnd} zone={row.timezone} /></td>
                      <td className="px-4 py-3.5 text-slate-600"><Timestamp iso={row.breachesAt} zone={row.timezone} /></td>
                      <td className="px-4 py-3.5">
                        <span className={`inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[10px] font-semibold ${badge.cls}`}>
                          <StatusIcon className="h-3 w-3" aria-hidden />
                          {row.status}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
};
