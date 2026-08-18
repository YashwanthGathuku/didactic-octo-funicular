/**
 * Feed contracts and their version history.
 *
 * The version list is the point of this screen. A contract row carries one
 * mutable set of terms; a version is what governed a particular business date.
 * An operator asking "why did this file breach at 14:30" needs the version that
 * was in effect then, not the terms somebody edited afterwards -- and without
 * the history, the answer the screen gives is confidently wrong.
 *
 * `current` is decided by the server against its own clock. Recomputing it here
 * from the browser's clock would let this screen disagree with the scheduler
 * that actually used the version.
 */

import React, { useCallback, useEffect, useState } from 'react';
import { GitBranch } from 'lucide-react';
import { getContractVersions, getContracts } from '../../api/endpoints';
import { useSession } from '../../state/SessionContext';
import type { ApiResult } from '../../api/client';
import type { Contract, ContractVersion } from '../../api/types';
import { EmptyState, LoadingState, ResultState } from './states';
import { Timestamp } from './Timestamp';

export const ContractsScreen: React.FC = () => {
  const { can } = useSession();
  const [result, setResult] = useState<ApiResult<Contract[]> | null>(null);
  const [selected, setSelected] = useState<number | string | null>(null);

  // Normalised once, here, rather than trusted from the wire.
  //
  // The server is fixed to send [] rather than null for an empty list, and this
  // is still here: a list endpoint that regresses to null must degrade to "no
  // contracts" rather than unmount the application. A screen that crashes tells
  // the operator nothing; a screen that says "none" is at worst slightly wrong.
  const contracts = result?.state === 'ok' && Array.isArray(result.data) ? result.data : [];

  const load = useCallback(() => {
    setResult(null);
    void getContracts().then(setResult);
  }, []);
  useEffect(load, [load]);

  return (
    <section aria-labelledby="contracts-heading" className="grid gap-4 lg:grid-cols-2">
      <div className="space-y-3">
        <div className="flex items-baseline justify-between gap-2">
          <h2 id="contracts-heading" className="text-sm font-semibold uppercase tracking-wide text-slate-300">
            Feed contracts
          </h2>
          {!can('contract:manage') && (
            /* Read access without edit access is a normal, valid state and it
               is said plainly, rather than by showing edit controls that fail. */
            <span className="text-[10px] text-slate-500">read-only — contract:manage not held</span>
          )}
        </div>

        {result === null && <LoadingState what="contracts" />}
        {result && <ResultState result={result} onRetry={load} needs="tenant:read" />}

        {result?.state === 'ok' && contracts.length === 0 && (
          <EmptyState
            title="No contracts are configured"
            detail="Nothing is expected from any partner until a contract exists, so the feed board will stay empty."
          />
        )}

        <ul className="space-y-1.5">
          {contracts.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => setSelected(c.id)}
                  aria-current={selected === c.id ? 'true' : undefined}
                  className={`w-full rounded border px-3 py-2 text-left ${
                    selected === c.id
                      ? 'border-sky-700 bg-sky-950/30'
                      : 'border-slate-800 bg-slate-900/40 hover:border-slate-700'
                  }`}
                >
                  <div className="text-xs font-medium text-slate-200">{c.name}</div>
                  <div className="mt-0.5 font-mono text-[10px] text-slate-500">
                    {c.filenamePattern}
                  </div>
                  <div className="mt-0.5 text-[10px] text-slate-500">
                    {c.expectedTime} {c.timezone} · grace {c.gracePeriodMinutes}m · {c.format}
                  </div>
                </button>
              </li>
          ))}
        </ul>
      </div>

      <VersionHistory contractId={selected} />
    </section>
  );
};

const VersionHistory: React.FC<{ contractId: number | string | null }> = ({ contractId }) => {
  const [result, setResult] =
    useState<ApiResult<{ contractId: string; versions: ContractVersion[] }> | null>(null);

  const load = useCallback(() => {
    if (contractId === null) return;
    setResult(null);
    void getContractVersions(contractId).then(setResult);
  }, [contractId]);
  useEffect(load, [load]);

  if (contractId === null) {
    return (
      <div className="rounded border border-dashed border-slate-800 p-6 text-center text-xs text-slate-500">
        Select a contract to see the versions that have governed it.
      </div>
    );
  }
  if (result === null) return <LoadingState what="version history" />;
  if (result.state !== 'ok') return <ResultState result={result} onRetry={load} needs="tenant:read" />;

  const versions = result.data.versions;

  return (
    <div className="space-y-2">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-slate-300">
        <GitBranch className="h-4 w-4" aria-hidden />
        Version history
      </h3>

      {versions.length === 0 ? (
        <EmptyState
          title="This contract has no versions"
          detail="Nothing has been materialized under it, so scheduled expectations will fall back to the contract row's own terms."
        />
      ) : (
        <ol className="space-y-1.5">
          {versions.map((v) => (
            <li
              key={v.id}
              className={`rounded border px-3 py-2 ${
                v.current ? 'border-emerald-800 bg-emerald-950/20' : 'border-slate-800 bg-slate-900/40'
              }`}
            >
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-xs font-medium text-slate-200">Version {v.version}</span>
                {v.current && (
                  <span className="rounded border border-emerald-700 px-1.5 py-0.5 text-[10px] text-emerald-300">
                    in effect
                  </span>
                )}
              </div>
              <p className="mt-0.5 font-mono text-[10px] text-slate-500">{v.filenamePattern}</p>
              <p className="mt-0.5 text-[11px] text-slate-400">
                {v.expectedLocal} {v.timezone} · grace {v.graceMinutes}m · {v.format} ·{' '}
                {v.balancedMode}
                {v.calendarId ? ` · calendar ${v.calendarId}` : ''}
              </p>
              <p className="mt-0.5 text-[10px] text-slate-500">
                effective <Timestamp iso={v.effectiveFrom} />
                {v.effectiveTo ? (
                  <>
                    {' '}until <Timestamp iso={v.effectiveTo} />
                  </>
                ) : (
                  ' onward'
                )}
              </p>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
};
