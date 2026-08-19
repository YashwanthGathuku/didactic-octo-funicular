/**
 * Expected-Feed Status Board (Light Fintech Theme)
 * 
 * Tracks contracted feed expectations, deadlines, and arrival statuses.
 */

import React, { useCallback, useMemo, useState } from 'react';
import { AlertOctagon, Check, Clock, Pause, Filter, CalendarClock, ShieldCheck, Layers } from 'lucide-react';
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

export const FeedBoard: React.FC<FeedBoardProps> = () => {
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
      {/* Control Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 id="board-heading" className="text-sm font-bold tracking-tight text-slate-900 flex items-center gap-2">
            <CalendarClock className="h-4 w-4 text-indigo-600" />
            Scheduled Feed Expectations
          </h2>
          <p className="text-xs text-slate-500">
            Real-time feed tracking against contracted SLA windows and timezones
          </p>
        </div>

        <div className="flex items-center gap-2">
          <label className="flex items-center gap-2 text-xs font-medium text-slate-600">
            <Filter className="h-3.5 w-3.5 text-slate-400" />
            <span>Filter Status:</span>
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value as ExpectationStatus | '')}
              className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-800 shadow-xs focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            >
              {STATUSES.map((s) => (
                <option key={s || 'all'} value={s}>
                  {s ? s : 'All Statuses'}
                </option>
              ))}
            </select>
          </label>
        </div>
      </div>

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="tenant:read" />}
      {list.loading && <LoadingState what="the feed board" />}

      {/* Visual Empty State with Quick Starter Cards */}
      {list.result?.state === 'ok' && rows.length === 0 && (
        <div className="space-y-4">
          <div className="fintech-card flex flex-col items-center justify-center p-10 text-center bg-white border border-slate-200">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-indigo-50 text-indigo-600 border border-indigo-100 shadow-xs">
              <CalendarClock className="h-7 w-7" />
            </div>
            
            <h3 className="mt-4 text-base font-bold text-slate-900">
              {status ? `No feeds matching status "${status}"` : 'No Feeds Currently Expected'}
            </h3>
            
            <p className="mt-1.5 max-w-md text-xs text-slate-500 leading-relaxed">
              {status
                ? 'The gateway answered successfully, but no materialized feeds match this status filter.'
                : 'No feed expectations are materialized in the current 14-day rolling window. You can ingest a live payment batch or test validation scenarios directly.'}
            </p>
          </div>

          {/* Educational Feature Cards */}
          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
            <div className="fintech-card p-4 space-y-2 bg-white border border-slate-200">
              <div className="flex items-center gap-2 text-indigo-600 font-bold text-xs">
                <ShieldCheck className="h-4 w-4" />
                <span>Deterministic Ingest</span>
              </div>
              <p className="text-xs text-slate-600 leading-normal">
                Validates ABA Mod-10 check digits and entry hash totals at the byte level with zero heap copies.
              </p>
            </div>

            <div className="fintech-card p-4 space-y-2 bg-white border border-slate-200">
              <div className="flex items-center gap-2 text-emerald-600 font-bold text-xs">
                <Layers className="h-4 w-4" />
                <span>Dual-Control Governance</span>
              </div>
              <p className="text-xs text-slate-600 leading-normal">
                Quarantined payment anomalies require two independent authenticated reviewers before clearing release.
              </p>
            </div>

            <div className="fintech-card p-4 space-y-2 bg-white border border-slate-200">
              <div className="flex items-center gap-2 text-sky-600 font-bold text-xs">
                <Clock className="h-4 w-4" />
                <span>Linear Audit Hash Chain</span>
              </div>
              <p className="text-xs text-slate-600 leading-normal">
                Every state change and validation decision is recorded in an immutable, cryptographically hashed audit chain.
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Populated Feed Table */}
      {rows.length > 0 && (
        <div className="fintech-card overflow-hidden bg-white border border-slate-200">
          <div className="overflow-x-auto">
            <table className="min-w-full text-left text-xs">
              <caption className="sr-only">
                Expected feeds, deadlines in partner timezone, and arrival statuses.
              </caption>
              <thead className="border-b border-slate-200 bg-slate-50 text-[11px] font-semibold uppercase tracking-wider text-slate-600">
                <tr>
                  <th scope="col" className="px-4 py-3.5">Partner / Feed Contract</th>
                  <th scope="col" className="px-4 py-3.5">Business Date</th>
                  <th scope="col" className="px-4 py-3.5">Due Deadline</th>
                  <th scope="col" className="px-4 py-3.5">Breach Threshold</th>
                  <th scope="col" className="px-4 py-3.5">Arrival Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 text-slate-700">
                {rows.map((r) => {
                  const badge = statusBadge[r.status] || { cls: 'badge-slate', Icon: Clock };
                  const StatusIcon = badge.Icon;
                  return (
                    <tr key={r.id} className="transition-colors hover:bg-slate-50/80">
                      <td className="px-4 py-3.5">
                        <div className="font-semibold text-slate-900">{r.partnerName}</div>
                        <div className="font-mono text-[11px] text-slate-500">{r.contractName}</div>
                      </td>
                      <td className="px-4 py-3.5 font-mono text-slate-700">
                        {r.businessDate ?? '—'}
                      </td>
                      <td className="px-4 py-3.5 text-slate-600">
                        <Timestamp iso={r.deliveryEnd} zone={r.timezone} />
                      </td>
                      <td className="px-4 py-3.5 text-slate-600">
                        <Timestamp iso={r.breachesAt} zone={r.timezone} />
                      </td>
                      <td className="px-4 py-3.5">
                        <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[10px] font-bold tracking-wide uppercase ${badge.cls}`}>
                          <StatusIcon className="h-3 w-3" aria-hidden />
                          {r.status}
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
