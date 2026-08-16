import React, { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Database, Lock, ShieldAlert, X } from 'lucide-react';
import {
  ConnectorAuthMode,
  ConnectorCatalogEntry,
  ConnectorDescriptor,
  ConnectorField,
  ConnectorStatus,
  getConnectorCatalog,
  getConnectorDescriptor,
  parseConnectionUri,
} from '../services/api';

/**
 * One generic connection wizard for every database.
 *
 * There is deliberately no PostgreSQL component, no Oracle component and no
 * per-provider field list in this file. Selecting a connector fetches a
 * server-owned descriptor and the form renders from it, so:
 *
 *   - adding a connector is a server change,
 *   - the security rules live where they are enforced rather than being
 *     duplicated in a client that can drift,
 *   - and a field the server declares write-only cannot be redisplayed here,
 *     because the API never sends it.
 *
 * This replaces the hardcoded Integration Hub that Prompt 01 deleted, which
 * rendered a fixed list of connectors and reported healthy connections to
 * databases it had never contacted.
 */

interface ConnectorWizardModalProps {
  onClose: () => void;
}

const STATUS_STYLE: Record<ConnectorStatus, string> = {
  AVAILABLE: 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30',
  IMPLEMENTING: 'bg-amber-500/10 text-amber-300 border-amber-500/30',
  PLANNED: 'bg-slate-500/10 text-slate-300 border-slate-500/30',
  DEGRADED: 'bg-orange-500/10 text-orange-300 border-orange-500/30',
  DISABLED: 'bg-rose-500/10 text-rose-300 border-rose-500/30',
};

export const ConnectorWizardModal: React.FC<ConnectorWizardModalProps> = ({ onClose }) => {
  const [catalog, setCatalog] = useState<ConnectorCatalogEntry[]>([]);
  const [catalogError, setCatalogError] = useState<string>('');
  const [selected, setSelected] = useState<string>('');
  const [descriptor, setDescriptor] = useState<ConnectorDescriptor | null>(null);
  const [authMode, setAuthMode] = useState<string>('');
  const [values, setValues] = useState<Record<string, string>>({});
  const [pasted, setPasted] = useState<string>('');
  const [parseNotes, setParseNotes] = useState<string[]>([]);
  const [parseError, setParseError] = useState<string>('');

  useEffect(() => {
    let cancelled = false;
    getConnectorCatalog().then((res) => {
      if (cancelled) return;
      if (res.state !== 'ok') {
        // An unreachable gateway is rendered as unavailable, never as an empty
        // catalog. An empty catalog would read as "no connectors exist".
        setCatalogError(res.error);
        return;
      }
      setCatalog(res.data.connectors);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!selected) {
      setDescriptor(null);
      return;
    }
    let cancelled = false;
    getConnectorDescriptor(selected).then((res) => {
      if (cancelled) return;
      if (res.state !== 'ok') {
        setCatalogError(res.error);
        return;
      }
      setDescriptor(res.data);
      const preferred =
        res.data.authModes.find((m) => m.preferred) ?? res.data.authModes[0];
      setAuthMode(preferred ? preferred.id : '');
      // Defaults come from the descriptor, so the server decides what a blank
      // field means.
      const seeded: Record<string, string> = {};
      for (const f of res.data.fields) {
        if (f.default) seeded[f.id] = f.default;
      }
      setValues(seeded);
      setParseNotes([]);
      setParseError('');
      setPasted('');
    });
    return () => {
      cancelled = true;
    };
  }, [selected]);

  const visibleFields = useMemo(() => {
    if (!descriptor) return [];
    // A field restricted to other authentication modes is hidden entirely
    // rather than disabled. A visible password box under "key pair" invites
    // someone to fill it in, and the value would then be stored for a mode
    // that never reads it.
    return descriptor.fields.filter(
      (f) => !f.appliesToAuth || f.appliesToAuth.length === 0 || f.appliesToAuth.includes(authMode),
    );
  }, [descriptor, authMode]);

  const handlePaste = async () => {
    if (!descriptor || !pasted.trim()) return;
    setParseError('');
    const res = await parseConnectionUri(descriptor.type, pasted);
    if (res.state !== 'ok') {
      setParseError(res.error);
      return;
    }
    setValues((prev) => ({ ...prev, ...res.data.fields }));
    setParseNotes([
      res.data.secretExtracted
        ? 'The credential was separated into the secret store and the pasted string discarded.'
        : 'No credential was present in the pasted string.',
      ...(res.data.warnings ?? []),
    ]);
    // Cleared immediately. Leaving it in state would keep the credential in the
    // component, in the DOM, and in anything that later serialises props.
    setPasted('');
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-lg border border-slate-700 bg-slate-900">
        <header className="flex items-center justify-between border-b border-slate-700 px-6 py-4">
          <div className="flex items-center gap-3">
            <Database className="h-5 w-5 text-sky-400" />
            <div>
              <h2 className="text-lg font-semibold text-slate-100">Source database connection</h2>
              <p className="text-xs text-slate-400">
                Fields are supplied by the server for the connector you choose.
              </p>
            </div>
          </div>
          <button onClick={onClose} aria-label="Close" className="text-slate-400 hover:text-slate-200">
            <X className="h-5 w-5" />
          </button>
        </header>

        <div className="grid flex-1 grid-cols-1 gap-0 overflow-hidden md:grid-cols-[280px_1fr]">
          <nav className="overflow-y-auto border-r border-slate-700 p-4">
            {catalogError && (
              <div className="mb-3 rounded border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
                The catalog is unavailable: {catalogError}
              </div>
            )}
            <ul className="space-y-2">
              {catalog.map((entry) => (
                <li key={entry.type}>
                  <button
                    onClick={() => entry.selectable && setSelected(entry.type)}
                    disabled={!entry.selectable}
                    title={entry.selectable ? undefined : entry.statusReason}
                    className={`w-full rounded border p-3 text-left transition ${
                      selected === entry.type
                        ? 'border-sky-500 bg-sky-500/10'
                        : 'border-slate-700 hover:border-slate-600'
                    } ${entry.selectable ? '' : 'cursor-not-allowed opacity-60'}`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm text-slate-100">{entry.displayName}</span>
                      <span
                        className={`rounded border px-1.5 py-0.5 text-[10px] font-medium ${
                          STATUS_STYLE[entry.status]
                        }`}
                      >
                        {entry.status}
                      </span>
                    </div>
                    {!entry.selectable && (
                      <p className="mt-1 text-[11px] leading-snug text-slate-400">
                        {entry.statusReason}
                      </p>
                    )}
                    {entry.conformance && (
                      <p className="mt-1 text-[11px] text-emerald-300/80">
                        Verified against {entry.conformance.serverVersion}
                      </p>
                    )}
                  </button>
                </li>
              ))}
            </ul>
            <p className="mt-4 text-[11px] leading-snug text-slate-500">
              A connector becomes selectable only after its driver passes the shared conformance
              suite against a real server. Until then it is shown and cannot be connected to.
            </p>
          </nav>

          <section className="overflow-y-auto p-6">
            {!descriptor && (
              <p className="text-sm text-slate-400">
                Choose a connector to see the fields it needs.
              </p>
            )}

            {descriptor && (
              <div className="space-y-5">
                <AuthModePicker
                  modes={descriptor.authModes}
                  selected={authMode}
                  onSelect={setAuthMode}
                />

                {descriptor.supportsUriPaste && (
                  <div className="rounded border border-slate-700 p-4">
                    <label className="text-xs font-medium text-slate-300">
                      Paste a connection string (optional)
                    </label>
                    <p className="mt-1 text-[11px] text-slate-500">
                      It is parsed on the server, the credential is moved into the secret store,
                      and the string itself is discarded. It is never saved or shown again.
                    </p>
                    <div className="mt-2 flex gap-2">
                      <input
                        type="password"
                        value={pasted}
                        onChange={(e) => setPasted(e.target.value)}
                        placeholder={descriptor.template}
                        className="flex-1 rounded border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-100"
                      />
                      <button
                        onClick={handlePaste}
                        className="rounded bg-sky-600 px-3 py-2 text-sm text-white hover:bg-sky-500"
                      >
                        Split
                      </button>
                    </div>
                    {parseError && (
                      <p className="mt-2 text-[11px] text-rose-300">{parseError}</p>
                    )}
                    {parseNotes.map((note, i) => (
                      <p key={i} className="mt-2 flex items-start gap-1.5 text-[11px] text-amber-200">
                        <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" />
                        {note}
                      </p>
                    ))}
                  </div>
                )}

                {descriptor.template && (
                  <p className="rounded border border-slate-800 bg-slate-950 px-3 py-2 font-mono text-[11px] text-slate-400">
                    {descriptor.template}
                  </p>
                )}

                {visibleFields.map((field) => (
                  <FieldInput
                    key={field.id}
                    field={field}
                    value={values[field.id] ?? ''}
                    onChange={(v) => setValues((prev) => ({ ...prev, [field.id]: v }))}
                  />
                ))}

                <p className="border-t border-slate-800 pt-4 text-[11px] leading-snug text-slate-500">
                  After saving, this screen shows a masked summary and which credentials are
                  configured. Saved passwords, keys, tokens, wallets, service-account files and
                  complete connection strings are never displayed again — replacing one is the
                  only way to change it.
                </p>
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
};

const AuthModePicker: React.FC<{
  modes: ConnectorAuthMode[];
  selected: string;
  onSelect: (id: string) => void;
}> = ({ modes, selected, onSelect }) => (
  <div>
    <span className="text-xs font-medium text-slate-300">Authentication</span>
    <div className="mt-2 flex flex-wrap gap-2">
      {modes.map((mode) => (
        <button
          key={mode.id}
          onClick={() => onSelect(mode.id)}
          className={`rounded border px-3 py-2 text-left text-xs ${
            selected === mode.id
              ? 'border-sky-500 bg-sky-500/10 text-slate-100'
              : 'border-slate-700 text-slate-300 hover:border-slate-600'
          }`}
        >
          <span className="block font-medium">{mode.label}</span>
          {mode.preferred && <span className="text-[10px] text-emerald-300">Recommended</span>}
          {mode.localTestingOnly && (
            <span className="flex items-center gap-1 text-[10px] text-amber-300">
              <ShieldAlert className="h-3 w-3" /> Local testing only
            </span>
          )}
        </button>
      ))}
    </div>
  </div>
);

const FieldInput: React.FC<{
  field: ConnectorField;
  value: string;
  onChange: (v: string) => void;
}> = ({ field, value, onChange }) => {
  const writeOnly = field.kind === 'SECRET';
  const selectedOption = field.options?.find((o) => o.value === value);

  return (
    <div>
      <label className="flex items-center gap-1.5 text-xs font-medium text-slate-300">
        {writeOnly && <Lock className="h-3 w-3 text-amber-300" />}
        {field.label}
        {field.required && <span className="text-rose-400">*</span>}
      </label>

      {field.kind === 'ENUM' ? (
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="mt-1 w-full rounded border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-100"
        >
          <option value="">Choose…</option>
          {field.options?.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      ) : field.kind === 'LIST' ? (
        <textarea
          value={value}
          onChange={(e) => onChange(e.target.value)}
          rows={2}
          placeholder="One per line"
          className="mt-1 w-full rounded border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-100"
        />
      ) : (
        <input
          // A write-only field renders empty with a password input, always. It
          // is never populated from a saved value, because the API does not
          // return one.
          type={writeOnly ? 'password' : field.kind === 'NUMBER' ? 'number' : 'text'}
          value={value}
          autoComplete={writeOnly ? 'new-password' : 'off'}
          onChange={(e) => onChange(e.target.value)}
          placeholder={writeOnly ? 'Enter to set or replace' : field.placeholder}
          className="mt-1 w-full rounded border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-100"
        />
      )}

      {field.help && <p className="mt-1 text-[11px] leading-snug text-slate-500">{field.help}</p>}

      {selectedOption?.insecure && (
        <p className="mt-1 flex items-start gap-1.5 text-[11px] text-amber-200">
          <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" />
          {selectedOption.help ?? 'This choice weakens a security control.'}
        </p>
      )}
    </div>
  );
};
