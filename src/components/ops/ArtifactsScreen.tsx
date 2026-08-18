/**
 * Artifacts and their validation results.
 *
 * The detail pane is where the product's honesty is most visible, so three
 * distinctions are made explicitly rather than left to the reader:
 *
 * `validationRun === null` means the artifact has not been validated. It does
 * not mean it was validated and found clean. Rendering "no findings" for both
 * would be the empty-file bug in a different place.
 *
 * `notCheckedRuleIds` is displayed. Silence does not imply coverage: a rule the
 * validator could not evaluate for lack of an authoritative source is not a
 * rule that passed.
 *
 * Evidence is redacted by the server where it is written. There is no masking
 * here to forget, because there is no un-redacted form to mask.
 */

import React, { useCallback, useEffect, useState } from 'react';
import { FileWarning, Search } from 'lucide-react';
import { getArtifact, getArtifacts } from '../../api/endpoints';
import { usePagedList } from '../../state/usePagedList';
import type { ApiResult } from '../../api/client';
import type { ArtifactDetail, ArtifactStatus, ArtifactSummary, Severity } from '../../api/types';
import { EmptyState, LoadingState, PartialBanner, ResultState } from './states';
import { Timestamp } from './Timestamp';

const STATUSES: Array<ArtifactStatus | ''> = [
  '', 'RECEIVED', 'VALIDATING', 'VALIDATED', 'QUARANTINED', 'APPROVED', 'RELEASED', 'REJECTED',
];

const severityStyle: Record<Severity, string> = {
  BLOCKING: 'border-rose-700 text-rose-300',
  WARNING: 'border-amber-700 text-amber-300',
  INFO: 'border-slate-700 text-slate-400',
};

export const ArtifactsScreen: React.FC<{ initialStatus?: ArtifactStatus }> = ({ initialStatus }) => {
  const [status, setStatus] = useState<ArtifactStatus | ''>(initialStatus ?? '');
  const [filename, setFilename] = useState('');
  const [debounced, setDebounced] = useState('');
  const [selected, setSelected] = useState<number | null>(null);

  // The filter is debounced so a search does not issue a request per keystroke.
  useEffect(() => {
    const t = setTimeout(() => setDebounced(filename.trim()), 250);
    return () => clearTimeout(t);
  }, [filename]);

  const fetchPage = useCallback(
    (cursor: string | undefined, signal: AbortSignal) =>
      getArtifacts({ status: status || undefined, filename: debounced || undefined, cursor, limit: 25 }, signal),
    [status, debounced],
  );
  const list = usePagedList<ArtifactSummary>(fetchPage, `artifacts:${status}:${debounced}`);

  return (
    <section aria-labelledby="artifacts-heading" className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
      <div className="space-y-3">
        <h2 id="artifacts-heading" className="text-sm font-semibold uppercase tracking-wide text-slate-300">
          Artifacts
        </h2>

        <div className="flex flex-wrap gap-2">
          <label className="flex items-center gap-2 text-xs text-slate-400">
            Status
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value as ArtifactStatus | '')}
              className="rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-200"
            >
              {STATUSES.map((s) => (
                <option key={s || 'all'} value={s}>{s || 'All'}</option>
              ))}
            </select>
          </label>
          <label className="flex flex-1 items-center gap-2 text-xs text-slate-400">
            <Search className="h-3.5 w-3.5" aria-hidden />
            <span className="sr-only">Filter by filename</span>
            <input
              value={filename}
              onChange={(e) => setFilename(e.target.value)}
              placeholder="filename contains…"
              className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-200"
            />
          </label>
        </div>

        {list.partial && <PartialBanner reason={list.partial} />}
        {list.result && <ResultState result={list.result} onRetry={list.reload} needs="tenant:read" />}
        {list.loading && <LoadingState what="artifacts" />}

        {list.result?.state === 'ok' && list.items.length === 0 && (
          <EmptyState
            title="No artifacts match"
            detail="The gateway answered; nothing matches this filter. Clear it to see everything."
          />
        )}

        <ul className="space-y-1.5">
          {list.items.map((a) => (
            <li key={a.id}>
              <button
                type="button"
                onClick={() => setSelected(a.id)}
                aria-current={selected === a.id ? 'true' : undefined}
                className={`w-full rounded border px-3 py-2 text-left transition ${
                  selected === a.id
                    ? 'border-sky-700 bg-sky-950/30'
                    : 'border-slate-800 bg-slate-900/40 hover:border-slate-700'
                }`}
              >
                <div className="flex items-baseline justify-between gap-2">
                  <span className="truncate font-mono text-xs text-slate-200">{a.filename}</span>
                  <span className="shrink-0 rounded border border-slate-700 px-1.5 py-0.5 text-[10px] text-slate-300">
                    {a.status}
                  </span>
                </div>
                <div className="mt-1 flex flex-wrap gap-2 text-[10px] text-slate-500">
                  <Timestamp iso={a.receivedAt} />
                  <span>{a.sizeBytes.toLocaleString()} bytes</span>
                  {a.blockingFindings > 0 && (
                    <span className="text-rose-300">{a.blockingFindings} blocking</span>
                  )}
                  {a.warningFindings > 0 && (
                    <span className="text-amber-300">{a.warningFindings} warning</span>
                  )}
                </div>
              </button>
            </li>
          ))}
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
      </div>

      <ArtifactDetailPane id={selected} />
    </section>
  );
};

const ArtifactDetailPane: React.FC<{ id: number | null }> = ({ id }) => {
  const [result, setResult] = useState<ApiResult<ArtifactDetail> | null>(null);

  const load = useCallback(() => {
    if (id === null) return;
    setResult(null);
    void getArtifact(id).then(setResult);
  }, [id]);

  useEffect(load, [load]);

  if (id === null) {
    return (
      <div className="rounded border border-dashed border-slate-800 p-6 text-center text-xs text-slate-500">
        Select an artifact to see its validation result.
      </div>
    );
  }
  if (result === null) return <LoadingState what="the validation result" />;
  if (result.state !== 'ok') return <ResultState result={result} onRetry={load} needs="tenant:read" />;

  const a = result.data;
  const run = a.validationRun;

  return (
    <article className="space-y-3 rounded border border-slate-800 bg-slate-900/40 p-4">
      <header>
        <h3 className="font-mono text-sm text-slate-100">{a.filename}</h3>
        <p className="mt-0.5 font-mono text-[10px] text-slate-500">sha256 {a.sha256}</p>
        <div className="mt-2 flex flex-wrap gap-3 text-[11px] text-slate-400">
          <span>Status <span className="text-slate-200">{a.status}</span></span>
          <span>{a.sizeBytes.toLocaleString()} bytes</span>
          <span>Received <Timestamp iso={a.receivedAt} /></span>
          {a.partnerName && <span>{a.partnerName}</span>}
        </div>
      </header>

      <section>
        <h4 className="text-xs font-semibold uppercase tracking-wide text-slate-400">Validation</h4>
        {run === null ? (
          /* Not validated. Not "validated, nothing found". */
          <p className="mt-1 flex items-start gap-1.5 rounded border border-slate-700 bg-slate-950/50 px-2 py-1.5 text-[11px] text-slate-300">
            <FileWarning className="mt-px h-3.5 w-3.5 shrink-0 text-slate-500" aria-hidden />
            This artifact has not been validated. No verdict exists for it yet — that is
            different from a validation that found nothing.
          </p>
        ) : (
          <dl className="mt-1 grid grid-cols-2 gap-x-4 gap-y-1 text-[11px] sm:grid-cols-3">
            <Field label="Outcome" value={run.outcome} />
            <Field label="Parser" value={`${run.parserName} ${run.parserVersion}`} />
            <Field label="Rule pack" value={run.rulePackVersion} />
            <Field label="Policy" value={run.policyVersion} />
            <Field label="Records parsed" value={run.recordsParsed.toLocaleString()} />
            <Field label="Parser ok" value={run.parserOk ? 'yes' : 'no'} />
            <Field
              label="Debits"
              value={`${(run.totalDebitsMinor / 100).toFixed(2)} (minor ${run.totalDebitsMinor})`}
            />
            <Field
              label="Credits"
              value={`${(run.totalCreditsMinor / 100).toFixed(2)} (minor ${run.totalCreditsMinor})`}
            />
            <Field label="Started" value={<Timestamp iso={run.startedAt} />} />
          </dl>
        )}
      </section>

      {a.notCheckedRuleIds && a.notCheckedRuleIds.length > 0 && (
        <section className="rounded border border-amber-800/60 bg-amber-950/20 px-2 py-1.5">
          <h4 className="text-[11px] font-semibold text-amber-200">Not checked</h4>
          <p className="text-[11px] text-amber-100/80">
            These rules could not be evaluated for lack of an authoritative source. Silence
            about them is not coverage.
          </p>
          <p className="mt-1 font-mono text-[10px] text-amber-200/70">
            {a.notCheckedRuleIds.join(', ')}
          </p>
        </section>
      )}

      <section>
        <h4 className="text-xs font-semibold uppercase tracking-wide text-slate-400">
          Findings ({a.findings.length})
        </h4>
        {a.findings.length === 0 ? (
          <p className="mt-1 text-[11px] text-slate-500">
            {run === null ? 'None recorded — nothing has run.' : 'The validator raised no findings.'}
          </p>
        ) : (
          <ul className="mt-1 space-y-1.5">
            {a.findings.map((f) => (
              <li key={f.id} className={`rounded border px-2 py-1.5 ${severityStyle[f.severity]}`}>
                <div className="flex items-baseline justify-between gap-2">
                  <span className="font-mono text-[11px]">{f.code}</span>
                  <span className="text-[10px]">{f.severity}</span>
                </div>
                <p className="mt-0.5 text-[11px] text-slate-300">{f.description}</p>
                <p className="mt-0.5 text-[10px] text-slate-500">
                  line {f.lineNumber} · byte {f.byteOffset}
                  {f.fieldStart > 0 ? ` · field ${f.fieldStart}–${f.fieldEnd}` : ''} · rule{' '}
                  {f.ruleVersion}
                  {f.provenance ? ` · ${f.provenance}` : ''}
                </p>
                {(f.expected || f.actual) && (
                  <p className="mt-0.5 font-mono text-[10px] text-slate-400">
                    expected {f.expected || '—'} · actual {f.actual || '—'}
                  </p>
                )}
                {f.evidence && (
                  <p className="mt-0.5 font-mono text-[10px] text-slate-500">
                    {f.evidence}{' '}
                    <span className="not-italic text-slate-600">(redacted at the server)</span>
                  </p>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      {a.history.length > 0 && (
        <section>
          <h4 className="text-xs font-semibold uppercase tracking-wide text-slate-400">History</h4>
          <ol className="mt-1 space-y-0.5">
            {a.history.map((t, i) => (
              <li key={i} className="text-[11px] text-slate-400">
                <Timestamp iso={t.occurredAt} /> — {t.fromState} → {t.toState} by {t.actorId}
                {t.reason ? ` (${t.reason})` : ''}
              </li>
            ))}
          </ol>
        </section>
      )}
    </article>
  );
};

const Field: React.FC<{ label: string; value: React.ReactNode }> = ({ label, value }) => (
  <div>
    <dt className="text-[10px] uppercase tracking-wide text-slate-500">{label}</dt>
    <dd className="text-slate-200">{value}</dd>
  </div>
);
