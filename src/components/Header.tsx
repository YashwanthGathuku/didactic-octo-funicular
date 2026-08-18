/**
 * The application header.
 *
 * What it no longer asserts, and why each was a defect rather than a cosmetic
 * problem:
 *
 * Two green status pills reading "Go + Moov ACH (:8080)" and "Astra 2.0 AI
 * (:8000)" were rendered unconditionally. They probed nothing. A gateway that
 * was down and an AI tier that was never configured both showed as up, at the
 * top of every screen, which is the strongest possible claim in the least
 * checked place.
 *
 * "Next Clearing: 16:45 FedACH Window" was a constant. It was correct for no
 * tenant in particular and would be read as correct for whichever one was on
 * screen.
 *
 * The clock ticked from `new Date()` in the browser. The server's clock is on
 * the health screen and in every session response; the browser's is settable by
 * the person reading it, so it is not shown as an authority here.
 *
 * What remains is identity, the two server-backed tools that still have their
 * own surface, and a status pill whose colour is decided by a real probe.
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

  // Three states, and "we do not know yet" is one of them. Rendering unknown as
  // connected is the same error as the pills this replaces.
  const [tone, label] =
    result === null
      ? ['border-slate-700 text-slate-400', 'Contacting gateway…']
      : result.state === 'ok'
        ? ['border-emerald-700 text-emerald-300', `Gateway reachable · ${session?.profile ?? ''}`]
        : ['border-amber-700 text-amber-300', 'Gateway unreachable'];

  return (
    <header className="sticky top-0 z-40 flex flex-wrap items-center justify-between gap-3 border-b border-slate-800 bg-slate-950/95 px-6 py-3 backdrop-blur">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded bg-sky-700">
          <ShieldCheck className="h-5 w-5 text-white" aria-hidden />
        </div>
        <div>
          <div className="flex items-center gap-2">
            <span className="text-base font-bold tracking-tight text-slate-50">SENTINEL FLOW</span>
            <span className="rounded border border-sky-800 px-1.5 py-0.5 text-[10px] text-sky-300">
              Pre-ledger gateway
            </span>
            <span className={`rounded border px-1.5 py-0.5 text-[10px] ${tone}`} role="status">
              {label}
            </span>
          </div>
          <p className="text-[11px] text-slate-500">
            Financial file reliability: expected feeds, deterministic validation, dual-control
            release
          </p>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onOpenConnectors}
          className="inline-flex items-center gap-1.5 rounded border border-slate-700 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-800"
        >
          <Database className="h-3.5 w-3.5" aria-hidden />
          Source connections
        </button>
        <button
          type="button"
          onClick={onOpenUpload}
          disabled={!can('artifact:upload')}
          title={can('artifact:upload') ? undefined : 'Uploading an artifact requires the operator role.'}
          className="inline-flex items-center gap-1.5 rounded border border-sky-700 bg-sky-950/40 px-3 py-1.5 text-xs font-medium text-sky-200 hover:bg-sky-900/50 disabled:cursor-not-allowed disabled:opacity-40"
        >
          <Activity className="h-3.5 w-3.5" aria-hidden />
          Ingest a file
        </button>
      </div>
    </header>
  );
};
