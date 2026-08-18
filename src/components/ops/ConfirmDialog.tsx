/**
 * The confirmation a consequential decision goes through.
 *
 * Accessibility here is not decoration. A release decision made with a keyboard
 * and a screen reader has to be as safe as one made with a mouse, and the ways
 * that fails are specific:
 *
 * - focus has to move into the dialog, or the reader stays on the page behind
 *   it and announces nothing
 * - focus has to be trapped, or Tab walks out of the dialog into the queue
 *   underneath and a reviewer confirms something they cannot see
 * - focus has to return to the control that opened it, or a keyboard user is
 *   dropped at the top of the document after every action
 * - Escape has to cancel, because the reflex for "I did not mean this" is
 *   Escape and nothing else
 * - the dialog needs role="dialog" aria-modal with a labelled title and
 *   description, or it is announced as an unnamed group
 *
 * The confirm button starts disabled when a reason is required, so the primary
 * action cannot be reached by pressing Enter on an empty form.
 */

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { AlertTriangle } from 'lucide-react';

export interface ConfirmDialogProps {
  title: string;
  body: string;
  confirmLabel: string;
  destructive?: boolean;
  requireReason?: boolean;
  minReasonLength?: number;
  reasonHint?: string;
  busy?: boolean;
  onCancel(): void;
  onConfirm(reason: string): void;
}

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])';

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  title,
  body,
  confirmLabel,
  destructive,
  requireReason,
  minReasonLength = 1,
  reasonHint,
  busy,
  onCancel,
  onConfirm,
}) => {
  const [reason, setReason] = useState('');
  const panel = useRef<HTMLDivElement>(null);
  const firstField = useRef<HTMLTextAreaElement | HTMLButtonElement>(null);
  // Captured on mount, restored on unmount.
  const opener = useRef<Element | null>(null);

  useEffect(() => {
    opener.current = document.activeElement;
    firstField.current?.focus();
    return () => {
      (opener.current as HTMLElement | null)?.focus?.();
    };
  }, []);

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onCancel();
        return;
      }
      if (e.key !== 'Tab') return;

      const nodes = panel.current?.querySelectorAll<HTMLElement>(FOCUSABLE);
      if (!nodes || nodes.length === 0) return;
      const first = nodes[0];
      const last = nodes[nodes.length - 1];

      // Wrap at both ends. Without this Tab from the last control lands on the
      // browser chrome and then on the page behind the dialog.
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    },
    [onCancel],
  );

  const reasonOk = !requireReason || reason.trim().length >= minReasonLength;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
      onKeyDown={onKeyDown}
    >
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-title"
        aria-describedby="confirm-body"
        className="w-full max-w-lg rounded border border-slate-700 bg-slate-900 p-4 shadow-xl"
      >
        <h2 id="confirm-title" className="flex items-start gap-2 text-sm font-semibold text-slate-100">
          {destructive && <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" aria-hidden />}
          {title}
        </h2>
        <p id="confirm-body" className="mt-2 text-xs leading-relaxed text-slate-300">
          {body}
        </p>

        {requireReason && (
          <div className="mt-3">
            <label htmlFor="confirm-reason" className="block text-xs font-medium text-slate-300">
              Reason
            </label>
            {reasonHint && (
              <p id="confirm-reason-hint" className="mt-0.5 text-[11px] text-slate-500">
                {reasonHint}
              </p>
            )}
            <textarea
              id="confirm-reason"
              ref={firstField as React.RefObject<HTMLTextAreaElement>}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              aria-describedby={reasonHint ? 'confirm-reason-hint confirm-reason-count' : 'confirm-reason-count'}
              aria-invalid={!reasonOk}
              className="mt-1 w-full rounded border border-slate-700 bg-slate-950 px-2 py-1.5 text-xs text-slate-100"
            />
            <p
              id="confirm-reason-count"
              className={`mt-1 text-[11px] ${reasonOk ? 'text-slate-500' : 'text-amber-300'}`}
              aria-live="polite"
            >
              {reason.trim().length} / {minReasonLength} characters minimum
            </p>
          </div>
        )}

        <div className="mt-4 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            ref={requireReason ? undefined : (firstField as React.RefObject<HTMLButtonElement>)}
            className="rounded border border-slate-600 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-800"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onConfirm(reason.trim())}
            disabled={!reasonOk || busy}
            className={`rounded border px-3 py-1.5 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-40 ${
              destructive
                ? 'border-amber-600 bg-amber-900/40 text-amber-100 hover:bg-amber-900/60'
                : 'border-sky-600 bg-sky-900/40 text-sky-100 hover:bg-sky-900/60'
            }`}
          >
            {busy ? 'Working…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
};
