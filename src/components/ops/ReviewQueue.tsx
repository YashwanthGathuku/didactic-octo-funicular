/**
 * Quarantine and dual-control review queue.
 *
 * This surface makes the release boundary explicit: deterministic verification
 * can make an artifact eligible for review, but only authorized humans can
 * approve or release it.
 */

import React, { useCallback, useState } from 'react';
import { CheckCircle2, ShieldAlert, XCircle, ShieldQuestion, Fingerprint, FileKey2 } from 'lucide-react';
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
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="max-w-3xl">
          <h2 id="queue-heading" className="flex items-center gap-2 text-sm font-semibold text-slate-950">
            <ShieldQuestion className="h-4 w-4 text-slate-500" aria-hidden />
            Dual-control review queue
          </h2>
          <p className="mt-0.5 text-xs leading-5 text-slate-500">
            Verification is not authorization. Review decisions bind human identity to the exact artifact digest, policy version, rule pack, and deterministic outcome.
          </p>
        </div>
        <div className="flex items-center gap-2 text-[11px] text-slate-500">
          <Fingerprint className="h-3.5 w-3.5" aria-hidden />
          <span>Distinct authorized reviewers required</span>
        </div>
      </div>

      {outcome && outcome.state !== 'ok' && (
        <div className="border-l-2 border-amber-500 bg-amber-50 px-4 py-3 text-xs text-amber-900" role="alert">
          <p className="font-semibold">
            {outcome.state === 'conflict'
              ? 'This decision changed while you were viewing it.'
              : 'Action refused by gateway.'}
          </p>
          <p className="mt-0.5">
            {outcome.error}
            {outcome.state === 'conflict' && outcome.detail ? ` — ${outcome.detail}` : ''}
          </p>
        </div>
      )}
      {outcome?.state === 'ok' && (
        <div className="border-l-2 border-emerald-600 bg-emerald-50 px-4 py-3 text-xs text-emerald-900" role="status">
          Decision recorded. Current state: <strong>{outcome.data.state}</strong>.
        </div>
      )}

      {list.partial && <PartialBanner reason={list.partial} />}
      {list.result && <ResultState result={list.result} onRetry={list.reload} needs="tenant:read" />}
      {list.loading && <LoadingState what="the review queue" />}

      {list.result?.state === 'ok' && list.items.length === 0 && (
        <EmptyState
          title="No artifacts pending review"
          detail="The gateway answered and no current release decisions require reviewer action."
        />
      )}

      <div className="space-y-3">
        {list.items.map((d) => {
          const held = approvalsHeld(d);
          const rejected = d.votes.some((v) => v.choice === 'REJECT');
          const met = held >= d.requiredApprovals && !rejected;
          return (
            <article key={d.id} className="surface-panel overflow-hidden">
              <div className="grid gap-0 xl:grid-cols-[minmax(0,1fr)_250px]">
                <div className="min-w-0 p-4 sm:p-5">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="text-sm font-semibold text-slate-950">Artifact #{d.artifactId}</h3>
                        <span className="rounded-md border border-slate-200 bg-slate-50 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-slate-600">
                          decision/{d.id}
                        </span>
                      </div>
                      <div className="mt-2 flex min-w-0 items-center gap-2 rounded-md border border-slate-200 bg-slate-50 px-2.5 py-2">
                        <FileKey2 className="h-3.5 w-3.5 shrink-0 text-slate-400" aria-hidden />
                        <code className="truncate font-mono text-[10px] text-slate-600">sha256/{d.artifactSha256}</code>
                      </div>
                    </div>
                    <span className="rounded-md border border-slate-300 bg-white px-2 py-1 text-[10px] font-semibold text-slate-700">
                      {d.state}
                    </span>
                  </div>

                  <dl className="mt-4 grid gap-x-5 gap-y-3 text-xs sm:grid-cols-2 lg:grid-cols-4">
                    <ReviewFact label="Policy" value={d.policyVersion} mono />
                    <ReviewFact label="Rule pack" value={d.rulePackVersion} mono />
                    <ReviewFact label="Deterministic outcome" value={d.outcome} />
                    <ReviewFact label="Proposed by" value={d.proposedBy} />
                  </dl>

                  <div className="mt-3 text-[11px] text-slate-500">
                    Proposed <Timestamp iso={d.proposedAt} />
                    {d.separationOfDuties ? ' · proposer exclusion / separation of duties enforced' : ''}
                  </div>

                  {d.votes.length > 0 && (
                    <div className="mt-4 border-t border-slate-200 pt-3">
                      <p className="text-[10px] font-semibold tracking-wide text-slate-500">REVIEW HISTORY</p>
                      <ul className="mt-2 divide-y divide-slate-100">
                        {d.votes.map((v, i) => (
                          <li key={`${v.actorId}-${i}`} className="grid gap-1 py-2 text-xs sm:grid-cols-[90px_1fr_auto] sm:items-start">
                            <span className={v.choice === 'APPROVE' ? 'font-semibold text-emerald-700' : 'font-semibold text-rose-700'}>
                              {v.choice}
                            </span>
                            <span className="text-slate-600">
                              <strong className="font-semibold text-slate-800">{v.actorId}</strong> · {v.role} · {v.reason || 'No reason recorded'}
                            </span>
                            <span className="text-slate-500"><Timestamp iso={v.at} /></span>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>

                <aside className="border-t border-slate-200 bg-slate-50/70 p-4 xl:border-l xl:border-t-0" aria-label={`Authorization state for decision ${d.id}`}>
                  <p className="text-[10px] font-semibold tracking-wide text-slate-500">AUTHORIZATION</p>
                  <p className="mt-2 text-2xl font-bold tracking-[-0.04em] text-slate-950 tabular-nums">
                    {held}<span className="text-sm font-medium text-slate-400"> / {d.requiredApprovals}</span>
                  </p>
                  <p className="mt-0.5 text-xs text-slate-500">distinct approvals held</p>
                  {rejected && <p className="mt-2 text-xs font-semibold text-rose-700">Reviewer rejection blocks normal release.</p>}
                  {!rejected && met && <p className="mt-2 text-xs font-semibold text-emerald-700">Approval threshold satisfied.</p>}
                  {!rejected && !met && <p className="mt-2 text-xs text-slate-600">Release remains disabled until threshold is met.</p>}

                  <div className="mt-4 grid grid-cols-2 gap-2 xl:grid-cols-1">
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
                </aside>
              </div>
            </article>
          );
        })}
      </div>

      {list.hasMore && (
        <button type="button" onClick={list.loadMore} disabled={list.loadingMore} className="secondary-action">
          {list.loadingMore ? 'Loading…' : 'Load more decisions'}
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
              ? `At least ${MIN_OVERRIDE_REASON} characters. Recorded in the tamper-evident evidence ledger.`
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

const ReviewFact: React.FC<{ label: string; value: string; mono?: boolean }> = ({ label, value, mono = false }) => (
  <div className="min-w-0">
    <dt className="text-[10px] font-medium text-slate-500">{label}</dt>
    <dd className={`mt-0.5 truncate font-semibold text-slate-800 ${mono ? 'font-mono text-[11px]' : ''}`}>{value}</dd>
  </div>
);

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
      return `Releases artifact ${a.decision.artifactId} past normal dual-control requirements with supervisor override. The justification is recorded in the tamper-evident append-only evidence ledger.`;
  }
}

function confirmLabel(a: PendingAction): string {
  return { approve: 'Approve', reject: 'Reject', release: 'Release', override: 'Override and release' }[a.kind];
}

const toneClasses: Record<string, string> = {
  emerald: 'border-emerald-300 bg-white text-emerald-800 hover:bg-emerald-50',
  rose: 'border-rose-300 bg-white text-rose-800 hover:bg-rose-50',
  sky: 'border-slate-900 bg-slate-950 text-white hover:bg-slate-800',
  amber: 'border-amber-300 bg-white text-amber-900 hover:bg-amber-50',
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
        className={`inline-flex min-h-9 w-full items-center justify-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${toneClasses[tone]}`}
      >
        <Icon className="h-3.5 w-3.5" aria-hidden />
        {label}
      </button>
      {!allowed && <span id={id} className="sr-only">{deniedReason}</span>}
    </span>
  );
};
