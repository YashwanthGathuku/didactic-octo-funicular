/**
 * Institutional Header Navigation Bar (Light Fintech Theme)
 * 
 * Clean, bright header with verified identity, live gateway readiness status,
 * and high-priority operator actions.
 */

import React from 'react';
import { Activity, Database, ShieldCheck, Layers } from 'lucide-react';
import { useSession } from '../state/SessionContext';

interface HeaderProps {
  onOpenUpload: () => void;
  onOpenConnectors: () => void;
}

export const Header: React.FC<HeaderProps> = ({ onOpenUpload, onOpenConnectors }) => {
  const { result, session, can } = useSession();

  const [dotClass, badgeClass, label] =
    result === null
      ? ['dot-pulse-amber', 'badge-slate', 'Probing Gateway…']
      : result.state === 'ok'
        ? ['dot-pulse-green', 'badge-emerald', `Gateway Live · ${session?.profile ?? 'local-demo'}`]
        : ['dot-pulse-red', 'badge-amber', 'Gateway Offline'];

  return (
    <header className="sticky top-0 z-40 border-b border-slate-200 bg-white/95 px-6 py-3.5 backdrop-blur-md transition-all shadow-xs">
      <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4">
        
        {/* Brand & System Status */}
        <div className="flex items-center gap-3.5">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-600 shadow-sm text-white">
            <ShieldCheck className="h-5 w-5" aria-hidden />
          </div>

          <div>
            <div className="flex items-center gap-2.5">
              <span className="text-base font-extrabold tracking-tight text-slate-900">
                SENTINEL<span className="text-indigo-600 font-semibold ml-1">FLOW</span>
              </span>
              
              <span className="inline-flex items-center gap-1 rounded-full border border-slate-200 bg-slate-100 px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-slate-700">
                <Layers className="h-3 w-3 text-indigo-600" />
                Pre-Ledger Gateway
              </span>

              <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-medium ${badgeClass}`} role="status">
                <span className={dotClass} aria-hidden />
                {label}
              </span>
            </div>

            <p className="text-[11px] text-slate-500 mt-0.5">
              Financial file reliability: scheduled expected feeds, zero-copy NACHA validation, dual-control release
            </p>
          </div>
        </div>

        {/* Global Action Toolbar */}
        <div className="flex items-center gap-2.5">
          <button
            type="button"
            onClick={onOpenConnectors}
            className="inline-flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3.5 py-2 text-xs font-semibold text-slate-700 shadow-xs transition-all hover:border-slate-300 hover:bg-slate-50"
          >
            <Database className="h-3.5 w-3.5 text-slate-500" aria-hidden />
            Source Connections
          </button>

          <button
            type="button"
            onClick={onOpenUpload}
            disabled={!can('artifact:upload')}
            title={can('artifact:upload') ? undefined : 'Uploading an artifact requires the operator role.'}
            className="inline-flex items-center gap-2 rounded-lg bg-indigo-600 px-4 py-2 text-xs font-semibold text-white shadow-xs transition-all hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-40"
          >
            <Activity className="h-3.5 w-3.5 text-indigo-200" aria-hidden />
            Ingest File
          </button>
        </div>

      </div>
    </header>
  );
};
