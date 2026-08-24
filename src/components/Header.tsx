/**
 * Global SentinelFlow header.
 *
 * The header is intentionally quiet: product identity, trusted runtime status,
 * and the two global operator actions. Screen navigation lives with the
 * operations console instead of competing with these actions.
 */

import React from 'react';
import { Activity, Database, ShieldCheck } from 'lucide-react';
import { useSession } from '../state/SessionContext';

interface HeaderProps {
  onOpenUpload: () => void;
  onOpenConnectors: () => void;
}

export const Header: React.FC<HeaderProps> = ({ onOpenUpload, onOpenConnectors }) => {
  const { result, session, can } = useSession();

  const [dotClass, statusLabel] =
    result === null
      ? ['dot-pulse-amber', 'Probing gateway']
      : result.state === 'ok'
        ? ['dot-pulse-green', `Gateway live · ${session?.profile ?? 'local-demo'}`]
        : ['dot-pulse-red', 'Gateway offline'];

  return (
    <header className="sticky top-0 z-40 border-b border-slate-200 bg-white/96 backdrop-blur-md">
      <div className="mx-auto flex max-w-[1440px] flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-6 lg:px-8">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-slate-300 bg-slate-950 text-white shadow-xs">
            <ShieldCheck className="h-[18px] w-[18px]" aria-hidden />
          </div>

          <div className="min-w-0">
            <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span className="text-[15px] font-extrabold tracking-[-0.035em] text-slate-950">
                SENTINEL<span className="font-medium text-slate-500">FLOW</span>
              </span>
              <span className="hidden text-xs font-medium text-slate-500 sm:inline">Pre-ledger financial control plane</span>
            </div>
            <div className="mt-0.5 flex items-center gap-2 text-[11px] text-slate-500" role="status" aria-live="polite">
              <span className={dotClass} aria-hidden />
              <span>{statusLabel}</span>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onOpenConnectors}
            className="secondary-action"
          >
            <Database className="h-3.5 w-3.5" aria-hidden />
            <span className="hidden sm:inline">Connections</span>
          </button>

          <button
            type="button"
            onClick={onOpenUpload}
            disabled={!can('artifact:upload')}
            title={can('artifact:upload') ? undefined : 'Uploading an artifact requires the operator role.'}
            className="primary-action"
          >
            <Activity className="h-3.5 w-3.5" aria-hidden />
            <span>Ingest file</span>
          </button>
        </div>
      </div>
    </header>
  );
};
