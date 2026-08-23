/**
 * Operations Console: Main Operations Dashboard (Light Theme)
 *
 * Provides unified segmented tab navigation, real-time KPI overview,
 * and live SSE connection status on a clean canvas.
 */

import React, { useEffect, useMemo, useState } from 'react';
import {
  BadgeCheck,
  CalendarClock,
  FileSearch,
  FlaskConical,
  GitBranch,
  HeartPulse,
  ScrollText,
  ShieldAlert,
  ShieldQuestion,
  CheckCircle2,
  Lock,
  Zap,
} from 'lucide-react';
import { useSession } from '../../state/SessionContext';
import { subscribe } from '../../api/stream';
import type { StreamState } from '../../api/stream';
import { FeedBoard } from './FeedBoard';
import { ArtifactsScreen } from './ArtifactsScreen';
import { ReviewQueue } from './ReviewQueue';
import { EvidenceTimeline } from './EvidenceTimeline';
import { ContractsScreen } from './ContractsScreen';
import { ServiceHealthScreen } from './ServiceHealthScreen';
import { SubmissionProofScreen } from './SubmissionProofScreen';
import { LoadingState, ResultState } from './states';

type ScreenId = 'board' | 'artifacts' | 'review' | 'evidence' | 'contracts' | 'health' | 'proof';

const SCREENS: Array<{ id: ScreenId; label: string; Icon: React.ElementType }> = [
  { id: 'board', label: 'Feed Board', Icon: CalendarClock },
  { id: 'artifacts', label: 'Artifacts', Icon: FileSearch },
  { id: 'review', label: 'Review Queue', Icon: ShieldQuestion },
  { id: 'evidence', label: 'Audit Evidence', Icon: ScrollText },
  { id: 'contracts', label: 'Feed Contracts', Icon: GitBranch },
  { id: 'health', label: 'System Health', Icon: HeartPulse },
  { id: 'proof', label: 'Submission Proof', Icon: BadgeCheck },
];

export const OperationsConsole: React.FC = () => {
  const { result, session } = useSession();
  const [screen, setScreen] = useState<ScreenId>('board');
  const [stream, setStream] = useState<StreamState>({ state: 'connecting' });
  const [generation, setGeneration] = useState(0);

  useEffect(() => {
    if (!session) return;
    const sub = subscribe({
      onEvent: () => setGeneration((g) => g + 1),
      onState: setStream,
    });
    return () => sub.close();
  }, [session]);

  const body = useMemo(() => {
    switch (screen) {
      case 'board': return <FeedBoard key={generation} onNavigateToUpload={() => {}} />;
      case 'artifacts': return <ArtifactsScreen key={generation} />;
      case 'review': return <ReviewQueue key={generation} />;
      case 'evidence': return <EvidenceTimeline key={generation} />;
      case 'contracts': return <ContractsScreen key={generation} />;
      case 'health': return <ServiceHealthScreen key={generation} />;
      case 'proof': return <SubmissionProofScreen />;
    }
  }, [screen, generation]);

  if (result === null) return <LoadingState what="your session" />;
  if (result.state !== 'ok') {
    return (
      <div className="mx-auto max-w-2xl p-6">
        <ResultState result={result} needs="tenant:read" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      {session?.demo && <DemoProfileBanner profile={session.profile} tenant={session.tenantId} />}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-500">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500">Active Tenant Scope</span>
            <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-indigo-50 text-indigo-600">
              <Lock className="h-4 w-4" />
            </div>
          </div>
          <p className="mt-2 font-mono text-lg font-bold text-slate-900 tracking-tight">{session?.tenantId ?? 'default'}</p>
          <div className="mt-2 flex items-center gap-1.5 text-xs text-slate-500">
            <span className="h-2 w-2 rounded-full bg-indigo-500" />
            <span>Row-level tenant isolation active</span>
          </div>
        </div>

        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-500">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500">Ingress Integrity</span>
            <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-emerald-50 text-emerald-600">
              <CheckCircle2 className="h-4 w-4" />
            </div>
          </div>
          <p className="mt-2 font-mono text-lg font-bold text-emerald-600 tracking-tight">100% Deterministic</p>
          <div className="mt-2 flex items-center gap-1.5 text-xs text-slate-500">
            <span className="h-2 w-2 rounded-full bg-emerald-500" />
            <span>Zero-copy NACHA fixed parser</span>
          </div>
        </div>

        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-500">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500">Audit Ledger Chain</span>
            <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-sky-50 text-sky-600">
              <Zap className="h-4 w-4" />
            </div>
          </div>
          <p className="mt-2 font-mono text-lg font-bold text-sky-600 tracking-tight">SHA-256 Linear</p>
          <div className="mt-2 flex items-center gap-1.5 text-xs text-slate-500">
            <span className="h-2 w-2 rounded-full bg-sky-500" />
            <span>Tamper-evident hash chaining</span>
          </div>
        </div>

        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-500">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500">Quarantine Governance</span>
            <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-amber-50 text-amber-600">
              <ShieldAlert className="h-4 w-4" />
            </div>
          </div>
          <p className="mt-2 font-mono text-lg font-bold text-amber-600 tracking-tight">Dual-Control</p>
          <div className="mt-2 flex items-center gap-1.5 text-xs text-slate-500">
            <span className="h-2 w-2 rounded-full bg-amber-500" />
            <span>Two-person release rule enforced</span>
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-3">
        <nav aria-label="Operations screens" className="flex flex-wrap gap-1 rounded-xl bg-slate-100/90 p-1 border border-slate-200/80">
          {SCREENS.map(({ id, label, Icon }) => {
            const active = screen === id;
            return (
              <button
                key={id}
                type="button"
                onClick={() => setScreen(id)}
                aria-current={active ? 'page' : undefined}
                className={`inline-flex items-center gap-2 rounded-lg px-3.5 py-2 text-xs font-semibold transition-all ${active ? 'nav-tab-active' : 'nav-tab-inactive'}`}
              >
                <Icon className={`h-4 w-4 ${active ? 'text-indigo-600' : 'text-slate-500'}`} aria-hidden />
                {label}
              </button>
            );
          })}
        </nav>

        <StreamIndicator state={stream} />
      </div>

      <main className="transition-all">{body}</main>

      <footer className="border-t border-slate-200 pt-4 text-xs text-slate-500 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span>Signed in as <strong className="text-slate-800 font-mono">{session?.subject}</strong></span>
          <span>·</span>
          <span>Tenant: <strong className="text-slate-800 font-mono">{session?.tenantId}</strong></span>
        </div>
        <div className="flex items-center gap-1.5">
          <span>Permissions:</span>
          <span className="font-mono text-slate-600">{session?.roles.join(', ') || 'operator'}</span>
        </div>
      </footer>
    </div>
  );
};

const DemoProfileBanner: React.FC<{ profile: string; tenant: string }> = ({ profile, tenant }) => (
  <div
    className="flex items-center justify-between gap-3 rounded-xl border border-indigo-200 bg-indigo-50/70 px-4 py-3 text-xs text-indigo-950 shadow-xs"
    role="note"
  >
    <div className="flex items-center gap-3">
      <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-100 text-indigo-700">
        <FlaskConical className="h-4 w-4" aria-hidden />
      </div>
      <div>
        <p className="font-semibold text-indigo-950">
          Development Sandbox Profile ({profile}) · Tenant: <span className="font-mono font-bold text-indigo-700">{tenant}</span>
        </p>
        <p className="text-[11px] text-indigo-700/80">All operational data is fetched from the configured gateway. Managed-cloud proof badges remain NOT_RUN until explicit live evidence is supplied.</p>
      </div>
    </div>

    <span className="inline-flex items-center rounded-md border border-indigo-200 bg-white px-2.5 py-1 text-[10px] font-bold uppercase tracking-wider text-indigo-700 shadow-2xs">
      Sandbox
    </span>
  </div>
);

const StreamIndicator: React.FC<{ state: StreamState }> = ({ state }) => {
  const [dotClass, text] = ((): [string, string] => {
    switch (state.state) {
      case 'connecting':
        return ['dot-pulse-amber', 'Connecting Live Stream…'];
      case 'open':
        return ['dot-pulse-green', `Live Stream Active (Event #${state.cursor})`];
      case 'idle':
        return ['dot-pulse-green', `Live Stream Idle (Event #${state.cursor})`];
      case 'reconnecting':
        return ['dot-pulse-amber', `Reconnecting (Attempt ${state.attempt})`];
      case 'denied':
        return ['dot-pulse-red', 'Live SSE stream denied'];
      case 'gap':
        return ['dot-pulse-amber', 'Event stream gap detected'];
    }
  })();

  return (
    <div className="inline-flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 shadow-2xs" role="status" aria-live="polite">
      <span className={dotClass} aria-hidden />
      <span>{text}</span>
    </div>
  );
};
