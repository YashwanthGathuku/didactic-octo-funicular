import React, { useState } from 'react';
import { 
  Database, 
  X, 
  Play, 
  Download, 
  Copy, 
  Check, 
  ShieldCheck, 
  Clock, 
  AlertCircle,
  Table as TableIcon
} from 'lucide-react';

interface SqlConsoleModalProps {
  onClose: () => void;
}

const PRESET_QUERIES = [
  {
    name: 'Quarantined Files & Violations',
    sql: `SELECT f.id, f.filename, f.status, f.size_bytes, v.code, v.severity, v.description 
FROM file_instances f 
LEFT JOIN validation_findings v ON f.id = v.file_id 
WHERE f.status = 'QUARANTINED' 
ORDER BY f.id DESC LIMIT 10;`
  },
  {
    name: 'Cryptographic Merkle Audit Ledger',
    sql: `SELECT id, sequence_number, event_type, actor, previous_hash, event_hash, created_at 
FROM audit_events 
ORDER BY sequence_number DESC LIMIT 15;`
  },
  {
    name: 'Counterparty Master & SLA Deadlines',
    sql: `SELECT p.name AS partner_name, c.format, c.cutoff_time_utc, c.grace_period_minutes, c.is_active 
FROM partners p 
JOIN file_contracts c ON p.id = c.partner_id;`
  },
  {
    name: 'Outbound Webhook Subscriptions',
    sql: `SELECT id, url, events, status, created_at 
FROM webhook_subscriptions;`
  }
];

export const SqlConsoleModal: React.FC<SqlConsoleModalProps> = ({ onClose }) => {
  const [query, setQuery] = useState<string>(PRESET_QUERIES[0].sql);
  const [results, setResults] = useState<{ columns: string[]; rows: any[][]; durationMs: number; rowCount: number } | null>({
    columns: ['id', 'filename', 'status', 'size_bytes', 'code', 'severity', 'description'],
    rows: [
      [1, 'MERIDIAN_ACH_COMMERCIAL_20260814_1645.txt', 'QUARANTINED', 648, 'ACH_ERR_0802_HASH_MISMATCH', 'FATAL', 'Calculated entry hash does not match batch trailer']
    ],
    durationMs: 0.42,
    rowCount: 1
  });
  const [isRunning, setIsRunning] = useState<boolean>(false);
  const [error, setError] = useState<string>('');
  const [copied, setCopied] = useState<boolean>(false);

  const handleRunQuery = async () => {
    setIsRunning(true);
    setError('');
    try {
      const res = await fetch('http://localhost:8080/api/v1/sql/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query })
      });

      if (!res.ok) {
        const errText = await res.text();
        setError(errText || 'SQL Execution Error');
        setResults(null);
      } else {
        const data = await res.json();
        setResults(data);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to connect to Go Gateway SQL engine');
    } finally {
      setIsRunning(false);
    }
  };

  const handleExportCsv = () => {
    if (!results || results.rows.length === 0) return;
    const header = results.columns.join(',');
    const rows = results.rows.map(r => r.map(c => `"${String(c).replace(/"/g, '""')}"`).join(','));
    const csvContent = 'data:text/csv;charset=utf-8,' + [header, ...rows].join('\n');
    const encodedUri = encodeURI(csvContent);
    const link = document.createElement('a');
    link.setAttribute('href', encodedUri);
    link.setAttribute('download', `SENTINEL_SQL_EXPORT_${Date.now()}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const handleCopyQuery = () => {
    navigator.clipboard.writeText(query);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(10, 15, 29, 0.85)',
      backdropFilter: 'blur(8px)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 100,
      padding: '24px'
    }}>
      <div style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-subtle)',
        borderRadius: '12px',
        width: '100%',
        maxWidth: '1150px',
        maxHeight: '92vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.5)'
      }}>
        {/* Header */}
        <div style={{
          padding: '18px 24px',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'rgba(14, 20, 34, 0.6)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{
              width: '36px',
              height: '36px',
              borderRadius: '8px',
              background: 'var(--accent-cyan-dim)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: '1px solid rgba(6, 182, 212, 0.4)'
            }}>
              <Database size={20} color="var(--accent-cyan)" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Institutional SQL Audit Console
                </h3>
                <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>Read-Only Replica</span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Directly query SQLite metadata, dead-letter tables, and cryptographic Merkle chains
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <button
              onClick={handleExportCsv}
              disabled={!results || results.rows.length === 0}
              className="btn btn-secondary"
              style={{ fontSize: '0.75rem', padding: '6px 12px', display: 'flex', alignItems: 'center', gap: '6px' }}
            >
              <Download size={14} />
              <span>Export CSV</span>
            </button>
            <button
              onClick={onClose}
              style={{
                background: 'transparent',
                border: 'none',
                color: 'var(--text-muted)',
                cursor: 'pointer',
                padding: '4px'
              }}
            >
              <X size={20} />
            </button>
          </div>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Query Presets Toolbar */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
            <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 600 }}>Preset Queries:</span>
            {PRESET_QUERIES.map((preset, idx) => (
              <button
                key={idx}
                onClick={() => setQuery(preset.sql)}
                style={{
                  background: 'rgba(255, 255, 255, 0.05)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '4px',
                  padding: '4px 10px',
                  fontSize: '0.7rem',
                  color: 'var(--text-secondary)',
                  cursor: 'pointer',
                  fontWeight: 500
                }}
              >
                {preset.name}
              </button>
            ))}
          </div>

          {/* SQL Input Area */}
          <div style={{
            background: 'var(--bg-primary)',
            border: '1px solid var(--border-subtle)',
            borderRadius: '8px',
            padding: '14px',
            display: 'flex',
            flexDirection: 'column',
            gap: '10px'
          }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'flex', alignItems: 'center', gap: '6px' }}>
                <ShieldCheck size={14} color="var(--accent-emerald)" /> SQL Editor (SELECT / EXPLAIN Guarded)
              </span>
              <button
                onClick={handleCopyQuery}
                style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '4px', fontSize: '0.7rem' }}
              >
                {copied ? <Check size={12} color="var(--accent-emerald)" /> : <Copy size={12} />}
                <span>{copied ? 'Copied' : 'Copy'}</span>
              </button>
            </div>

            <textarea
              rows={4}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="font-mono"
              style={{
                width: '100%',
                background: 'rgba(0, 0, 0, 0.4)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                padding: '10px 12px',
                color: '#F8FAFC',
                fontSize: '0.8125rem',
                lineHeight: 1.4
              }}
            />

            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                Protected with strict regex token filtering. Mutating statements are blocked at runtime.
              </div>
              <button
                onClick={handleRunQuery}
                disabled={isRunning}
                className="btn btn-primary"
                style={{ fontSize: '0.75rem', padding: '6px 16px', display: 'flex', alignItems: 'center', gap: '6px' }}
              >
                <Play size={14} />
                <span>{isRunning ? 'Executing Query...' : 'Execute Query'}</span>
              </button>
            </div>
          </div>

          {/* Error Message */}
          {error && (
            <div style={{
              padding: '12px 16px',
              borderRadius: '6px',
              background: 'rgba(239, 68, 68, 0.1)',
              border: '1px solid rgba(239, 68, 68, 0.3)',
              color: 'var(--accent-crimson)',
              fontSize: '0.75rem',
              display: 'flex',
              alignItems: 'center',
              gap: '8px'
            }}>
              <AlertCircle size={16} />
              <span>{error}</span>
            </div>
          )}

          {/* Results Table */}
          {results && (
            <div style={{
              background: 'var(--bg-primary)',
              border: '1px solid var(--border-subtle)',
              borderRadius: '8px',
              padding: '16px',
              display: 'flex',
              flexDirection: 'column',
              gap: '10px'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <TableIcon size={16} color="var(--accent-cyan)" />
                  <span style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    Query Results ({results.rowCount} {results.rowCount === 1 ? 'row' : 'rows'})
                  </span>
                </div>
                <span className="font-mono" style={{ fontSize: '0.7rem', color: 'var(--accent-emerald)', display: 'flex', alignItems: 'center', gap: '4px' }}>
                  <Clock size={12} /> Execution Time: {results.durationMs.toFixed(2)} ms
                </span>
              </div>

              <div style={{ overflowX: 'auto', maxHeight: '300px' }}>
                <table style={{ width: '100%', fontSize: '0.75rem', borderCollapse: 'collapse', textAlign: 'left' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border-subtle)', color: 'var(--text-muted)', background: 'rgba(0,0,0,0.2)' }}>
                      {results.columns.map((col, idx) => (
                        <th key={idx} style={{ padding: '8px 12px', fontWeight: 600 }}>{col}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {results.rows.map((row, rIdx) => (
                      <tr 
                        key={rIdx} 
                        style={{ 
                          borderBottom: '1px solid rgba(255, 255, 255, 0.03)',
                          background: rIdx % 2 === 0 ? 'rgba(255, 255, 255, 0.01)' : 'transparent'
                        }}
                      >
                        {row.map((cell, cIdx) => (
                          <td 
                            key={cIdx} 
                            className="font-mono"
                            style={{ 
                              padding: '8px 12px', 
                              color: typeof cell === 'number' ? 'var(--accent-cyan)' : '#F8FAFC',
                              whiteSpace: 'nowrap'
                            }}
                          >
                            {cell === null ? '<NULL>' : String(cell)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
