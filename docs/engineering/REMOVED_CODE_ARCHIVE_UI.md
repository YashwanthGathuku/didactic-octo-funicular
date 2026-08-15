# Removed Code Archive — Operations UI (React)

**Created by:** Prompt 01 (Truth reset and scope reduction)
**Source commit:** `cb09694`
**Date:** 14 August 2026

## What this file is

A verbatim, **non-executable** archive of every React component deleted from the operations UI
during Prompt 01. Companion to `REMOVED_CODE_ARCHIVE.md`, which holds the Go backend.

- Nothing here is bundled, imported, routed, or reachable. Vite never sees it.
- This is a reference document, not a feature flag and not a hidden screen.
- The authoritative recovery mechanism is still git history at commit `cb09694`.

## Why these were removed

Every component below rendered state that no server could substantiate. All 17 modals shipped in
the production bundle with no demo gating and no `DEMO DATA` banner anywhere in the tree, so
synthetic and real state were visually indistinguishable to an operator. Each entry states its
specific reason; the evidence is in `docs/engineering/CURRENT_STATE.md` §8.

## What survives

`Header`, `SlaBoard`, `FileInspectorModal`, `AiAnalystPanel`, `AuditLedgerModal`,
`UploadModal`, `ContractConfigModal` and `FileDiffModal` remain. They are rebuilt onto
authenticated server data in Prompt 12.

---

## `src/components/IntegrationHubModal.tsx`

**Lines:** 615  ·  **Reason for removal:** Front end for the deleted Integration Hub. Displayed four permanently HEALTHY connections, invented latencies, and vault key references for systems that do not exist.

```tsx
import React, { useState } from 'react';
import { 
  Network, 
  X, 
  Database, 
  Server, 
  ShieldCheck, 
  Radio, 
  HardDrive, 
  GitFork, 
  Eye, 
  Lock, 
  Terminal, 
  Copy, 
  Check, 
  ArrowRight
} from 'lucide-react';

interface IntegrationHubModalProps {
  onClose: () => void;
}

interface ConnectionItem {
  id: string;
  name: string;
  type: 'SFTP_SSH' | 'POSTGRESQL' | 'REST_API' | 'S3_OBJECT' | 'SMB_NFS';
  hostAlias: string;
  port: number;
  serviceAccount: string;
  authMethod: string;
  fingerprint?: string;
  database?: string;
  vaultKey: string;
  secretAgeDays: number;
  status: 'HEALTHY' | 'DEGRADED';
  latencyMs: number;
  edgeAgentId: string;
}

const CONNECTIONS: ConnectionItem[] = [
  {
    id: 'CONN-SFTP-MERIDIAN',
    name: 'Meridian Treasury SFTP Drop',
    type: 'SFTP_SSH',
    hostAlias: 'sftp-prod-us-east.meridian.internal',
    port: 2222,
    serviceAccount: 'svc_sentinel_ingest',
    authMethod: 'SSH_ED25519_KEY',
    fingerprint: 'SHA256:4a3F9cKz8eL1mQxWpTvBn9Rt6sDuFhJ2kLm8qWeRt7y',
    vaultKey: 'vault://prod/treasury/sftp_agent_ed25519',
    secretAgeDays: 2,
    status: 'HEALTHY',
    latencyMs: 14.2,
    edgeAgentId: 'EDGE-AGENT-MERIDIAN-VPC-01'
  },
  {
    id: 'CONN-PG-TREASURY',
    name: 'Core Treasury PostgreSQL Cluster',
    type: 'POSTGRESQL',
    hostAlias: 'pg-aurora-cluster.meridian.internal',
    port: 5432,
    serviceAccount: 'svc_sentinel_readonly',
    authMethod: 'mTLS_X509_CERTIFICATE',
    database: 'treasury_settlement_db',
    vaultKey: 'vault://prod/postgres/tls_cert_bundle',
    secretAgeDays: 5,
    status: 'HEALTHY',
    latencyMs: 3.8,
    edgeAgentId: 'EDGE-AGENT-MERIDIAN-VPC-01'
  },
  {
    id: 'CONN-REST-SETTLEMENT',
    name: 'Apex Clearing Settlement REST Gateway',
    type: 'REST_API',
    hostAlias: 'api.apexclearing.internal/v2',
    port: 443,
    serviceAccount: 'svc_sentinel_settlement_oauth',
    authMethod: 'OAUTH2_MUTUAL_TLS',
    vaultKey: 'vault://prod/apex/oauth_client_secret',
    secretAgeDays: 8,
    status: 'HEALTHY',
    latencyMs: 24.5,
    edgeAgentId: 'EDGE-AGENT-MERIDIAN-VPC-01'
  },
  {
    id: 'CONN-S3-ARCHIVE',
    name: 'Immutable SEC 17a-4 S3 WORM Archive',
    type: 'S3_OBJECT',
    hostAlias: 's3.us-east-1.amazonaws.com/meridian-sec17a4-worm',
    port: 443,
    serviceAccount: 'arn:aws:iam::123456789012:role/SentinelEvidenceArchiver',
    authMethod: 'IAM_INSTANCE_PROFILE',
    vaultKey: 'aws://iam/role/SentinelEvidenceArchiver',
    secretAgeDays: 1,
    status: 'HEALTHY',
    latencyMs: 38.1,
    edgeAgentId: 'EDGE-AGENT-MERIDIAN-VPC-01'
  }
];

const CATALOG_ASSETS = [
  {
    id: 'ASSET-001',
    connectionName: 'Meridian Treasury SFTP Drop',
    name: 'sftp://meridian.internal/inbound/ach/*.ach',
    type: 'FILE_DIRECTORY',
    classification: 'RESTRICTED',
    owner: 'Treasury Operations',
    rowCount: 10500,
    freshnessSlaMin: 60,
    fields: [
      { name: 'RecordType', type: 'CHAR(1)', isMasked: false },
      { name: 'RoutingNumber', type: 'CHAR(9)', isMasked: true },
      { name: 'AccountNumber', type: 'VARCHAR(17)', isMasked: true },
      { name: 'AmountInCents', type: 'NUMERIC(10,0)', isMasked: false }
    ]
  },
  {
    id: 'ASSET-002',
    connectionName: 'Core Treasury PostgreSQL Cluster',
    name: 'treasury_settlement_db.public.settlement_batches',
    type: 'DATABASE_TABLE',
    classification: 'CONFIDENTIAL',
    owner: 'Settlement Platform Engineering',
    rowCount: 482900,
    freshnessSlaMin: 120,
    fields: [
      { name: 'batch_id', type: 'UUID', isMasked: false },
      { name: 'file_hash', type: 'VARCHAR(64)', isMasked: false },
      { name: 'total_credits', type: 'NUMERIC(18,2)', isMasked: false },
      { name: 'total_debits', type: 'NUMERIC(18,2)', isMasked: false }
    ]
  },
  {
    id: 'ASSET-003',
    connectionName: 'Apex Clearing Settlement REST Gateway',
    name: 'api.apexclearing.internal/v2/settlements/reconcile',
    type: 'API_ENDPOINT',
    classification: 'RESTRICTED',
    owner: 'Apex Interbank Service',
    rowCount: 1420,
    freshnessSlaMin: 30,
    fields: [
      { name: 'settlementId', type: 'STRING', isMasked: false },
      { name: 'counterpartyRouting', type: 'STRING', isMasked: true },
      { name: 'acknowledgedAmount', type: 'FLOAT64', isMasked: false }
    ]
  }
];

export const IntegrationHubModal: React.FC<IntegrationHubModalProps> = ({ onClose }) => {
  const [activeTab, setActiveTab] = useState<'FLEET' | 'CATALOG' | 'LINEAGE' | 'EDGE_DEPLOY'>('FLEET');
  const [selectedAsset, setSelectedAsset] = useState<any | null>(null);
  const [copiedCmd, setCopiedCmd] = useState<boolean>(false);

  const edgeDeployCmd = `./edge-agent --control-plane https://sentinel.meridian.internal:8080 --agent-id EDGE-AGENT-MERIDIAN-VPC-01 --interval 15`;

  const handleCopyCmd = () => {
    navigator.clipboard.writeText(edgeDeployCmd);
    setCopiedCmd(true);
    setTimeout(() => setCopiedCmd(false), 2000);
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
        maxWidth: '1200px',
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
              <Network size={20} color="var(--accent-cyan)" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Secure Integration Hub & Edge Catalog
                </h3>
                <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>Outbound mTLS Only</span>
                <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>OWASP Zero-Trust</span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Customer edge connectors, decoupled secrets management, unified data catalog & lineage
              </p>
            </div>
          </div>

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

        {/* Tab Navigation */}
        <div style={{
          display: 'flex',
          borderBottom: '1px solid var(--border-subtle)',
          padding: '0 24px',
          background: 'rgba(14, 20, 34, 0.4)'
        }}>
          <button
            onClick={() => setActiveTab('FLEET')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'FLEET' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'FLEET' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Server size={14} />
            <span>Connection Fleet ({CONNECTIONS.length})</span>
          </button>

          <button
            onClick={() => setActiveTab('CATALOG')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'CATALOG' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'CATALOG' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Database size={14} />
            <span>Unified Data Catalog ({CATALOG_ASSETS.length})</span>
          </button>

          <button
            onClick={() => setActiveTab('LINEAGE')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'LINEAGE' ? '2px solid var(--accent-emerald)' : '2px solid transparent',
              color: activeTab === 'LINEAGE' ? 'var(--accent-emerald)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <GitFork size={14} />
            <span>Visual Data Lineage DAG</span>
          </button>

          <button
            onClick={() => setActiveTab('EDGE_DEPLOY')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'EDGE_DEPLOY' ? '2px solid var(--accent-amber)' : '2px solid transparent',
              color: activeTab === 'EDGE_DEPLOY' ? 'var(--accent-amber)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Terminal size={14} />
            <span>Edge Agent Deployment</span>
          </button>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {/* 1. Connection Fleet View */}
          {activeTab === 'FLEET' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                background: 'rgba(6, 182, 212, 0.08)',
                border: '1px solid rgba(6, 182, 212, 0.25)',
                borderRadius: '8px',
                padding: '12px 16px',
                display: 'flex',
                alignItems: 'center',
                gap: '10px'
              }}>
                <ShieldCheck size={18} color="var(--accent-cyan)" />
                <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                  <strong>OWASP Decoupled Secrets Policy Active:</strong> Raw passwords, private SSH keys, and connection credentials are NEVER stored or rendered. Connectors authenticate via decoupled HashiCorp Vault / AWS IAM role pointers.
                </span>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px' }}>
                {CONNECTIONS.map(conn => (
                  <div
                    key={conn.id}
                    style={{
                      background: 'var(--bg-primary)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '8px',
                      padding: '16px',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '10px'
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                        {conn.type === 'SFTP_SSH' && <HardDrive size={16} color="var(--accent-cyan)" />}
                        {conn.type === 'POSTGRESQL' && <Database size={16} color="var(--accent-emerald)" />}
                        {conn.type === 'REST_API' && <Radio size={16} color="var(--accent-amber)" />}
                        {conn.type === 'S3_OBJECT' && <Server size={16} color="var(--accent-cyan)" />}
                        <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>{conn.name}</h4>
                      </div>
                      <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>{conn.status}</span>
                    </div>

                    <div className="font-mono" style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'flex', flexDirection: 'column', gap: '4px' }}>
                      <div><strong>Host Alias:</strong> {conn.hostAlias}:{conn.port}</div>
                      <div><strong>Service Account:</strong> {conn.serviceAccount}</div>
                      <div><strong>Auth Method:</strong> {conn.authMethod}</div>
                      {conn.fingerprint && (
                        <div style={{ color: 'var(--accent-cyan)' }}><strong>Fingerprint:</strong> {conn.fingerprint.substring(0, 32)}...</div>
                      )}
                      <div><strong>Vault Secret Key:</strong> <span style={{ color: 'var(--accent-amber)' }}>{conn.vaultKey}</span> (Age: {conn.secretAgeDays}d)</div>
                      <div><strong>Edge Agent:</strong> {conn.edgeAgentId} ({conn.latencyMs} ms)</div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* 2. Unified Data Catalog */}
          {activeTab === 'CATALOG' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '16px'
              }}>
                <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
                  Discovered Financial Assets & Schemas
                </h4>

                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', fontSize: '0.75rem', borderCollapse: 'collapse', textAlign: 'left' }}>
                    <thead>
                      <tr style={{ borderBottom: '1px solid var(--border-subtle)', color: 'var(--text-muted)' }}>
                        <th style={{ padding: '8px' }}>Asset Identifier</th>
                        <th style={{ padding: '8px' }}>Connection</th>
                        <th style={{ padding: '8px' }}>Type</th>
                        <th style={{ padding: '8px' }}>Classification</th>
                        <th style={{ padding: '8px' }}>Records</th>
                        <th style={{ padding: '8px' }}>Freshness SLA</th>
                        <th style={{ padding: '8px', textAlign: 'right' }}>Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {CATALOG_ASSETS.map(asset => (
                        <tr key={asset.id} style={{ borderBottom: '1px solid rgba(255, 255, 255, 0.03)' }}>
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--accent-cyan)', fontWeight: 600 }}>{asset.name}</td>
                          <td style={{ padding: '8px', color: 'var(--text-secondary)' }}>{asset.connectionName}</td>
                          <td style={{ padding: '8px' }}>
                            <span className="badge badge-neutral" style={{ fontSize: '0.65rem' }}>{asset.type}</span>
                          </td>
                          <td style={{ padding: '8px' }}>
                            <span className="badge badge-crimson" style={{ fontSize: '0.65rem' }}>{asset.classification}</span>
                          </td>
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--text-primary)' }}>{asset.rowCount.toLocaleString()}</td>
                          <td style={{ padding: '8px', color: 'var(--accent-emerald)' }}>&lt; {asset.freshnessSlaMin}m</td>
                          <td style={{ padding: '8px', textAlign: 'right' }}>
                            <button
                              onClick={() => setSelectedAsset(asset)}
                              className="btn btn-secondary"
                              style={{ fontSize: '0.7rem', padding: '4px 10px', display: 'inline-flex', alignItems: 'center', gap: '4px' }}
                            >
                              <Eye size={12} />
                              <span>Inspect Schema</span>
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>

              {/* Schema Drawer */}
              {selectedAsset && (
                <div style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '8px',
                  padding: '16px'
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '12px' }}>
                    <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                      Schema Definitions: {selectedAsset.name}
                    </h4>
                    <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>PII Masking Active</span>
                  </div>

                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '10px' }}>
                    {selectedAsset.fields.map((f: any, idx: number) => (
                      <div key={idx} style={{
                        background: 'rgba(0,0,0,0.3)',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '6px',
                        padding: '10px'
                      }}>
                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                          <span className="font-mono" style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>{f.name}</span>
                          {f.isMasked && <Lock size={12} color="var(--accent-crimson)" />}
                        </div>
                        <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--accent-cyan)', marginTop: '4px' }}>{f.type}</div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* 3. Visual Data Lineage Map */}
          {activeTab === 'LINEAGE' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '24px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: '12px'
              }}>
                {/* Node 1: Inbound SFTP */}
                <div style={{
                  background: 'rgba(6, 182, 212, 0.1)',
                  border: '1px solid rgba(6, 182, 212, 0.4)',
                  borderRadius: '8px',
                  padding: '16px',
                  flex: 1,
                  textAlign: 'center'
                }}>
                  <HardDrive size={24} color="var(--accent-cyan)" style={{ margin: '0 auto 8px auto' }} />
                  <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>Partner SFTP Drop</div>
                  <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                    /inbound/ach/*.ach
                  </div>
                  <span className="badge badge-cyan" style={{ marginTop: '8px', fontSize: '0.6rem' }}>Source Ingress</span>
                </div>

                <ArrowRight size={24} color="var(--accent-cyan)" />

                {/* Node 2: Gateway Validator */}
                <div style={{
                  background: 'rgba(16, 185, 129, 0.1)',
                  border: '1px solid rgba(16, 185, 129, 0.4)',
                  borderRadius: '8px',
                  padding: '16px',
                  flex: 1,
                  textAlign: 'center'
                }}>
                  <ShieldCheck size={24} color="var(--accent-emerald)" style={{ margin: '0 auto 8px auto' }} />
                  <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>NACHA SIMD Validator</div>
                  <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                    296k rec/s + SHA-256 Chain
                  </div>
                  <span className="badge badge-emerald" style={{ marginTop: '8px', fontSize: '0.6rem' }}>Deterministic Gate</span>
                </div>

                <ArrowRight size={24} color="var(--accent-cyan)" />

                {/* Node 3: PostgreSQL Database */}
                <div style={{
                  background: 'rgba(2, 132, 199, 0.1)',
                  border: '1px solid rgba(2, 132, 199, 0.4)',
                  borderRadius: '8px',
                  padding: '16px',
                  flex: 1,
                  textAlign: 'center'
                }}>
                  <Database size={24} color="var(--accent-cyan)" style={{ margin: '0 auto 8px auto' }} />
                  <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>PostgreSQL Staging DB</div>
                  <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                    public.settlement_batches
                  </div>
                  <span className="badge badge-cyan" style={{ marginTop: '8px', fontSize: '0.6rem' }}>Core Ledger Staging</span>
                </div>

                <ArrowRight size={24} color="var(--accent-cyan)" />

                {/* Node 4: Apex Settlement API */}
                <div style={{
                  background: 'rgba(245, 158, 11, 0.1)',
                  border: '1px solid rgba(245, 158, 11, 0.4)',
                  borderRadius: '8px',
                  padding: '16px',
                  flex: 1,
                  textAlign: 'center'
                }}>
                  <Radio size={24} color="var(--accent-amber)" style={{ margin: '0 auto 8px auto' }} />
                  <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>Apex Settlement API</div>
                  <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                    /v2/settlements/reconcile
                  </div>
                  <span className="badge badge-amber" style={{ marginTop: '8px', fontSize: '0.6rem' }}>Interbank Egress</span>
                </div>
              </div>
            </div>
          )}

          {/* 4. Edge Agent Deployment */}
          {activeTab === 'EDGE_DEPLOY' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '18px'
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px' }}>
                  <Terminal size={18} color="var(--accent-cyan)" />
                  <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    Customer Edge Agent Outbound Deployment
                  </h4>
                </div>
                <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '14px', lineHeight: 1.5 }}>
                  The Customer Edge Agent executes locally within your private VPC or on-prem datacenter. It makes outbound-only mTLS calls to Sentinel Flow, avoiding opening any inbound firewall ports into internal databases, SSH bastions, or shared storage volumes.
                </p>

                <div style={{
                  background: 'rgba(0, 0, 0, 0.5)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '6px',
                  padding: '12px',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between'
                }}>
                  <code className="font-mono" style={{ fontSize: '0.75rem', color: '#F8FAFC' }}>
                    {edgeDeployCmd}
                  </code>
                  <button
                    onClick={handleCopyCmd}
                    className="btn btn-secondary"
                    style={{ fontSize: '0.7rem', padding: '4px 10px', display: 'flex', alignItems: 'center', gap: '4px' }}
                  >
                    {copiedCmd ? <Check size={12} color="var(--accent-emerald)" /> : <Copy size={12} />}
                    <span>{copiedCmd ? 'Copied' : 'Copy'}</span>
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
```

---

## `src/components/VaultAndInstantPaymentsModal.tsx`

**Lines:** 657  ·  **Reason for removal:** Front end for the deleted vault and the fabricated FedNow/RTP settlement simulator.

```tsx
import React, { useState } from 'react';
import { 
  Lock, 
  X, 
  ShieldCheck, 
  Zap, 
  Key, 
  Eye, 
  Server, 
  RefreshCw, 
  Play
} from 'lucide-react';

interface VaultAndInstantPaymentsModalProps {
  onClose: () => void;
}

export const VaultAndInstantPaymentsModal: React.FC<VaultAndInstantPaymentsModalProps> = ({ onClose }) => {
  const [activeTab, setActiveTab] = useState<'VAULT' | 'FEDNOW' | 'FAILOVER'>('VAULT');

  // Vault Sandbox State
  const [tenantId, setTenantId] = useState<string>('TENANT-MERIDIAN-PROD');
  const [fieldType, setFieldType] = useState<string>('ROUTING_NUMBER');
  const [rawValue, setRawValue] = useState<string>('021000021');
  const [tokenizedResult, setTokenizedResult] = useState<{ masked: string; tokenKey: string } | null>({
    masked: '0210****1',
    tokenKey: 'TOK-ROU-4a3F9cKz8eL1'
  });
  const [detokenizedValue, setDetokenizedValue] = useState<string | null>(null);
  const [supervisorId, setSupervisorId] = useState<string>('SUP-VAULT-9912');
  const [auditReason, setAuditReason] = useState<string>('Subpoena compliance audit review per SEC Rule 17a-4.');
  const [isTokenizing, setIsTokenizing] = useState<boolean>(false);

  // FedNow Validator State
  const [fedNowXml, setFedNowXml] = useState<string>(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>FEDNOW-2026-MSG-001</MsgId>
      <CreDtTm>2026-08-14T10:00:00Z</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf><SttlmMtd>CLRG</SttlmMtd></SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId><EndToEndId>E2E-FEDNOW-8891</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="USD">150000.00</IntrBkSttlmAmt>
      <DbtrAgt><FinInstnId><ClrSysMmbId><MmbId>021000021</MmbId></ClrSysMmbId></FinInstnId></DbtrAgt>
      <CdtrAgt><FinInstnId><ClrSysMmbId><MmbId>121000358</MmbId></ClrSysMmbId></FinInstnId></CdtrAgt>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`);
  const [fedNowResult, setFedNowResult] = useState<any | null>(null);
  const [isValidatingFedNow, setIsValidatingFedNow] = useState<boolean>(false);

  // DR Failover State
  const [failoverResult, setFailoverResult] = useState<any | null>(null);
  const [isFailingOver, setIsFailingOver] = useState<boolean>(false);

  const handleTokenize = async () => {
    setIsTokenizing(true);
    try {
      const res = await fetch('http://localhost:8080/api/v1/vault/tokenize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tenantId, fieldType, rawValue })
      });
      if (res.ok) {
        const data = await res.json();
        setTokenizedResult({ masked: data.maskedValue, tokenKey: data.tokenKey });
        setDetokenizedValue(null);
      }
    } catch (e) {
      console.warn('Backend vault call fallback', e);
      setTokenizedResult({ masked: '0210****1', tokenKey: 'TOK-ROU-4a3F9cKz8eL1' });
    } finally {
      setIsTokenizing(false);
    }
  };

  const handleDetokenize = async () => {
    if (!tokenizedResult) return;
    try {
      const res = await fetch('http://localhost:8080/api/v1/vault/detokenize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tokenKey: tokenizedResult.tokenKey,
          supervisorId: supervisorId,
          auditReason: auditReason
        })
      });
      if (res.ok) {
        const data = await res.json();
        setDetokenizedValue(data.detokenized);
      }
    } catch (e) {
      console.warn('Backend detokenize fallback', e);
      setDetokenizedValue(rawValue);
    }
  };

  const handleValidateFedNow = async () => {
    setIsValidatingFedNow(true);
    try {
      const res = await fetch('http://localhost:8080/api/v1/instant-payments/validate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ payloadXml: fedNowXml })
      });
      if (res.ok) {
        const data = await res.json();
        setFedNowResult(data);
      }
    } catch (e) {
      console.warn('Backend instant payment fallback', e);
      setFedNowResult({
        isCompliant: false,
        findings: ['Debtor routing ABA 021000021 failed Federal Reserve Mod10 validation.'],
        transaction: {
          network: 'FEDNOW',
          amountUsd: 150000,
          validationLatencyMs: 1.42,
          slaThresholdMs: 2500,
          status: 'QUARANTINED'
        }
      });
    } finally {
      setIsValidatingFedNow(false);
    }
  };

  const handleSimulateFailover = async () => {
    setIsFailingOver(true);
    try {
      const res = await fetch('http://localhost:8080/api/v1/chaos/failover/simulate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      });
      if (res.ok) {
        const data = await res.json();
        setFailoverResult(data);
      }
    } catch (e) {
      console.warn('Backend failover fallback', e);
      setFailoverResult({
        primaryRegion: 'us-east-1 (N. Virginia Active)',
        standbyRegion: 'us-west-2 (Oregon Standby)',
        rpoSeconds: 0.00,
        rtoMilliseconds: 42.5,
        replicatedBlocksCount: 4829,
        dataLossTransactionCount: 0,
        standbyHealthStatus: 'ACTIVE_PROMOTED'
      });
    } finally {
      setIsFailingOver(false);
    }
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
              background: 'rgba(99, 102, 241, 0.15)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: '1px solid rgba(99, 102, 241, 0.4)'
            }}>
              <Lock size={20} color="#818CF8" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Institutional Privacy Vault & FedNow Instant Gateway
                </h3>
                <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>FIPS 140-3 Compliant</span>
                <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>FedNow / RTP Certified</span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Format-preserving tokenization, FedNow/RTP ISO 20022 parsing & multi-region zero-data-loss DR failover
              </p>
            </div>
          </div>

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

        {/* Tab Navigation */}
        <div style={{
          display: 'flex',
          borderBottom: '1px solid var(--border-subtle)',
          padding: '0 24px',
          background: 'rgba(14, 20, 34, 0.4)'
        }}>
          <button
            onClick={() => setActiveTab('VAULT')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'VAULT' ? '2px solid #818CF8' : '2px solid transparent',
              color: activeTab === 'VAULT' ? '#818CF8' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Key size={14} />
            <span>Zero-Knowledge Tokenization Vault</span>
          </button>

          <button
            onClick={() => setActiveTab('FEDNOW')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'FEDNOW' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'FEDNOW' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Zap size={14} />
            <span>FedNow & RTP Instant Payments</span>
          </button>

          <button
            onClick={() => setActiveTab('FAILOVER')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'FAILOVER' ? '2px solid var(--accent-emerald)' : '2px solid transparent',
              color: activeTab === 'FAILOVER' ? 'var(--accent-emerald)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Server size={14} />
            <span>Multi-Region DR Failover (RPO=0)</span>
          </button>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {/* Tab 1: Tokenization Vault */}
          {activeTab === 'VAULT' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: '20px' }}>
              {/* Left Column: Tokenization Sandbox */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  Format-Preserving Encryption (FPE) Sandbox
                </h4>

                <div style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '8px',
                  padding: '16px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '12px'
                }}>
                  <div>
                    <label style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
                      Tenant Context:
                    </label>
                    <select
                      value={tenantId}
                      onChange={(e) => setTenantId(e.target.value)}
                      className="input"
                      style={{ width: '100%', fontSize: '0.75rem', padding: '6px 10px' }}
                    >
                      <option value="TENANT-MERIDIAN-PROD">TENANT-MERIDIAN-PROD (FPE AES-256)</option>
                      <option value="TENANT-APEX-GLOBAL">TENANT-APEX-GLOBAL (HMAC SHA-256)</option>
                    </select>
                  </div>

                  <div>
                    <label style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
                      Financial Field Type:
                    </label>
                    <select
                      value={fieldType}
                      onChange={(e) => setFieldType(e.target.value)}
                      className="input"
                      style={{ width: '100%', fontSize: '0.75rem', padding: '6px 10px' }}
                    >
                      <option value="ROUTING_NUMBER">ABA Routing Number (9 Digits)</option>
                      <option value="ACCOUNT_NUMBER">Deposit Account Number (4-17 Digits)</option>
                      <option value="INDIVIDUAL_NAME">Individual / Corporate Entity Name</option>
                    </select>
                  </div>

                  <div>
                    <label style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
                      Raw Cleartext Value:
                    </label>
                    <input
                      type="text"
                      value={rawValue}
                      onChange={(e) => setRawValue(e.target.value)}
                      className="input font-mono"
                      style={{ width: '100%', fontSize: '0.75rem', padding: '6px 10px' }}
                    />
                  </div>

                  <button
                    onClick={handleTokenize}
                    disabled={isTokenizing}
                    className="btn btn-primary"
                    style={{ fontSize: '0.75rem', padding: '8px 14px', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px' }}
                  >
                    {isTokenizing ? <RefreshCw size={14} className="animate-spin" /> : <Lock size={14} />}
                    <span>{isTokenizing ? 'Tokenizing...' : 'Tokenize & Mask Field'}</span>
                  </button>

                  {tokenizedResult && (
                    <div style={{
                      background: 'rgba(0, 0, 0, 0.4)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '6px',
                      padding: '12px',
                      marginTop: '4px',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '8px'
                    }}>
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Masked Value:</span>
                        <code className="font-mono" style={{ fontSize: '0.8125rem', color: 'var(--accent-emerald)', fontWeight: 600 }}>
                          {tokenizedResult.masked}
                        </code>
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Secure Token Key:</span>
                        <code className="font-mono" style={{ fontSize: '0.75rem', color: 'var(--accent-cyan)' }}>
                          {tokenizedResult.tokenKey}
                        </code>
                      </div>
                    </div>
                  )}
                </div>
              </div>

              {/* Right Column: Supervised Detokenize Gate */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  Supervised Detokenization Gate
                </h4>

                <div style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid rgba(239, 68, 68, 0.3)',
                  borderRadius: '8px',
                  padding: '16px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '12px'
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <ShieldCheck size={18} color="var(--accent-crimson)" />
                    <span style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                      Audit-Logged Detokenization
                    </span>
                  </div>

                  <div>
                    <label style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
                      Supervisor Operator ID:
                    </label>
                    <input
                      type="text"
                      value={supervisorId}
                      onChange={(e) => setSupervisorId(e.target.value)}
                      className="input font-mono"
                      style={{ width: '100%', fontSize: '0.75rem', padding: '6px 10px' }}
                    />
                  </div>

                  <div>
                    <label style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
                      Subpoena / Audit Justification:
                    </label>
                    <textarea
                      value={auditReason}
                      onChange={(e) => setAuditReason(e.target.value)}
                      rows={2}
                      className="input"
                      style={{ width: '100%', fontSize: '0.75rem', padding: '6px 10px', resize: 'none' }}
                    />
                  </div>

                  <button
                    onClick={handleDetokenize}
                    className="btn btn-secondary"
                    style={{ fontSize: '0.75rem', padding: '8px 14px', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px', borderColor: 'rgba(239, 68, 68, 0.4)' }}
                  >
                    <Eye size={14} color="var(--accent-crimson)" />
                    <span>Authorize Detokenize Access</span>
                  </button>

                  {detokenizedValue && (
                    <div style={{
                      background: 'rgba(239, 68, 68, 0.1)',
                      border: '1px solid rgba(239, 68, 68, 0.4)',
                      borderRadius: '6px',
                      padding: '10px',
                      fontSize: '0.75rem',
                      color: 'var(--text-primary)'
                    }}>
                      <div><strong>Detokenized Cleartext:</strong> <code className="font-mono" style={{ color: '#F87171' }}>{detokenizedValue}</code></div>
                      <div style={{ fontSize: '0.65rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                        Logged in Merkle chain as AUDIT_DETOKENIZE_ACCESS.
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}

          {/* Tab 2: FedNow & RTP Instant Payments */}
          {activeTab === 'FEDNOW' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr', gap: '20px' }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                    FedNow pacs.008 Instant Payment XML
                  </h4>
                  <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>SLA Target &lt; 2,500 ms</span>
                </div>

                <textarea
                  value={fedNowXml}
                  onChange={(e) => setFedNowXml(e.target.value)}
                  rows={14}
                  className="input font-mono"
                  style={{ width: '100%', fontSize: '0.7rem', padding: '12px', resize: 'none', background: '#0F172A', color: '#E2E8F0' }}
                />

                <button
                  onClick={handleValidateFedNow}
                  disabled={isValidatingFedNow}
                  className="btn btn-primary"
                  style={{ fontSize: '0.75rem', padding: '10px 16px', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px' }}
                >
                  {isValidatingFedNow ? <RefreshCw size={14} className="animate-spin" /> : <Zap size={14} />}
                  <span>{isValidatingFedNow ? 'Validating Instant Payment...' : 'Validate FedNow XML Message'}</span>
                </button>
              </div>

              {/* FedNow Result Card */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  Validation & Latency Telemetry
                </h4>

                {fedNowResult ? (
                  <div style={{
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '16px',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '12px'
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Status:</span>
                      <span className={`badge ${fedNowResult.isCompliant ? 'badge-emerald' : 'badge-crimson'}`}>
                        {fedNowResult.transaction.status}
                      </span>
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Validation Latency:</span>
                      <code className="font-mono" style={{ color: 'var(--accent-emerald)', fontWeight: 600 }}>
                        {fedNowResult.transaction.validationLatencyMs} ms (&lt; 2,500ms SLA)
                      </code>
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Settlement Amount:</span>
                      <code className="font-mono" style={{ color: 'var(--text-primary)' }}>
                        ${fedNowResult.transaction.amountUsd.toLocaleString()} USD
                      </code>
                    </div>

                    {fedNowResult.findings && fedNowResult.findings.length > 0 && (
                      <div style={{
                        background: 'rgba(239, 68, 68, 0.1)',
                        border: '1px solid rgba(239, 68, 68, 0.3)',
                        borderRadius: '6px',
                        padding: '10px',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '6px'
                      }}>
                        <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--accent-crimson)' }}>
                          Validation Findings:
                        </div>
                        {fedNowResult.findings.map((f: string, idx: number) => (
                          <div key={idx} style={{ fontSize: '0.7rem', color: 'var(--text-secondary)' }}>
                            • {f}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                ) : (
                  <div style={{
                    background: 'var(--bg-primary)',
                    border: '1px dashed var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '32px',
                    textAlign: 'center',
                    color: 'var(--text-muted)',
                    fontSize: '0.75rem'
                  }}>
                    Click "Validate FedNow XML Message" to verify sub-second routing and XML schemas.
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Tab 3: Disaster Recovery Failover */}
          {activeTab === 'FAILOVER' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '20px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between'
              }}>
                <div>
                  <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    Simulate Cross-Region Active-Standby Failover
                  </h4>
                  <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                    Simulates complete primary datacenter outage (us-east-1) and promotes synchronous standby (us-west-2).
                  </p>
                </div>

                <button
                  onClick={handleSimulateFailover}
                  disabled={isFailingOver}
                  className="btn btn-primary"
                  style={{ fontSize: '0.75rem', padding: '10px 16px', display: 'flex', alignItems: 'center', gap: '6px' }}
                >
                  {isFailingOver ? <RefreshCw size={14} className="animate-spin" /> : <Play size={14} />}
                  <span>{isFailingOver ? 'Failing Over...' : 'Trigger Multi-Region DR Test'}</span>
                </button>
              </div>

              {failoverResult && (
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(4, 1fr)',
                  gap: '14px'
                }}>
                  <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
                    <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Recovery Point Objective (RPO)</div>
                    <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--accent-emerald)', marginTop: '4px' }}>
                      {failoverResult.rpoSeconds}s (0 Data Loss)
                    </div>
                  </div>

                  <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
                    <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Recovery Time Objective (RTO)</div>
                    <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--accent-cyan)', marginTop: '4px' }}>
                      {failoverResult.rtoMilliseconds} ms
                    </div>
                  </div>

                  <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
                    <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Replicated Merkle Blocks</div>
                    <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--text-primary)', marginTop: '4px' }}>
                      {failoverResult.replicatedBlocksCount.toLocaleString()}
                    </div>
                  </div>

                  <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
                    <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Standby Region Status</div>
                    <div className="badge badge-emerald" style={{ marginTop: '8px', display: 'inline-block' }}>
                      {failoverResult.standbyHealthStatus}
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
```

---

## `src/components/SqlConsoleModal.tsx`

**Lines:** 353  ·  **Reason for removal:** Front end for the deleted arbitrary SQL console, through which webhook secrets were readable.

```tsx
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
```

---

## `src/components/AgentSwarmModal.tsx`

**Lines:** 395  ·  **Reason for removal:** Rendered the scripted four-agent transcript as if it were live deliberation.

```tsx
import React, { useState } from 'react';
import { 
  Bot, 
  X, 
  ShieldCheck, 
  Cpu, 
  CheckCircle2, 
  Search, 
  Play, 
  RefreshCw,
  GitMerge,
  UserCheck
} from 'lucide-react';

interface AgentSwarmModalProps {
  onClose: () => void;
  onApproveConsensus?: (action: string) => void;
}

interface AgentMessage {
  id: string;
  agentRole: 'LEAD_SUPERVISOR' | 'FORMAT_VALIDATOR' | 'LINEAGE_RECON' | 'AUDIT_COMPLIANCE';
  agentName: string;
  stepType: 'THOUGHT' | 'TOOL_CALL' | 'OBSERVATION' | 'CONCLUSION';
  content: string;
  toolName?: string;
  toolParameters?: string;
  confidence: number;
}

const DEFAULT_MESSAGES: AgentMessage[] = [
  {
    id: 'MSG-1',
    agentRole: 'LEAD_SUPERVISOR',
    agentName: 'Astra Lead Supervisor',
    stepType: 'THOUGHT',
    content: 'Incident #101 detected on Inbound NACHA Transmission File #501. Initializing multi-agent triage: (1) Format syntax validation, (2) Blast radius assessment, (3) SEC compliance verification.',
    confidence: 0.99
  },
  {
    id: 'MSG-2',
    agentRole: 'FORMAT_VALIDATOR',
    agentName: 'Syntax & Mod10 Inspector',
    stepType: 'TOOL_CALL',
    toolName: 'validate_routing_mod10',
    toolParameters: '{"routingNumber": "021000021"}',
    content: 'Executing deterministic Federal Reserve Mod10 checksum formula on Transit Routing Number 021000021.',
    confidence: 0.99
  },
  {
    id: 'MSG-3',
    agentRole: 'FORMAT_VALIDATOR',
    agentName: 'Syntax & Mod10 Inspector',
    stepType: 'OBSERVATION',
    content: 'Violation confirmed: Federal Reserve check digit calculation yields 8, but record specifies 1. Violates Nacha 2025 Operating Rules, Section 3.2.',
    confidence: 0.99
  },
  {
    id: 'MSG-4',
    agentRole: 'LINEAGE_RECON',
    agentName: 'Settlement Lineage Recon',
    stepType: 'TOOL_CALL',
    toolName: 'check_staging_leakage',
    toolParameters: '{"dbTable": "public.settlement_batches", "fileHash": "0a9b...c4"}',
    content: 'Scanning downstream PostgreSQL staging ledger for potential leaked transactions.',
    confidence: 0.98
  },
  {
    id: 'MSG-5',
    agentRole: 'LINEAGE_RECON',
    agentName: 'Settlement Lineage Recon',
    stepType: 'OBSERVATION',
    content: 'Zero leakage detected. Sentinel Gateway quarantined the payload at ingress boundary. Core settlement ledgers are fully isolated.',
    confidence: 0.99
  },
  {
    id: 'MSG-6',
    agentRole: 'AUDIT_COMPLIANCE',
    agentName: 'SEC 17a-4 Audit Defense',
    stepType: 'CONCLUSION',
    content: 'SHA-256 Merkle audit entry anchored to immutable chain. Non-repudiable evidence package generated for compliance archive.',
    confidence: 1.00
  },
  {
    id: 'MSG-7',
    agentRole: 'LEAD_SUPERVISOR',
    agentName: 'Astra Lead Supervisor',
    stepType: 'CONCLUSION',
    content: 'Unanimous Consensus Reached: Contain file in Dead-Letter Quarantine, dispatch Nacha Section 3.2 remediation notice to counterparty, require Tier-3 human supervisor cryptographic approval.',
    confidence: 0.985
  }
];

export const AgentSwarmModal: React.FC<AgentSwarmModalProps> = ({ onClose, onApproveConsensus }) => {
  const [messages, setMessages] = useState<AgentMessage[]>(DEFAULT_MESSAGES);
  const [isDeliberating, setIsDeliberating] = useState<boolean>(false);
  const [isApproved, setIsApproved] = useState<boolean>(false);

  const handleRunSwarm = async () => {
    setIsDeliberating(true);
    try {
      const res = await fetch('http://localhost:8080/api/v1/swarm/deliberate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          incidentId: 101,
          fileId: 501,
          findings: ['INVALID_MOD10_ROUTING'],
          rawData: '6220210000218420000245000999888800John Doe'
        })
      });
      if (res.ok) {
        const data = await res.json();
        if (data.messages && data.messages.length > 0) {
          setMessages(data.messages);
        }
      }
    } catch (e) {
      console.warn('Backend swarm dispatch, using cached consensus', e);
    } finally {
      setIsDeliberating(false);
    }
  };

  const handleSignOff = () => {
    setIsApproved(true);
    if (onApproveConsensus) {
      onApproveConsensus('QUARANTINE_AND_DISPATCH_CORRECTED_RESEND_NOTICE');
    }
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
        maxWidth: '1100px',
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
              background: 'rgba(139, 92, 246, 0.15)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: '1px solid rgba(139, 92, 246, 0.4)'
            }}>
              <Bot size={20} color="#A78BFA" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Astra Multi-Agent Collaborative Swarm
                </h3>
                <span className="badge" style={{ background: 'rgba(139, 92, 246, 0.2)', color: '#C4B5FD', fontSize: '0.65rem' }}>
                  4 Autonomous Agents Active
                </span>
                <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>Authority Tier 2</span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Collaborative ReAct reasoning, parallel blast-radius audit & human-in-the-loop sign-off
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <button
              onClick={handleRunSwarm}
              disabled={isDeliberating}
              className="btn btn-primary"
              style={{ fontSize: '0.75rem', padding: '6px 12px', display: 'flex', alignItems: 'center', gap: '6px' }}
            >
              {isDeliberating ? <RefreshCw size={14} className="animate-spin" /> : <Play size={14} />}
              <span>{isDeliberating ? 'Swarm Deliberating...' : 'Trigger Swarm Re-Triage'}</span>
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

        {/* 4 Agent Fleet Status Bar */}
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(4, 1fr)',
          gap: '10px',
          padding: '14px 24px',
          background: 'rgba(14, 20, 34, 0.4)',
          borderBottom: '1px solid var(--border-subtle)'
        }}>
          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '6px', padding: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Cpu size={16} color="var(--accent-cyan)" />
            <div>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>Lead Supervisor</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>● Orchestrating</div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '6px', padding: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Search size={16} color="var(--accent-amber)" />
            <div>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>Syntax & Mod10</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>● Verified (100%)</div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '6px', padding: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            <GitMerge size={16} color="var(--accent-emerald)" />
            <div>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>Lineage Recon</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>● Blast Radius 0</div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '6px', padding: '10px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            <ShieldCheck size={16} color="#A78BFA" />
            <div>
              <div style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>Audit Defense</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>● Merkle Proof Signed</div>
            </div>
          </div>
        </div>

        {/* Body: Thought Stream and Consensus */}
        <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr', gap: '16px', padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {/* Left Column: Inter-Agent Reasoning Transcript */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Inter-Agent ReAct Reasoning Stream
            </h4>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {messages.map((m, idx) => {
                let badgeColor = 'badge-cyan';
                if (m.agentRole === 'FORMAT_VALIDATOR') badgeColor = 'badge-amber';
                if (m.agentRole === 'LINEAGE_RECON') badgeColor = 'badge-emerald';
                if (m.agentRole === 'AUDIT_COMPLIANCE') badgeColor = 'badge-neutral';

                return (
                  <div
                    key={idx}
                    style={{
                      background: 'var(--bg-primary)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '8px',
                      padding: '12px',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '6px'
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <span className={`badge ${badgeColor}`} style={{ fontSize: '0.65rem' }}>{m.agentName}</span>
                        <span style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>[{m.stepType}]</span>
                      </div>
                      <span className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--accent-emerald)' }}>
                        {(m.confidence * 100).toFixed(1)}% Conf
                      </span>
                    </div>

                    <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                      {m.content}
                    </p>

                    {m.toolName && (
                      <div style={{
                        background: 'rgba(0, 0, 0, 0.4)',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '4px',
                        padding: '6px 8px',
                        marginTop: '4px'
                      }}>
                        <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--accent-cyan)' }}>
                          <strong>Tool:</strong> {m.toolName}()
                        </div>
                        {m.toolParameters && (
                          <div className="font-mono" style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>
                            {m.toolParameters}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>

          {/* Right Column: Multi-Agent Consensus & Sign-Off */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
            <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Swarm Consensus & Execution Gate
            </h4>

            <div style={{
              background: 'var(--bg-primary)',
              border: '1px solid rgba(139, 92, 246, 0.4)',
              borderRadius: '8px',
              padding: '16px',
              display: 'flex',
              flexDirection: 'column',
              gap: '12px'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Consensus State:</span>
                <span className="badge badge-emerald" style={{ fontSize: '0.7rem' }}>UNANIMOUS (4/4)</span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Overall Confidence:</span>
                <span className="font-mono" style={{ fontSize: '0.875rem', fontWeight: 700, color: 'var(--accent-emerald)' }}>
                  98.5%
                </span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Recommended Action:</span>
                <span className="badge badge-crimson" style={{ fontSize: '0.65rem' }}>QUARANTINE & RESEND</span>
              </div>

              <div style={{
                background: 'rgba(239, 68, 68, 0.08)',
                border: '1px solid rgba(239, 68, 68, 0.25)',
                borderRadius: '6px',
                padding: '10px',
                fontSize: '0.75rem',
                color: 'var(--text-secondary)',
                lineHeight: 1.4
              }}>
                <strong>Authority Tier 3 Boundary Enforced:</strong> The autonomous swarm is prohibited from releasing funds or waiving compliance rules without human supervisor cryptographic sign-off.
              </div>

              <button
                onClick={handleSignOff}
                disabled={isApproved}
                className="btn btn-primary"
                style={{
                  padding: '10px 16px',
                  fontSize: '0.8125rem',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: '8px',
                  marginTop: '8px',
                  background: isApproved ? 'var(--accent-emerald)' : undefined
                }}
              >
                {isApproved ? <CheckCircle2 size={16} /> : <UserCheck size={16} />}
                <span>{isApproved ? 'Consensus Approved & Executed' : 'Supervisor Dual-Control Sign-Off'}</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
```

---

## `src/components/SelfHealingModal.tsx`

**Lines:** 451  ·  **Reason for removal:** Front end for the deleted self-healing apply endpoint, which accepted a caller-supplied supervisor identity.

```tsx
import React, { useState } from 'react';
import { 
  Wrench, 
  X, 
  ShieldCheck, 
  CheckCircle2, 
  AlertTriangle, 
  TrendingUp, 
  RefreshCw
} from 'lucide-react';

interface SelfHealingModalProps {
  onClose: () => void;
  onApplyHealedFile?: (repairedContent: string) => void;
}

interface RepairPatchItem {
  lineNumber: number;
  originalText: string;
  repairedText: string;
  repairReason: string;
  calculatedFix: string;
}

const DEFAULT_PATCHES: RepairPatchItem[] = [
  {
    lineNumber: 3,
    originalText: '6220210000218420000245000999888800John Doe                 0021000020000001',
    repairedText: '6220210000288420000245000999888800John Doe                 0021000020000001',
    repairReason: 'Federal Reserve Mod10 Check Digit Mismatch',
    calculatedFix: "Replaced check digit '1' with calculated Mod10 digit '8' for ABA prefix 02100002"
  },
  {
    lineNumber: 4,
    originalText: '820000000100021000020000002450000000000000000001234567                         021000020000001',
    repairedText: '820000000100021000020000002450000000000000000001234567                         021000020000001',
    repairReason: 'Batch Control Record 8 Entry Hash Verified',
    calculatedFix: 'Mathematically verified 10-digit Entry Hash sum equals 0002100002'
  }
];

const DRIFT_METRICS = [
  {
    name: 'TransactionAmountCents',
    type: 'NUMERICAL_VALUE',
    baseline: '$24.50',
    current: '$24.86',
    divergence: '4.2%',
    status: 'STABLE',
    explanation: 'Median ticket size is well within historical ±5% standard deviation band.'
  },
  {
    name: 'SecClassCode_CCD_Ratio',
    type: 'CATEGORICAL_RATIO',
    baseline: '85.0%',
    current: '84.0%',
    divergence: '1.5%',
    status: 'STABLE',
    explanation: 'Corporate Credit/Debit mix aligns with recurring commercial payroll profile.'
  },
  {
    name: 'DiscretionaryData_NullRate',
    type: 'NULL_RATE',
    baseline: '2.0%',
    current: '18.0%',
    divergence: '38.0%',
    status: 'MODERATE_DRIFT',
    explanation: 'Null field frequency increased 9x. Counterparty likely modified upstream ERP export schema.'
  },
  {
    name: 'HourlyArrivalKurtosis',
    type: 'TIMING_DISTRIBUTION',
    baseline: '3.10',
    current: '3.15',
    divergence: '2.0%',
    status: 'STABLE',
    explanation: 'Peak transmission delivery window remains centered around 14:00 UTC cutoff.'
  }
];

export const SelfHealingModal: React.FC<SelfHealingModalProps> = ({ onClose, onApplyHealedFile }) => {
  const [activeTab, setActiveTab] = useState<'HEALING' | 'DRIFT'>('HEALING');
  const [supervisorId, setSupervisorId] = useState<string>('SUP-OPS-7741');
  const [approvalNote, setApprovalNote] = useState<string>('Authorized mathematical Mod10 check digit correction per Nacha Operating Rules Article 2.');
  const [isApplying, setIsApplying] = useState<boolean>(false);
  const [appliedSuccess, setAppliedSuccess] = useState<boolean>(false);

  const handleApply = async () => {
    setIsApplying(true);
    try {
      const repairedContent = `101 021000028 1234567892608141430A094101MERIDIAN CUSTODY        SENTINEL FLOW          \n5200PAYROLL   CORP INC        0001234567PPDDIRECT PAY260814260814   1021000020000001\n6220210000288420000245000999888800John Doe                 0021000020000001\n820000000100021000020000002450000000000000000001234567                         021000020000001\n900000100000100000001000210000200000024500000000000000                         \n`;

      const res = await fetch('http://localhost:8080/api/v1/healing/apply', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          proposalId: 'HEAL-101-2026',
          supervisorId: supervisorId,
          approvalNote: approvalNote,
          repairedContent: repairedContent
        })
      });

      if (res.ok) {
        setAppliedSuccess(true);
        if (onApplyHealedFile) {
          onApplyHealedFile(repairedContent);
        }
      }
    } catch (e) {
      console.warn('Backend heal call, applying mock state', e);
      setAppliedSuccess(true);
    } finally {
      setIsApplying(false);
    }
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
        maxWidth: '1100px',
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
              background: 'rgba(16, 185, 129, 0.15)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: '1px solid rgba(16, 185, 129, 0.4)'
            }}>
              <Wrench size={20} color="var(--accent-emerald)" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Self-Healing File Repair & Drift Profiler
                </h3>
                <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>Deterministic Patch Engine</span>
                <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>Authority Tier 3 Gated</span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Mathematical file redlining, dry-run validation & continuous 30-day statistical drift monitoring
              </p>
            </div>
          </div>

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

        {/* Tab Navigation */}
        <div style={{
          display: 'flex',
          borderBottom: '1px solid var(--border-subtle)',
          padding: '0 24px',
          background: 'rgba(14, 20, 34, 0.4)'
        }}>
          <button
            onClick={() => setActiveTab('HEALING')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'HEALING' ? '2px solid var(--accent-emerald)' : '2px solid transparent',
              color: activeTab === 'HEALING' ? 'var(--accent-emerald)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Wrench size={14} />
            <span>Autonomous Self-Healing Repair ({DEFAULT_PATCHES.length} Patches)</span>
          </button>

          <button
            onClick={() => setActiveTab('DRIFT')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'DRIFT' ? '2px solid var(--accent-amber)' : '2px solid transparent',
              color: activeTab === 'DRIFT' ? 'var(--accent-amber)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <TrendingUp size={14} />
            <span>Continuous Schema & Volume Drift Profiler</span>
          </button>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {/* Tab 1: Self-Healing File Repair */}
          {activeTab === 'HEALING' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1.3fr 1fr', gap: '20px' }}>
              {/* Left Column: Side-by-side Patch Inspector */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                    Proposed Mathematical Patches (Dry-Run Passed)
                  </h4>
                  <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>Confidence 99.5%</span>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                  {DEFAULT_PATCHES.map((patch, idx) => (
                    <div
                      key={idx}
                      style={{
                        background: 'var(--bg-primary)',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '8px',
                        padding: '14px',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '8px'
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <span className="badge badge-neutral" style={{ fontSize: '0.65rem' }}>Line {patch.lineNumber}</span>
                          <span style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-primary)' }}>{patch.repairReason}</span>
                        </div>
                        <CheckCircle2 size={14} color="var(--accent-emerald)" />
                      </div>

                      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', fontSize: '0.7rem' }}>
                        <div style={{ color: 'var(--text-muted)' }}>Original Corrupt Text:</div>
                        <code className="font-mono" style={{ background: 'rgba(239, 68, 68, 0.1)', color: '#F87171', padding: '4px 6px', borderRadius: '4px', overflowX: 'auto' }}>
                          {patch.originalText}
                        </code>

                        <div style={{ color: 'var(--text-muted)', marginTop: '4px' }}>Repaired Output Text:</div>
                        <code className="font-mono" style={{ background: 'rgba(16, 185, 129, 0.1)', color: '#34D399', padding: '4px 6px', borderRadius: '4px', overflowX: 'auto' }}>
                          {patch.repairedText}
                        </code>
                      </div>

                      <p style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', marginTop: '2px' }}>
                        <strong>Fix Logic:</strong> {patch.calculatedFix}
                      </p>
                    </div>
                  ))}
                </div>
              </div>

              {/* Right Column: Supervisor Sign-off */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                <h4 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  Dual-Control Supervisor Authorization
                </h4>

                <div style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid rgba(16, 185, 129, 0.4)',
                  borderRadius: '8px',
                  padding: '16px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '12px'
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <ShieldCheck size={18} color="var(--accent-emerald)" />
                    <span style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                      Non-Repudiable Repair Commitment
                    </span>
                  </div>

                  <div>
                    <label style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
                      Supervisor Operator ID:
                    </label>
                    <input
                      type="text"
                      value={supervisorId}
                      onChange={(e) => setSupervisorId(e.target.value)}
                      className="input font-mono"
                      style={{ width: '100%', fontSize: '0.75rem', padding: '6px 10px' }}
                    />
                  </div>

                  <div>
                    <label style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }}>
                      Audit Reason & Rule Citation:
                    </label>
                    <textarea
                      value={approvalNote}
                      onChange={(e) => setApprovalNote(e.target.value)}
                      rows={3}
                      className="input"
                      style={{ width: '100%', fontSize: '0.75rem', padding: '6px 10px', resize: 'none' }}
                    />
                  </div>

                  <div style={{
                    background: 'rgba(6, 182, 212, 0.08)',
                    border: '1px solid rgba(6, 182, 212, 0.25)',
                    borderRadius: '6px',
                    padding: '8px 10px',
                    fontSize: '0.7rem',
                    color: 'var(--text-secondary)'
                  }}>
                    Applying patch commits a new block to the SEC 17a-4 Merkle chain with before/after SHA-256 digests and releases the batch into core settlement processing.
                  </div>

                  <button
                    onClick={handleApply}
                    disabled={isApplying || appliedSuccess}
                    className="btn btn-primary"
                    style={{
                      padding: '10px 16px',
                      fontSize: '0.8125rem',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      gap: '8px',
                      background: appliedSuccess ? 'var(--accent-emerald)' : undefined
                    }}
                  >
                    {isApplying ? (
                      <RefreshCw size={16} className="animate-spin" />
                    ) : appliedSuccess ? (
                      <CheckCircle2 size={16} />
                    ) : (
                      <Wrench size={16} />
                    )}
                    <span>
                      {isApplying ? 'Applying & Re-Ingesting...' : appliedSuccess ? 'Healed & Ingested to Ledger' : 'Authorize & Apply Repair Patch'}
                    </span>
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* Tab 2: Continuous Schema & Volume Drift Profiler */}
          {activeTab === 'DRIFT' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                background: 'rgba(245, 158, 11, 0.08)',
                border: '1px solid rgba(245, 158, 11, 0.3)',
                borderRadius: '8px',
                padding: '12px 16px',
                display: 'flex',
                alignItems: 'center',
                gap: '10px'
              }}>
                <AlertTriangle size={18} color="var(--accent-amber)" />
                <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                  <strong>Continuous Drift Profiler:</strong> Evaluating 30-day historical distribution against today's transmission batch. Flagged <strong>1 significant field divergence</strong> (DiscretionaryData null rate).
                </span>
              </div>

              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '16px'
              }}>
                <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
                  Statistical Field Value & Timing Divergence Matrix (Kolmogorov-Smirnov Test)
                </h4>

                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', fontSize: '0.75rem', borderCollapse: 'collapse', textAlign: 'left' }}>
                    <thead>
                      <tr style={{ borderBottom: '1px solid var(--border-subtle)', color: 'var(--text-muted)' }}>
                        <th style={{ padding: '8px' }}>Field Name</th>
                        <th style={{ padding: '8px' }}>Metric Type</th>
                        <th style={{ padding: '8px' }}>30d Baseline</th>
                        <th style={{ padding: '8px' }}>Today's Batch</th>
                        <th style={{ padding: '8px' }}>D-Score</th>
                        <th style={{ padding: '8px' }}>Status</th>
                        <th style={{ padding: '8px' }}>Explanation</th>
                      </tr>
                    </thead>
                    <tbody>
                      {DRIFT_METRICS.map((metric, idx) => (
                        <tr key={idx} style={{ borderBottom: '1px solid rgba(255, 255, 255, 0.03)' }}>
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--accent-cyan)', fontWeight: 600 }}>{metric.name}</td>
                          <td style={{ padding: '8px', color: 'var(--text-secondary)' }}>{metric.type}</td>
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--text-primary)' }}>{metric.baseline}</td>
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--text-primary)' }}>{metric.current}</td>
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--accent-emerald)' }}>{metric.divergence}</td>
                          <td style={{ padding: '8px' }}>
                            <span className={`badge ${metric.status === 'STABLE' ? 'badge-emerald' : 'badge-amber'}`} style={{ fontSize: '0.65rem' }}>
                              {metric.status}
                            </span>
                          </td>
                          <td style={{ padding: '8px', color: 'var(--text-muted)' }}>{metric.explanation}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
```

---

## `src/components/ChaosMonkeyModal.tsx`

**Lines:** 462  ·  **Reason for removal:** Chaos daemon UI. Resilience is proven by integration tests and runbooks in Prompt 14, not a simulator screen.

```tsx
import React, { useState, useEffect } from 'react';
import { 
  Flame, 
  X, 
  Play, 
  Pause, 
  Activity, 
  Radio, 
  Send, 
  CheckCircle2
} from 'lucide-react';

interface ChaosMonkeyModalProps {
  onClose: () => void;
  onTriggerScenario?: (scenario: string) => void;
}

interface FaultHistoryItem {
  id: string;
  timestamp: string;
  scenario: string;
  targetPartner: string;
  status: 'INJECTED' | 'ISOLATED' | 'RECOVERED';
  recoveryLatencyMs: number;
}

export const ChaosMonkeyModal: React.FC<ChaosMonkeyModalProps> = ({
  onClose,
  onTriggerScenario
}) => {
  const [isDaemonActive, setIsDaemonActive] = useState<boolean>(false);
  const [intervalSec, setIntervalSec] = useState<number>(10);
  const [faultHistory, setFaultHistory] = useState<FaultHistoryItem[]>([
    {
      id: 'FAULT-001',
      timestamp: new Date(Date.now() - 45000).toLocaleTimeString(),
      scenario: 'Worker Mid-Stream SIGKILL',
      targetPartner: 'Central Clearing Network',
      status: 'RECOVERED',
      recoveryLatencyMs: 4.8
    },
    {
      id: 'FAULT-002',
      timestamp: new Date(Date.now() - 15000).toLocaleTimeString(),
      scenario: 'Nacha Entry Hash Sum Mismatch',
      targetPartner: 'Meridian Custody Bank',
      status: 'ISOLATED',
      recoveryLatencyMs: 1.2
    }
  ]);

  // Webhook Tab State
  const [activeTab, setActiveTab] = useState<'CHAOS' | 'WEBHOOKS'>('CHAOS');
  const [webhookUrl, setWebhookUrl] = useState<string>('https://core-banking.internal.meridian.com/events/v1');
  const [webhookSecret, setWebhookSecret] = useState<string>('whsec_institutional_live_key_9988');
  const [webhookStatus, setWebhookStatus] = useState<string>('');
  const [isTestingWebhook, setIsTestingWebhook] = useState<boolean>(false);

  // Background Autonomous Chaos Scheduler
  useEffect(() => {
    let timer: any = null;
    if (isDaemonActive) {
      timer = setInterval(() => {
        const scenarios = [
          { name: 'Worker Mid-Stream SIGKILL', partner: 'Apex Clearing Corp', type: 'WORKER_CRASH', latency: 4.8 },
          { name: 'Nacha Entry Hash Sum Mismatch', partner: 'Meridian Custody Bank', type: 'HASH_CORRUPTION', latency: 1.1 },
          { name: 'Missing SLA Window Deadline', partner: 'Central Clearing Network', type: 'MISSING_FILE', latency: 2.3 },
          { name: 'SWIFT MT103 Missing Tag :59:', partner: 'Atlantic Custody & Trust', type: 'SWIFT_FAULT', latency: 0.9 }
        ];
        const randomFault = scenarios[Math.floor(Math.random() * scenarios.length)];
        
        // Trigger scenario callback if present
        if (onTriggerScenario) {
          onTriggerScenario(randomFault.type);
        }

        const newLog: FaultHistoryItem = {
          id: `FAULT-${Date.now().toString().slice(-4)}`,
          timestamp: new Date().toLocaleTimeString(),
          scenario: randomFault.name,
          targetPartner: randomFault.partner,
          status: 'RECOVERED',
          recoveryLatencyMs: randomFault.latency
        };

        setFaultHistory(prev => [newLog, ...prev.slice(0, 7)]);
      }, intervalSec * 1000);
    }
    return () => {
      if (timer) clearInterval(timer);
    };
  }, [isDaemonActive, intervalSec, onTriggerScenario]);

  const handleTestWebhook = async () => {
    setIsTestingWebhook(true);
    setWebhookStatus('');
    try {
      const res = await fetch('http://localhost:8080/api/v1/webhooks/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: webhookUrl, secret: webhookSecret })
      });
      if (res.ok) {
        setWebhookStatus('Delivered (HTTP 200 OK) with HMAC-SHA256 signature.');
      } else {
        setWebhookStatus('Mock delivery completed (Simulated Endpoint)');
      }
    } catch {
      setWebhookStatus('Mock delivery completed (Simulated Endpoint)');
    } finally {
      setIsTestingWebhook(false);
    }
  };

  const averageMttr = (
    faultHistory.reduce((acc, curr) => acc + curr.recoveryLatencyMs, 0) / (faultHistory.length || 1)
  ).toFixed(2);

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
        maxWidth: '1050px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.5)'
      }}>
        {/* Modal Header */}
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
              background: isDaemonActive ? 'rgba(239, 68, 68, 0.2)' : 'rgba(245, 158, 11, 0.2)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: isDaemonActive ? '1px solid rgba(239, 68, 68, 0.4)' : '1px solid rgba(245, 158, 11, 0.4)'
            }}>
              <Flame size={20} color={isDaemonActive ? 'var(--accent-crimson)' : 'var(--accent-amber)'} />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Autonomous Chaos Monkey & Outbound Webhook Pub/Sub
                </h3>
                <span className={`badge ${isDaemonActive ? 'badge-crimson' : 'badge-neutral'}`} style={{ fontSize: '0.65rem' }}>
                  {isDaemonActive ? 'DAEMON RUNNING' : 'DAEMON IDLE'}
                </span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Continuous automated resilience testing, worker fault injection, and signed event streaming
              </p>
            </div>
          </div>

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

        {/* Tab Header */}
        <div style={{
          display: 'flex',
          borderBottom: '1px solid var(--border-subtle)',
          padding: '0 24px',
          background: 'rgba(14, 20, 34, 0.4)'
        }}>
          <button
            onClick={() => setActiveTab('CHAOS')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'CHAOS' ? '2px solid var(--accent-crimson)' : '2px solid transparent',
              color: activeTab === 'CHAOS' ? 'var(--accent-crimson)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Flame size={14} />
            <span>Autonomous Chaos Daemon</span>
          </button>
          <button
            onClick={() => setActiveTab('WEBHOOKS')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'WEBHOOKS' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'WEBHOOKS' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Radio size={14} />
            <span>Outbound Webhook Pub/Sub (HMAC-SHA256)</span>
          </button>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {activeTab === 'CHAOS' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
              {/* Controls Bar */}
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '16px',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between'
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
                  <button
                    onClick={() => setIsDaemonActive(!isDaemonActive)}
                    className={isDaemonActive ? 'btn btn-secondary' : 'btn btn-primary'}
                    style={{
                      background: isDaemonActive ? 'rgba(239, 68, 68, 0.2)' : undefined,
                      borderColor: isDaemonActive ? 'rgba(239, 68, 68, 0.5)' : undefined,
                      color: isDaemonActive ? 'var(--accent-crimson)' : undefined,
                      display: 'flex',
                      alignItems: 'center',
                      gap: '8px',
                      padding: '8px 16px',
                      fontSize: '0.8125rem'
                    }}
                  >
                    {isDaemonActive ? <Pause size={14} /> : <Play size={14} />}
                    <span>{isDaemonActive ? 'Halt Chaos Daemon' : 'Engage Autonomous Chaos'}</span>
                  </button>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Fault Interval:</span>
                    {[5, 10, 30, 60].map(sec => (
                      <button
                        key={sec}
                        onClick={() => setIntervalSec(sec)}
                        style={{
                          background: intervalSec === sec ? 'var(--accent-cyan-dim)' : 'rgba(255, 255, 255, 0.05)',
                          border: intervalSec === sec ? '1px solid var(--accent-cyan)' : '1px solid var(--border-subtle)',
                          color: intervalSec === sec ? 'var(--accent-cyan)' : 'var(--text-muted)',
                          padding: '4px 8px',
                          borderRadius: '4px',
                          fontSize: '0.7rem',
                          fontWeight: 600,
                          cursor: 'pointer'
                        }}
                      >
                        {sec}s
                      </button>
                    ))}
                  </div>
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
                  <div style={{ textAlign: 'right' }}>
                    <div style={{ fontSize: '0.65rem', color: 'var(--text-muted)', textTransform: 'uppercase' }}>Avg Recovery MTTR</div>
                    <div className="font-mono" style={{ fontSize: '1.1rem', fontWeight: 700, color: 'var(--accent-emerald)' }}>
                      {averageMttr} ms
                    </div>
                  </div>
                </div>
              </div>

              {/* Live Resilience Stream Table */}
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '16px'
              }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '12px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <Activity size={16} color="var(--accent-cyan)" />
                    <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                      Autonomous Fault Injection & Self-Healing Stream
                    </h4>
                  </div>
                  <span className="badge badge-neutral" style={{ fontSize: '0.65rem' }}>Deterministic Re-Leasing</span>
                </div>

                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', fontSize: '0.75rem', borderCollapse: 'collapse', textAlign: 'left' }}>
                    <thead>
                      <tr style={{ borderBottom: '1px solid var(--border-subtle)', color: 'var(--text-muted)' }}>
                        <th style={{ padding: '8px' }}>Fault ID</th>
                        <th style={{ padding: '8px' }}>Timestamp</th>
                        <th style={{ padding: '8px' }}>Injected Failure Scenario</th>
                        <th style={{ padding: '8px' }}>Target Institution</th>
                        <th style={{ padding: '8px' }}>Resilience Status</th>
                        <th style={{ padding: '8px', textAlign: 'right' }}>Recovery Latency</th>
                      </tr>
                    </thead>
                    <tbody>
                      {faultHistory.map((item, i) => (
                        <tr
                          key={item.id + i}
                          style={{
                            borderBottom: '1px solid rgba(255, 255, 255, 0.03)',
                            background: i === 0 && isDaemonActive ? 'rgba(239, 68, 68, 0.05)' : 'transparent'
                          }}
                        >
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--text-muted)' }}>{item.id}</td>
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--text-secondary)' }}>{item.timestamp}</td>
                          <td style={{ padding: '8px', fontWeight: 600, color: 'var(--text-primary)' }}>{item.scenario}</td>
                          <td style={{ padding: '8px', color: 'var(--accent-cyan)' }}>{item.targetPartner}</td>
                          <td style={{ padding: '8px' }}>
                            <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>
                              {item.status}
                            </span>
                          </td>
                          <td className="font-mono" style={{ padding: '8px', textAlign: 'right', color: 'var(--accent-emerald)', fontWeight: 600 }}>
                            {item.recoveryLatencyMs} ms
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'WEBHOOKS' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '16px'
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px' }}>
                  <Radio size={16} color="var(--accent-cyan)" />
                  <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    Outbound Core Banking Webhook Dispatcher
                  </h4>
                </div>
                <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '14px' }}>
                  Deliver cryptographic HMAC-SHA256 signed JSON notifications downstream upon file release, quarantine, or human supervisor approval.
                </p>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                  <div>
                    <label style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                      Subscriber Destination URL:
                    </label>
                    <input
                      type="text"
                      value={webhookUrl}
                      onChange={(e) => setWebhookUrl(e.target.value)}
                      className="font-mono"
                      style={{
                        width: '100%',
                        background: 'rgba(0, 0, 0, 0.3)',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '6px',
                        padding: '8px 12px',
                        color: 'var(--text-primary)',
                        fontSize: '0.75rem'
                      }}
                    />
                  </div>

                  <div>
                    <label style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '4px' }}>
                      HMAC Shared Secret Signing Key:
                    </label>
                    <input
                      type="text"
                      value={webhookSecret}
                      onChange={(e) => setWebhookSecret(e.target.value)}
                      className="font-mono"
                      style={{
                        width: '100%',
                        background: 'rgba(0, 0, 0, 0.3)',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '6px',
                        padding: '8px 12px',
                        color: 'var(--text-primary)',
                        fontSize: '0.75rem'
                      }}
                    />
                  </div>

                  <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '6px' }}>
                    <button
                      onClick={handleTestWebhook}
                      disabled={isTestingWebhook}
                      className="btn btn-primary"
                      style={{ fontSize: '0.75rem', padding: '6px 14px', display: 'flex', alignItems: 'center', gap: '6px' }}
                    >
                      <Send size={14} />
                      <span>{isTestingWebhook ? 'Dispatching Test Ping...' : 'Dispatch HMAC Test Ping'}</span>
                    </button>
                  </div>

                  {webhookStatus && (
                    <div style={{
                      padding: '10px 14px',
                      borderRadius: '6px',
                      background: 'rgba(16, 185, 129, 0.1)',
                      border: '1px solid rgba(16, 185, 129, 0.3)',
                      color: 'var(--accent-emerald)',
                      fontSize: '0.75rem',
                      display: 'flex',
                      alignItems: 'center',
                      gap: '8px'
                    }}>
                      <CheckCircle2 size={16} />
                      <span>{webhookStatus}</span>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
```

---

## `src/components/ExecutiveDeckModal.tsx`

**Lines:** 354  ·  **Reason for removal:** Executive reporting built on hardcoded benchmark, SLA and compliance figures.

```tsx
import React, { useState } from 'react';
import { 
  Briefcase, 
  X, 
  ChevronRight, 
  ChevronLeft, 
  ShieldCheck, 
  TrendingUp, 
  Server, 
  Cpu, 
  CheckCircle2, 
  Award
} from 'lucide-react';

interface ExecutiveDeckModalProps {
  onClose: () => void;
}

export const ExecutiveDeckModal: React.FC<ExecutiveDeckModalProps> = ({ onClose }) => {
  const [currentSlide, setCurrentSlide] = useState<number>(0);

  const slides = [
    {
      title: "Executive Summary: The $5B Invisible Financial Plumbing Crisis",
      subtitle: "Why Global Custody, Settlement & Treasury Run on Fragile File Transfers",
      icon: <TrendingUp size={24} color="var(--accent-cyan)" />,
      badge: "PROBLEM SPACE & MARKET SIZE",
      content: (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(3, 1fr)',
            gap: '12px'
          }}>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '16px' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', textTransform: 'uppercase' }}>Daily Custody Flow</div>
              <div className="font-mono" style={{ fontSize: '1.5rem', fontWeight: 700, color: 'var(--accent-cyan)', marginTop: '4px' }}>$50+ Trillion</div>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: '2px' }}>Exchanged via batch files daily</div>
            </div>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '16px' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', textTransform: 'uppercase' }}>MOVEit Ransomware Impact</div>
              <div className="font-mono" style={{ fontSize: '1.5rem', fontWeight: 700, color: 'var(--accent-crimson)', marginTop: '4px' }}>2,700+ Orgs</div>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: '2px' }}>$10B+ in global breach costs</div>
            </div>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '16px' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', textTransform: 'uppercase' }}>Pre-Ledger Gateway TAM</div>
              <div className="font-mono" style={{ fontSize: '1.5rem', fontWeight: 700, color: 'var(--accent-emerald)', marginTop: '4px' }}>$4.8 Billion</div>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: '2px' }}>Growing at 12.4% CAGR</div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '16px' }}>
            <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>
              The Operational Blind Spot in Tier-1 Custody:
            </h4>
            <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', lineHeight: 1.6 }}>
              While financial institutions invest billions in core ledgers and modern APIs, <strong>counterparty communication remains 80%+ batch files</strong> (NACHA ACH, ISO 20022 XML, BAI2, SWIFT MT). When a file arrives late or corrupt, failures cascade silently into overnight settlement, triggering multi-million dollar regulatory fines and market chaos (e.g. ICBC $9B US Treasury settlement outage).
            </p>
          </div>
        </div>
      )
    },
    {
      title: "System Architecture: Pre-Ledger Reliability & Fault-Isolation",
      subtitle: "Deterministic Go Gateway + Cryptographic Merkle Chain + Off-Hot-Path AI",
      icon: <Server size={24} color="var(--accent-emerald)" />,
      badge: "TECHNICAL ARCHITECTURE",
      content: (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          <div style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(3, 1fr)',
            gap: '12px'
          }}>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
              <span className="badge badge-emerald" style={{ fontSize: '0.65rem' }}>GATEWAY TIER</span>
              <h5 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', margin: '6px 0 4px 0' }}>Go Monolith Engine</h5>
              <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)', lineHeight: 1.4 }}>
                SIMD 94-char fixed-width streaming parser, Mod10 ABA check digit algorithm, and high-throughput SHA-256 calculation (148 MB/s).
              </p>
            </div>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
              <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>AUDIT TIER</span>
              <h5 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', margin: '6px 0 4px 0' }}>SHA-256 Merkle Chain</h5>
              <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)', lineHeight: 1.4 }}>
                Tamper-evident append-only event ledger providing SEC Rule 17a-4 / SOX 404 non-repudiation certificates from Genesis to Tip.
              </p>
            </div>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
              <span className="badge badge-amber" style={{ fontSize: '0.65rem' }}>AI COPILOT TIER</span>
              <h5 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', margin: '6px 0 4px 0' }}>Astra 2.0 Off-Hot-Path</h5>
              <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)', lineHeight: 1.4 }}>
                Python FastAPI agent with strict Authority Tier boundaries. Investigates exceptions without risking hot-path latency.
              </p>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
            <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '4px' }}>
              Key Engineering Decision: Zero Autonomous Hot-Path Actions
            </h4>
            <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
              The AI never directly touches production ledgers or authorizes file releases. It operates strictly in <strong>Authority Tier 2</strong> (drafting notices and runbook citations) and requires dual-control cryptographic human approval for Tier 3 containment.
            </p>
          </div>
        </div>
      )
    },
    {
      title: "Enterprise Applied AI Architecture: Astra 2.0",
      subtitle: "Reflect-Reason-Resolve (RRR) Applied AI Framework for Custody Operations",
      icon: <Cpu size={24} color="var(--accent-amber)" />,
      badge: "APPLIED FINTECH AI ARCHITECTURE",
      content: (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          <div style={{
            background: 'rgba(245, 158, 11, 0.1)',
            border: '1px solid rgba(245, 158, 11, 0.3)',
            borderRadius: '8px',
            padding: '14px'
          }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '6px' }}>
              <Award size={16} color="var(--accent-amber)" />
              <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: '#F8FAFC' }}>
                Engineered to Global Custody & Tier-1 Clearing Standards
              </h4>
            </div>
            <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
              Incorporates advanced enterprise architecture paradigms: Ground-truth runbook retrieval (Nacha Operating Rules 2025/2026), verifiable token provenance, and strict supervisor dual-control gates.
            </p>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '12px' }}>
              <h5 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--accent-cyan)', marginBottom: '4px' }}>
                1. Grounded Runbook RAG
              </h5>
              <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)', lineHeight: 1.4 }}>
                Anchored to Nacha Article 2.2.1 and Federal Reserve Operating Circular 4. Eliminates hallucinated compliance policies.
              </p>
            </div>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '12px' }}>
              <h5 style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--accent-emerald)', marginBottom: '4px' }}>
                2. 100% Adversarial Defense
              </h5>
              <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)', lineHeight: 1.4 }}>
                Evaluated against prompt injection, CEO impersonation, and SQL injections with 0% unauthorized execution rate.
              </p>
            </div>
          </div>
        </div>
      )
    },
    {
      title: "Reliability Benchmarks & Compliance Proofs",
      subtitle: "Verified Engineering Target Metrics and SEC 17a-4 Regulatory Deliverables",
      icon: <ShieldCheck size={24} color="var(--accent-emerald)" />,
      badge: "VERIFICATION & METRICS",
      content: (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '10px' }}>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '12px' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Parse Velocity</div>
              <div className="font-mono" style={{ fontSize: '1.125rem', fontWeight: 700, color: 'var(--accent-emerald)', marginTop: '2px' }}>296k rec/s</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>148 MB/s streaming</div>
            </div>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '12px' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>SHA-256 Hash</div>
              <div className="font-mono" style={{ fontSize: '1.125rem', fontWeight: 700, color: 'var(--accent-cyan)', marginTop: '2px' }}>227 MB/s</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>Hardware accelerated</div>
            </div>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '12px' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Recovery SLA</div>
              <div className="font-mono" style={{ fontSize: '1.125rem', fontWeight: 700, color: 'var(--accent-amber)', marginTop: '2px' }}>&lt; 5 ms</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>Mid-stream crash resume</div>
            </div>
            <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '12px' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>SEC 17a-4</div>
              <div className="font-mono" style={{ fontSize: '1.125rem', fontWeight: 700, color: 'var(--text-primary)', marginTop: '2px' }}>100% Signed</div>
              <div style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>Merkle chain export</div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
            <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '6px' }}>
              What Recruiter / Engineering Questions This Answers:
            </h4>
            <ul style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.6, paddingLeft: '18px' }}>
              <li><strong>Distributed Systems:</strong> "How do you achieve deterministic file deduplication across worker crashes?" $\rightarrow$ PostgreSQL advisory locks + atomic SHA-256 lease acquisition.</li>
              <li><strong>Applied AI:</strong> "How do you keep LLMs from making catastrophic settlement errors?" $\rightarrow$ Constrained authority tiers + dual-control supervisory sign-off gates.</li>
              <li><strong>Regulatory Architecture:</strong> "How do you prove compliance to FINRA / SEC examiners?" $\rightarrow$ Cryptographic Merkle chain certificate linking every file to its exact validation finding.</li>
            </ul>
          </div>
        </div>
      )
    }
  ];

  const slide = slides[currentSlide];

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(7, 11, 18, 0.9)',
      backdropFilter: 'blur(10px)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 110,
      padding: '24px'
    }}>
      <div style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-subtle)',
        borderRadius: '14px',
        width: '100%',
        maxWidth: '920px',
        maxHeight: '92vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.8)',
        overflow: 'hidden'
      }}>
        {/* Header */}
        <div style={{
          padding: '16px 24px',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'rgba(14, 20, 34, 0.7)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{
              width: '36px',
              height: '36px',
              borderRadius: '8px',
              background: 'linear-gradient(135deg, #0284C7 0%, #0369A1 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <Briefcase size={20} color="#FFFFFF" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1.0625rem', fontWeight: 700, color: 'var(--text-primary)' }}>
                  Executive Briefing & Architectural Presentation
                </h3>
                <span className="badge badge-cyan">{slide.badge}</span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Slide {currentSlide + 1} of {slides.length} — Sentinel Flow Enterprise Portfolio
              </p>
            </div>
          </div>

          <button 
            className="btn btn-secondary" 
            onClick={onClose}
            style={{ padding: '6px', borderRadius: '50%' }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Slide Body */}
        <div style={{ padding: '28px 32px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: '20px' }}>
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: '14px' }}>
            <div style={{
              padding: '10px',
              borderRadius: '8px',
              background: 'var(--bg-primary)',
              border: '1px solid var(--border-subtle)'
            }}>
              {slide.icon}
            </div>
            <div>
              <h2 style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--text-primary)', letterSpacing: '-0.01em' }}>
                {slide.title}
              </h2>
              <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', marginTop: '2px' }}>
                {slide.subtitle}
              </p>
            </div>
          </div>

          {slide.content}
        </div>

        {/* Footer Navigation */}
        <div style={{
          padding: '16px 24px',
          borderTop: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'rgba(14, 20, 34, 0.7)'
        }}>
          {/* Slide Indicator Dots */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {slides.map((_, idx) => (
              <div
                key={idx}
                onClick={() => setCurrentSlide(idx)}
                style={{
                  width: idx === currentSlide ? '24px' : '8px',
                  height: '8px',
                  borderRadius: '4px',
                  background: idx === currentSlide ? 'var(--accent-cyan)' : 'var(--border-subtle)',
                  cursor: 'pointer',
                  transition: 'all 0.2s ease'
                }}
              />
            ))}
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <button
              className="btn btn-secondary"
              disabled={currentSlide === 0}
              onClick={() => setCurrentSlide(prev => Math.max(0, prev - 1))}
              style={{ display: 'flex', alignItems: 'center', gap: '4px' }}
            >
              <ChevronLeft size={16} />
              <span>Previous</span>
            </button>

            {currentSlide < slides.length - 1 ? (
              <button
                className="btn btn-primary"
                onClick={() => setCurrentSlide(prev => Math.min(slides.length - 1, prev + 1))}
                style={{ display: 'flex', alignItems: 'center', gap: '4px' }}
              >
                <span>Next Slide</span>
                <ChevronRight size={16} />
              </button>
            ) : (
              <button
                className="btn btn-primary"
                onClick={onClose}
                style={{ background: 'var(--accent-emerald)', borderColor: 'var(--accent-emerald)', color: '#000', fontWeight: 700 }}
              >
                <span>Explore Live Cockpit</span>
                <CheckCircle2 size={16} />
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
```

---

## `src/components/BenchmarkModal.tsx`

**Lines:** 418  ·  **Reason for removal:** Benchmark panel. No performance figure may reach the UI until a reproducible result artifact exists (Prompt 13).

```tsx
import React, { useState } from 'react';
import { 
  Activity, 
  X, 
  Play, 
  ShieldCheck, 
  Zap, 
  CheckCircle2, 
  Lock, 
  BarChart3, 
  Cpu
} from 'lucide-react';
import { SentinelApi } from '../services/api';

interface BenchmarkModalProps {
  onClose: () => void;
}

export const BenchmarkModal: React.FC<BenchmarkModalProps> = ({ onClose }) => {
  const [activeTab, setActiveTab] = useState<'BENCHMARK' | 'ADVERSARIAL_EVALS'>('BENCHMARK');
  const [recordCount, setRecordCount] = useState<number>(25000);
  const [benchmarkResult, setBenchmarkResult] = useState<any | null>(null);
  const [evalResult, setEvalResult] = useState<any | null>(null);
  const [isRunning, setIsRunning] = useState<boolean>(false);

  // Run Go Streaming Benchmark
  const handleRunBenchmark = async () => {
    setIsRunning(true);
    try {
      const data = await SentinelApi.runBenchmark(recordCount);
      setBenchmarkResult(data);
    } catch (e: any) {
      alert(`Benchmark error: ${e.message}`);
    } finally {
      setIsRunning(false);
    }
  };

  // Run Python AI Adversarial Evals
  const handleRunEvals = async () => {
    setIsRunning(true);
    try {
      const data = await SentinelApi.runEvals();
      setEvalResult(data);
    } catch (e: any) {
      alert(`AI Evals error: ${e.message}`);
    } finally {
      setIsRunning(false);
    }
  };

  return (
    <div style={{
      position: 'fixed',
      inset: 0,
      background: 'rgba(7, 11, 18, 0.85)',
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
        maxWidth: '880px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.7)',
        overflow: 'hidden'
      }}>
        {/* Modal Header */}
        <div style={{
          padding: '16px 24px',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'rgba(14, 20, 34, 0.6)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div style={{
              width: '32px',
              height: '32px',
              borderRadius: '6px',
              background: 'rgba(16, 185, 129, 0.2)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <Activity size={18} color="var(--accent-emerald)" />
            </div>
            <div>
              <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                System Performance Telemetry & AI Adversarial Evals
              </h3>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Measure streaming throughput (100k NACHA records/sec) and verify 100% prompt injection containment.
              </p>
            </div>
          </div>

          <button 
            className="btn btn-secondary" 
            onClick={onClose}
            style={{ padding: '6px', borderRadius: '50%' }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Tab Switcher */}
        <div style={{
          display: 'flex',
          borderBottom: '1px solid var(--border-subtle)',
          background: 'var(--bg-primary)',
          padding: '0 24px'
        }}>
          <button
            onClick={() => setActiveTab('BENCHMARK')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'BENCHMARK' ? '2px solid var(--accent-emerald)' : '2px solid transparent',
              color: activeTab === 'BENCHMARK' ? 'var(--accent-emerald)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Zap size={14} />
            <span>Go Streaming Benchmark (100k Records/Sec)</span>
          </button>

          <button
            onClick={() => setActiveTab('ADVERSARIAL_EVALS')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'ADVERSARIAL_EVALS' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'ADVERSARIAL_EVALS' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <ShieldCheck size={14} />
            <span>Astra 2.0 Adversarial AI Evals</span>
          </button>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '24px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: '20px' }}>
          {activeTab === 'BENCHMARK' && (
            <div>
              {/* Parameter Selection Bar */}
              <div style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                padding: '16px',
                borderRadius: '8px',
                marginBottom: '20px'
              }}>
                <div>
                  <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    Streaming Batch Volume Scale:
                  </h4>
                  <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '2px' }}>
                    Simulates in-memory 94-char fixed-width NACHA batch streaming + SHA-256 + Mod10 check digits.
                  </p>
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                  {[10000, 25000, 50000, 100000].map(cnt => (
                    <button
                      key={cnt}
                      onClick={() => setRecordCount(cnt)}
                      className={`btn ${recordCount === cnt ? 'btn-primary' : 'btn-secondary'}`}
                      style={{ fontSize: '0.75rem', padding: '6px 12px' }}
                    >
                      {(cnt / 1000).toFixed(0)}k Records
                    </button>
                  ))}

                  <button
                    className="btn btn-primary"
                    disabled={isRunning}
                    onClick={handleRunBenchmark}
                    style={{ background: 'var(--accent-emerald)', borderColor: 'var(--accent-emerald)', color: '#000', fontWeight: 700 }}
                  >
                    <Play size={14} />
                    <span>{isRunning ? 'Running SIMD Stream...' : 'Run Benchmark'}</span>
                  </button>
                </div>
              </div>

              {/* Benchmark Results Display */}
              {benchmarkResult ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px' }}>
                    <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
                      <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Throughput</span>
                      <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--accent-emerald)', marginTop: '4px' }}>
                        {benchmarkResult.throughputMBPerSec?.toFixed(1)} MB/s
                      </div>
                      <span style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>Streaming Parsing</span>
                    </div>

                    <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
                      <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Parse Velocity</span>
                      <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--accent-cyan)', marginTop: '4px' }}>
                        {benchmarkResult.recordsPerSecond?.toLocaleString(undefined, { maximumFractionDigits: 0 })} rec/s
                      </div>
                      <span style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>94-char records verified</span>
                    </div>

                    <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
                      <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>SHA-256 Speed</span>
                      <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--text-primary)', marginTop: '4px' }}>
                        {benchmarkResult.sha256ThroughputMBs?.toFixed(1)} MB/s
                      </div>
                      <span style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>Hardware Accelerated</span>
                    </div>

                    <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border-subtle)', borderRadius: '8px', padding: '14px' }}>
                      <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Total Duration</span>
                      <div className="font-mono" style={{ fontSize: '1.25rem', fontWeight: 700, color: 'var(--accent-amber)', marginTop: '4px' }}>
                        {benchmarkResult.durationMs?.toFixed(1)} ms
                      </div>
                      <span style={{ fontSize: '0.65rem', color: 'var(--text-muted)' }}>End-to-End Latency</span>
                    </div>
                  </div>

                  {/* Engine Details */}
                  <div style={{
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '14px',
                    fontSize: '0.75rem',
                    color: 'var(--text-secondary)'
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                      <span>Engine: <strong style={{ color: 'var(--text-primary)' }}>{benchmarkResult.engineIdentifier}</strong></span>
                      <span>Total Streamed: <strong className="font-mono">{(benchmarkResult.totalBytesStreamed / (1024 * 1024)).toFixed(2)} MB</strong></span>
                    </div>
                    <div>
                      Heap Allocation: <strong className="font-mono">{benchmarkResult.allocatedMemoryKB} KB</strong> ({benchmarkResult.totalAllocations} mallocs)
                    </div>
                  </div>
                </div>
              ) : (
                <div style={{
                  textAlign: 'center',
                  padding: '40px',
                  background: 'var(--bg-primary)',
                  borderRadius: '8px',
                  border: '1px dashed var(--border-subtle)',
                  color: 'var(--text-muted)'
                }}>
                  <BarChart3 size={32} style={{ margin: '0 auto 8px auto', opacity: 0.5 }} />
                  <p style={{ fontSize: '0.875rem' }}>Select volume scale above and click <strong>"Run Benchmark"</strong> to execute live performance test.</p>
                </div>
              )}
            </div>
          )}

          {activeTab === 'ADVERSARIAL_EVALS' && (
            <div>
              <div style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                padding: '16px',
                borderRadius: '8px',
                marginBottom: '16px'
              }}>
                <div>
                  <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    Prompt Injection & Jailbreak Attack Dataset:
                  </h4>
                  <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '2px' }}>
                    Evaluates Astra 2.0 guardrails against instruction overrides, executive impersonation, prompt leaks, and SQL injections.
                  </p>
                </div>

                <button
                  className="btn btn-primary"
                  disabled={isRunning}
                  onClick={handleRunEvals}
                  style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
                >
                  <Play size={14} />
                  <span>{isRunning ? 'Evaluating Guardrails...' : 'Run Adversarial Evals'}</span>
                </button>
              </div>

              {/* Eval Results Display */}
              {evalResult ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                  {/* Summary Bar */}
                  <div style={{
                    background: 'rgba(16, 185, 129, 0.15)',
                    border: '1px solid rgba(16, 185, 129, 0.4)',
                    borderRadius: '8px',
                    padding: '12px 16px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between'
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                      <CheckCircle2 size={20} color="var(--accent-emerald)" />
                      <div>
                        <span style={{ fontWeight: 600, fontSize: '0.875rem', color: '#F8FAFC' }}>
                          Guardrail Defense Pass Rate: {evalResult.passRatePct}%
                        </span>
                        <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: '2px' }}>
                          {evalResult.passedTests} of {evalResult.totalTests} attacks contained | 0 unauthorized executions
                        </p>
                      </div>
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <Lock size={14} color="var(--accent-emerald)" />
                      <span className="badge badge-success">ZERO ESCALATION BREACH</span>
                    </div>
                  </div>

                  {/* Attack Breakdown List */}
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                    {evalResult.results?.map((res: any) => (
                      <div
                        key={res.testId}
                        style={{
                          background: 'var(--bg-primary)',
                          border: '1px solid var(--border-subtle)',
                          borderRadius: '6px',
                          padding: '10px 14px',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between'
                        }}
                      >
                        <div>
                          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                            <span className="font-mono" style={{ fontSize: '0.75rem', color: 'var(--accent-cyan)', fontWeight: 600 }}>
                              {res.testId}
                            </span>
                            <span style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                              {res.name}
                            </span>
                            <span className="badge badge-neutral" style={{ fontSize: '0.65rem' }}>
                              {res.category}
                            </span>
                          </div>
                          <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: '3px' }}>
                            Defense: <strong style={{ color: 'var(--accent-emerald)' }}>{res.defenseStatus}</strong> — {res.defenseNote}
                          </p>
                        </div>

                        <span className="font-mono" style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                          {res.latencyMs} ms
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              ) : (
                <div style={{
                  textAlign: 'center',
                  padding: '40px',
                  background: 'var(--bg-primary)',
                  borderRadius: '8px',
                  border: '1px dashed var(--border-subtle)',
                  color: 'var(--text-muted)'
                }}>
                  <Cpu size={32} style={{ margin: '0 auto 8px auto', opacity: 0.5 }} />
                  <p style={{ fontSize: '0.875rem' }}>Click <strong>"Run Adversarial Evals"</strong> to test Astra 2.0 prompt injection defense guardrails.</p>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div style={{
          padding: '14px 24px',
          borderTop: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          background: 'rgba(14, 20, 34, 0.6)'
        }}>
          <button className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
```

---

## `src/components/InfrastructureConfigModal.tsx`

**Lines:** 353  ·  **Reason for removal:** Connector setup wizard for the deleted Integration Hub. Collected a vault key and mTLS auth method for connectors with no backend. Prompt 16 builds the real metadata-driven wizard.

```tsx
import React, { useState } from 'react';
import { 
  X, 
  Settings, 
  Database, 
  Search, 
  BookOpen, 
  Save, 
  Lock 
} from 'lucide-react';

interface InfrastructureConfigModalProps {
  onClose: () => void;
}

type IntegrationType = 'POSTGRESQL' | 'SFTP_SSH' | 'REST_API' | 'S3_OBJECT';

export const InfrastructureConfigModal: React.FC<InfrastructureConfigModalProps> = ({ onClose }) => {
  const [selectedType, setSelectedType] = useState<IntegrationType>('POSTGRESQL');
  const [searchQuery, setSearchQuery] = useState('');
  
  // Form State
  const [name, setName] = useState('');
  const [host, setHost] = useState('');
  const [port, setPort] = useState('');
  const [serviceAccount, setServiceAccount] = useState('');
  const [vaultKey, setVaultKey] = useState('');
  const [authMethod, setAuthMethod] = useState('mTLS_X509_CERTIFICATE');

  const handleSave = () => {
    // Mock save
    console.log("Saving Configuration:", { name, type: selectedType, host, port, serviceAccount, authMethod, vaultKey });
    onClose();
  };

  // Mock Documentation based on selected type and search
  const renderDocumentation = () => {
    let docTitle = "";
    let docContent = "";
    let docCode = "";

    switch (selectedType) {
      case 'POSTGRESQL':
        docTitle = "PostgreSQL Cluster Configuration";
        docContent = "To securely connect to the Core PostgreSQL Cluster, you must use mTLS. Ensure your Edge Agent has the X.509 certificate bundle available via HashiCorp Vault. Default port is 5432. Do not use password authentication in production.";
        docCode = "vault://prod/postgres/tls_cert_bundle\n# psql \"sslmode=verify-full sslcert=client.crt sslkey=client.key ...\"";
        break;
      case 'SFTP_SSH':
        docTitle = "SFTP / SSH Drop Zone Setup";
        docContent = "SFTP connections require an ED25519 SSH Key. Use the Vault pointer to reference the private key. You must configure the host fingerprint in the Edge Agent to prevent MITM attacks. Default port is usually 22 or 2222.";
        docCode = "vault://prod/treasury/sftp_agent_ed25519\n# ssh-keyscan -p 2222 sftp.meridian.internal > known_hosts";
        break;
      case 'REST_API':
        docTitle = "REST API Webhook Gateway";
        docContent = "Integrations via REST API require OAuth2 client credentials or Mutual TLS. Provide the OAuth2 Client Secret via the Vault Key pointer. Webhook subscriptions will use HMAC SHA-256 for payload verification.";
        docCode = "vault://prod/apex/oauth_client_secret\n# POST /v2/settlements/reconcile\n# Authorization: Bearer <token>";
        break;
      case 'S3_OBJECT':
        docTitle = "S3 WORM Archive (SEC 17a-4)";
        docContent = "Immutable object storage connections use AWS IAM Instance Profiles or federated identity. Define the ARN in the Service Account field and use the Vault pointer for the IAM Role assumption.";
        docCode = "aws://iam/role/SentinelEvidenceArchiver\n# Policy: s3:PutObject, s3:PutObjectRetention";
        break;
    }

    if (searchQuery.length > 0 && !docTitle.toLowerCase().includes(searchQuery.toLowerCase()) && !docContent.toLowerCase().includes(searchQuery.toLowerCase())) {
       return (
         <div style={{ color: 'var(--text-muted)', textAlign: 'center', marginTop: '40px' }}>
            <Search size={24} style={{ margin: '0 auto 8px auto', opacity: 0.5 }} />
            No official documentation found for "{searchQuery}".
         </div>
       );
    }

    return (
      <div style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-subtle)',
        borderRadius: '8px',
        padding: '16px',
        marginTop: '16px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px' }}>
          <BookOpen size={16} color="var(--accent-cyan)" />
          <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>{docTitle}</h4>
        </div>
        <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.6, marginBottom: '12px' }}>
          {docContent}
        </p>
        <div style={{
          background: '#0F172A',
          border: '1px solid #334155',
          borderRadius: '4px',
          padding: '10px',
          color: '#38BDF8',
          fontFamily: 'monospace',
          fontSize: '0.7rem',
          whiteSpace: 'pre-wrap'
        }}>
          {docCode}
        </div>
        <div style={{ marginTop: '12px', fontSize: '0.65rem', color: 'var(--text-muted)', display: 'flex', justifyContent: 'flex-end' }}>
          Source: Sentinel Flow Official Docs v1.0
        </div>
      </div>
    );
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
        background: 'var(--bg-primary)',
        border: '1px solid var(--border-subtle)',
        borderRadius: '12px',
        width: '100%',
        maxWidth: '1100px',
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
              <Settings size={20} color="var(--accent-cyan)" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Infrastructure Configuration Setup
                </h3>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Provision integration connections securely with Official Documentation guidance
              </p>
            </div>
          </div>

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

        {/* Modal Body - Dual Pane */}
        <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
          
          {/* LEFT PANE: Configuration Form */}
          <div style={{ 
            flex: 1, 
            padding: '24px', 
            overflowY: 'auto', 
            borderRight: '1px solid var(--border-subtle)' 
          }}>
            <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '20px', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Database size={16} color="var(--text-secondary)"/>
              Connection Parameters
            </h4>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '6px' }}>Integration Type</label>
                <select 
                  className="input" 
                  value={selectedType}
                  onChange={(e) => setSelectedType(e.target.value as IntegrationType)}
                  style={{ width: '100%', fontSize: '0.8125rem' }}
                >
                  <option value="POSTGRESQL">PostgreSQL Cluster</option>
                  <option value="SFTP_SSH">SFTP / SSH Drop Zone</option>
                  <option value="REST_API">REST API / Webhook</option>
                  <option value="S3_OBJECT">S3 Object Storage / WORM</option>
                </select>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '6px' }}>Connection Name</label>
                <input 
                  type="text" 
                  className="input" 
                  placeholder="e.g. Core Treasury DB" 
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  style={{ width: '100%', fontSize: '0.8125rem' }} 
                />
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: '12px' }}>
                <div>
                  <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '6px' }}>Host / Endpoint URL</label>
                  <input 
                    type="text" 
                    className="input" 
                    placeholder="e.g. db.internal.local" 
                    value={host}
                    onChange={(e) => setHost(e.target.value)}
                    style={{ width: '100%', fontSize: '0.8125rem' }} 
                  />
                </div>
                <div>
                  <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '6px' }}>Port</label>
                  <input 
                    type="text" 
                    className="input" 
                    placeholder="e.g. 5432" 
                    value={port}
                    onChange={(e) => setPort(e.target.value)}
                    style={{ width: '100%', fontSize: '0.8125rem' }} 
                  />
                </div>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '6px' }}>Service Account / ARN</label>
                <input 
                  type="text" 
                  className="input" 
                  placeholder="e.g. svc_sentinel_ingest" 
                  value={serviceAccount}
                  onChange={(e) => setServiceAccount(e.target.value)}
                  style={{ width: '100%', fontSize: '0.8125rem' }} 
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '6px' }}>Authentication Method</label>
                <select 
                  className="input" 
                  value={authMethod}
                  onChange={(e) => setAuthMethod(e.target.value)}
                  style={{ width: '100%', fontSize: '0.8125rem' }}
                >
                  <option value="mTLS_X509_CERTIFICATE">mTLS X.509 Certificate</option>
                  <option value="SSH_ED25519_KEY">SSH ED25519 Key</option>
                  <option value="OAUTH2_MUTUAL_TLS">OAuth2 + mTLS</option>
                  <option value="IAM_INSTANCE_PROFILE">AWS IAM Instance Profile</option>
                </select>
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '6px' }}>Vault Secret Pointer URI</label>
                <div style={{ position: 'relative' }}>
                  <Lock size={14} style={{ position: 'absolute', left: '10px', top: '10px', color: 'var(--text-muted)' }} />
                  <input 
                    type="text" 
                    className="input" 
                    placeholder="vault://prod/postgres/tls_cert_bundle" 
                    value={vaultKey}
                    onChange={(e) => setVaultKey(e.target.value)}
                    style={{ width: '100%', fontSize: '0.8125rem', paddingLeft: '32px' }} 
                  />
                </div>
                <div style={{ fontSize: '0.65rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                  Raw secrets are never stored. Provide the Vault or AWS Secrets Manager URI.
                </div>
              </div>
            </div>

            <div style={{ marginTop: '32px', display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
              <button className="btn btn-secondary" onClick={onClose} style={{ fontSize: '0.8125rem' }}>Cancel</button>
              <button className="btn btn-primary" onClick={handleSave} style={{ fontSize: '0.8125rem', display: 'flex', alignItems: 'center', gap: '6px' }}>
                <Save size={14} />
                <span>Save Configuration</span>
              </button>
            </div>
          </div>

          {/* RIGHT PANE: Documentation Assistant */}
          <div style={{ 
            flex: 1, 
            padding: '24px', 
            background: 'rgba(10, 15, 29, 0.4)',
            display: 'flex',
            flexDirection: 'column'
          }}>
            <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '16px', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <BookOpen size={16} color="var(--accent-cyan)"/>
              Official Documentation Search
            </h4>

            <div style={{ position: 'relative', marginBottom: '8px' }}>
              <Search size={16} style={{ position: 'absolute', left: '12px', top: '10px', color: 'var(--text-muted)' }} />
              <input 
                type="text" 
                className="input" 
                placeholder="Search integration docs, ports, accuracy guidelines..." 
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                style={{ width: '100%', paddingLeft: '38px', fontSize: '0.8125rem' }}
              />
            </div>

            <div style={{ flex: 1, overflowY: 'auto' }}>
               {renderDocumentation()}
               
               <div style={{
                  background: 'rgba(16, 185, 129, 0.05)',
                  border: '1px solid rgba(16, 185, 129, 0.2)',
                  borderRadius: '8px',
                  padding: '16px',
                  marginTop: '16px'
               }}>
                 <h5 style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--accent-emerald)', marginBottom: '8px' }}>
                   Accuracy & Retrieval Guidelines
                 </h5>
                 <ul style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', margin: 0, paddingLeft: '16px', display: 'flex', flexDirection: 'column', gap: '6px' }}>
                   <li>Double-check host aliases match internal DNS resolution.</li>
                   <li>Verify standard port allocations (e.g. 5432 for PostgreSQL, 2222 for SFTP Edge).</li>
                   <li>Ensure the Service Account possesses the principle of least privilege.</li>
                   <li>Vault pointers must follow the exact URI schema `&lt;provider&gt;://&lt;path&gt;`.</li>
                 </ul>
               </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
```

---

## `src/components/ChaosControls.tsx`

**Lines:** 95  ·  **Reason for removal:** Dashboard control strip framed as a 'Disaster Scenarios & Chaos Verification Harness'; two of its five actions targeted the deleted /chaos/trigger route.

```tsx
import React from 'react';
import { AlertTriangle, ShieldX, CheckCircle, RefreshCcw, Flame } from 'lucide-react';

interface ChaosControlsProps {
  onSimulateMissingFile: () => void;
  onSimulateMalformedNacha: () => void;
  onSimulateValidNacha: () => void;
  onSimulateWorkerCrashRecovery: () => void;
  onResetAll: () => void;
  isSimulating: boolean;
}

export const ChaosControls: React.FC<ChaosControlsProps> = ({
  onSimulateMissingFile,
  onSimulateMalformedNacha,
  onSimulateValidNacha,
  onSimulateWorkerCrashRecovery,
  onResetAll,
  isSimulating
}) => {
  return (
    <div className="glass-panel" style={{ padding: '16px 20px', background: 'var(--bg-secondary)' }}>
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        flexWrap: 'wrap',
        gap: '12px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Flame size={18} color="var(--accent-amber)" />
          <div>
            <h3 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              Disaster Scenarios & Chaos Verification Harness
            </h3>
            <p style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
              Demonstrate failure modes, pre-flight quarantine, missing-file deadlines & crash recovery
            </p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
          <button
            className="btn btn-danger"
            onClick={onSimulateMissingFile}
            disabled={isSimulating}
            style={{ fontSize: '0.75rem', padding: '6px 12px' }}
          >
            <AlertTriangle size={13} />
            <span>1. Trigger Missing File (FedACH 16:45 Cutoff)</span>
          </button>

          <button
            className="btn btn-danger"
            onClick={onSimulateMalformedNacha}
            disabled={isSimulating}
            style={{ fontSize: '0.75rem', padding: '6px 12px' }}
          >
            <ShieldX size={13} />
            <span>2. Drop Malformed NACHA (Quarantine)</span>
          </button>

          <button
            className="btn btn-success"
            onClick={onSimulateValidNacha}
            disabled={isSimulating}
            style={{ fontSize: '0.75rem', padding: '6px 12px' }}
          >
            <CheckCircle size={13} />
            <span>3. Drop Valid NACHA (Nominal STP)</span>
          </button>

          <button
            className="btn btn-secondary"
            onClick={onSimulateWorkerCrashRecovery}
            disabled={isSimulating}
            style={{ fontSize: '0.75rem', padding: '6px 12px' }}
          >
            <RefreshCcw size={13} />
            <span>4. Worker Crash / Idempotent Recovery</span>
          </button>

          <button
            className="btn btn-secondary"
            onClick={onResetAll}
            style={{ fontSize: '0.75rem', padding: '6px 10px', color: 'var(--text-muted)' }}
            title="Reset Environment"
          >
            Reset
          </button>
        </div>
      </div>
    </div>
  );
};
```

---

## `src/App.tsx` — client-side simulation handlers

**Lines:** 75-338 (264 lines)  ·  **Reason for removal:** These five handlers
fabricated operational state entirely in the browser. `handleSimulateMissingFile`
mutated an expectation to OVERDUE locally, then ran the browser-side
`evaluateMissingFileIncident` and `runExceptionAnalyst` and appended to a
browser-side event store, producing an incident, an AI verdict and an audit entry
that no server had any record of. Real ingestion remains available through
`UploadModal`, which posts to the gateway.

```tsx
  // Handler: Simulate Missing File (FedACH 16:45 Cutoff Breach)
  const handleSimulateMissingFile = async () => {
    setIsSimulating(true);
    
    // Pick the Meridian expectation occurrence and simulate past due time
    setOccurrences(prev => prev.map(occ => {
      if (occ.id === 'EXP-MERIDIAN-TODAY') {
        const pastDueTime = new Date(Date.now() - 3600000).toISOString();
        const pastGraceTime = new Date(Date.now() - 1800000).toISOString();
        return {
          ...occ,
          dueAtUtc: pastDueTime,
          graceExpiresAtUtc: pastGraceTime,
          status: 'OVERDUE'
        };
      }
      return occ;
    }));

    const meridianOcc = occurrences.find(o => o.id === 'EXP-MERIDIAN-TODAY')!;
    const contract = contracts.find(c => c.id === meridianOcc.contractId)!;
    const partner = partners.find(p => p.id === meridianOcc.partnerId)!;

    const incident = evaluateMissingFileIncident(
      { ...meridianOcc, dueAtUtc: new Date(Date.now() - 3600000).toISOString(), graceExpiresAtUtc: new Date(Date.now() - 1800000).toISOString() },
      contract,
      partner
    );

    if (incident) {
      setIncidents(prev => [incident, ...prev.filter(i => i.id !== incident.id)]);

      await eventStore.appendEvent(
        'TENANT-DEFAULT',
        incident.id,
        'INCIDENT',
        'INCIDENT_MISSING_FILE_OPENED',
        'SCHEDULER_DEADLINE_ENGINE',
        { incidentType: incident.type, occurrenceId: meridianOcc.id, deadline: incident.slaDeadlineUtc },
        `CORR-${Date.now()}`
      );

      // Trigger automatic background AI triage analysis
      const agentRun = runExceptionAnalyst(incident, undefined, contract, partner);
      setActiveAgentRun({ agentRun, incident });
    }

    setIsSimulating(false);
  };

  // Handler: Simulate Malformed NACHA File Drop (Out of Balance / Hash Mismatch)
  const handleSimulateMalformedNacha = async () => {
    setIsSimulating(true);

    const rawContent = SAMPLE_CORRUPTED_NACHA;
    const contract = contracts[0]; // Meridian Commercial ACH
    const partner = partners[0];
    let sha256 = await calculateSha256(rawContent);

    // Run deterministic streaming NACHA validation
    const { result } = parseAndValidateNacha(rawContent, contract);

    // Ingest into live Go Gateway backend
    try {
      const apiRes = await SentinelApi.ingestRawNacha('MERIDIAN_ACH_COMMERCIAL_20260814_1645.txt', rawContent);
      if (apiRes && apiRes.hash) {
        sha256 = apiRes.hash;
      }
    } catch (e) {
      console.warn('Go Gateway ingestion skipped, using client validator:', e);
    }

    const fileInstanceId = `FILE-CORRUPTED-${Date.now()}`;
    const newFile: FileInstance = {
      id: fileInstanceId,
      sourceEventId: `SRC-EVT-SFTP-${Date.now()}`,
      partnerId: partner.id,
      contractId: contract.id,
      occurrenceId: 'EXP-MERIDIAN-TODAY',
      filename: 'MERIDIAN_ACH_COMMERCIAL_20260814_1645.txt',
      byteSize: rawContent.length,
      sha256Hash: sha256,
      s3Uri: `s3://sentinel-originals/quarantine/${fileInstanceId}.txt`,
      state: 'QUARANTINED',
      receivedAtUtc: new Date().toISOString(),
      validationResult: result,
      quarantineReason: `${result.findings.filter(f => f.severity === 'FATAL' || f.severity === 'ERROR' || f.severity === 'CRITICAL').length} Nacha specification violation(s) detected.`
    };

    setFiles(prev => [newFile, ...prev]);

    // Update occurrence state to QUARANTINED
    setOccurrences(prev => prev.map(o => o.id === 'EXP-MERIDIAN-TODAY' ? { ...o, status: 'QUARANTINED', matchedFileInstanceId: newFile.id } : o));

    // Create Incident for Quarantined Batch
    const newIncident: Incident = {
      id: `INC-CORRUPT-${Date.now()}`,
      type: 'NACHA_ENTRY_HASH_MISMATCH',
      severity: 'CRITICAL',
      title: `Quarantined ACH Batch: ${newFile.filename} (${partner.name})`,
      occurrenceId: 'EXP-MERIDIAN-TODAY',
      fileInstanceId: newFile.id,
      partnerId: partner.id,
      status: 'OPEN',
      openedAtUtc: new Date().toISOString(),
      slaDeadlineUtc: '16:45:00 UTC',
      resolutionNote: `Pre-flight validation halted release. ${result.findings[0]?.message || 'Corrupted control records.'}`
    };

    setIncidents(prev => [newIncident, ...prev]);

    // Record Append-Only Domain Event
    await eventStore.appendEvent(
      'TENANT-DEFAULT',
      newFile.id,
      'FILE',
      'FILE_QUARANTINED_PRE_FLIGHT',
      'VALIDATOR_NACHA_ENGINE',
      { filename: newFile.filename, sha256, findingsCount: result.findings.length, outcome: 'QUARANTINED' },
      `CORR-${Date.now()}`
    );

    // Auto-launch AI Exception Analyst (via Python AI tier or client)
    try {
      const aiRes = await SentinelApi.triggerTriage(newIncident.id);
      const agentRun: AgentRun = {
        id: `AGENT-RUN-${Date.now()}`,
        incidentId: newIncident.id,
        agentVersion: aiRes.agent_version || 'Astra 2.0 RRR Standard',
        modelIdentifier: 'astra-2.0-financial-reasoning',
        ranAtUtc: new Date().toISOString(),
        inputDigest: sha256.substring(0, 16),
        citedEventIds: [`EVT-${Date.now()}`],
        citedFindingCodes: ['ACH_ERR_0802_HASH_MISMATCH'],
        citedRunbookSections: aiRes.citations,
        findingsSummary: aiRes.summary,
        hypotheses: [
          {
            hypothesis: 'Batch Control Entry Hash Mismatch',
            confidence: 'HIGH',
            supportingEvidence: ['Trailer entry hash differs from computed entry hash sum.']
          }
        ],
        proposedActionPlan: aiRes.proposed_actions.map((act, i) => ({
          step: i + 1,
          action: act.description,
          authorityTier: 2 as const,
          requiresHumanApproval: true
        })),
        metrics: {
          durationMs: aiRes.metrics?.durationMs || 120,
          inputTokens: aiRes.metrics?.inputTokens || 450,
          outputTokens: aiRes.metrics?.outputTokens || 210,
          estimatedCostUsd: aiRes.metrics?.estimatedCostUsd || 0.00045
        }
      };
      setActiveAgentRun({ agentRun, incident: newIncident });
    } catch {
      const agentRun = runExceptionAnalyst(newIncident, newFile, contract, partner, result);
      setActiveAgentRun({ agentRun, incident: newIncident });
    }

    setInspectedFile({ file: newFile, rawContent });
    setIsSimulating(false);
  };

  // Handler: Simulate Valid Balanced NACHA File Drop (Nominal STP)
  const handleSimulateValidNacha = async () => {
    setIsSimulating(true);

    const rawContent = SAMPLE_VALID_NACHA;
    const contract = contracts[0];
    const partner = partners[0];
    let sha256 = await calculateSha256(rawContent);

    const { result } = parseAndValidateNacha(rawContent, contract);

    // Ingest into live Go Gateway backend
    try {
      const apiRes = await SentinelApi.ingestRawNacha('MERIDIAN_ACH_COMMERCIAL_20260814_1645.txt', rawContent);
      if (apiRes && apiRes.hash) {
        sha256 = apiRes.hash;
      }
    } catch (e) {
      console.warn('Go Gateway ingestion skipped, using client validator:', e);
    }

    const fileInstanceId = `FILE-VALID-${Date.now()}`;
    const newFile: FileInstance = {
      id: fileInstanceId,
      sourceEventId: `SRC-EVT-SFTP-${Date.now()}`,
      partnerId: partner.id,
      contractId: contract.id,
      occurrenceId: 'EXP-MERIDIAN-TODAY',
      filename: 'MERIDIAN_ACH_COMMERCIAL_20260814_1645.txt',
      byteSize: rawContent.length,
      sha256Hash: sha256,
      s3Uri: `s3://sentinel-originals/valid/${fileInstanceId}.txt`,
      state: 'VALID',
      receivedAtUtc: new Date().toISOString(),
      validationResult: result
    };

    setFiles(prev => [newFile, ...prev]);

    // Update occurrence state to VALID / RELEASED
    setOccurrences(prev => prev.map(o => o.id === 'EXP-MERIDIAN-TODAY' ? { ...o, status: 'VALID', matchedFileInstanceId: newFile.id } : o));

    // Resolve any open incidents for this occurrence
    setIncidents(prev => prev.filter(i => i.occurrenceId !== 'EXP-MERIDIAN-TODAY'));

    // Record in Audit Ledger
    await eventStore.appendEvent(
      'TENANT-DEFAULT',
      newFile.id,
      'FILE',
      'FILE_VALIDATED_AND_RELEASED',
      'VALIDATOR_NACHA_ENGINE',
      { filename: newFile.filename, sha256, totalDebits: result.totalDebitsUsd, totalCredits: result.totalCreditsUsd },
      `CORR-${Date.now()}`
    );

    setInspectedFile({ file: newFile, rawContent });
    setIsSimulating(false);
  };

  // Handler: Simulate Mid-Validation Worker Crash & Recovery
  const handleSimulateWorkerCrashRecovery = async () => {
    setIsSimulating(true);

    // Log simulated crash
    await eventStore.appendEvent(
      'TENANT-DEFAULT',
      'WORKER-POD-4',
      'FILE',
      'WORKER_MID_STREAM_TERMINATION',
      'CHAOS_HARNESS',
      { signal: 'SIGKILL', fileRef: 'MERIDIAN_ACH_COMMERCIAL_20260814_1645.txt' },
      `CORR-CHAOS-${Date.now()}`
    );

    // Simulate recovery through PostgreSQL job re-leasing
    setTimeout(async () => {
      await eventStore.appendEvent(
        'TENANT-DEFAULT',
        'WORKER-POD-5',
        'FILE',
        'JOB_LEASE_EXPIRED_AND_REACQUIRED',
        'SCHEDULER_RECOVERY_WORKER',
        { status: 'RESUMED_STREAMING', arrivalAcknowledged: true, duplicateSuppressed: true },
        `CORR-CHAOS-${Date.now()}`
      );
      setIsSimulating(false);
    }, 800);
  };

  // Reset State
  const handleResetAll = () => {
    setOccurrences(generateInitialOccurrences());
    setFiles([]);
    setIncidents([]);
    setActiveAgentRun(null);
    setInspectedFile(null);
  };
```

---

