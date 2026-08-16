import React, { useCallback, useEffect, useState } from 'react';
import {
  AlertTriangle,
  CircleHelp,
  KeyRound,
  Loader2,
  RefreshCw,
  ShieldCheck,
  ShieldX,
  Trash2,
} from 'lucide-react';
import {
  ConnectionHealthState,
  SourceConnection,
  deleteConnection,
  getConnections,
  replaceConnectionSecret,
  testConnection,
} from '../services/api';

/**
 * Saved source connections: what exists, whether it works, and when that was
 * last actually checked.
 *
 * The health column is the point of the screen. NEVER_CHECKED renders as its
 * own state with its own icon and no colour that reads as success, because a
 * connection nobody has tested showing green is precisely the defect the
 * connector platform exists to prevent — the Integration Hub reported healthy
 * connections to databases it had never contacted.
 *
 * Nothing here masks a credential, because nothing here receives one. The API
 * returns which credential fields are configured and never their values, so a
 * bug in this file cannot disclose one.
 */

const HEALTH_PRESENTATION: Record<
  ConnectionHealthState,
  { label: string; className: string; Icon: React.ComponentType<{ className?: string }> }
> = {
  // Deliberately grey, not green. "Not checked" is an absence of evidence.
  NEVER_CHECKED: {
    label: 'Never checked',
    className: 'text-slate-300 border-slate-600 bg-slate-700/30',
    Icon: CircleHelp,
  },
  HEALTHY: {
    label: 'Healthy',
    className: 'text-emerald-300 border-emerald-500/30 bg-emerald-500/10',
    Icon: ShieldCheck,
  },
  DEGRADED: {
    label: 'Degraded',
    className: 'text-amber-300 border-amber-500/30 bg-amber-500/10',
    Icon: AlertTriangle,
  },
  FAILED: {
    label: 'Failed',
    className: 'text-rose-300 border-rose-500/30 bg-rose-500/10',
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
      // Rendered as unavailable, never as an empty list. An empty list would
      // read as "this tenant has no connections", which is a different and
      // possibly reassuring statement.
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
    // Re-read rather than patching local state from the response, so what the
    // screen shows is what the server stored.
    await load();
    setBusy(null);
  };

  if (!loaded) {
    return (
      <div className="flex items-center gap-2 p-6 text-sm text-slate-400">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading connections…
      </div>
    );
  }

  if (unavailable) {
    return (
      <div className="m-6 rounded border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-200">
        <p className="font-medium">Connections are unavailable</p>
        <p className="mt-1 text-xs">{unavailable}</p>
      </div>
    );
  }

  if (connections.length === 0) {
    return (
      <div className="m-6 rounded border border-slate-700 p-6 text-sm text-slate-400">
        <p>No source connections are configured.</p>
        <p className="mt-2 text-xs text-slate-500">
          A connection can only be created against a connector whose driver has passed the
          shared conformance suite against a real server.
        </p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-slate-100">Source connections</h2>
        <button
          onClick={() => void load()}
          className="flex items-center gap-1.5 rounded border border-slate-700 px-2 py-1 text-xs text-slate-300 hover:border-slate-600"
        >
          <RefreshCw className="h-3 w-3" /> Refresh
        </button>
      </div>

      <ul className="space-y-3">
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
    // Cleared immediately either way. Leaving it in state would keep the new
    // credential in this component and in the DOM after it has been stored.
    setValue('');
    setReplacing('');
    setMessage(res.state === 'ok' ? 'Credential replaced.' : `Not replaced: ${res.error}`);
    onChanged();
  };

  return (
    <li className="rounded border border-slate-700 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-slate-100">{connection.displayName}</p>
          <p className="text-xs text-slate-400">
            {connection.connectorType} · {connection.authMode} ·{' '}
            {connection.resourceAllowlist.length} approved resource
            {connection.resourceAllowlist.length === 1 ? '' : 's'}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <span
            className={`flex items-center gap-1.5 rounded border px-2 py-1 text-[11px] ${presentation.className}`}
          >
            <Icon className="h-3 w-3" />
            {presentation.label}
          </span>
          <button
            onClick={onCheck}
            disabled={busy}
            className="rounded border border-slate-700 px-2 py-1 text-xs text-slate-300 hover:border-slate-600 disabled:opacity-50"
          >
            {busy ? 'Checking…' : 'Check now'}
          </button>
          <button
            onClick={async () => {
              await deleteConnection(connection.id);
              onChanged();
            }}
            aria-label="Delete connection"
            className="rounded border border-slate-700 p-1 text-slate-400 hover:border-rose-500/40 hover:text-rose-300"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-1 text-[11px] text-slate-400 sm:grid-cols-3">
        <div>
          <dt className="inline text-slate-500">Last checked: </dt>
          <dd className="inline">
            {connection.health.State === 'NEVER_CHECKED'
              ? 'never'
              : new Date(connection.health.CheckedAt).toLocaleString()}
          </dd>
        </div>
        <div>
          <dt className="inline text-slate-500">Last used: </dt>
          <dd className="inline">
            {connection.lastUsedAt ? new Date(connection.lastUsedAt).toLocaleString() : 'never'}
          </dd>
        </div>
        <div>
          <dt className="inline text-slate-500">Rate limit: </dt>
          <dd className="inline">{connection.maxPerMinute}/min</dd>
        </div>
      </dl>

      {connection.health.State === 'FAILED' && connection.health.Detail && (
        <p className="mt-2 text-[11px] text-rose-300">
          {connection.health.ErrorCategory}: {connection.health.Detail}
        </p>
      )}

      {connection.conformanceServer && (
        <p className="mt-2 text-[11px] text-slate-500">
          Driver verified against {connection.conformanceServer}
          {connection.conformanceCommit ? ` (commit ${connection.conformanceCommit})` : ''}
        </p>
      )}

      {(connection.weakSecrets?.length ?? 0) > 0 && (
        <p className="mt-2 flex items-start gap-1.5 text-[11px] text-amber-200">
          <AlertTriangle className="mt-0.5 h-3 w-3 shrink-0" />
          {connection.weakSecrets!.join(', ')} {connection.weakSecrets!.length === 1 ? 'is' : 'are'}{' '}
          shorter than this platform's minimum. The credential is stored sealed; ask the
          database owner to lengthen it.
        </p>
      )}

      <div className="mt-3 flex flex-wrap items-center gap-2">
        {connection.secretsConfigured.map((field) => (
          <span
            key={field}
            className="flex items-center gap-1 rounded border border-slate-700 px-2 py-0.5 text-[11px] text-slate-300"
          >
            <KeyRound className="h-3 w-3 text-amber-300" />
            {field}
            <button
              onClick={() => setReplacing(field)}
              className="ml-1 text-sky-400 hover:text-sky-300"
            >
              replace
            </button>
          </span>
        ))}
      </div>

      {replacing && (
        <div className="mt-3 rounded border border-slate-700 p-3">
          <label className="text-[11px] text-slate-300">
            New value for {replacing}. The current one cannot be shown — nothing can read it.
          </label>
          <div className="mt-2 flex gap-2">
            <input
              type="password"
              autoComplete="new-password"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              className="flex-1 rounded border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-slate-100"
            />
            <button
              onClick={() => void submitReplacement()}
              className="rounded bg-sky-600 px-3 py-2 text-sm text-white hover:bg-sky-500"
            >
              Replace
            </button>
            <button
              onClick={() => {
                setValue('');
                setReplacing('');
              }}
              className="rounded border border-slate-700 px-3 py-2 text-sm text-slate-300"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {message && <p className="mt-2 text-[11px] text-slate-400">{message}</p>}
    </li>
  );
};
