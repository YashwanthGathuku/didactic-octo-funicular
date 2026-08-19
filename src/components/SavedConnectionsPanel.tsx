import React, { useCallback, useEffect, useState } from 'react';
import {
  AlertTriangle,
  CircleHelp,
  KeyRound,
  Loader2,
  RefreshCw,
  ShieldCheck,
  ShieldX,
  Database,
} from 'lucide-react';
import {
  ConnectionHealthState,
  SourceConnection,
  getConnections,
  replaceConnectionSecret,
  testConnection,
} from '../services/api';

const HEALTH_PRESENTATION: Record<
  ConnectionHealthState,
  { label: string; className: string; Icon: React.ComponentType<{ className?: string }> }
> = {
  NEVER_CHECKED: {
    label: 'Never checked',
    className: 'badge-slate',
    Icon: CircleHelp,
  },
  HEALTHY: {
    label: 'Healthy',
    className: 'badge-emerald',
    Icon: ShieldCheck,
  },
  DEGRADED: {
    label: 'Degraded',
    className: 'badge-amber',
    Icon: AlertTriangle,
  },
  FAILED: {
    label: 'Failed',
    className: 'badge-rose',
    Icon: ShieldX,
  },
};

export const SavedConnectionsPanel: React.FC = () => {
  const [connections, setConnections] = useState<SourceConnection[]>([]);
  const [unavailable, setUnavailable] = useState<string>('');
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState<number | null>(null);

  const load = useCallback(async () => {
    const res = await getConnections();
    setLoaded(true);
    if (res.state !== 'ok') {
      setUnavailable(res.error);
      return;
    }
    setUnavailable('');
    setConnections(res.data.connections ?? []);
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const runCheck = async (id: number) => {
    setBusy(id);
    await testConnection(id);
    await load();
    setBusy(null);
  };

  if (!loaded) {
    return (
      <div className="flex items-center gap-2 p-6 text-xs text-slate-400">
        <Loader2 className="h-4 w-4 animate-spin text-indigo-400" /> Loading connection metadata…
      </div>
    );
  }

  if (unavailable) {
    return (
      <div className="m-6 rounded-xl border border-amber-500/30 bg-amber-500/10 p-4 text-xs text-amber-200">
        <p className="font-semibold">Connections unavailable</p>
        <p className="mt-1 font-mono text-[11px] text-amber-300/80">{unavailable}</p>
      </div>
    );
  }

  if (connections.length === 0) {
    return (
      <div className="p-8 text-center space-y-2">
        <Database className="h-8 w-8 text-slate-600 mx-auto" />
        <p className="text-xs font-semibold text-slate-300">No source connections configured</p>
        <p className="text-[11px] text-slate-500 max-w-sm mx-auto">
          Source connections can only be configured against connectors whose driver has passed the 21-point conformance suite.
        </p>
      </div>
    );
  }

  return (
    <div className="p-5 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400">Configured Endpoints ({connections.length})</h3>
        <button
          onClick={() => void load()}
          className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 px-2.5 py-1 text-xs text-slate-300 hover:bg-white/10"
        >
          <RefreshCw className="h-3 w-3" /> Refresh
        </button>
      </div>

      <ul className="space-y-2.5">
        {connections.map((c) => (
          <ConnectionRow
            key={c.id}
            connection={c}
            busy={busy === c.id}
            onCheck={() => void runCheck(c.id)}
            onChanged={() => void load()}
          />
        ))}
      </ul>
    </div>
  );
};

const ConnectionRow: React.FC<{
  connection: SourceConnection;
  busy: boolean;
  onCheck: () => void;
  onChanged: () => void;
}> = ({ connection, busy, onCheck, onChanged }) => {
  const [replacing, setReplacing] = useState<string>('');
  const [value, setValue] = useState<string>('');
  const [message, setMessage] = useState<string>('');

  const presentation = HEALTH_PRESENTATION[connection.health.State] ?? HEALTH_PRESENTATION.NEVER_CHECKED;
  const { Icon } = presentation;

  const submitReplacement = async () => {
    const res = await replaceConnectionSecret(connection.id, replacing, value);
    setValue('');
    setReplacing('');
    setMessage(res.state === 'ok' ? 'Credential replaced.' : `Not replaced: ${res.error}`);
    onChanged();
  };

  return (
    <li className="rounded-xl border border-white/[0.08] bg-[#0A0F1D] p-4 transition-all hover:border-white/15">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-bold text-white">{connection.displayName}</p>
          <p className="font-mono text-[11px] text-slate-400 mt-0.5">
            {connection.connectorType} · {connection.authMode} ·{' '}
            {connection.resourceAllowlist.length} approved resource
            {connection.resourceAllowlist.length === 1 ? '' : 's'}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <span className={`inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-[10px] font-bold uppercase tracking-wider ${presentation.className}`}>
            <Icon className="h-3 w-3" />
            {presentation.label}
          </span>
          <button
            onClick={onCheck}
            disabled={busy}
            className="rounded-lg border border-white/10 bg-white/[0.03] px-2.5 py-1 text-xs font-medium text-slate-300 hover:bg-white/10 disabled:opacity-50"
          >
            {busy ? 'Probing…' : 'Check Now'}
          </button>
        </div>
      </div>

      {replacing ? (
        <div className="mt-3 flex items-center gap-2 rounded-lg border border-white/10 bg-black/40 p-2 text-xs">
          <span className="font-mono text-slate-400">Secret key: {replacing}</span>
          <input
            type="password"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="Enter new credential value"
            className="flex-1 rounded border border-white/10 bg-[#070B14] px-2 py-1 text-xs text-white"
          />
          <button
            onClick={submitReplacement}
            className="rounded bg-indigo-600 px-3 py-1 font-semibold text-white hover:bg-indigo-500"
          >
            Save
          </button>
          <button
            onClick={() => setReplacing('')}
            className="rounded border border-white/10 px-2 py-1 text-slate-400 hover:text-white"
          >
            Cancel
          </button>
        </div>
      ) : (
        <div className="mt-3 flex items-center justify-between text-xs text-slate-400">
          <div className="flex items-center gap-2">
            <KeyRound className="h-3.5 w-3.5 text-slate-500" />
            <span className="text-[11px]">Configured secret references: {connection.secretsConfigured.join(', ') || 'none'}</span>
          </div>
          {connection.secretsConfigured.length > 0 && (
            <button
              onClick={() => setReplacing(connection.secretsConfigured[0])}
              className="text-[11px] text-indigo-400 hover:text-indigo-300 underline"
            >
              Rotate Secret
            </button>
          )}
        </div>
      )}

      {message && <p className="mt-2 text-xs text-emerald-400">{message}</p>}
    </li>
  );
};
