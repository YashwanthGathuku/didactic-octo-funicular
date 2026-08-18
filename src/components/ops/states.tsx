/**
 * The states every server-backed view has to be able to render.
 *
 * They exist as shared components because the alternative is each screen
 * inventing its own, and the one that gets skipped is always `unavailable` --
 * which is how an outage renders as an empty list. A screen built on these
 * cannot skip it: the ApiResult union has no default branch.
 */

import React from 'react';
import {
  AlertTriangle,
  Ban,
  CloudOff,
  Inbox,
  Loader2,
  LogIn,
  RefreshCw,
} from 'lucide-react';
import type { ApiResult } from '../../api/client';

const box =
  'flex flex-col items-center justify-center gap-3 rounded border border-slate-700 bg-slate-900/60 px-6 py-10 text-center';

export const LoadingState: React.FC<{ what?: string }> = ({ what = 'data' }) => (
  <div className={box} role="status" aria-live="polite">
    <Loader2 className="h-6 w-6 animate-spin text-slate-400" aria-hidden />
    <p className="text-sm text-slate-400">Loading {what}…</p>
  </div>
);

/**
 * Empty is a *result*, and it is only ever rendered after a successful read.
 * The distinction from Unavailable is the entire point of this file.
 */
export const EmptyState: React.FC<{ title: string; detail?: string }> = ({ title, detail }) => (
  <div className={box}>
    <Inbox className="h-6 w-6 text-slate-500" aria-hidden />
    <p className="text-sm font-medium text-slate-300">{title}</p>
    {detail && <p className="max-w-md text-xs text-slate-500">{detail}</p>}
  </div>
);

export const UnavailableState: React.FC<{ error: string; onRetry?: () => void }> = ({
  error,
  onRetry,
}) => (
  <div
    className={`${box} border-amber-700/60 bg-amber-950/30`}
    role="alert"
    data-testid="unavailable"
  >
    <CloudOff className="h-6 w-6 text-amber-400" aria-hidden />
    <p className="text-sm font-semibold text-amber-200">Unavailable</p>
    <p className="max-w-md text-xs text-amber-100/80">
      This view could not be loaded from the gateway, so nothing is shown. It is not
      empty — it is unknown.
    </p>
    <p className="max-w-md font-mono text-[11px] text-amber-200/70">{error}</p>
    {onRetry && (
      <button
        type="button"
        onClick={onRetry}
        className="mt-1 inline-flex items-center gap-2 rounded border border-amber-600 px-3 py-1.5 text-xs font-medium text-amber-100 hover:bg-amber-900/40"
      >
        <RefreshCw className="h-3.5 w-3.5" aria-hidden />
        Retry
      </button>
    )}
  </div>
);

export const ForbiddenState: React.FC<{ error: string; needs?: string }> = ({ error, needs }) => (
  <div className={`${box} border-slate-600`} role="alert">
    <Ban className="h-6 w-6 text-slate-400" aria-hidden />
    <p className="text-sm font-semibold text-slate-200">Not permitted</p>
    <p className="max-w-md text-xs text-slate-400">
      Your account does not hold the permission this view requires
      {needs ? ` (${needs})` : ''}. Signing in again will not change that — an
      administrator has to grant it.
    </p>
    <p className="font-mono text-[11px] text-slate-500">{error}</p>
  </div>
);

export const UnauthenticatedState: React.FC<{ error: string }> = ({ error }) => (
  <div className={`${box} border-sky-800`} role="alert">
    <LogIn className="h-6 w-6 text-sky-400" aria-hidden />
    <p className="text-sm font-semibold text-sky-200">Session expired</p>
    <p className="max-w-md text-xs text-sky-100/80">
      The gateway no longer recognises this session. Sign in again to continue.
    </p>
    <p className="font-mono text-[11px] text-sky-300/70">{error}</p>
  </div>
);

/**
 * Partial data: the page loaded and some of it could not be read.
 *
 * Rendered above the content rather than instead of it, because the rows that
 * did load are still worth showing — but a list that quietly omitted a
 * quarantined artifact would be worse than one that admits it is incomplete.
 */
export const PartialBanner: React.FC<{ reason: string }> = ({ reason }) => (
  <div
    className="mb-3 flex items-start gap-2 rounded border border-amber-700/60 bg-amber-950/30 px-3 py-2"
    role="alert"
  >
    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" aria-hidden />
    <p className="text-xs text-amber-100">
      <span className="font-semibold">This page is incomplete.</span> {reason}. What is
      shown is correct; what is missing is unknown.
    </p>
  </div>
);

/**
 * Renders any non-ok ApiResult.
 *
 * Returns null for `ok`, so a caller writes
 * `<ResultState result={r} …/>` above its content and cannot forget a state:
 * adding a new member to the union makes this switch a compile error.
 */
export function ResultState<T>({
  result,
  onRetry,
  needs,
}: {
  result: ApiResult<T>;
  onRetry?: () => void;
  needs?: string;
}): React.ReactElement | null {
  switch (result.state) {
    case 'ok':
      return null;
    case 'unauthenticated':
      return <UnauthenticatedState error={result.error} />;
    case 'forbidden':
      return <ForbiddenState error={result.error} needs={needs} />;
    case 'notFound':
      return <EmptyState title="Not found" detail={result.error} />;
    case 'conflict':
      return <UnavailableState error={`${result.error}${result.detail ? `: ${result.detail}` : ''}`} onRetry={onRetry} />;
    case 'invalid':
      return (
        <UnavailableState
          error={`${result.error}${result.detail ? `: ${result.detail}` : ''}`}
          onRetry={onRetry}
        />
      );
    case 'unavailable':
      return <UnavailableState error={result.error} onRetry={onRetry} />;
  }
}
