import React, { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Database, Lock, ShieldAlert, X } from 'lucide-react';
import {
  ConnectorAuthMode,
  ConnectorCatalogEntry,
  ConnectorDescriptor,
  ConnectorField,
  ConnectorStatus,
  SourceConnection,
  createConnection,
  getConnectorCatalog,
  getConnectorDescriptor,
  parseConnectionUri,
} from '../services/api';

interface ConnectorWizardModalProps {
  onClose: () => void;
}

const STATUS_STYLE: Record<ConnectorStatus, string> = {
  AVAILABLE: 'badge-emerald',
  IMPLEMENTING: 'badge-amber',
  PLANNED: 'badge-slate',
  DEGRADED: 'badge-amber',
  DISABLED: 'badge-rose',
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
  const [displayName, setDisplayName] = useState<string>('');
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string>('');
  const [saved, setSaved] = useState<SourceConnection | null>(null);

  useEffect(() => {
    let cancelled = false;
    getConnectorCatalog().then((res) => {
      if (cancelled) return;
      if (res.state !== 'ok') {
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
      const seeded: Record<string, string> = {};
      for (const f of res.data.fields) {
        if (f.default) seeded[f.id] = f.default;
      }
      setValues(seeded);
    });
    return () => {
      cancelled = true;
    };
  }, [selected]);

  const visibleFields = useMemo(() => {
    if (!descriptor) return [];
    return descriptor.fields.filter(
      (f) => !f.appliesToAuth || f.appliesToAuth.includes(authMode),
    );
  }, [descriptor, authMode]);

  const handlePaste = async () => {
    if (!selected || !pasted.trim()) return;
    setParseError('');
    setParseNotes([]);
    const res = await parseConnectionUri(selected, pasted.trim());
    if (res.state !== 'ok') {
      setParseError(res.error);
      return;
    }
    setValues((prev) => ({ ...prev, ...res.data.fields }));
    if (res.data.warnings) setParseNotes(res.data.warnings);
    setPasted('');
  };

  const save = async () => {
    if (!descriptor || !displayName.trim()) return;
    setSaving(true);
    setSaveError('');
    setSaved(null);
    const res = await createConnection({
      connectorType: descriptor.type,
      displayName: displayName.trim(),
      authMode,
      fields: values,
      resourceAllowlist: [],
    });
    setSaving(false);
    if (res.state !== 'ok') {
      setSaveError(res.error);
      return;
    }
    setSaved(res.data);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 p-4 backdrop-blur-sm">
      <div className="flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl">
        <header className="flex items-center justify-between border-b border-slate-200 bg-slate-50 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600 border border-indigo-100 shadow-xs">
              <Database className="h-5 w-5" />
            </div>
            <div>
              <h2 className="text-sm font-bold text-slate-900">Source Database Connection</h2>
              <p className="text-xs text-slate-500">
                Configure verified database drivers and credentials with zero client-side retention.
              </p>
            </div>
          </div>
          <button onClick={onClose} aria-label="Close" className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-200 hover:text-slate-700">
            <X className="h-4 w-4" />
          </button>
        </header>

        <div className="grid flex-1 grid-cols-1 gap-0 overflow-hidden md:grid-cols-[280px_1fr]">
          <nav className="overflow-y-auto border-r border-slate-200 bg-slate-50/70 p-4">
            {catalogError && (
              <div className="mb-3 rounded-xl border border-amber-200 bg-amber-50 p-3 text-xs text-amber-900">
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
                    className={`w-full rounded-xl border p-3 text-left transition-all ${
                      selected === entry.type
                        ? 'border-indigo-600 bg-indigo-50/80 shadow-xs ring-1 ring-indigo-600'
                        : 'border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50'
                    } ${entry.selectable ? '' : 'cursor-not-allowed opacity-50'}`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-xs font-bold text-slate-900">{entry.displayName}</span>
                      <span
                        className={`rounded-full px-2 py-0.5 text-[9px] font-bold uppercase tracking-wider ${
                          STATUS_STYLE[entry.status]
                        }`}
                      >
                        {entry.status}
                      </span>
                    </div>
                    {!entry.selectable && (
                      <p className="mt-1 text-[11px] leading-snug text-slate-500">
                        {entry.statusReason}
                      </p>
                    )}
                    {entry.conformance && (
                      <p className="mt-1 text-[11px] text-emerald-700 font-medium">
                        Verified vs {entry.conformance.serverVersion}
                      </p>
                    )}
                  </button>
                </li>
              ))}
            </ul>
            <p className="mt-4 text-[11px] leading-snug text-slate-500">
              A connector becomes selectable only after passing the conformance suite.
            </p>
          </nav>

          <section className="overflow-y-auto p-6">
            {!descriptor && (
              <div className="flex h-full min-h-[300px] flex-col items-center justify-center text-center text-slate-400">
                <Database className="h-8 w-8 text-slate-400 mb-2" />
                <p className="text-xs font-semibold text-slate-700">Choose a connector to configure credentials</p>
              </div>
            )}

            {descriptor && (
              <div className="space-y-5">
                <AuthModePicker
                  modes={descriptor.authModes}
                  selected={authMode}
                  onSelect={setAuthMode}
                />

                {descriptor.supportsUriPaste && (
                  <div className="rounded-xl border border-slate-200 bg-slate-50 p-4">
                    <label className="text-xs font-bold text-slate-700">
                      Paste Connection String (Optional)
                    </label>
                    <p className="mt-0.5 text-[11px] text-slate-500">
                      Parsed on the server; credentials are encrypted directly into the secret vault.
                    </p>
                    <div className="mt-2 flex gap-2">
                      <input
                        type="password"
                        value={pasted}
                        onChange={(e) => setPasted(e.target.value)}
                        placeholder={descriptor.template}
                        className="flex-1 rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs text-slate-900 shadow-xs focus:border-indigo-500 focus:outline-none"
                      />
                      <button
                        onClick={handlePaste}
                        className="rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 shadow-xs"
                      >
                        Parse
                      </button>
                    </div>
                    {parseError && (
                      <p className="mt-2 text-xs text-rose-600">{parseError}</p>
                    )}
                    {parseNotes.map((note, i) => (
                      <p key={i} className="mt-2 flex items-start gap-1.5 text-xs text-amber-800">
                        <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0 text-amber-600" />
                        {note}
                      </p>
                    ))}
                  </div>
                )}

                {descriptor.template && (
                  <p className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 font-mono text-[11px] text-slate-600">
                    Template: {descriptor.template}
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

                <label className="block">
                  <span className="text-xs font-bold text-slate-700">Connection Display Name</span>
                  <input
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    placeholder="e.g. treasury-warehouse-prod"
                    className="mt-1 w-full rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs text-slate-900 shadow-xs focus:border-indigo-500 focus:outline-none"
                  />
                </label>

                {saveError && (
                  <p
                    role="alert"
                    className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-xs text-rose-900"
                  >
                    {saveError}
                  </p>
                )}
                {saved && (
                  <p
                    role="status"
                    className="rounded-xl border border-emerald-200 bg-emerald-50 p-3 text-xs text-emerald-900 font-medium"
                  >
                    Saved as connection #{saved.id}.
                  </p>
                )}

                <div className="flex justify-end gap-2 border-t border-slate-200 pt-3">
                  <button
                    type="button"
                    onClick={onClose}
                    className="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50 shadow-2xs"
                  >
                    Close
                  </button>
                  <button
                    type="button"
                    onClick={() => void save()}
                    disabled={saving || !displayName.trim() || !descriptor}
                    className="rounded-lg bg-indigo-600 px-4 py-1.5 text-xs font-semibold text-white hover:bg-indigo-700 disabled:opacity-40 shadow-xs"
                  >
                    {saving ? 'Saving…' : 'Save Connection'}
                  </button>
                </div>
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
    <span className="text-xs font-bold text-slate-700">Authentication Mode</span>
    <div className="mt-2 flex flex-wrap gap-2">
      {modes.map((mode) => (
        <button
          key={mode.id}
          onClick={() => onSelect(mode.id)}
          className={`rounded-xl border p-2.5 text-left text-xs transition-all ${
            selected === mode.id
              ? 'border-indigo-600 bg-indigo-50/80 text-indigo-950 ring-1 ring-indigo-600 font-bold'
              : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300'
          }`}
        >
          <span className="block font-semibold">{mode.label}</span>
          {mode.preferred && <span className="text-[10px] text-emerald-700 font-bold">Recommended</span>}
          {mode.localTestingOnly && (
            <span className="flex items-center gap-1 text-[10px] text-amber-700">
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
      <label className="flex items-center gap-1.5 text-xs font-bold text-slate-700">
        {writeOnly && <Lock className="h-3 w-3 text-amber-600" />}
        {field.label}
        {field.required && <span className="text-rose-500">*</span>}
      </label>

      {field.kind === 'ENUM' ? (
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="mt-1 w-full rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs text-slate-900 shadow-xs focus:border-indigo-500 focus:outline-none"
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
          className="mt-1 w-full rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs text-slate-900 shadow-xs focus:border-indigo-500 focus:outline-none"
        />
      ) : (
        <input
          type={writeOnly ? 'password' : field.kind === 'NUMBER' ? 'number' : 'text'}
          value={value}
          autoComplete={writeOnly ? 'new-password' : 'off'}
          onChange={(e) => onChange(e.target.value)}
          placeholder={writeOnly ? 'Enter to set or replace' : field.placeholder}
          className="mt-1 w-full rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs text-slate-900 shadow-xs focus:border-indigo-500 focus:outline-none"
        />
      )}

      {field.help && <p className="mt-1 text-[11px] leading-snug text-slate-500">{field.help}</p>}

      {selectedOption?.insecure && (
        <p className="mt-1 flex items-start gap-1.5 text-xs text-amber-800">
          <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0 text-amber-600" />
          {selectedOption.help ?? 'This choice weakens a security control.'}
        </p>
      )}
    </div>
  );
};
