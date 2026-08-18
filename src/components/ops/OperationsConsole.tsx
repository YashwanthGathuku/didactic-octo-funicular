/**
 * The operations console: the six server-backed screens and the live stream.
 *
 * Two things live at this level because they are properties of the whole
 * application rather than of any one screen.
 *
 * **Demo mode is a visible profile.** The server says whether it is a demo
 * build and the banner is not dismissible. Every screen underneath shows real
 * server state -- there is no mock path anywhere in this tree -- but a demo
 * deployment's data is a demo tenant's data, and an operator has to be able to
 * tell at a glance which one they are looking at. That was the original defect:
 * a build that presented simulated state as verified infrastructure behaviour.
 *
 * **The stream is one connection, shared.** Six screens each opening their own
 * would be six cursors to keep in step. A screen reacts to events by reloading
 * itself, not by patching its rows from an event payload: patching means the
 * screen's state is derived from two sources that must agree, and when they
 * stop agreeing the screen shows something no server ever said.
 */

import React, { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  CalendarClock,
  FileSearch,
  FlaskConical,
  GitBranch,
  HeartPulse,
  Radio,
  ScrollText,
  ShieldQuestion,
  WifiOff,
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
import { LoadingState, ResultState } from './states';

type ScreenId = 'board' | 'artifacts' | 'review' | 'evidence' | 'contracts' | 'health';

const SCREENS: Array<{ id: ScreenId; label: string; Icon: React.ElementType }> = [
  { id: 'board', label: 'Feed board', Icon: CalendarClock },
  { id: 'artifacts', label: 'Artifacts', Icon: FileSearch },
  { id: 'review', label: 'Review queue', Icon: ShieldQuestion },
  { id: 'evidence', label: 'Evidence', Icon: ScrollText },
  { id: 'contracts', label: 'Contracts', Icon: GitBranch },
  { id: 'health', label: 'Health', Icon: HeartPulse },
];

export const OperationsConsole: React.FC = () => {
  const { result, session } = useSession();
  const [screen, setScreen] = useState<ScreenId>('board');
  const [stream, setStream] = useState<StreamState>({ state: 'connecting' });
  // Bumped on every event, and used as a React key so the active screen
  // remounts and re-reads. Coarse on purpose: correctness over cleverness,
  // since every screen already handles its own loading state.
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
      case 'board': return <FeedBoard key={generation} />;
      case 'artifacts': return <ArtifactsScreen key={generation} />;
      case 'review': return <ReviewQueue key={generation} />;
      case 'evidence': return <EvidenceTimeline key={generation} />;
      case 'contracts': return <ContractsScreen key={generation} />;
      case 'health': return <ServiceHealthScreen key={generation} />;
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
    <div className="space-y-4">
      {session?.demo && <DemoProfileBanner profile={session.profile} tenant={session.tenantId} />}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <nav aria-label="Operations screens" className="flex flex-wrap gap-1">
          {SCREENS.map(({ id, label, Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => setScreen(id)}
              aria-current={screen === id ? 'page' : undefined}
              className={`inline-flex items-center gap-1.5 rounded border px-3 py-1.5 text-xs font-medium ${
                screen === id
                  ? 'border-sky-700 bg-sky-950/40 text-sky-200'
                  : 'border-slate-800 text-slate-400 hover:border-slate-700 hover:text-slate-200'
              }`}
            >
              <Icon className="h-3.5 w-3.5" aria-hidden />
              {label}
            </button>
          ))}
        </nav>
        <StreamIndicator state={stream} />
      </div>

      <main>{body}</main>

      <footer className="border-t border-slate-800 pt-2 text-[10px] text-slate-600">
        Signed in as {session?.subject} · tenant {session?.tenantId} · roles{' '}
        {session?.roles.join(', ') || 'none'}
      </footer>
    </div>
  );
};

/**
 * Not dismissible, and it names the profile the server reported.
 *
 * A demo build that could be made to look like production by closing a banner
 * is a demo build that will eventually be mistaken for production.
 */
const DemoProfileBanner: React.FC<{ profile: string; tenant: string }> = ({ profile, tenant }) => (
  <div
    className="flex items-start gap-2 rounded border border-fuchsia-700 bg-fuchsia-950/40 px-3 py-2"
    role="note"
  >
    <FlaskConical className="mt-0.5 h-4 w-4 shrink-0 text-fuchsia-300" aria-hidden />
    <p className="text-xs text-fuchsia-100">
      <span className="font-semibold">Demo profile ({profile}).</span> Every value on these
      screens is read from this gateway and nothing is simulated — but this is the demo
      tenant ({tenant}) with a named demo principal, reachable on loopback only. It is not a
      production deployment and must not be read as one.
    </p>
  </div>
);

/**
 * The live connection's state, always visible.
 *
 * "Connected and quiet" and "the connection died twenty minutes ago" look
 * identical on a screen that shows neither, and the second one means every
 * number on the page is stale without saying so.
 */
const StreamIndicator: React.FC<{ state: StreamState }> = ({ state }) => {
  const [tone, Icon, text] = ((): [string, React.ElementType, string] => {
    switch (state.state) {
      case 'connecting':
        return ['text-slate-400', Radio, 'Connecting to live updates…'];
      case 'open':
        return ['text-emerald-300', Activity, `Live · event ${state.cursor}`];
      case 'idle':
        return ['text-slate-400', Radio, `Live · quiet since event ${state.cursor}`];
      case 'reconnecting':
        return ['text-amber-300', WifiOff, `Reconnecting (attempt ${state.attempt}) — this view may be stale`];
      case 'denied':
        return ['text-rose-300', WifiOff, 'Live updates are not permitted for this account'];
      case 'gap':
        return ['text-amber-300', WifiOff, 'Events were missed — reload to get a complete view'];
    }
  })();

  return (
    <p className={`inline-flex items-center gap-1.5 text-[11px] ${tone}`} role="status" aria-live="polite">
      <Icon className="h-3.5 w-3.5" aria-hidden />
      {text}
    </p>
  );
};
