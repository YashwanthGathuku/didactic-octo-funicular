/**
 * SentinelFlow operations console.
 *
 * The information architecture follows the operator journey rather than a
 * generic dashboard pattern: operate -> review -> evidence -> platform proof.
 */

import React, { useEffect, useMemo, useState } from 'react';
import {
  BadgeCheck,
  CalendarClock,
  FileSearch,
  FlaskConical,
  GitBranch,
  HeartPulse,
  Lock,
  ScrollText,
  ShieldAlert,
  ShieldQuestion,
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

type ScreenDefinition = {
  id: ScreenId;
  label: string;
  description: string;
  Icon: React.ElementType;
};

const SCREENS: Record<ScreenId, ScreenDefinition> = {
  board: {
    id: 'board',
    label: 'Feed Board',
    description: 'Expected-file arrivals, anomalies, quarantine state, and active operational work.',
    Icon: CalendarClock,
  },
  artifacts: {
    id: 'artifacts',
    label: 'Artifacts',
    description: 'Immutable originals, derived candidates, validation state, and release lineage.',
    Icon: FileSearch,
  },
  review: {
    id: 'review',
    label: 'Review Queue',
    description: 'Human investigation and dual-control authorization for verified derived artifacts.',
    Icon: ShieldQuestion,
  },
  evidence: {
    id: 'evidence',
    label: 'Audit Evidence',
    description: 'Tamper-evident event history and the evidence behind each governed decision.',
    Icon: ScrollText,
  },
  contracts: {
    id: 'contracts',
    label: 'Feed Contracts',
    description: 'Deterministic expectations for partner feeds, schedules, and operational obligations.',
    Icon: GitBranch,
  },
  health: {
    id: 'health',
    label: 'System Health',
    description: 'Gateway, workers, dependencies, and service readiness without financial payload exposure.',
    Icon: HeartPulse,
  },
  proof: {
    id: 'proof',
    label: 'Submission Proof',
    description: 'Authority boundaries and separately tracked live Google managed-service evidence.',
    Icon: BadgeCheck,
  },
};

const NAV_GROUPS: Array<{ label: string; screens: ScreenId[] }> = [
  { label: 'Operate', screens: ['board', 'artifacts', 'review'] },
  { label: 'Govern', screens: ['evidence', 'contracts'] },
  { label: 'Platform', screens: ['health'] },
  { label: 'Demo', screens: ['proof'] },
];

interface OperationsConsoleProps {
  onOpenUpload: () => void;
}

export const OperationsConsole: React.FC<OperationsConsoleProps> = ({ onOpenUpload }) => {
  const { result, session } = useSession();
  const [screen, setScreen] = useState<ScreenId>('board');
  const [stream, setStream] = useState<StreamState>({ state: 'connecting' });
  const [generation, setGeneration] = useState(0);

  useEffect(() => {
    if (!session) return;
    const sub = subscribe({
      onEvent: () => setGeneration((value) => value + 1),
      onState: setStream,
    });
    return () => sub.close();
  }, [session]);

  const body = useMemo(() => {
    switch (screen) {
      case 'board': return <FeedBoard key={generation} onNavigateToUpload={onOpenUpload} />;
      case 'artifacts': return <ArtifactsScreen key={generation} />;
      case 'review': return <ReviewQueue key={generation} />;
      case 'evidence': return <EvidenceTimeline key={generation} />;
      case 'contracts': return <ContractsScreen key={generation} />;
      case 'health': return <ServiceHealthScreen key={generation} />;
      case 'proof': return <SubmissionProofScreen />;
    }
  }, [screen, generation, onOpenUpload]);

  if (result === null) return <LoadingState what="your session" />;
  if (result.state !== 'ok') {
    return (
      <div className="mx-auto max-w-2xl p-6">
        <ResultState result={result} needs="tenant:read" />
      </div>
    );
  }

  const activeScreen = SCREENS[screen];

  return (
    <div className="mx-auto max-w-[1440px]">
      {session?.demo && <DemoProfileBanner profile={session.profile} tenant={session.tenantId} />}

      <div className="mt-4 grid gap-4 lg:grid-cols-[210px_minmax(0,1fr)] lg:gap-5">
        <aside className="surface-panel h-fit overflow-hidden lg:sticky lg:top-[76px]" aria-label="Operations navigation">
          <div className="border-b border-slate-200 px-3 py-3.5">
            <p className="text-xs font-semibold text-slate-950">Control room</p>
            <p className="mt-0.5 text-[11px] leading-4 text-slate-500">Financial truth stays outside model authority.</p>
          </div>

          <nav className="flex gap-1 overflow-x-auto p-2 lg:block lg:space-y-3" aria-label="Operations screens">
            {NAV_GROUPS.map((group) => (
              <div key={group.label} className="contents lg:block">
                <p className="hidden px-2 pb-1 text-[10px] font-semibold tracking-wide text-slate-400 lg:block">
                  {group.label}
                </p>
                <div className="flex gap-1 lg:block lg:space-y-0.5">
                  {group.screens.map((id) => {
                    const definition = SCREENS[id];
                    const active = screen === id;
                    const Icon = definition.Icon;
                    return (
                      <button
                        key={id}
                        type="button"
                        onClick={() => setScreen(id)}
                        aria-current={active ? 'page' : undefined}
                        className={`ops-nav-item ${active ? 'ops-nav-item-active' : ''}`}
                      >
                        <Icon className="h-4 w-4 shrink-0" aria-hidden />
                        <span className="whitespace-nowrap">{definition.label}</span>
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </nav>

          <div className="hidden border-t border-slate-200 px-3 py-3 text-[11px] leading-4 text-slate-500 lg:block">
            <p className="truncate">{session?.subject}</p>
            <p className="mt-0.5 truncate font-mono text-[10px] text-slate-400">tenant/{session?.tenantId}</p>
          </div>
        </aside>

        <section className="min-w-0 space-y-4" aria-labelledby="active-screen-heading">
          <header className="flex flex-wrap items-end justify-between gap-3 px-1 pb-1">
            <div className="max-w-3xl">
              <p className="text-[11px] font-medium text-slate-500">Operations / {activeScreen.label}</p>
              <h1 id="active-screen-heading" className="mt-1 text-[22px] font-bold tracking-[-0.03em] text-slate-950 sm:text-2xl">
                {activeScreen.label}
              </h1>
              <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-600">{activeScreen.description}</p>
            </div>
            <StreamIndicator state={stream} />
          </header>

          <AuthorityStrip tenant={session?.tenantId ?? 'default'} />

          <div className="min-w-0">{body}</div>
        </section>
      </div>

      <footer className="mt-5 flex flex-wrap items-center justify-between gap-2 border-t border-slate-200 px-1 pt-3 text-[11px] text-slate-500">
        <span>Signed in as <strong className="font-medium text-slate-700">{session?.subject}</strong></span>
        <span>Permissions: <span className="font-mono text-slate-600">{session?.roles.join(', ') || 'operator'}</span></span>
      </footer>
    </div>
  );
};

const AuthorityStrip: React.FC<{ tenant: string }> = ({ tenant }) => (
  <section className="control-strip" aria-label="SentinelFlow authority boundaries">
    <ControlFact icon={Lock} label="Tenant scope" value={tenant} mono />
    <ControlFact icon={GitBranch} label="Financial truth" value="Go validators + policy" />
    <ControlFact icon={ShieldAlert} label="Agent authority" value="Investigate + propose" />
    <ControlFact icon={BadgeCheck} label="Release authority" value="Dual human control" />
  </section>
);

const ControlFact: React.FC<{
  icon: React.ElementType;
  label: string;
  value: string;
  mono?: boolean;
}> = ({ icon: Icon, label, value, mono = false }) => (
  <div className="min-w-0 px-3 py-2.5 sm:px-4">
    <div className="flex items-center gap-2 text-[10px] font-medium text-slate-500">
      <Icon className="h-3.5 w-3.5 text-slate-400" aria-hidden />
      <span>{label}</span>
    </div>
    <p className={`mt-1 truncate text-xs font-semibold text-slate-800 ${mono ? 'font-mono' : ''}`}>{value}</p>
  </div>
);

const DemoProfileBanner: React.FC<{ profile: string; tenant: string }> = ({ profile, tenant }) => (
  <div className="demo-notice" role="note">
    <FlaskConical className="h-4 w-4 shrink-0 text-slate-500" aria-hidden />
    <p className="min-w-0 text-xs text-slate-600">
      <strong className="font-semibold text-slate-800">Development sandbox</strong>
      {' · '}{profile}{' · tenant/'}<span className="font-mono">{tenant}</span>
      <span className="hidden sm:inline"> · Managed-cloud badges remain NOT_RUN until real evidence is captured.</span>
    </p>
  </div>
);

const StreamIndicator: React.FC<{ state: StreamState }> = ({ state }) => {
  const [dotClass, text] = ((): [string, string] => {
    switch (state.state) {
      case 'connecting':
        return ['dot-pulse-amber', 'Connecting stream'];
      case 'open':
        return ['dot-pulse-green', `Live · event ${state.cursor}`];
      case 'idle':
        return ['dot-pulse-green', `Idle · event ${state.cursor}`];
      case 'reconnecting':
        return ['dot-pulse-amber', `Reconnecting · ${state.attempt}`];
      case 'denied':
        return ['dot-pulse-red', 'Stream denied'];
      case 'gap':
        return ['dot-pulse-amber', 'Stream gap'];
    }
  })();

  return (
    <div className="inline-flex items-center gap-2 text-xs font-medium text-slate-600" role="status" aria-live="polite">
      <span className={dotClass} aria-hidden />
      <span>{text}</span>
    </div>
  );
};
