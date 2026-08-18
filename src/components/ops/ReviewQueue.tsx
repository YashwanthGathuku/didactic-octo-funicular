/**
 * The quarantine and review queue: where a person decides whether to release.
 *
 * Three things this screen must get right, and each of them is a requirement
 * because getting it wrong is invisible.
 *
 * A consequential action is confirmed, and the confirmation restates what is
 * about to happen in the system's own terms rather than asking "are you sure?".
 * "Are you sure" is answered yes by reflex.
 *
 * A concurrency conflict is shown as a conflict. The server refuses a vote or a
 * release against a stale integrity digest and names what changed; the screen
 * repeats that and re-reads, because retrying the same write would either fail
 * again or -- worse -- succeed against a decision that is no longer the one the
 * reviewer looked at.
 *
 * Controls the caller may not use are rendered as unavailable with the reason,
 * not hidden and not offered. Hiding them makes the product look like it lacks
 * the feature; offering them produces a refusal that reads as a bug.
 */

import React, { useCallback, useState } from 'react';
import { CheckCircle2, ShieldAlert, XCircle } from 'lucide-react';
import {
  getReviewQueue,
  overrideRelease,
  releaseDecision,
  voteOnDecision,
} from '../../api/endpoints';
import { usePagedList } from '../../state/usePagedList';
import { useSession } from '../../state/SessionContext';
import type { ApiResult } from '../../api/client';
import type { Decision } from '../../api/types';
import { EmptyState, LoadingState, PartialBanner, ResultState } from './states';
import { Timestamp } from './Timestamp';
import { ConfirmDialog } from './ConfirmDialog';

/** The minimum the server enforces on an override justification. */
const MIN_OVERRIDE_REASON = 20;

type PendingAction =
  | { kind: 'approve' | 'reject'; decision: Decision }
  | { kind: 'release'; decision: Decision }
  | { kind: 'override'; decision: Decision };

export const ReviewQueue: React.FC = () => {
  const { can } = useSession();
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [outcome, setOutcome] = useState<ApiResult<Decision> | null>(null);
  const [busy, setBusy] = useState(false);

  const fetchPage = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      getReviewQueue({ cursor, limit: 25 }, signal),
    [],
  );
  const list = usePagedList<Decision>(fetchPage, 'review-queue');

  const run = useCallback(
    async (action: PendingAction, reason: string) => {
      setBusy(true);
      let r: ApiResult<Decision>;
      switch (action.kind) {
        case 'approve':
          r = await voteOnDecision(action.decision.id, true, reason);
          break;
        case 'reject':
          r = await voteOnDecision(action.decision.id, false, reason);
          break;
        case 'release':
          r = await releaseDecision(action.decision.id);
          break;
        case 'override':
          r = await overrideRelease(action.decision.id, reason);
          break;
      }
      setBusy(false);
      setOutcome(r);
      setPending(null);
      // Re-read on every outcome, success or conflict. After a conflict the
      // local copy is known stale; after a success it is stale too, because the
      // decision's row version and vote list both moved.
      list.reload();
    },
    [list],
  );

  const approvalsHeld = (d: Decision): number =>
    new Set(
      d.votes
        .filter((v) => v.choice === 'APPROVE')
        .map((v) => v.actorId),
    ).size;

  return (
    <section aria-labelledby="queue-heading" className="space-y-3">
      <h2 id="queue-heading" className="text-sm font-semibold uppercase tracking-wide text-slate-300">
        Review queue
      </h2>

      {outcome && outcome.state !== 'ok' && (
        <div
          className="rounded border border-amber-700/60 bg-amber-950/30 px-3 py-2 text-xs text-amber-100"
          role="alert"
        >
          <p className="font-semibold">
            {outcome.state === 'conflict'
              ? 'This decision changed while you were looking at it.'
              : 'That action was refused.'}
          </p>
          <p className="mt-0.5">
            {outcome.error}
            {outcome.state === 'conflict' && outcome.detail ? ` — ${outcome.detail}` : ''}
          </p>
          {outcome.state === 'conflict' && (
            <p className="mt-1 text-amber-200/80">
              The queue has been re-read. Review the decision as it now stands before acting
              again — the previous action was not applied.
            </p>
          )}
        </div>
      )}
      {outcome?.state === 'ok' && (
        <p className="rounded border border-emerald-800 bg-emerald-950/30 px-3 py-2 text-xs text-emerald-200" role="status">
          Recorded. The decision now reads {outcome.data.state}.
        </p>
      )}

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="tenant:read" />}
      {list.loading && <LoadingState what="the review queue" />}

      {list.result?.state === 'ok' && list.items.length === 0 && (
        <EmptyState
          title="Nothing is waiting for a decision"
          detail="The gateway answered and the queue is empty. No artifact is currently proposed for release."
        />
      )}

      <ul className="space-y-3">
        {list.items.map((d) => {
          const held = approvalsHeld(d);
          const rejected = d.votes.some((v) => v.choice === 'REJECT');
          const met = held >= d.requiredApprovals && !rejected;
          return (
            <li key={d.id} className="rounded border border-slate-800 bg-slate-900/50 p-3">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-sm font-medium text-slate-200">
                    Decision {d.id} · artifact {d.artifactId}
                  </h3>
                  <p className="mt-0.5 font-mono text-[10px] text-slate-500">
                    sha256 {d.artifactSha256.slice(0, 16)}…
                  </p>
                  <p className="mt-1 text-[11px] text-slate-400">
                    Proposed by {d.proposedBy} at <Timestamp iso={d.proposedAt} />
                  </p>
                  <p className="text-[11px] text-slate-500">
                    policy {d.policyVersion} · rules {d.rulePackVersion} · outcome {d.outcome}
                  </p>
                </div>
                <div className="text-right">
                  <span className="rounded border border-slate-600 px-2 py-0.5 text-[11px] text-slate-300">
                    {d.state}
                  </span>
                  <p className="mt-1 text-[11px] text-slate-400">
                    {held} of {d.requiredApprovals} approvals
                    {d.separationOfDuties ? ' · separation of duties on' : ''}
                  </p>
                  {rejected && (
                    /* One rejection is enough. A reviewer who examined the
                       file and refused it is not outvoted. */
                    <p className="mt-0.5 text-[11px] text-rose-300">Rejected — a release requires agreement</p>
                  )}
                </div>
              </div>

              {d.votes.length > 0 && (
                <ul className="mt-2 space-y-1 border-t border-slate-800 pt-2">
                  {d.votes.map((v, i) => (
                    <li key={`${v.actorId}-${i}`} className="text-[11px] text-slate-400">
                      <span className={v.choice === 'APPROVE' ? 'text-emerald-300' : 'text-rose-300'}>
                        {v.choice}
                      </span>{' '}
                      by {v.actorId} ({v.role}) — {v.reason || 'no reason recorded'} ·{' '}
                      <Timestamp iso={v.at} />
                    </li>
                  ))}
                </ul>
              )}

              <div className="mt-3 flex flex-wrap gap-2">
                <ActionButton
                  label="Approve"
                  Icon={CheckCircle2}
                  tone="emerald"
                  allowed={can('release:approve')}
                  deniedReason="Approving a release requires the reviewer or release supervisor role."
                  onClick={() => { setOutcome(null); setPending({ kind: 'approve', decision: d }); }}
                />
                <ActionButton
                  label="Reject"
                  Icon={XCircle}
                  tone="rose"
                  allowed={can('release:approve')}
                  deniedReason="Rejecting a release requires the reviewer or release supervisor role."
                  onClick={() => { setOutcome(null); setPending({ kind: 'reject', decision: d }); }}
                />
                <ActionButton
                  label="Release"
                  Icon={CheckCircle2}
                  tone="sky"
                  allowed={can('release:approve') && met}
                  deniedReason={
                    !can('release:approve')
                      ? 'Releasing requires the reviewer or release supervisor role.'
                      : rejected
                        ? 'This decision was rejected. A rejection is not an unmet control; it is a person saying no.'
                        : `The approval threshold is not met (${held} of ${d.requiredApprovals}).`
                  }
                  onClick={() => { setOutcome(null); setPending({ kind: 'release', decision: d }); }}
                />
                <ActionButton
                  label="Override"
                  Icon={ShieldAlert}
                  tone="amber"
                  allowed={can('release:override')}
                  deniedReason="Overriding dual control requires the release supervisor role, which this account does not hold."
                  onClick={() => { setOutcome(null); setPending({ kind: 'override', decision: d }); }}
                />
              </div>
            </li>
          );
        })}
      </ul>

      {list.hasMore && (
        <button
          type="button"
          onClick={list.loadMore}
          disabled={list.loadingMore}
          className="rounded border border-slate-700 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-800 disabled:opacity-50"
        >
          {list.loadingMore ? 'Loading…' : 'Load more'}
        </button>
      )}

      {pending && (
        <ConfirmDialog
          title={confirmTitle(pending)}
          body={confirmBody(pending)}
          confirmLabel={confirmLabel(pending)}
          destructive={pending.kind === 'override' || pending.kind === 'release'}
          requireReason={pending.kind !== 'release'}
          minReasonLength={pending.kind === 'override' ? MIN_OVERRIDE_REASON : 1}
          reasonHint={
            pending.kind === 'override'
              ? `At least ${MIN_OVERRIDE_REASON} characters. This is recorded in a separately reportable register with your name and role, and an auditor will read it.`
              : 'Recorded against your verified identity on the decision.'
          }
          busy={busy}
          onCancel={() => setPending(null)}
          onConfirm={(reason) => void run(pending, reason)}
        />
      )}
    </section>
  );
};

function confirmTitle(a: PendingAction): string {
  switch (a.kind) {
    case 'approve': return `Approve decision ${a.decision.id}?`;
    case 'reject': return `Reject decision ${a.decision.id}?`;
    case 'release': return `Release artifact ${a.decision.artifactId}?`;
    case 'override': return `Override dual control on decision ${a.decision.id}?`;
  }
}

/**
 * The confirmation restates the consequence in the system's own terms.
 *
 * "Are you sure?" is answered yes by reflex. "This releases artifact 41 for
 * delivery" is answered by reading it.
 */
function confirmBody(a: PendingAction): string {
  const held = new Set(a.decision.votes.filter((v) => v.choice === 'APPROVE').map((v) => v.actorId)).size;
  switch (a.kind) {
    case 'approve':
      return `Records your approval of artifact ${a.decision.artifactId} against the current integrity digest. It will count towards the ${a.decision.requiredApprovals} required, and it stops counting if the artifact, findings, policy, rule pack, contract or validation run change.`;
    case 'reject':
      return `Records your rejection of artifact ${a.decision.artifactId}. One rejection is enough: this decision cannot then be released by approvals, and an override cannot bypass it.`;
    case 'release':
      return `Releases artifact ${a.decision.artifactId} with ${held} of ${a.decision.requiredApprovals} approvals. The artifact walks VALIDATED → APPROVED → RELEASED and both edges are recorded. This is not reversible from this screen.`;
    case 'override':
      return `Releases artifact ${a.decision.artifactId} past the controls with ${held} of ${a.decision.requiredApprovals} approvals. The validation findings are not changed — they stay exactly as the validator wrote them. Your identity, role, what was bypassed and your reason are written to the override register.`;
  }
}

function confirmLabel(a: PendingAction): string {
  return { approve: 'Approve', reject: 'Reject', release: 'Release', override: 'Override and release' }[a.kind];
}

const toneClasses: Record<string, string> = {
  emerald: 'border-emerald-700 text-emerald-300 hover:bg-emerald-950/40',
  rose: 'border-rose-700 text-rose-300 hover:bg-rose-950/40',
  sky: 'border-sky-700 text-sky-300 hover:bg-sky-950/40',
  amber: 'border-amber-700 text-amber-300 hover:bg-amber-950/40',
};

/**
 * A control the caller may not use is disabled and says why.
 *
 * `title` and `aria-describedby` both carry the reason, so it reaches a mouse
 * user and a screen-reader user alike. A disabled button with no explanation is
 * indistinguishable from a broken one.
 */
const ActionButton: React.FC<{
  label: string;
  Icon: React.ElementType;
  tone: keyof typeof toneClasses;
  allowed: boolean;
  deniedReason: string;
  onClick(): void;
}> = ({ label, Icon, tone, allowed, deniedReason, onClick }) => {
  const id = `why-${label.toLowerCase()}-${React.useId()}`;
  return (
    <span className="inline-flex flex-col">
      <button
        type="button"
        onClick={onClick}
        disabled={!allowed}
        title={allowed ? undefined : deniedReason}
        aria-describedby={allowed ? undefined : id}
        className={`inline-flex items-center gap-1.5 rounded border px-3 py-1.5 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-40 ${toneClasses[tone]}`}
      >
        <Icon className="h-3.5 w-3.5" aria-hidden />
        {label}
      </button>
      {!allowed && (
        <span id={id} className="sr-only">
          {deniedReason}
        </span>
      )}
    </span>
  );
};
