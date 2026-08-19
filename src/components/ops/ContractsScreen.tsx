/**
 * Feed Contracts & Version History (Light Fintech Theme)
 * 
 * Manages partner file expectations, business calendars, and versioned SLA terms.
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

  const contracts = result?.state === 'ok' && Array.isArray(result.data) ? result.data : [];

  const load = useCallback(() => {
    setResult(null);
    void getContracts().then(setResult);
  }, []);
  useEffect(load, [load]);

  return (
    <section aria-labelledby="contracts-heading" className="grid gap-5 lg:grid-cols-2">
      <div className="space-y-3.5">
        <div className="flex items-baseline justify-between gap-2">
          <h2 id="contracts-heading" className="text-sm font-bold tracking-tight text-slate-900 flex items-center gap-2">
            <GitBranch className="h-4 w-4 text-indigo-600" />
            Feed Contracts
          </h2>
          {!can('contract:manage') && (
            <span className="text-[11px] text-slate-500 font-medium">Read-Only (contract:manage required)</span>
          )}
        </div>

        {result === null && <LoadingState what="contracts" />}
        {result && <ResultState result={result} onRetry={load} needs="tenant:read" />}

        {result?.state === 'ok' && contracts.length === 0 && (
          <EmptyState
            title="No contracts configured"
            detail="Nothing is expected from any partner until a feed contract exists."
          />
        )}

        <ul className="space-y-2">
          {contracts.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                onClick={() => setSelected(c.id)}
                aria-current={selected === c.id ? 'true' : undefined}
                className={`w-full rounded-xl border p-3.5 text-left transition-all ${
                  selected === c.id
                    ? 'border-indigo-500 bg-indigo-50/50 shadow-xs'
                    : 'border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50 shadow-xs'
                }`}
              >
                <div className="text-xs font-bold text-slate-900">{c.name}</div>
                <div className="mt-0.5 font-mono text-[11px] text-slate-500">
                  Pattern: {c.filenamePattern}
                </div>
                <div className="mt-1 text-xs text-slate-600">
                  {c.expectedTime} {c.timezone} · Grace: {c.gracePeriodMinutes}m · {c.format}
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
      <div className="fintech-card flex h-full min-h-[250px] flex-col items-center justify-center p-8 text-center text-slate-400 bg-white border border-slate-200">
        <GitBranch className="h-8 w-8 text-slate-400 mb-2" />
        <p className="text-xs font-semibold text-slate-700">Select a feed contract</p>
        <p className="text-[11px] text-slate-500 mt-1 max-w-xs">
          View immutable version history and governing rules for that contract.
        </p>
      </div>
    );
  }
  if (result === null) return <LoadingState what="version history" />;
  if (result.state !== 'ok') return <ResultState result={result} onRetry={load} needs="tenant:read" />;

  const versions = result.data.versions;

  return (
    <div className="space-y-3">
      <h3 className="flex items-center gap-2 text-sm font-bold text-slate-900">
        <GitBranch className="h-4 w-4 text-indigo-600" aria-hidden />
        Version History
      </h3>

      {versions.length === 0 ? (
        <EmptyState
          title="This contract has no past versions"
          detail="Nothing has been materialized under it yet; expectations will derive from the contract row."
        />
      ) : (
        <ol className="space-y-2">
          {versions.map((v) => (
            <li
              key={v.id}
              className={`rounded-xl border p-3.5 transition-all ${
                v.current
                  ? 'border-emerald-200 bg-emerald-50 text-emerald-950 shadow-xs'
                  : 'border-slate-200 bg-white text-slate-900 shadow-xs'
              }`}
            >
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-xs font-bold">Version {v.version}</span>
                {v.current && (
                  <span className="rounded-full border border-emerald-300 bg-emerald-100 px-2 py-0.5 text-[10px] font-bold uppercase text-emerald-800">
                    Active in Effect
                  </span>
                )}
              </div>
              <p className="mt-1 font-mono text-[11px] text-slate-600">{v.filenamePattern}</p>
              <p className="mt-0.5 text-xs text-slate-600">
                {v.expectedLocal} {v.timezone} · Grace: {v.graceMinutes}m · {v.format} · {v.balancedMode}
              </p>
              <p className="mt-1 text-[11px] text-slate-500 font-mono">
                Effective from <Timestamp iso={v.effectiveFrom} />
                {v.effectiveTo ? <> until <Timestamp iso={v.effectiveTo} /></> : ' onward'}
              </p>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
};
