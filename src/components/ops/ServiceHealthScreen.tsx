/**
 * Measured service health and queue state.
 *
 * Every number is read from database state at request time. There is no
 * constant here and there must not be one -- the screen this replaces reported
 * sentinel_worker_pool_active as the literal 8.
 *
 * The rule the layout enforces: only a component the server says it *measured*
 * can render green. NOT_CONFIGURED and UNKNOWN are their own colours, because a
 * dependency nobody configured is not healthy and one nobody probed is not
 * healthy either.
 */

import React, { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, CircleHelp, CircleSlash, Gauge, ServerCog } from 'lucide-react';
import { getServiceHealth } from '../../api/endpoints';
import type { ApiResult } from '../../api/client';
import type { ComponentHealth, ServiceHealth } from '../../api/types';
import { LoadingState, ResultState } from './states';
import { Age, Timestamp } from './Timestamp';

const tone: Record<ComponentHealth['status'], string> = {
  OK: 'border-emerald-700 text-emerald-300',
  DEGRADED: 'border-amber-700 text-amber-300',
  UNAVAILABLE: 'border-rose-700 text-rose-300',
  NOT_CONFIGURED: 'border-slate-700 text-slate-400',
  UNKNOWN: 'border-slate-600 text-slate-300',
};

const Component: React.FC<{ name: string; health: ComponentHealth }> = ({ name, health }) => (
  <div className={`rounded border bg-slate-900/50 p-3 ${tone[health.status]}`}>
    <div className="flex items-center justify-between gap-2">
      <span className="text-xs font-medium text-slate-300">{name}</span>
      <span className="text-[11px] font-semibold">{health.status}</span>
    </div>
    {health.detail && <p className="mt-1 text-[11px] text-slate-400">{health.detail}</p>}
    <div className="mt-2 flex items-center gap-2 text-[10px]">
      {health.measured ? (
        <span className="inline-flex items-center gap-1 text-slate-400">
          <Gauge className="h-3 w-3" aria-hidden />
          measured
          {health.latencyMs !== undefined && health.latencyMs > 0 && ` · ${health.latencyMs}ms`}
        </span>
      ) : (
        /* The distinction the whole screen exists for. An unmeasured
           component is never rendered as healthy, and it says why. */
        <span className="inline-flex items-center gap-1 text-slate-500">
          <CircleHelp className="h-3 w-3" aria-hidden />
          not measured — nothing probed this
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
    // Refreshed on a timer because queue depth is the one reading that is only
    // useful when it is current. Thirty seconds, not one: a health screen that
    // polls every second is a load generator pointed at the thing it measures.
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
        <h2 id="health-heading" className="text-sm font-semibold uppercase tracking-wide text-slate-300">
          Service health
        </h2>
        <p className="text-[11px] text-slate-500">
          As of <Timestamp iso={h.serverNow} /> · profile {h.profile}
        </p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <Component name="Database" health={h.database} />
        <Component name="Scheduler horizon" health={h.scheduler} />
        <Component name="Object store" health={h.objectStore} />
        <Component name="AI tier (optional)" health={h.aiTier} />
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <div className="rounded border border-slate-800 bg-slate-900/50 p-3">
          <h3 className="mb-2 flex items-center gap-2 text-xs font-semibold text-slate-300">
            <ServerCog className="h-4 w-4" aria-hidden />
            Validation queue
          </h3>
          {q === null ? (
            /* null is not zero. A queue whose depth could not be read must not
               render as an empty queue, which is what "0" would say. */
            <p className="flex items-center gap-2 text-xs text-amber-300">
              <CircleSlash className="h-3.5 w-3.5" aria-hidden />
              Queue depth could not be read. This is not the same as an empty queue.
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
                <div key={label} className="rounded bg-slate-950/60 px-2 py-1.5">
                  <dt className="text-[10px] uppercase tracking-wide text-slate-500">{label}</dt>
                  <dd
                    className={`tabular-nums text-base ${
                      label === 'Dead' && n > 0 ? 'text-rose-300' : 'text-slate-200'
                    }`}
                  >
                    {n}
                  </dd>
                </div>
              ))}
            </dl>
          )}
          {q?.oldestQueuedAgeSeconds !== undefined && q?.oldestQueuedAgeSeconds !== null && (
            <p className="mt-2 text-[11px] text-slate-400">
              Oldest waiting job: <Age seconds={q.oldestQueuedAgeSeconds} />. A shallow queue
              that is not moving is worse than a deep one that is.
            </p>
          )}
        </div>

        <div className="rounded border border-slate-800 bg-slate-900/50 p-3">
          <h3 className="mb-2 text-xs font-semibold text-slate-300">Outbox</h3>
          {o === null ? (
            <p className="text-xs text-amber-300">Outbox depth could not be read.</p>
          ) : (
            <>
              <dl className="grid grid-cols-2 gap-2 text-xs">
                <div className="rounded bg-slate-950/60 px-2 py-1.5">
                  <dt className="text-[10px] uppercase tracking-wide text-slate-500">Undelivered</dt>
                  <dd className="tabular-nums text-base text-slate-200">{o.undelivered}</dd>
                </div>
                <div className="rounded bg-slate-950/60 px-2 py-1.5">
                  <dt className="text-[10px] uppercase tracking-wide text-slate-500">Dead</dt>
                  <dd
                    className={`tabular-nums text-base ${o.dead > 0 ? 'text-rose-300' : 'text-slate-200'}`}
                  >
                    {o.dead}
                  </dd>
                </div>
              </dl>
              {o.undelivered > 0 && (
                /* The known last-mile gap, stated rather than left for an
                   operator to infer from a number that only ever grows. */
                <p className="mt-2 flex items-start gap-1.5 text-[11px] text-amber-200">
                  <AlertTriangle className="mt-px h-3.5 w-3.5 shrink-0" aria-hidden />
                  <span>
                    Events are published and nothing is subscribed to deliver them. Oldest:{' '}
                    <Age seconds={o.oldestUndeliveredAgeSeconds} />.
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
