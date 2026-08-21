/**
 * The Quarantine and Review Queue (Light Fintech Theme)
 * 
 * Where authorized reviewers inspect anomalies and enforce dual-control release governance.
 */

import React, { useCallback, useState } from 'react';
import { CheckCircle2, ShieldAlert, XCircle, ShieldQuestion } from 'lucide-react';
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
    <section aria-labelledby="queue-heading" className="space-y-4">
      <div>
        <h2 id="queue-heading" className="text-sm font-bold tracking-tight text-slate-900 flex items-center gap-2">
          <ShieldQuestion className="h-4 w-4 text-indigo-600" />
          Quarantine Review Queue
        </h2>
        <p className="text-xs text-slate-500">
          Enforce identity-bound dual-control approval with cryptographic artifact and policy integrity binding on quarantined payment batches
        </p>
      </div>

      {outcome && outcome.state !== 'ok' && (
        <div
          className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-xs text-amber-900 shadow-2xs"
          role="alert"
        >
          <p className="font-bold">
            {outcome.state === 'conflict'
              ? 'This decision changed while you were looking at it.'
              : 'Action refused by gateway.'}
          </p>
          <p className="mt-0.5">
            {outcome.error}
            {outcome.state === 'conflict' && outcome.detail ? ` — ${outcome.detail}` : ''}
          </p>
        </div>
      )}
      {outcome?.state === 'ok' && (
        <p className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-xs text-emerald-900 font-medium" role="status">
          Decision recorded successfully. State updated to <span className="font-bold">{outcome.data.state}</span>.
        </p>
      )}

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="tenant:read" />}
      {list.loading && <LoadingState what="the review queue" />}

      {list.result?.state === 'ok' && list.items.length === 0 && (
        <EmptyState
          title="No artifacts pending review"
          detail="The gateway answered and the quarantine queue is clear. All ingested files have settled."
        />
      )}

      <ul className="space-y-3">
        {list.items.map((d) => {
          const held = approvalsHeld(d);
          const rejected = d.votes.some((v) => v.choice === 'REJECT');
          const met = held >= d.requiredApprovals && !rejected;
          return (
            <li key={d.id} className="fintech-card p-4 bg-white border border-slate-200">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="text-sm font-bold text-slate-900">
                    Decision #{d.id} · Artifact #{d.artifactId}
                  </h3>
                  <p className="mt-0.5 font-mono text-[11px] text-slate-500 font-semibold">
                    SHA-256: {d.artifactSha256.slice(0, 16)}…
                  </p>
                  <p className="mt-1 text-xs text-slate-600">
                    Proposed by <strong className="text-slate-800">{d.proposedBy}</strong> at <Timestamp iso={d.proposedAt} />
                  </p>
                  <p className="text-[11px] text-slate-500 mt-0.5">
                    Policy: {d.policyVersion} · Rules: {d.rulePackVersion} · Outcome: {d.outcome}
                  </p>
                </div>
                <div className="text-right">
                  <span className="inline-flex items-center rounded-full border border-slate-200 bg-slate-50 px-2.5 py-0.5 text-xs font-bold text-slate-800">
                    {d.state}
                  </span>
                  <p className="mt-1.5 text-xs text-slate-600 font-medium">
                    {held} of {d.requiredApprovals} approvals
                    {d.separationOfDuties ? ' · Separation of Duties ON' : ''}
                  </p>
                  {rejected && (
                    <p className="mt-0.5 text-xs font-bold text-rose-600">Rejected by Reviewer</p>
                  )}
                </div>
              </div>

              {d.votes.length > 0 && (
                <ul className="mt-3 space-y-1 border-t border-slate-100 pt-2.5">
                  {d.votes.map((v, i) => (
                    <li key={`${v.actorId}-${i}`} className="text-xs text-slate-600">
                      <span className={v.choice === 'APPROVE' ? 'font-bold text-emerald-600' : 'font-bold text-rose-600'}>
                        {v.choice}
                      </span>{' '}
                      by <strong className="text-slate-800">{v.actorId}</strong> ({v.role}) — {v.reason || 'No reason recorded'} ·{' '}
                      <Timestamp iso={v.at} />
                    </li>
                  ))}
                </ul>
              )}

              <div className="mt-3.5 flex flex-wrap gap-2 pt-1 border-t border-slate-100">
                <ActionButton
                  label="Approve"
                  Icon={CheckCircle2}
                  tone="emerald"
                  allowed={can('release:approve')}
                  deniedReason="Approving a release requires the reviewer role."
                  onClick={() => { setOutcome(null); setPending({ kind: 'approve', decision: d }); }}
                />
                <ActionButton
                  label="Reject"
                  Icon={XCircle}
                  tone="rose"
                  allowed={can('release:approve')}
                  deniedReason="Rejecting a release requires the reviewer role."
                  onClick={() => { setOutcome(null); setPending({ kind: 'reject', decision: d }); }}
                />
                <ActionButton
                  label="Release"
                  Icon={CheckCircle2}
                  tone="sky"
                  allowed={can('release:approve') && met}
                  deniedReason={
                    !can('release:approve')
                      ? 'Releasing requires the reviewer role.'
                      : rejected
                        ? 'This decision was rejected by an authorized reviewer.'
                        : `The approval threshold is not met (${held} of ${d.requiredApprovals}).`
                  }
                  onClick={() => { setOutcome(null); setPending({ kind: 'release', decision: d }); }}
                />
                <ActionButton
                  label="Override"
                  Icon={ShieldAlert}
                  tone="amber"
                  allowed={can('release:override')}
                  deniedReason="Overriding dual control requires the release supervisor role."
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
          className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 shadow-2xs"
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
              ? `At least ${MIN_OVERRIDE_REASON} characters. Recorded in the tamper-evident ledger.`
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

function confirmBody(a: PendingAction): string {
  const held = new Set(a.decision.votes.filter((v) => v.choice === 'APPROVE').map((v) => v.actorId)).size;
  switch (a.kind) {
    case 'approve':
      return `Records your approval of artifact ${a.decision.artifactId}. Requires ${a.decision.requiredApprovals} approvals.`;
    case 'reject':
      return `Records your rejection of artifact ${a.decision.artifactId}. This prevents automated clearing release.`;
    case 'release':
      return `Releases artifact ${a.decision.artifactId} with ${held} of ${a.decision.requiredApprovals} approvals. Transitions state to RELEASED.`;
    case 'override':
      return `Releases artifact ${a.decision.artifactId} past controls with supervisor override. Reason is written to the immutable audit register.`;
  }
}

function confirmLabel(a: PendingAction): string {
  return { approve: 'Approve', reject: 'Reject', release: 'Release', override: 'Override and release' }[a.kind];
}

const toneClasses: Record<string, string> = {
  emerald: 'border-emerald-200 bg-emerald-50 text-emerald-800 hover:bg-emerald-100',
  rose: 'border-rose-200 bg-rose-50 text-rose-800 hover:bg-rose-100',
  sky: 'border-indigo-200 bg-indigo-50 text-indigo-800 hover:bg-indigo-100',
  amber: 'border-amber-200 bg-amber-50 text-amber-800 hover:bg-amber-100',
};

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
        className={`inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-semibold shadow-2xs transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${toneClasses[tone]}`}
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
