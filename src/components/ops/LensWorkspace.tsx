import React, { useMemo, useState } from 'react';
import {
  ArrowRight,
  BarChart3,
  GitBranch,
  LineChart,
  LockKeyhole,
  Search,
  ShieldCheck,
} from 'lucide-react';
import {
  addLensInvestigationNode,
  createLensInvestigation,
  runLensQuery,
} from '../../api/endpoints';
import type {
  LensInvestigationNode,
  LensQueryIntent,
  LensQueryResult,
} from '../../api/types';

const startOfWindow = (days: number): { start: string; end: string } => {
  const end = new Date();
  const start = new Date(end.getTime() - days * 24 * 60 * 60 * 1000);
  return { start: start.toISOString(), end: end.toISOString() };
};

const PRESETS: Array<{ label: string; question: string; intent: () => LensQueryIntent }> = [
  {
    label: 'Return trend',
    question: 'Why did ACH return activity change this month?',
    intent: () => ({
      schema_version: '1.0',
      dataset_id: 'ach_return_intelligence',
      time_range: startOfWindow(30),
      metrics: ['return_count'],
      dimensions: ['day', 'return_code'],
      order_by: [{ field: 'day', direction: 'ASC' }],
      limit: 300,
    }),
  },
  {
    label: 'Partner concentration',
    question: 'Which partners are driving ACH returns?',
    intent: () => ({
      schema_version: '1.0',
      dataset_id: 'ach_return_intelligence',
      time_range: startOfWindow(30),
      metrics: ['return_count', 'associated_amount_cents'],
      dimensions: ['partner_id'],
      order_by: [{ field: 'return_count', direction: 'DESC' }],
      limit: 50,
    }),
  },
  {
    label: 'Blocking controls',
    question: 'Where are deterministic blocking findings clustering?',
    intent: () => ({
      schema_version: '1.0',
      dataset_id: 'validation_findings',
      time_range: startOfWindow(30),
      metrics: ['finding_count', 'blocking_count'],
      dimensions: ['code'],
      order_by: [{ field: 'blocking_count', direction: 'DESC' }],
      limit: 50,
    }),
  },
  {
    label: 'Agent latency',
    question: 'How is governed agent runtime latency trending?',
    intent: () => ({
      schema_version: '1.0',
      dataset_id: 'agent_operations',
      time_range: startOfWindow(30),
      metrics: ['run_count', 'avg_latency_ms'],
      dimensions: ['day', 'agent_name'],
      order_by: [{ field: 'day', direction: 'ASC' }],
      limit: 300,
    }),
  },
];

export const LensWorkspace: React.FC = () => {
  const [activePreset, setActivePreset] = useState(0);
  const [result, setResult] = useState<LensQueryResult | null>(null);
  const [investigationId, setInvestigationId] = useState<string | null>(null);
  const [nodes, setNodes] = useState<LensInvestigationNode[]>([]);
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [nodeResults, setNodeResults] = useState<Record<string, LensQueryResult>>({});
  const [nodePresetIndexes, setNodePresetIndexes] = useState<Record<string, number>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const current = PRESETS[activePreset];

  const run = async (presetIndex: number, parentNodeId?: string) => {
    setBusy(true);
    setError('');
    const preset = PRESETS[presetIndex];
    const intent = preset.intent();

    let invId = investigationId;
    if (!invId) {
      const created = await createLensInvestigation('ACH operations intelligence');
      if (created.state !== 'ok') {
        setError(created.error);
        setBusy(false);
        return;
      }
      invId = created.data.id;
      setInvestigationId(invId);
    }

    const query = await runLensQuery(intent);
    if (query.state !== 'ok') {
      setError(query.error);
      setBusy(false);
      return;
    }

    const persisted = await addLensInvestigationNode(invId, preset.question, intent, parentNodeId);
    if (persisted.state !== 'ok') {
      setError(persisted.error);
      setBusy(false);
      return;
    }

    // The server executes the semantic query again when journaling the node.
    // Matching hashes prove the UI did not persist a different result than it displayed.
    if (persisted.data.result_hash !== query.data.provenance.result_hash) {
      setError('Lens result changed between display and append-only journal persistence. Re-run the investigation.');
      setBusy(false);
      return;
    }

    setActivePreset(presetIndex);
    setResult(query.data);
    setNodes((previous) => [...previous, persisted.data]);
    setNodeResults((previous) => ({ ...previous, [persisted.data.id]: query.data }));
    setNodePresetIndexes((previous) => ({ ...previous, [persisted.data.id]: presetIndex }));
    setSelectedNode(persisted.data.id);
    setBusy(false);
  };

  return (
    <section className="lens-shell" aria-label="SentinelFlow Lens governed analytics workspace">
      <header className="lens-brief">
        <div>
          <div className="lens-kicker"><Search className="h-3.5 w-3.5" aria-hidden /> Governed analytics · advisory A1</div>
          <h2>Turn financial exceptions into evidence-backed investigations.</h2>
          <p>Lens executes typed semantic intents against tenant-scoped, allowlisted datasets. It can explain financial truth; it cannot create or authorize it.</p>
        </div>
        <div className="lens-law" aria-label="Lens authority law">
          <LockKeyhole className="h-4 w-4" aria-hidden />
          <span>Analytics ≠ financial authority</span>
        </div>
      </header>

      <div className="lens-question-strip" role="group" aria-label="Investigation questions">
        {PRESETS.map((preset, index) => (
          <button
            key={preset.label}
            type="button"
            onClick={() => run(index, selectedNode ?? undefined)}
            disabled={busy}
            className={activePreset === index ? 'lens-question-active' : 'lens-question'}
          >
            <span>{preset.label}</span>
            <ArrowRight className="h-3.5 w-3.5" aria-hidden />
          </button>
        ))}
      </div>

      {error && <div className="lens-error" role="alert">{error}</div>}

      {!result ? (
        <div className="lens-empty">
          <div>
            <p className="lens-empty-label">Start with a governed question</p>
            <h3>{current.question}</h3>
            <p>The first query creates an append-only Investigation Thread. Synthetic demo observations remain visibly non-authoritative.</p>
            <button className="primary-action mt-5" type="button" disabled={busy} onClick={() => run(activePreset)}>
              {busy ? 'Running governed query…' : 'Run investigation'}
            </button>
          </div>
          <ol>
            <li><span>01</span> semantic intent only</li>
            <li><span>02</span> tenant scope injected server-side</li>
            <li><span>03</span> allowlisted compiler + bounded query</li>
            <li><span>04</span> result + query hashes journaled</li>
          </ol>
        </div>
      ) : (
        <div className="lens-workspace-grid">
          <aside className="lens-thread" aria-label="Investigation Thread">
            <div className="lens-panel-heading">
              <div><GitBranch className="h-4 w-4" aria-hidden /><span>Investigation Thread</span></div>
              <span>{nodes.length} nodes</span>
            </div>
            <div className="lens-thread-list">
              {nodes.map((node, index) => (
                <button
                  key={node.id}
                  type="button"
                  className={selectedNode === node.id ? 'lens-thread-node lens-thread-node-active' : 'lens-thread-node'}
                  onClick={() => {
                    setSelectedNode(node.id);
                    if (nodeResults[node.id]) setResult(nodeResults[node.id]);
                    if (nodePresetIndexes[node.id] !== undefined) setActivePreset(nodePresetIndexes[node.id]);
                  }}
                >
                  <span className="lens-node-index">{String(index + 1).padStart(2, '0')}</span>
                  <span>
                    <strong>{node.question}</strong>
                    <small>{node.query_hash.slice(0, 10)}…</small>
                  </span>
                </button>
              ))}
            </div>
          </aside>

          <main className="lens-canvas">
            <div className="lens-panel-heading">
              <div>{result.chart.kind === 'line' ? <LineChart className="h-4 w-4" aria-hidden /> : <BarChart3 className="h-4 w-4" aria-hidden />}<span>Analytical canvas</span></div>
              <span>{result.provenance.row_count} rows</span>
            </div>
            <div className="lens-finding">
              <p>{current.question}</p>
              <h3>{result.chart.title}</h3>
              <span>{result.provenance.source_class.replace(/_/g, ' ')}</span>
            </div>
            <LensChart result={result} />
            <div className="lens-branch-row">
              <span>Branch this investigation:</span>
              {PRESETS.filter((_, i) => i !== activePreset).slice(0, 3).map((preset) => {
                const index = PRESETS.indexOf(preset);
                return <button key={preset.label} type="button" disabled={busy} onClick={() => run(index, selectedNode ?? undefined)}>{preset.label}</button>;
              })}
            </div>
          </main>

          <aside className="lens-provenance" aria-label="Lens provenance">
            <div className="lens-panel-heading">
              <div><ShieldCheck className="h-4 w-4" aria-hidden /><span>Provenance</span></div>
            </div>
            <ProvenanceFact label="Source class" value={result.provenance.source_class} />
            <ProvenanceFact label="Dataset" value={result.dataset_id} />
            <ProvenanceFact label="Query hash" value={result.provenance.query_hash} mono />
            <ProvenanceFact label="Result hash" value={result.provenance.result_hash} mono />
            <ProvenanceFact label="Authority" value={result.provenance.advisory_only ? 'ADVISORY ONLY' : 'UNKNOWN'} />
            <div className="lens-evidence-block">
              <p>Authoritative evidence refs</p>
              {result.provenance.evidence_refs.length === 0 ? (
                <span>None. Synthetic observations cannot satisfy a financial control.</span>
              ) : result.provenance.evidence_refs.map((ref) => <code key={ref}>{ref}</code>)}
            </div>
          </aside>
        </div>
      )}
    </section>
  );
};

const ProvenanceFact: React.FC<{ label: string; value: string; mono?: boolean }> = ({ label, value, mono }) => (
  <div className="lens-provenance-fact">
    <span>{label}</span>
    <strong className={mono ? 'font-mono' : ''}>{mono && value.length > 22 ? `${value.slice(0, 22)}…` : value.replace(/_/g, ' ')}</strong>
  </div>
);

const LensChart: React.FC<{ result: LensQueryResult }> = ({ result }) => {
  const rows = result.rows;
  const xKey = result.chart.x;
  const yKey = result.chart.y;
  const seriesKey = result.chart.series;

  const chartData = useMemo(() => {
    if (!xKey || !yKey) return null;
    const values = rows.map((row) => Number(row[yKey] ?? 0));
    const max = Math.max(1, ...values);
    if (result.chart.kind === 'bar') {
      return rows.slice(0, 16).map((row) => ({
        label: String(row[xKey] ?? '—'),
        value: Number(row[yKey] ?? 0),
        pct: Number(row[yKey] ?? 0) / max,
      }));
    }
    const groups = new Map<string, Array<{ x: string; y: number }>>();
    for (const row of rows) {
      const key = seriesKey ? String(row[seriesKey] ?? 'all') : 'all';
      const group = groups.get(key) ?? [];
      group.push({ x: String(row[xKey] ?? ''), y: Number(row[yKey] ?? 0) });
      groups.set(key, group);
    }
    return { groups, max };
  }, [rows, result.chart.kind, xKey, yKey, seriesKey]);

  if (!xKey || !yKey || !chartData) return <LensTable result={result} />;

  if (Array.isArray(chartData)) {
    return (
      <div className="lens-bars" aria-label={`${result.chart.title} bar chart`}>
        {chartData.map((item) => (
          <div key={item.label} className="lens-bar-row">
            <span title={item.label}>{item.label}</span>
            <div><i style={{ transform: `scaleX(${Math.max(0.02, item.pct)})` }} /></div>
            <strong>{formatValue(item.value, yKey)}</strong>
          </div>
        ))}
      </div>
    );
  }

  const width = 760;
  const height = 260;
  const padding = 28;
  const allDates = Array.from(new Set(rows.map((row) => String(row[xKey] ?? '')))).sort();
  const xPos = (x: string) => padding + (allDates.length <= 1 ? 0 : (allDates.indexOf(x) / (allDates.length - 1)) * (width - padding * 2));
  const yPos = (y: number) => height - padding - (y / chartData.max) * (height - padding * 2);

  return (
    <div className="lens-line-wrap">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${result.chart.title} line chart`}>
        {[0.25, 0.5, 0.75, 1].map((p) => <line key={p} x1={padding} x2={width - padding} y1={height - padding - p * (height - padding * 2)} y2={height - padding - p * (height - padding * 2)} className="lens-gridline" />)}
        {Array.from(chartData.groups.entries()).map(([name, points], seriesIndex) => {
          const path = points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${xPos(point.x)} ${yPos(point.y)}`).join(' ');
          return <path key={name} d={path} className={`lens-series lens-series-${seriesIndex % 4}`} />;
        })}
      </svg>
      <div className="lens-legend">
        {Array.from(chartData.groups.keys()).slice(0, 8).map((name, index) => <span key={name}><i className={`lens-legend-dot lens-series-bg-${index % 4}`} />{name}</span>)}
      </div>
    </div>
  );
};

const LensTable: React.FC<{ result: LensQueryResult }> = ({ result }) => (
  <div className="overflow-x-auto">
    <table className="lens-table">
      <thead><tr>{result.columns.map((column) => <th key={column}>{column.replace(/_/g, ' ')}</th>)}</tr></thead>
      <tbody>{result.rows.slice(0, 30).map((row, index) => <tr key={index}>{result.columns.map((column) => <td key={column}>{String(row[column] ?? '—')}</td>)}</tr>)}</tbody>
    </table>
  </div>
);

function formatValue(value: number, key: string): string {
  if (key.endsWith('_cents')) return `$${(value / 100).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
  if (key.endsWith('_ms')) return `${Math.round(value)} ms`;
  return value.toLocaleString(undefined, { maximumFractionDigits: 1 });
}
