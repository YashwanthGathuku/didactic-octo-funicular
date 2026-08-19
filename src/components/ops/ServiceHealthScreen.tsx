/**
 * Measured Service Health & Queue State (Light Fintech Theme)
 * 
 * Real-time instrumentation of worker pools, database latencies, and outbox backlogs.
 */

import React, { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, CircleHelp, CircleSlash, Gauge, ServerCog, HeartPulse } from 'lucide-react';
import { getServiceHealth } from '../../api/endpoints';
import type { ApiResult } from '../../api/client';
import type { ComponentHealth, ServiceHealth } from '../../api/types';
import { LoadingState, ResultState } from './states';
import { Age, Timestamp } from './Timestamp';

const tone: Record<ComponentHealth['status'], string> = {
  OK: 'border-emerald-200 bg-emerald-50 text-emerald-950',
  DEGRADED: 'border-amber-200 bg-amber-50 text-amber-950',
  UNAVAILABLE: 'border-rose-200 bg-rose-50 text-rose-950',
  NOT_CONFIGURED: 'border-slate-200 bg-slate-50 text-slate-700',
  UNKNOWN: 'border-slate-200 bg-slate-50 text-slate-700',
};

const statusPill: Record<ComponentHealth['status'], string> = {
  OK: 'badge-emerald',
  DEGRADED: 'badge-amber',
  UNAVAILABLE: 'badge-rose',
  NOT_CONFIGURED: 'badge-slate',
  UNKNOWN: 'badge-slate',
};

const Component: React.FC<{ name: string; health: ComponentHealth }> = ({ name, health }) => (
  <div className={`rounded-xl border p-4 shadow-xs transition-all ${tone[health.status]}`}>
    <div className="flex items-center justify-between gap-2">
      <span className="text-xs font-bold">{name}</span>
      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wider ${statusPill[health.status]}`}>
        {health.status}
      </span>
    </div>
    {health.detail && <p className="mt-1 text-xs text-slate-600">{health.detail}</p>}
    <div className="mt-3 flex items-center gap-2 text-xs font-medium">
      {health.measured ? (
        <span className="inline-flex items-center gap-1 text-slate-600">
          <Gauge className="h-3.5 w-3.5 text-indigo-600" aria-hidden />
          Measured latency
          {health.latencyMs !== undefined && health.latencyMs > 0 && `: ${health.latencyMs}ms`}
        </span>
      ) : (
        <span className="inline-flex items-center gap-1 text-slate-500 text-[11px]">
          <CircleHelp className="h-3.5 w-3.5" aria-hidden />
          Not configured / Unprobed
        </span>
      )}
    </div>
  </div>
);

export const ServiceHealthScreen: React.FC = () => {
  const [result, setResult] = useState<ApiResult<ServiceHealth> | null>(null);

  const load = useCallback(() => {
    setResult(null);
    void getServiceHealth().then(setResult);
  }, []);

  useEffect(() => {
    load();
    const t = setInterval(load, 30_000);
    return () => clearInterval(t);
  }, [load]);

  if (result === null) return <LoadingState what="service health" />;
  if (result.state !== 'ok') return <ResultState result={result} onRetry={load} needs="tenant:read" />;

  const h = result.data;
  const q = h.queue;
  const o = h.outbox;

  return (
    <section aria-labelledby="health-heading" className="space-y-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="health-heading" className="text-sm font-bold tracking-tight text-slate-900 flex items-center gap-2">
          <HeartPulse className="h-4 w-4 text-indigo-600" />
          Service Health & Runtime Instrumentation
        </h2>
        <p className="text-xs text-slate-500">
          Measured at <Timestamp iso={h.serverNow} /> · Profile: <strong className="font-mono text-slate-700">{h.profile}</strong>
        </p>
      </div>

      <div className="grid gap-3.5 sm:grid-cols-2 lg:grid-cols-4">
        <Component name="Database Engine" health={h.database} />
        <Component name="Scheduler Horizon" health={h.scheduler} />
        <Component name="Object Storage" health={h.objectStore} />
        <Component name="Python AI Tier" health={h.aiTier} />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {/* Worker Queue Card */}
        <div className="fintech-card p-4 bg-white border border-slate-200">
          <h3 className="mb-3 flex items-center gap-2 text-xs font-bold text-slate-900">
            <ServerCog className="h-4 w-4 text-indigo-600" aria-hidden />
            Asynchronous Worker Pool
          </h3>
          {q === null ? (
            <p className="flex items-center gap-2 text-xs text-amber-800 bg-amber-50 p-3 rounded-lg">
              <CircleSlash className="h-4 w-4 text-amber-600" aria-hidden />
              Queue depth could not be read from database state.
            </p>
          ) : (
            <dl className="grid grid-cols-3 gap-2 text-xs sm:grid-cols-5">
              {(
                [
                  ['Queued', q.queued],
                  ['Leased', q.leased],
                  ['Running', q.running],
                  ['Retrying', q.retryable],
                  ['Dead', q.dead],
                ] as const
              ).map(([label, n]) => (
                <div key={label} className="rounded-xl bg-slate-50 border border-slate-100 p-2 text-center">
                  <dt className="text-[10px] font-bold uppercase tracking-wider text-slate-500">{label}</dt>
                  <dd
                    className={`tabular-nums text-base font-bold mt-0.5 ${
                      label === 'Dead' && n > 0 ? 'text-rose-600' : 'text-slate-900'
                    }`}
                  >
                    {n}
                  </dd>
                </div>
              ))}
            </dl>
          )}
          {q?.oldestQueuedAgeSeconds !== undefined && q?.oldestQueuedAgeSeconds !== null && (
            <p className="mt-3 text-xs text-slate-500">
              Oldest in-flight job: <Age seconds={q.oldestQueuedAgeSeconds} />
            </p>
          )}
        </div>

        {/* Outbox Card */}
        <div className="fintech-card p-4 bg-white border border-slate-200">
          <h3 className="mb-3 text-xs font-bold text-slate-900">Transactional Outbox Dispatch</h3>
          {o === null ? (
            <p className="text-xs text-amber-800 bg-amber-50 p-3 rounded-lg">Outbox depth could not be read.</p>
          ) : (
            <>
              <dl className="grid grid-cols-2 gap-2 text-xs">
                <div className="rounded-xl bg-slate-50 border border-slate-100 p-2 text-center">
                  <dt className="text-[10px] font-bold uppercase tracking-wider text-slate-500">Undelivered Events</dt>
                  <dd className="tabular-nums text-base font-bold text-slate-900 mt-0.5">{o.undelivered}</dd>
                </div>
                <div className="rounded-xl bg-slate-50 border border-slate-100 p-2 text-center">
                  <dt className="text-[10px] font-bold uppercase tracking-wider text-slate-500">Dead-Letter</dt>
                  <dd
                    className={`tabular-nums text-base font-bold mt-0.5 ${o.dead > 0 ? 'text-rose-600' : 'text-slate-900'}`}
                  >
                    {o.dead}
                  </dd>
                </div>
              </dl>
              {o.undelivered > 0 && (
                <p className="mt-3 flex items-start gap-1.5 text-xs text-amber-900 bg-amber-50 p-2.5 rounded-lg border border-amber-200">
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-600" aria-hidden />
                  <span>
                    Events queued in outbox. Oldest undelivered: <Age seconds={o.oldestUndeliveredAgeSeconds} />.
                  </span>
                </p>
              )}
            </>
          )}
        </div>
      </div>
    </section>
  );
};
