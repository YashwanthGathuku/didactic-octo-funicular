/**
 * Operation States (Light Fintech Theme)
 * 
 * Shared UI components for loading, empty, unavailable, forbidden, and partial data states.
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
  'flex flex-col items-center justify-center gap-3 rounded-xl border border-slate-200 bg-white px-6 py-10 text-center shadow-xs';

export const LoadingState: React.FC<{ what?: string }> = ({ what = 'data' }) => (
  <div className={box} role="status" aria-live="polite">
    <Loader2 className="h-6 w-6 animate-spin text-indigo-600" aria-hidden />
    <p className="text-sm font-medium text-slate-600">Loading {what}…</p>
  </div>
);

export const EmptyState: React.FC<{ title: string; detail?: string }> = ({ title, detail }) => (
  <div className={box}>
    <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-slate-100 text-slate-500">
      <Inbox className="h-6 w-6" aria-hidden />
    </div>
    <p className="text-sm font-bold text-slate-800">{title}</p>
    {detail && <p className="max-w-md text-xs text-slate-500">{detail}</p>}
  </div>
);

export const UnavailableState: React.FC<{ error: string; onRetry?: () => void }> = ({
  error,
  onRetry,
}) => (
  <div
    className={`${box} border-amber-200 bg-amber-50/80`}
    role="alert"
    data-testid="unavailable"
  >
    <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-amber-100 text-amber-600">
      <CloudOff className="h-6 w-6" aria-hidden />
    </div>
    <p className="text-sm font-bold text-amber-900">Gateway View Unavailable</p>
    <p className="max-w-md text-xs text-amber-800">
      This view could not be loaded from the gateway server.
    </p>
    <p className="max-w-md font-mono text-[11px] text-amber-700">{error}</p>
    {onRetry && (
      <button
        type="button"
        onClick={onRetry}
        className="mt-1 inline-flex items-center gap-2 rounded-lg border border-amber-300 bg-white px-3 py-1.5 text-xs font-semibold text-amber-800 hover:bg-amber-100 shadow-2xs"
      >
        <RefreshCw className="h-3.5 w-3.5" aria-hidden />
        Retry Connection
      </button>
    )}
  </div>
);

export const ForbiddenState: React.FC<{ error: string; needs?: string }> = ({ error, needs }) => (
  <div className={`${box} border-slate-200 bg-slate-50`} role="alert">
    <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-slate-200 text-slate-600">
      <Ban className="h-6 w-6" aria-hidden />
    </div>
    <p className="text-sm font-bold text-slate-900">Permission Required</p>
    <p className="max-w-md text-xs text-slate-600">
      Your account does not hold the permission this view requires
      {needs ? ` (${needs})` : ''}.
    </p>
    <p className="font-mono text-[11px] text-slate-500">{error}</p>
  </div>
);

export const UnauthenticatedState: React.FC<{ error: string }> = ({ error }) => (
  <div className={`${box} border-sky-200 bg-sky-50`} role="alert">
    <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-sky-100 text-sky-600">
      <LogIn className="h-6 w-6" aria-hidden />
    </div>
    <p className="text-sm font-bold text-sky-950">Session Expired</p>
    <p className="max-w-md text-xs text-sky-800">
      The gateway no longer recognises this session. Sign in again to continue.
    </p>
    <p className="font-mono text-[11px] text-sky-700">{error}</p>
  </div>
);

export const PartialBanner: React.FC<{ reason: string }> = ({ reason }) => (
  <div
    className="mb-3 flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 px-3.5 py-2.5 shadow-2xs"
    role="alert"
  >
    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden />
    <p className="text-xs text-amber-900">
      <span className="font-bold">Partial View:</span> {reason}.
    </p>
  </div>
);

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
