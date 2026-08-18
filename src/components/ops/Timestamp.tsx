/**
 * A timestamp an operator can act on.
 *
 * Two properties the guide requires and one this product specifically needs.
 *
 * The instant is rendered in the *source* zone by default, because a partner
 * disputing a breach checks the deadline against their own agreement, and that
 * agreement is written in their zone. A board that shows only UTC forces every
 * such conversation through a mental conversion, which is where the mistakes
 * are.
 *
 * UTC is always available on inspection -- the `title` and the expandable form
 * -- because the source zone is ambiguous exactly twice a year, and the
 * evidence record is in UTC.
 *
 * Both come from the server's strings. Nothing here recomputes an instant from
 * the browser's clock or the browser's zone: the browser's clock is settable by
 * the person reading the screen, and its zone is wherever they happen to be.
 */

import React from 'react';

export interface TimestampProps {
  /** RFC3339, as every API field is. */
  iso: string | undefined | null;
  /** IANA zone the agreement is written in, when one governs. */
  zone?: string;
  /** A pre-formatted local reading the server derived, e.g. "09:00:00". */
  localLabel?: string;
  className?: string;
}

function inZone(iso: string, zone: string): string | null {
  try {
    return new Intl.DateTimeFormat('en-CA', {
      timeZone: zone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
      timeZoneName: 'short',
    }).format(new Date(iso));
  } catch {
    // An unknown zone is a data problem worth seeing, not a crash. Returning
    // null falls back to UTC, which is never wrong, only less useful.
    return null;
  }
}

export const Timestamp: React.FC<TimestampProps> = ({ iso, zone, localLabel, className }) => {
  if (!iso) return <span className={`text-slate-500 ${className ?? ''}`}>—</span>;

  const utc = `${iso.replace('T', ' ').replace(/Z$/, '')} UTC`;
  const local = zone ? inZone(iso, zone) : null;

  // The title carries both readings, so hovering answers "what is this in UTC"
  // without a second control and without leaving the row.
  const title = local ? `${local}\n${utc}${zone ? `\nzone: ${zone}` : ''}` : utc;

  return (
    <time dateTime={iso} title={title} className={`tabular-nums ${className ?? ''}`}>
      {local ?? utc}
      {local && (
        <span className="ml-1 text-[10px] text-slate-500" aria-hidden>
          / {utc}
        </span>
      )}
      {localLabel && !local && (
        <span className="ml-1 text-[10px] text-slate-500">({localLabel})</span>
      )}
      {/* Only when there is something to disambiguate.
          When no source zone governs, the visible text is already the UTC
          reading, and an sr-only copy of it makes a screen reader announce the
          same timestamp twice -- which is how a row of six timestamps becomes
          twelve. With a zone, the two readings differ and the spelled-out form
          earns its place: the abbreviated zone name is not pronounceable as a
          zone. */}
      {local && (
        <span className="sr-only">
          {local}, which is {utc}
          {zone ? `, in time zone ${zone}` : ''}
        </span>
      )}
    </time>
  );
};

/** Relative age, for queue and backlog readings where the delta is the point. */
export const Age: React.FC<{ seconds: number | null | undefined }> = ({ seconds }) => {
  if (seconds === null || seconds === undefined) return <span className="text-slate-500">—</span>;
  const s = Math.max(0, Math.floor(seconds));
  if (s < 60) return <span className="tabular-nums">{s}s</span>;
  if (s < 3600) return <span className="tabular-nums">{Math.floor(s / 60)}m</span>;
  if (s < 86400) return <span className="tabular-nums">{Math.floor(s / 3600)}h</span>;
  return <span className="tabular-nums">{Math.floor(s / 86400)}d</span>;
};
