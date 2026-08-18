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



---

# Prompt 12 — the client-side cockpit

Removed when the operations UI was rebuilt on server data. 3,211 lines across
twelve files, all of which either rendered state no server had produced or
duplicated a server implementation in the browser.

The pattern common to all of them: each could render a plausible screen with the
gateway switched off. That is the property Prompt 12 removes, and it is why
these are deletions rather than refactors — a component that *can* fall back to
local state will, on the day it matters.

**Source commit:** `da31823`


## `src/App.tsx`

**548 lines** · the cockpit built on the synthetic corpus. Held SYNTHETIC_PARTNERS, SYNTHETIC_CONTRACTS and generateInitialOccurrences() as component state, so the board rendered a partner list no server had ever heard of. Also owned a client-side TamperEvidentEventStore that appended a GATEWAY_INITIALIZED event to a hash chain in the browser, and a catch block that, when AI triage failed, called runExceptionAnalyst and presented the result as an analysis -- a fabricated verdict produced by exactly the fallback path Prompt 12 forbids.

```tsx
import React, { useState, useEffect } from 'react';
import { Header } from './components/Header';
import { DemoDataBanner } from './components/DemoDataBanner';
import { SlaBoard } from './components/SlaBoard';
import { FileInspectorModal } from './components/FileInspectorModal';
import { AiAnalystPanel } from './components/AiAnalystPanel';
import { AuditLedgerModal } from './components/AuditLedgerModal';
import { UploadModal } from './components/UploadModal';
import { ContractConfigModal } from './components/ContractConfigModal';
import { FileDiffModal } from './components/FileDiffModal';
import { ConnectorWizardModal } from './components/ConnectorWizardModal';
import { SavedConnectionsPanel } from './components/SavedConnectionsPanel';

import {
  Partner,
  FileContract,
  ExpectationOccurrence,
  FileInstance,
  Incident,
  Approval,
  AgentRun
} from './types/financial';

import {
  SYNTHETIC_PARTNERS,
  SYNTHETIC_CONTRACTS,
  generateInitialOccurrences,
  SAMPLE_VALID_NACHA,
  SAMPLE_CORRUPTED_NACHA
} from './mockData/syntheticCorpus';

// The browser-side NACHA parser and deadline engine are no longer imported.
// They duplicated server logic, so a verdict could be rendered that no server
// had produced. Validation state now comes from the gateway. The files remain
// in the tree for Prompt 12 to fold into a typed API client or delete.
//
// runExceptionAnalyst is retained: it is a deterministic matcher from finding
// codes to approved runbook passages, not a model call. It is mislabelled as
// "AI" in the panel that renders it; Prompt 15 replaces it with the real
// read-only analyst and corrects the naming.
import { runExceptionAnalyst } from './ai/exceptionAnalyst';
import { TamperEvidentEventStore } from './audit/hashChain';
import { SentinelApi, ApiIngestionResult } from './services/api';

import {
  Activity,
  ShieldCheck,
  AlertTriangle,
  FileText,
  Terminal,
  Database
} from 'lucide-react';

const eventStore = new TamperEvidentEventStore();

export const App: React.FC = () => {
  // Core Platform State
  const [partners] = useState<Partner[]>(SYNTHETIC_PARTNERS);
  const [contracts] = useState<FileContract[]>(SYNTHETIC_CONTRACTS);
  const [occurrences, setOccurrences] = useState<ExpectationOccurrence[]>(generateInitialOccurrences());
  const [files, setFiles] = useState<FileInstance[]>([]);
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [activeAgentRun, setActiveAgentRun] = useState<{ agentRun: AgentRun; incident: Incident } | null>(null);

  // Selected Inspect Modals
  const [inspectedFile, setInspectedFile] = useState<{ file: FileInstance; rawContent: string } | null>(null);
  const [showAuditModal, setShowAuditModal] = useState<boolean>(false);
  const [showUploadModal, setShowUploadModal] = useState<boolean>(false);
  const [showContractsModal, setShowContractsModal] = useState<boolean>(false);
  const [showDiffModal, setShowDiffModal] = useState<boolean>(false);
  const [showConnectorWizard, setShowConnectorWizard] = useState<boolean>(false);

  // Initialize initial Genesis domain event
  useEffect(() => {
    eventStore.appendEvent(
      'TENANT-DEFAULT',
      'SYS-GATEWAY',
      'FILE',
      'GATEWAY_INITIALIZED',
      'SYSTEM_KERNEL',
      { version: 'v1.0.0-PROD', engine: 'Moov-ACH-Nacha2025' },
      'CORR-INIT-001'
    );
  }, []);

  // Human Approval Action from Eliza AI Panel
  const handleApproveAction = async (actionType: Approval['actionType'], reason: string) => {
    if (!activeAgentRun) return;

    try {
      await SentinelApi.approveIncident(activeAgentRun.incident.id, 'SUPERVISOR_OPERATOR_GATHU', `${actionType}: ${reason}`);
    } catch (e) {
      console.warn('Go Gateway approval sync notice:', e);
    }

    await eventStore.appendEvent(
      'TENANT-DEFAULT',
      activeAgentRun.incident.id,
      'APPROVAL',
      'HUMAN_SUPERVISOR_ACTION_COMMITTED',
      'SUPERVISOR_OPERATOR_GATHU',
      { actionType, reason, incidentId: activeAgentRun.incident.id },
      `CORR-${Date.now()}`
    );

    // Resolve or update incident
    setIncidents(prev => prev.map(inc => inc.id === activeAgentRun.incident.id ? { ...inc, status: 'RESOLVED', resolutionNote: `Authorized by Human Supervisor (${actionType}): ${reason}` } : inc));
  };

  // Ingested File Handler from Upload Modal or SFTP
  const handleFileIngested = async (apiResult: ApiIngestionResult, rawContent: string) => {
    const contract = contracts[0];
    const partner = partners[0];

    const newFile: FileInstance = {
      id: `FILE-${apiResult.fileId || Date.now()}`,
      sourceEventId: `SRC-EVT-DIRECT-${Date.now()}`,
      partnerId: partner.id,
      contractId: contract.id,
      occurrenceId: 'EXP-MERIDIAN-TODAY',
      filename: apiResult.filename,
      byteSize: apiResult.sizeBytes,
      sha256Hash: apiResult.hash,
      s3Uri: `s3://sentinel-originals/${apiResult.status.toLowerCase()}/${apiResult.filename}`,
      state: apiResult.status === 'VALIDATED' ? 'VALID' : 'QUARANTINED',
      receivedAtUtc: new Date().toISOString(),
      validationResult: {
        runId: `VAL-RUN-${Date.now()}`,
        // The parser and rule-pack versions were fixed strings asserting a
        // Moov version and a Nacha rule pack edition regardless of what ran.
        // The policy version below is the one the server actually applied.
        parserVersion: '',
        rulePackVersion: apiResult.policyVersion,
        startedAtUtc: new Date().toISOString(),
        completedAtUtc: new Date().toISOString(),
        outcome: apiResult.status === 'VALIDATED' ? 'VALID' : 'QUARANTINED',
        totalRecordsParsed: apiResult.totalRecordsParsed,
        totalDebitsMinor: apiResult.totalDebitsMinor,
        totalCreditsMinor: apiResult.totalCreditsMinor,
        policyVersion: apiResult.policyVersion,
        contractId: apiResult.contractId,
        notCheckedRuleIds: apiResult.notCheckedRuleIds,
        findings: apiResult.findings.map(f => ({
          id: `FND-${f.id}`,
          code: f.code,
          ruleVersion: f.ruleVersion,
          provenance: f.provenance,
          severity: f.severity as any,
          lineNumber: f.lineNumber,
          byteOffset: f.byteOffset,
          fieldStart: f.fieldStart,
          fieldEnd: f.fieldEnd,
          message: f.description,
          evidence: f.evidence,
          expected: f.expected,
          actual: f.actual
        })),
        // resourceMetrics deliberately omitted: the gateway does not measure or
        // report them. This previously attached a fixed 4.2ms / 1.8MB / 28.5MB/s
        // to every ingested file regardless of the file.
      }
    };

    setFiles(prev => [newFile, ...prev]);

    if (apiResult.status === 'QUARANTINED') {
      const newIncident: Incident = {
        id: `INC-${apiResult.incidentId || Date.now()}`,
        type: 'NACHA_ENTRY_HASH_MISMATCH',
        severity: 'CRITICAL',
        title: `Quarantined ACH Batch: ${newFile.filename}`,
        occurrenceId: 'EXP-MERIDIAN-TODAY',
        fileInstanceId: newFile.id,
        partnerId: partner.id,
        status: 'OPEN',
        openedAtUtc: new Date().toISOString(),
        slaDeadlineUtc: '16:45:00 UTC',
        resolutionNote: `Pre-flight validation halted release. ${apiResult.findings[0]?.description || 'Invalid records detected.'}`
      };
      setIncidents(prev => [newIncident, ...prev]);
      setOccurrences(prev => prev.map(o => o.id === 'EXP-MERIDIAN-TODAY' ? { ...o, status: 'QUARANTINED', matchedFileInstanceId: newFile.id } : o));

      // Trigger AI triage
      try {
        const aiRes = await SentinelApi.triggerTriage(newIncident.id);
        const agentRun: AgentRun = {
          id: `AGENT-RUN-${Date.now()}`,
          incidentId: newIncident.id,
          agentVersion: aiRes.agent_version || 'Astra 2.0 RRR Standard',
          modelIdentifier: 'astra-2.0-financial-reasoning',
          ranAtUtc: new Date().toISOString(),
          inputDigest: apiResult.hash.substring(0, 16),
          citedEventIds: [`EVT-${Date.now()}`],
          citedFindingCodes: apiResult.findings.map(f => f.code),
          citedRunbookSections: aiRes.citations,
          findingsSummary: aiRes.summary,
          hypotheses: [
            {
              hypothesis: 'Nacha Control Specification Violation',
              confidence: 'HIGH',
              supportingEvidence: apiResult.findings.map(f => f.description)
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
        const agentRun = runExceptionAnalyst(newIncident, newFile, contract, partner, newFile.validationResult);
        setActiveAgentRun({ agentRun, incident: newIncident });
      }
    } else {
      setOccurrences(prev => prev.map(o => o.id === 'EXP-MERIDIAN-TODAY' ? { ...o, status: 'VALID', matchedFileInstanceId: newFile.id } : o));
      setIncidents(prev => prev.filter(i => i.occurrenceId !== 'EXP-MERIDIAN-TODAY'));
    }

    setInspectedFile({ file: newFile, rawContent });
  };

  const openIncidentsCount = incidents.filter(i => i.status === 'OPEN' || i.status === 'IN_INVESTIGATION').length;
  const quarantinedCount = files.filter(f => f.state === 'QUARANTINED').length;

  return (
    <div style={{ minHeight: '100vh', background: 'var(--bg-primary)', display: 'flex', flexDirection: 'column' }}>
      {/* Platform Header */}
      <Header 
        onOpenAudit={() => setShowAuditModal(true)}
        onOpenUpload={() => setShowUploadModal(true)}
        onOpenContracts={() => setShowContractsModal(true)}
        onOpenDiff={() => setShowDiffModal(true)}
        onOpenConnectors={() => setShowConnectorWizard(true)}
        openIncidentsCount={openIncidentsCount}
        quarantinedCount={quarantinedCount}
      />

      {/* Main Operations Cockpit Content */}
      <main style={{ padding: '24px', display: 'flex', flexDirection: 'column', gap: '20px', flex: 1 }}>
        <DemoDataBanner />

        {/*
          Saved source connections, with the health the server actually
          recorded. It sits in the cockpit rather than behind a modal because a
          connection that has never been checked, or that started failing
          overnight, is something an operator should meet rather than go
          looking for.
        */}
        <section style={{ border: '1px solid var(--border-subtle, #334155)', borderRadius: '8px' }}>
          <SavedConnectionsPanel />
        </section>


        {/* Top Section: Expected Delivery Windows & SLA Radar */}
        <SlaBoard 
          occurrences={occurrences}
          contracts={contracts}
          partners={partners}
          onSelectOccurrence={(occ) => {
            const matchedFile = files.find(f => f.id === occ.matchedFileInstanceId);
            if (matchedFile) {
              setInspectedFile({
                file: matchedFile,
                rawContent: matchedFile.state === 'VALID' ? SAMPLE_VALID_NACHA : SAMPLE_CORRUPTED_NACHA
              });
            }
          }}
        />

        {/* Middle Split Grid: In-Flight Radar & Incidents Queue */}
        <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: '20px' }}>
          {/* Live In-Flight & Ingested File Stream */}
          <div className="glass-panel" style={{ padding: '20px' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <Activity size={18} color="var(--accent-cyan)" />
                <h2 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Inbound Transmission Stream & Validation Ledger
                </h2>
              </div>
              <span className="badge badge-neutral">{files.length} Transmissions</span>
            </div>

            {files.length === 0 ? (
              <div style={{
                textAlign: 'center',
                padding: '40px 20px',
                background: 'var(--bg-secondary)',
                borderRadius: '8px',
                border: '1px dashed var(--border-subtle)',
                color: 'var(--text-muted)'
              }}>
                <Database size={28} style={{ margin: '0 auto 8px auto', opacity: 0.5 }} />
                <p style={{ fontSize: '0.875rem' }}>No file transmissions received yet in this window.</p>
                <p style={{ fontSize: '0.75rem', marginTop: '4px' }}>
                  Use the <strong>Chaos Controls</strong> above to simulate a real SFTP file drop or test validation.
                </p>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {files.map(file => {
                  const partner = partners.find(p => p.id === file.partnerId);
                  const isQuarantined = file.state === 'QUARANTINED';

                  return (
                    <div 
                      key={file.id}
                      style={{
                        background: 'var(--bg-secondary)',
                        border: `1px solid ${isQuarantined ? 'rgba(239, 68, 68, 0.3)' : 'var(--border-subtle)'}`,
                        borderRadius: '6px',
                        padding: '12px 16px',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between'
                      }}
                    >
                      <div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                          <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                            {file.filename}
                          </span>
                          <span className={`badge ${isQuarantined ? 'badge-danger' : 'badge-success'}`}>
                            {file.state}
                          </span>
                        </div>
                        <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                          Partner: <strong style={{ color: 'var(--text-secondary)' }}>{partner?.name}</strong> | Size: {file.byteSize} bytes | SHA: {file.sha256Hash.substring(0, 16)}...
                        </div>
                      </div>

                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                        <button
                          className="btn btn-secondary"
                          onClick={() => setInspectedFile({
                            file,
                            rawContent: file.state === 'VALID' ? SAMPLE_VALID_NACHA : SAMPLE_CORRUPTED_NACHA
                          })}
                          style={{ fontSize: '0.75rem', padding: '5px 10px' }}
                        >
                          <FileText size={13} />
                          <span>Inspect</span>
                        </button>

                        {isQuarantined && (
                          <button
                            className="btn btn-primary"
                            onClick={() => {
                              const relatedInc = incidents.find(i => i.fileInstanceId === file.id) || {
                                id: `INC-${file.id}`,
                                type: 'NACHA_ENTRY_HASH_MISMATCH',
                                severity: 'CRITICAL',
                                title: `Quarantined ACH Batch: ${file.filename}`,
                                fileInstanceId: file.id,
                                partnerId: file.partnerId,
                                status: 'OPEN',
                                openedAtUtc: file.receivedAtUtc,
                                slaDeadlineUtc: '16:45:00 UTC'
                              };
                              const agentRun = runExceptionAnalyst(relatedInc, file, contracts[0], partner, file.validationResult);
                              setActiveAgentRun({ agentRun, incident: relatedInc });
                            }}
                            style={{ fontSize: '0.75rem', padding: '5px 10px' }}
                          >
                            <Terminal size={13} />
                            <span>AI Triage</span>
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* Incidents & Operational Exceptions Queue */}
          <div className="glass-panel" style={{ padding: '20px' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <AlertTriangle size={18} color="var(--accent-crimson)" />
                <h2 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Active Operational Exceptions Queue
                </h2>
              </div>
              <span className="badge badge-danger">{incidents.length} Incidents</span>
            </div>

            {incidents.length === 0 ? (
              <div style={{
                textAlign: 'center',
                padding: '40px 20px',
                background: 'var(--bg-secondary)',
                borderRadius: '8px',
                border: '1px dashed var(--border-subtle)',
                color: 'var(--accent-emerald)'
              }}>
                <ShieldCheck size={28} style={{ margin: '0 auto 8px auto' }} />
                <p style={{ fontSize: '0.875rem', fontWeight: 600 }}>All Systems Nominal</p>
                <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '4px' }}>
                  Zero active SLA breaches, quarantine holds, or missing counterparty files.
                </p>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {incidents.map(inc => {
                  const partner = partners.find(p => p.id === inc.partnerId);
                  const isResolved = inc.status === 'RESOLVED';

                  return (
                    <div 
                      key={inc.id}
                      style={{
                        background: 'var(--bg-secondary)',
                        border: `1px solid ${isResolved ? 'rgba(16, 185, 129, 0.3)' : 'rgba(239, 68, 68, 0.3)'}`,
                        borderRadius: '6px',
                        padding: '12px 16px'
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                          <span className={`badge ${isResolved ? 'badge-success' : 'badge-danger'}`}>
                            {inc.type}
                          </span>
                          <span className="badge badge-neutral">{inc.severity}</span>
                        </div>
                        <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                          Opened: {inc.openedAtUtc.substring(11, 19)} UTC
                        </span>
                      </div>

                      <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                        {inc.title}
                      </h4>

                      <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '4px' }}>
                        {inc.resolutionNote || 'Investigation pending. Awaiting operator resolution or AI triage.'}
                      </p>

                      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '10px' }}>
                        <button
                          className="btn btn-primary"
                          onClick={() => {
                            const file = files.find(f => f.id === inc.fileInstanceId);
                            const agentRun = runExceptionAnalyst(inc, file, contracts[0], partner, file?.validationResult);
                            setActiveAgentRun({ agentRun, incident: inc });
                          }}
                          style={{ fontSize: '0.75rem', padding: '4px 10px' }}
                        >
                          <Terminal size={12} />
                          <span>Investigate with AI Analyst</span>
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* Bottom Section: Active Eliza AI Exception Analyst Workspace */}
        {activeAgentRun && (
          <AiAnalystPanel 
            agentRun={activeAgentRun.agentRun}
            incident={activeAgentRun.incident}
            onApproveAction={handleApproveAction}
            onClose={() => setActiveAgentRun(null)}
          />
        )}
      </main>

      {/* Deep Semantic File Inspector Modal */}
      {inspectedFile && (
        <FileInspectorModal 
          fileInstance={inspectedFile.file}
          partner={partners.find(p => p.id === inspectedFile.file.partnerId)}
          contract={contracts.find(c => c.id === inspectedFile.file.contractId)}
          rawContent={inspectedFile.rawContent}
          onClose={() => setInspectedFile(null)}
          onLaunchAiTriage={(file) => {
            const relInc = incidents.find(i => i.fileInstanceId === file.id) || {
              id: `INC-${file.id}`,
              type: 'NACHA_ENTRY_HASH_MISMATCH',
              severity: 'CRITICAL',
              title: `Quarantined ACH Batch: ${file.filename}`,
              fileInstanceId: file.id,
              partnerId: file.partnerId,
              status: 'OPEN',
              openedAtUtc: file.receivedAtUtc,
              slaDeadlineUtc: '16:45:00 UTC'
            };
            const agentRun = runExceptionAnalyst(relInc, file, contracts[0], partners[0], file.validationResult);
            setActiveAgentRun({ agentRun, incident: relInc });
            setInspectedFile(null);
          }}
        />
      )}

      {/* Metadata-driven source connection wizard */}
      {showConnectorWizard && (
        <ConnectorWizardModal onClose={() => setShowConnectorWizard(false)} />
      )}

      {/* Cryptographic Append-Only Audit Modal */}
      {showAuditModal && (
        <AuditLedgerModal 
          eventStore={eventStore}
          onClose={() => setShowAuditModal(false)}
        />
      )}

      {/* Direct Ingestion & Preset Harness Modal */}
      {showUploadModal && (
        <UploadModal 
          onClose={() => setShowUploadModal(false)}
          onFileIngested={(result, rawContent) => {
            handleFileIngested(result, rawContent);
            setShowUploadModal(false);
          }}
        />
      )}

      {/* Counterparty & Contract Configuration Manager Modal */}
      {showContractsModal && (
        <ContractConfigModal 
          onClose={() => setShowContractsModal(false)}
        />
      )}

      {/* Visual File Redliner & Fixed-Width Field Matrix Modal */}
      {showDiffModal && (
        <FileDiffModal 
          onClose={() => setShowDiffModal(false)}
        />
      )}
    </div>
  );
};
export default App;
```


## `src/components/SlaBoard.tsx`

**131 lines** · computed SLA state in the browser with assessOccurrenceSla. A second implementation of the scheduling the gateway performs, so the board could disagree with the deadline that was actually judged against. Replaced by FeedBoard, which renders what /sla-board returns and computes nothing.

```tsx
import React from 'react';
import { ExpectationOccurrence, Partner, FileContract } from '../types/financial';
import { assessOccurrenceSla } from '../scheduler/deadlineEngine';
import { Clock, ShieldAlert, CheckCircle2, AlertCircle, ArrowUpRight } from 'lucide-react';

interface SlaBoardProps {
  occurrences: ExpectationOccurrence[];
  contracts: FileContract[];
  partners: Partner[];
  onSelectOccurrence: (occurrence: ExpectationOccurrence) => void;
}

export const SlaBoard: React.FC<SlaBoardProps> = ({
  occurrences,
  contracts,
  partners,
  onSelectOccurrence
}) => {
  return (
    <div className="glass-panel" style={{ padding: '20px' }}>
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        marginBottom: '16px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Clock size={18} color="var(--accent-cyan)" />
          <h2 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
            Expected Delivery Windows & SLA Breach Radar
          </h2>
        </div>
        <span className="badge badge-neutral">
          {occurrences.length} Active Windows
        </span>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: '16px' }}>
        {occurrences.map(occ => {
          const contract = contracts.find(c => c.id === occ.contractId);
          const partner = partners.find(p => p.id === occ.partnerId);
          const assessment = assessOccurrenceSla(occ);

          return (
            <div 
              key={occ.id}
              onClick={() => onSelectOccurrence(occ)}
              style={{
                background: 'var(--bg-secondary)',
                border: `1px solid ${assessment.isBreached ? 'rgba(239, 68, 68, 0.4)' : 'var(--border-subtle)'}`,
                borderRadius: '8px',
                padding: '16px',
                cursor: 'pointer',
                transition: 'all 0.2s ease',
                position: 'relative',
                overflow: 'hidden'
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.borderColor = 'var(--border-active)';
                e.currentTarget.style.transform = 'translateY(-2px)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.borderColor = assessment.isBreached ? 'rgba(239, 68, 68, 0.4)' : 'var(--border-subtle)';
                e.currentTarget.style.transform = 'translateY(0)';
              }}
            >
              {/* Top Row: Partner & Status */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '8px' }}>
                <div>
                  <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                    {partner?.name || 'Counterparty'}
                  </span>
                  <h3 style={{ fontSize: '0.9375rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '2px' }}>
                    {contract?.name || occ.expectedDescription}
                  </h3>
                </div>
                <span className={`badge badge-${assessment.badgeVariant}`}>
                  {assessment.statusLabel}
                </span>
              </div>

              {/* Middle Row: Times & Breach Probability */}
              <div style={{
                background: 'var(--bg-primary)',
                padding: '10px 12px',
                borderRadius: '6px',
                margin: '12px 0',
                display: 'grid',
                gridTemplateColumns: '1fr 1fr',
                gap: '8px',
                fontSize: '0.8125rem'
              }}>
                <div>
                  <div style={{ color: 'var(--text-muted)', fontSize: '0.7rem' }}>SLA DEADLINE (UTC)</div>
                  <div className="font-mono" style={{ fontWeight: 600, color: 'var(--text-primary)', marginTop: '2px' }}>
                    {occ.dueAtUtc.substring(11, 19)}
                  </div>
                </div>
                <div>
                  <div style={{ color: 'var(--text-muted)', fontSize: '0.7rem' }}>GRACE EXPIRY</div>
                  <div className="font-mono" style={{ fontWeight: 600, color: assessment.isBreached ? 'var(--accent-crimson)' : 'var(--text-secondary)', marginTop: '2px' }}>
                    {occ.graceExpiresAtUtc.substring(11, 19)}
                  </div>
                </div>
              </div>

              {/* Bottom Progress & Risk Indicator */}
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: '0.75rem' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  {assessment.isBreached ? (
                    <ShieldAlert size={14} color="var(--accent-crimson)" />
                  ) : assessment.occurrence.status === 'VALID' ? (
                    <CheckCircle2 size={14} color="var(--accent-emerald)" />
                  ) : (
                    <AlertCircle size={14} color="var(--accent-amber)" />
                  )}
                  <span style={{ color: 'var(--text-secondary)' }}>
                    Breach Probability: <strong style={{ color: assessment.breachProbability > 0.5 ? 'var(--accent-crimson)' : 'var(--text-primary)' }}>{(assessment.breachProbability * 100).toFixed(0)}%</strong>
                  </span>
                </div>
                <span style={{ color: 'var(--accent-cyan)', display: 'flex', alignItems: 'center', gap: '2px' }}>
                  Inspect <ArrowUpRight size={12} />
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
```


## `src/components/AuditLedgerModal.tsx`

**241 lines** · rendered the browser-side TamperEvidentEventStore. It verified a chain the browser had built, which proves nothing about the record the server holds. Replaced by EvidenceTimeline, which reads the server chain and states its verification separately from the paged list.

```tsx
import React, { useState, useEffect } from 'react';
import { DomainEvent } from '../types/financial';
import { TamperEvidentEventStore } from '../audit/hashChain';
import { X, ShieldCheck, Download, CheckCircle } from 'lucide-react';

interface AuditLedgerModalProps {
  eventStore: TamperEvidentEventStore;
  onClose: () => void;
}

export const AuditLedgerModal: React.FC<AuditLedgerModalProps> = ({
  eventStore,
  onClose
}) => {
  const [events, setEvents] = useState<DomainEvent[]>([]);
  const [integrity, setIntegrity] = useState<{ isValid: boolean; totalEvents: number; error?: string }>({
    isValid: true,
    totalEvents: 0
  });

  useEffect(() => {
    const loadAndVerify = async () => {
      const allEvents = eventStore.getEvents();
      setEvents(allEvents);
      const res = await eventStore.verifyIntegrity();
      setIntegrity(res);
    };
    loadAndVerify();
  }, [eventStore]);

  const handleExportJson = () => {
    const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(events, null, 2));
    const downloadAnchor = document.createElement('a');
    downloadAnchor.setAttribute("href", dataStr);
    downloadAnchor.setAttribute("download", `sentinel-flow-audit-ledger-${Date.now()}.json`);
    document.body.appendChild(downloadAnchor);
    downloadAnchor.click();
    downloadAnchor.remove();
  };

  const handleExportCompliancePackage = async () => {
    try {
      const pkg = await fetch('http://localhost:8080/api/v1/compliance/export').then(r => r.json());
      const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(pkg, null, 2));
      const downloadAnchor = document.createElement('a');
      downloadAnchor.setAttribute("href", dataStr);
      downloadAnchor.setAttribute("download", `SEC_17a4_COMPLIANCE_PROOF_PACKAGE_${Date.now()}.json`);
      document.body.appendChild(downloadAnchor);
      downloadAnchor.click();
      downloadAnchor.remove();
    } catch {
      handleExportJson();
    }
  };

  return (
    <div style={{
      position: 'fixed',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      background: 'rgba(5, 8, 14, 0.85)',
      backdropFilter: 'blur(8px)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 110,
      padding: '24px'
    }}>
      <div style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-medium)',
        borderRadius: '12px',
        width: '100%',
        maxWidth: '920px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 20px 40px rgba(0, 0, 0, 0.8)',
        overflow: 'hidden'
      }}>
        {/* Header */}
        <div style={{
          padding: '16px 24px',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'var(--bg-card)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{
              width: '32px',
              height: '32px',
              borderRadius: '6px',
              background: 'var(--accent-emerald-dim)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: '1px solid rgba(16, 185, 129, 0.4)'
            }}>
              <ShieldCheck size={18} color="var(--accent-emerald)" />
            </div>
            <div>
              <h2 style={{ fontSize: '1.0625rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                Append-Only Audit Ledger
              </h2>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Application hash chain — linear SHA-256 predecessor links, verified by recomputation
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <button 
              className="btn btn-primary"
              onClick={handleExportCompliancePackage}
              style={{ fontSize: '0.8125rem', padding: '6px 12px', display: 'flex', alignItems: 'center', gap: '6px', background: 'var(--accent-emerald)', borderColor: 'var(--accent-emerald)', color: '#000', fontWeight: 700 }}
            >
              <Download size={14} />
              <span>Download Evidence Export</span>
            </button>
            <button 
              className="btn btn-secondary"
              onClick={handleExportJson}
              style={{ fontSize: '0.8125rem', padding: '6px 12px' }}
            >
              <Download size={14} />
              <span>Export Raw Ledger (JSON)</span>
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

        {/* Verification Status Banner */}
        <div style={{
          padding: '12px 24px',
          background: integrity.isValid ? 'var(--accent-emerald-dim)' : 'var(--accent-crimson-dim)',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: '0.8125rem'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {integrity.isValid ? (
              <>
                <CheckCircle size={16} color="var(--accent-emerald)" />
                <span style={{ color: 'var(--accent-emerald)', fontWeight: 600 }}>
                  Cryptographic Chain Verified ({events.length} blocks without tampering)
                </span>
              </>
            ) : (
              <span style={{ color: 'var(--accent-crimson)', fontWeight: 600 }}>
                {integrity.error}
              </span>
            )}
          </div>
          <span className="font-mono" style={{ color: 'var(--text-secondary)', fontSize: '0.75rem' }}>
            Algorithm: SHA-256 linear hash chain (not a Merkle tree)
          </span>
        </div>

        {/* Events List */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {events.length === 0 ? (
            <div style={{ textAlign: 'center', color: 'var(--text-muted)', padding: '40px' }}>
              No domain events recorded in session yet.
            </div>
          ) : (
            events.map((evt, idx) => (
              <div 
                key={evt.id}
                style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '8px',
                  padding: '14px 16px',
                  position: 'relative'
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '8px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <span className="badge badge-cyan">#{idx + 1} {evt.eventType}</span>
                    <span className="badge badge-neutral">{evt.aggregateType}:{evt.aggregateId}</span>
                  </div>
                  <span className="font-mono" style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                    {evt.timestampUtc}
                  </span>
                </div>

                {/* Hashes Row */}
                <div style={{
                  background: 'var(--bg-secondary)',
                  padding: '8px 12px',
                  borderRadius: '4px',
                  fontSize: '0.7rem',
                  fontFamily: 'var(--font-mono)',
                  marginBottom: '8px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '4px'
                }}>
                  <div style={{ display: 'flex', gap: '8px' }}>
                    <span style={{ color: 'var(--text-dim)', width: '90px' }}>PREV HASH:</span>
                    <span style={{ color: 'var(--text-muted)' }}>{evt.previousHash}</span>
                  </div>
                  <div style={{ display: 'flex', gap: '8px' }}>
                    <span style={{ color: 'var(--accent-cyan)', width: '90px', fontWeight: 600 }}>BLOCK HASH:</span>
                    <span style={{ color: 'var(--accent-cyan)' }}>{evt.currentHash}</span>
                  </div>
                </div>

                {/* Payload snippet */}
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                  <span style={{ color: 'var(--text-muted)' }}>Actor: </span>
                  <strong>{evt.actor}</strong>
                  <span style={{ margin: '0 8px', color: 'var(--border-subtle)' }}>|</span>
                  <span style={{ color: 'var(--text-muted)' }}>Correlation ID: </span>
                  <span className="font-mono">{evt.correlationId}</span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
```


## `src/components/AiAnalystPanel.tsx`

**311 lines** · fed by the triage fallback. When the AI tier was unreachable the panel showed a locally generated "analysis" with confidence, hypotheses and citations, indistinguishable from one a model produced.

```tsx
import React, { useState } from 'react';
import { AgentRun, Incident, Approval } from '../types/financial';
import { Bot, Shield, CheckCircle2, AlertOctagon, Send, Lock, Copy, Check } from 'lucide-react';

interface AiAnalystPanelProps {
  agentRun: AgentRun;
  incident: Incident;
  onApproveAction: (actionType: Approval['actionType'], reason: string) => void;
  onClose: () => void;
}

export const AiAnalystPanel: React.FC<AiAnalystPanelProps> = ({
  agentRun,
  incident,
  onApproveAction,
  onClose
}) => {
  const [approvalReason, setApprovalReason] = useState<string>('');
  const [copiedDraft, setCopiedDraft] = useState<boolean>(false);
  const [submittedAction, setSubmittedAction] = useState<boolean>(false);

  const handleCopyDraft = () => {
    if (agentRun.draftExternalPartnerNotice) {
      navigator.clipboard.writeText(agentRun.draftExternalPartnerNotice);
      setCopiedDraft(true);
      setTimeout(() => setCopiedDraft(false), 2000);
    }
  };

  const handleExecuteApproval = (actionType: Approval['actionType']) => {
    onApproveAction(actionType, approvalReason || 'Operator confirmed pre-flight exception and authorized workflow transition.');
    setSubmittedAction(true);
  };

  return (
    <div className="glass-panel" style={{ padding: '24px', border: '1px solid rgba(99, 102, 241, 0.3)' }}>
      {/* Top Banner: Model & Agent ID */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingBottom: '16px',
        borderBottom: '1px solid var(--border-subtle)',
        marginBottom: '20px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <div style={{
            width: '36px',
            height: '36px',
            borderRadius: '8px',
            background: 'var(--accent-violet-dim)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            border: '1px solid rgba(99, 102, 241, 0.4)'
          }}>
            <Bot size={20} color="var(--accent-violet)" />
          </div>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                {agentRun.agentVersion}
              </h3>
              <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>
                Constrained RRR-Model
              </span>
            </div>
            <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
              Execution Target: <strong style={{ color: 'var(--text-secondary)' }}>{agentRun.modelIdentifier}</strong> | Run ID: {agentRun.id}
            </p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <span className="badge badge-neutral" style={{ fontSize: '0.7rem' }}>
            Latency: {agentRun.metrics.durationMs}ms
          </span>
          <span className="badge badge-neutral" style={{ fontSize: '0.7rem' }}>
            Tokens: {agentRun.metrics.inputTokens + agentRun.metrics.outputTokens}
          </span>
        </div>
      </div>

      {/* Safety Notice: Zero Autonomous Execution */}
      <div style={{
        background: 'rgba(99, 102, 241, 0.08)',
        border: '1px solid rgba(99, 102, 241, 0.25)',
        borderRadius: '6px',
        padding: '10px 14px',
        marginBottom: '20px',
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        fontSize: '0.8125rem',
        color: '#C7D2FE'
      }}>
        <Shield size={16} color="var(--accent-violet)" />
        <span>
          <strong>Constrained Agent Safety Guarantee:</strong> This AI agent holds read-only tools and cannot directly alter financial records or bypass dual-control release policies without signed human supervisor authorization.
        </span>
      </div>

      {/* Findings Summary */}
      <div style={{ marginBottom: '20px' }}>
        <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>
          1. Evidence Synthesis & Citations
        </h4>
        <div style={{
          background: 'var(--bg-primary)',
          border: '1px solid var(--border-subtle)',
          borderRadius: '6px',
          padding: '14px',
          fontSize: '0.875rem',
          color: 'var(--text-secondary)',
          lineHeight: 1.6
        }}>
          {agentRun.findingsSummary}

          <div style={{ marginTop: '12px', display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
            {agentRun.citedFindingCodes.map((code, idx) => (
              <span key={idx} className="badge badge-danger" style={{ fontSize: '0.7rem' }}>
                Cited Finding: {code}
              </span>
            ))}
            {agentRun.citedRunbookSections.map((rb, idx) => (
              <span key={idx} className="badge badge-cyan" style={{ fontSize: '0.7rem' }}>
                {rb}
              </span>
            ))}
          </div>
        </div>
      </div>

      {/* Hypotheses */}
      <div style={{ marginBottom: '20px' }}>
        <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>
          2. Root-Cause Hypotheses
        </h4>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          {agentRun.hypotheses.map((hypo, idx) => (
            <div 
              key={idx}
              style={{
                background: 'var(--bg-secondary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                padding: '12px 14px'
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                <span style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Hypothesis #{idx + 1}: {hypo.hypothesis}
                </span>
                <span className={`badge ${hypo.confidence === 'HIGH' ? 'badge-warning' : 'badge-neutral'}`}>
                  Confidence: {hypo.confidence}
                </span>
              </div>
              <ul style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', paddingLeft: '20px', marginTop: '4px' }}>
                {hypo.supportingEvidence.map((ev, i) => (
                  <li key={i}>{ev}</li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </div>

      {/* Action Plan & Human Approval Gate */}
      <div style={{ marginBottom: '20px' }}>
        <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '8px' }}>
          3. Proposed Remediation Plan & Authority Tiers
        </h4>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          {agentRun.proposedActionPlan.map((action, idx) => (
            <div 
              key={idx}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '10px 14px',
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                fontSize: '0.8125rem'
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                <span className="badge badge-neutral">Step {action.step}</span>
                <span style={{ color: 'var(--text-primary)' }}>{action.action}</span>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span className="badge badge-cyan">Tier {action.authorityTier}</span>
                {action.requiresHumanApproval ? (
                  <span className="badge badge-danger">Requires Approval</span>
                ) : (
                  <span className="badge badge-success">Automated / Read-Only</span>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* External Partner Notification Draft */}
      {agentRun.draftExternalPartnerNotice && (
        <div style={{ marginBottom: '20px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
            <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              4. Drafted Counterparty SLA Notification
            </h4>
            <button 
              className="btn btn-secondary"
              onClick={handleCopyDraft}
              style={{ fontSize: '0.75rem', padding: '4px 10px' }}
            >
              {copiedDraft ? <Check size={12} color="var(--accent-emerald)" /> : <Copy size={12} />}
              <span>{copiedDraft ? 'Copied to Clipboard' : 'Copy Draft Notice'}</span>
            </button>
          </div>
          <div className="log-viewer" style={{ whiteSpace: 'pre-wrap', maxHeight: '180px' }}>
            {agentRun.draftExternalPartnerNotice}
          </div>
        </div>
      )}

      {/* Human Supervisor Approval Box */}
      <div style={{
        background: 'var(--bg-card)',
        border: '1px solid var(--border-medium)',
        borderRadius: '8px',
        padding: '16px',
        marginTop: '12px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
          <Lock size={16} color="var(--accent-amber)" />
          <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
            Human Supervisor Sign-Off & Dual-Control Gate
          </h4>
        </div>

        {submittedAction ? (
          <div style={{
            padding: '12px',
            background: 'var(--accent-emerald-dim)',
            border: '1px solid rgba(16, 185, 129, 0.4)',
            borderRadius: '6px',
            color: 'var(--accent-emerald)',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            fontSize: '0.875rem'
          }}>
            <CheckCircle2 size={18} />
            <span>Action authorized and committed to tamper-evident audit ledger.</span>
          </div>
        ) : (
          <div>
            <input 
              type="text"
              placeholder="Enter a justification for this decision (required, recorded in the audit ledger)..."
              value={approvalReason}
              onChange={(e) => setApprovalReason(e.target.value)}
              style={{
                width: '100%',
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '6px',
                padding: '10px 14px',
                color: 'var(--text-primary)',
                fontSize: '0.8125rem',
                marginBottom: '12px',
                outline: 'none'
              }}
            />

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
              <button 
                className="btn btn-secondary"
                onClick={onClose}
                style={{ fontSize: '0.8125rem' }}
              >
                Close Panel
              </button>
              {incident.type === 'MISSING_FILE_DEADLINE' ? (
                <button 
                  className="btn btn-primary"
                  onClick={() => handleExecuteApproval('WAIVE_MISSING_FILE')}
                  style={{ fontSize: '0.8125rem' }}
                >
                  <Send size={14} />
                  <span>Authorize Temporary SLA Extension</span>
                </button>
              ) : (
                <button 
                  className="btn btn-danger"
                  onClick={() => handleExecuteApproval('EXCEPTIONAL_RELEASE')}
                  style={{ fontSize: '0.8125rem' }}
                >
                  <AlertOctagon size={14} />
                  <span>Authorize Exceptional Release</span>
                </button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
```


## `src/components/FileDiffModal.tsx`

**507 lines** · diffed SAMPLE_VALID_NACHA against SAMPLE_CORRUPTED_NACHA -- two constants from the synthetic corpus. It compared demo data with demo data and presented the result as a file comparison.

```tsx
import React, { useState } from 'react';
import { 
  GitCompare, 
  X, 
  CheckCircle2, 
  AlertTriangle, 
  Layers, 
  ShieldCheck, 
  Key, 
  Copy, 
  Check, 
  FileText 
} from 'lucide-react';
import { SAMPLE_VALID_NACHA, SAMPLE_CORRUPTED_NACHA } from '../mockData/syntheticCorpus';

interface FileDiffModalProps {
  currentFilename?: string;
  currentContent?: string;
  onClose: () => void;
}

export const FileDiffModal: React.FC<FileDiffModalProps> = ({
  currentFilename = 'MERIDIAN_ACH_COMMERCIAL_20260814_1645.txt',
  currentContent = SAMPLE_CORRUPTED_NACHA,
  onClose
}) => {
  const [selectedLineIndex, setSelectedLineIndex] = useState<number>(0);
  const [copied, setCopied] = useState<boolean>(false);
  const [activeTab, setActiveTab] = useState<'DIFF' | 'SECURITY_VERIFY'>('DIFF');

  // Security Verification Tool State
  const [sshKeyInput, setSshKeyInput] = useState<string>(
    'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIH1N8sDqR3kH8m9qWeRt7y4a3F9cKz8eL1mQxWpTvBn9 treasury-operator@meridian.internal'
  );
  const [sshResult, setSshResult] = useState<any>(null);
  const [isVerifyingSsh, setIsVerifyingSsh] = useState<boolean>(false);

  const baselineLines = SAMPLE_VALID_NACHA.split('\n');
  const targetLines = currentContent.split(/\r?\n/).filter(l => l.length > 0);

  const selectedTargetLine = targetLines[selectedLineIndex] || '';

  // Decompose NACHA Record 6 or 5 into Field Matrix
  const getFieldMatrix = (line: string) => {
    if (!line || line.length === 0) return [];
    const recordType = line.charAt(0);

    if (recordType === '1') {
      return [
        { offset: '00-01', length: 1, name: 'Record Type', value: line.substring(0, 1), rule: 'Constant "1" (File Header)' },
        { offset: '01-03', length: 2, name: 'Priority Code', value: line.substring(1, 3), rule: 'Default "01"' },
        { offset: '03-13', length: 10, name: 'Immediate Destination', value: line.substring(3, 13), rule: '10-char Routing / Routing Space' },
        { offset: '13-23', length: 10, name: 'Immediate Origin', value: line.substring(13, 23), rule: '10-char ODFI Origin' },
        { offset: '23-29', length: 6, name: 'File Creation Date', value: line.substring(23, 29), rule: 'YYMMDD format' },
        { offset: '29-33', length: 4, name: 'File Creation Time', value: line.substring(29, 33), rule: 'HHMM format' },
        { offset: '33-34', length: 1, name: 'File ID Modifier', value: line.substring(33, 34), rule: 'A-Z or 0-9' },
        { offset: '34-37', length: 3, name: 'Record Size', value: line.substring(34, 37), rule: 'Constant "094"' },
        { offset: '37-39', length: 2, name: 'Blocking Factor', value: line.substring(37, 39), rule: 'Constant "10"' },
        { offset: '39-40', length: 1, name: 'Format Code', value: line.substring(39, 40), rule: 'Constant "1"' },
        { offset: '40-63', length: 23, name: 'Immediate Destination Name', value: line.substring(40, 63), rule: 'Receiving Gateway' },
        { offset: '63-86', length: 23, name: 'Immediate Origin Name', value: line.substring(63, 86), rule: 'Originating Institution' },
      ];
    }

    if (recordType === '5') {
      return [
        { offset: '00-01', length: 1, name: 'Record Type', value: line.substring(0, 1), rule: 'Constant "5" (Batch Header)' },
        { offset: '01-04', length: 3, name: 'Service Class Code', value: line.substring(1, 4), rule: '200 (Mixed), 220 (Credits), 225 (Debits)' },
        { offset: '04-20', length: 16, name: 'Company Name', value: line.substring(4, 20), rule: 'Originator Name' },
        { offset: '20-40', length: 20, name: 'Company Discretionary Data', value: line.substring(20, 40), rule: 'Optional Discretionary' },
        { offset: '40-50', length: 10, name: 'Company Identification', value: line.substring(40, 50), rule: '10-char IRS/ODFI ID' },
        { offset: '50-53', length: 3, name: 'Standard Entry Class (SEC)', value: line.substring(50, 53), rule: 'PPD, CCD, CTX, WEB' },
        { offset: '53-63', length: 10, name: 'Company Entry Description', value: line.substring(53, 63), rule: 'Purpose (e.g. PAYROLL)' },
        { offset: '63-69', length: 6, name: 'Company Descriptive Date', value: line.substring(63, 69), rule: 'YYMMDD format' },
        { offset: '69-75', length: 6, name: 'Effective Entry Date', value: line.substring(69, 75), rule: 'Settlement date' },
        { offset: '75-78', length: 3, name: 'Settlement Date', value: line.substring(75, 78), rule: 'Julian date (reserved)' },
        { offset: '78-79', length: 1, name: 'Originator Status Code', value: line.substring(78, 79), rule: 'Constant "1"' },
        { offset: '79-87', length: 8, name: 'ODFI Identification', value: line.substring(79, 87), rule: 'Originating Routing prefix' },
        { offset: '87-94', length: 7, name: 'Batch Number', value: line.substring(87, 94), rule: 'Sequential batch integer' }
      ];
    }

    if (recordType === '6') {
      return [
        { offset: '00-01', length: 1, name: 'Record Type', value: line.substring(0, 1), rule: 'Constant "6" (Entry Detail)' },
        { offset: '01-03', length: 2, name: 'Transaction Code', value: line.substring(1, 3), rule: '22 (DDA Credit), 27 (DDA Debit)' },
        { offset: '03-12', length: 9, name: 'Receiving DFI Routing', value: line.substring(3, 12), rule: 'Must pass Mod10 Check Digit' },
        { offset: '12-29', length: 17, name: 'DFI Account Number', value: line.substring(12, 29), rule: 'Left-justified account' },
        { offset: '29-39', length: 10, name: 'Amount (in cents)', value: line.substring(29, 39), rule: '$$$$.cc numeric' },
        { offset: '39-54', length: 15, name: 'Individual ID / Number', value: line.substring(39, 54), rule: 'Employee / Invoice Ref' },
        { offset: '54-76', length: 22, name: 'Individual Name', value: line.substring(54, 76), rule: 'Counterparty Name' },
        { offset: '76-78', length: 2, name: 'Discretionary Data', value: line.substring(76, 78), rule: '2-char optional' },
        { offset: '78-79', length: 1, name: 'Addenda Record Indicator', value: line.substring(78, 79), rule: '0 = No addenda, 1 = Addenda' },
        { offset: '79-94', length: 15, name: 'Trace Number', value: line.substring(79, 94), rule: 'ODFI ID (8) + Sequence (7)' }
      ];
    }

    if (recordType === '8') {
      return [
        { offset: '00-01', length: 1, name: 'Record Type', value: line.substring(0, 1), rule: 'Constant "8" (Batch Control)' },
        { offset: '01-04', length: 3, name: 'Service Class Code', value: line.substring(1, 4), rule: 'Matches Record 5 Service Class' },
        { offset: '04-10', length: 6, name: 'Entry/Addenda Count', value: line.substring(4, 10), rule: 'Total count of 6 & 7 records' },
        { offset: '10-20', length: 10, name: 'Entry Hash', value: line.substring(10, 20), rule: '10-digit sum of routing prefixes' },
        { offset: '20-32', length: 12, name: 'Total Debit Amount', value: line.substring(20, 32), rule: 'Total debit cents' },
        { offset: '32-44', length: 12, name: 'Total Credit Amount', value: line.substring(32, 44), rule: 'Total credit cents' },
        { offset: '44-54', length: 10, name: 'Company Identification', value: line.substring(44, 54), rule: 'Matches Record 5 Company ID' },
        { offset: '79-87', length: 8, name: 'ODFI Identification', value: line.substring(79, 87), rule: 'Originating Routing prefix' },
        { offset: '87-94', length: 7, name: 'Batch Number', value: line.substring(87, 94), rule: 'Matches Record 5 Batch Number' }
      ];
    }

    return [
      { offset: '00-94', length: line.length, name: `Record Type ${recordType}`, value: line, rule: 'Standard 94-char fixed-width record' }
    ];
  };

  const handleVerifySsh = async () => {
    setIsVerifyingSsh(true);
    try {
      const res = await fetch('http://localhost:8080/api/v1/security/verify-key', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key: sshKeyInput })
      });
      if (res.ok) {
        const data = await res.json();
        setSshResult(data);
      } else {
        setSshResult({ isValid: false, statusReason: 'Invalid public key format or unsupported algorithm.' });
      }
    } catch {
      // Fallback
      setSshResult({
        isValid: true,
        keyType: 'ssh-ed25519',
        fingerprint: 'SHA256:4a3F9cKz8eL1mQxWpTvBn9Rt6sDuFhJ2kLm8qWeRt7y',
        keyBits: 256,
        verifiedAt: new Date().toISOString(),
        comment: 'treasury-operator@meridian.internal'
      });
    } finally {
      setIsVerifyingSsh(false);
    }
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(currentContent);
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
              <GitCompare size={20} color="var(--accent-cyan)" />
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Visual File Redliner & Fixed-Width Matrix Inspector
                </h3>
                              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Inspect byte offsets, rule boundaries, and side-by-side golden baseline diffs
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <button
              onClick={handleCopy}
              className="btn btn-secondary"
              style={{ fontSize: '0.75rem', padding: '6px 12px', display: 'flex', alignItems: 'center', gap: '6px' }}
            >
              {copied ? <Check size={14} color="var(--accent-emerald)" /> : <Copy size={14} />}
              <span>{copied ? 'Copied' : 'Copy Content'}</span>
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

        {/* Tab Navigation */}
        <div style={{
          display: 'flex',
          borderBottom: '1px solid var(--border-subtle)',
          padding: '0 24px',
          background: 'rgba(14, 20, 34, 0.4)'
        }}>
          <button
            onClick={() => setActiveTab('DIFF')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'DIFF' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'DIFF' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Layers size={14} />
            <span>Side-by-Side Diff & Field Matrix</span>
          </button>
          <button
            onClick={() => setActiveTab('SECURITY_VERIFY')}
            style={{
              padding: '12px 16px',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'SECURITY_VERIFY' ? '2px solid var(--accent-emerald)' : '2px solid transparent',
              color: activeTab === 'SECURITY_VERIFY' ? 'var(--accent-emerald)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <ShieldCheck size={14} />
            <span>SFTP Key Vault & Signature Verifier</span>
          </button>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {activeTab === 'DIFF' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
              {/* Dual File Line Viewer */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px' }}>
                {/* Baseline Golden Template */}
                <div style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '8px',
                  padding: '14px'
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '8px' }}>
                    <span style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--accent-emerald)', display: 'flex', alignItems: 'center', gap: '6px' }}>
                      <CheckCircle2 size={14} /> Golden Baseline (Expected Structure)
                    </span>
                    <span className="badge badge-neutral" style={{ fontSize: '0.65rem' }}>94 Chars / Line</span>
                  </div>

                  <div className="font-mono" style={{ fontSize: '0.7rem', display: 'flex', flexDirection: 'column', gap: '4px' }}>
                    {baselineLines.slice(0, 7).map((line, idx) => (
                      <div
                        key={idx}
                        onClick={() => setSelectedLineIndex(idx)}
                        style={{
                          padding: '6px 8px',
                          borderRadius: '4px',
                          cursor: 'pointer',
                          background: selectedLineIndex === idx ? 'rgba(6, 182, 212, 0.15)' : 'rgba(255, 255, 255, 0.02)',
                          border: selectedLineIndex === idx ? '1px solid var(--accent-cyan)' : '1px solid transparent',
                          color: '#F8FAFC',
                          whiteSpace: 'nowrap',
                          overflow: 'hidden',
                          textOverflow: 'ellipsis'
                        }}
                      >
                        <span style={{ color: 'var(--text-muted)', marginRight: '8px' }}>{idx + 1}</span>
                        {line}
                      </div>
                    ))}
                  </div>
                </div>

                {/* Target Counterparty File */}
                <div style={{
                  background: 'var(--bg-primary)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: '8px',
                  padding: '14px'
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '8px' }}>
                    <span style={{ fontSize: '0.75rem', fontWeight: 600, color: 'var(--accent-crimson)', display: 'flex', alignItems: 'center', gap: '6px' }}>
                      <AlertTriangle size={14} /> Ingested Transmission ({currentFilename})
                    </span>
                    <span className="badge badge-crimson" style={{ fontSize: '0.65rem' }}>Quarantine Candidate</span>
                  </div>

                  <div className="font-mono" style={{ fontSize: '0.7rem', display: 'flex', flexDirection: 'column', gap: '4px' }}>
                    {targetLines.slice(0, 7).map((line, idx) => {
                      const isMismatch = line !== baselineLines[idx];
                      return (
                        <div
                          key={idx}
                          onClick={() => setSelectedLineIndex(idx)}
                          style={{
                            padding: '6px 8px',
                            borderRadius: '4px',
                            cursor: 'pointer',
                            background: selectedLineIndex === idx 
                              ? 'rgba(239, 68, 68, 0.2)' 
                              : isMismatch ? 'rgba(239, 68, 68, 0.08)' : 'rgba(255, 255, 255, 0.02)',
                            border: selectedLineIndex === idx 
                              ? '1px solid var(--accent-crimson)' 
                              : isMismatch ? '1px dashed rgba(239, 68, 68, 0.4)' : '1px solid transparent',
                            color: isMismatch ? '#FCA5A5' : '#F8FAFC',
                            whiteSpace: 'nowrap',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis'
                          }}
                        >
                          <span style={{ color: 'var(--text-muted)', marginRight: '8px' }}>{idx + 1}</span>
                          {line}
                        </div>
                      );
                    })}
                  </div>
                </div>
              </div>

              {/* Field Matrix Decomposition for Selected Line */}
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '16px'
              }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '12px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <FileText size={16} color="var(--accent-cyan)" />
                    <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                      Line {selectedLineIndex + 1} Field Offset Matrix (Record Type {selectedTargetLine.charAt(0) || '1'})
                    </h4>
                  </div>
                  <span className="font-mono" style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                    Record Length: {selectedTargetLine.length} / 94 Chars
                  </span>
                </div>

                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', fontSize: '0.75rem', borderCollapse: 'collapse', textAlign: 'left' }}>
                    <thead>
                      <tr style={{ borderBottom: '1px solid var(--border-subtle)', color: 'var(--text-muted)' }}>
                        <th style={{ padding: '8px' }}>Offset</th>
                        <th style={{ padding: '8px' }}>Field Name</th>
                        <th style={{ padding: '8px' }}>Len</th>
                        <th style={{ padding: '8px' }}>Extracted Value</th>
                        <th style={{ padding: '8px' }}>Nacha Specification Rule</th>
                      </tr>
                    </thead>
                    <tbody>
                      {getFieldMatrix(selectedTargetLine).map((field, i) => (
                        <tr 
                          key={i} 
                          style={{ 
                            borderBottom: '1px solid rgba(255, 255, 255, 0.03)',
                            background: i % 2 === 0 ? 'rgba(255, 255, 255, 0.01)' : 'transparent'
                          }}
                        >
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--accent-cyan)' }}>{field.offset}</td>
                          <td style={{ padding: '8px', fontWeight: 600, color: 'var(--text-primary)' }}>{field.name}</td>
                          <td className="font-mono" style={{ padding: '8px', color: 'var(--text-muted)' }}>{field.length}</td>
                          <td className="font-mono" style={{ padding: '8px', color: '#F8FAFC', background: 'rgba(0,0,0,0.2)', borderRadius: '4px' }}>
                            {field.value || '<EMPTY>'}
                          </td>
                          <td style={{ padding: '8px', color: 'var(--text-secondary)' }}>{field.rule}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'SECURITY_VERIFY' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div style={{
                background: 'var(--bg-primary)',
                border: '1px solid var(--border-subtle)',
                borderRadius: '8px',
                padding: '16px'
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px' }}>
                  <Key size={16} color="var(--accent-emerald)" />
                  <h4 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    SFTP Public Key Fingerprint & Cryptographic Verifier
                  </h4>
                </div>
                <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '12px' }}>
                  Test counterparty SSH public keys (Ed25519 / RSA-4096) before authorizing SFTP ingress tunnels.
                </p>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                  <textarea
                    rows={3}
                    className="font-mono"
                    value={sshKeyInput}
                    onChange={(e) => setSshKeyInput(e.target.value)}
                    style={{
                      width: '100%',
                      background: 'rgba(0, 0, 0, 0.3)',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: '6px',
                      padding: '10px',
                      color: 'var(--text-primary)',
                      fontSize: '0.75rem'
                    }}
                  />

                  <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                    <button
                      className="btn btn-primary"
                      disabled={isVerifyingSsh}
                      onClick={handleVerifySsh}
                      style={{ fontSize: '0.75rem', padding: '6px 14px' }}
                    >
                      {isVerifyingSsh ? 'Computing Fingerprint...' : 'Verify SSH Key'}
                    </button>
                  </div>
                </div>

                {sshResult && (
                  <div style={{
                    marginTop: '14px',
                    padding: '12px',
                    borderRadius: '6px',
                    background: sshResult.isValid ? 'rgba(16, 185, 129, 0.1)' : 'rgba(239, 68, 68, 0.1)',
                    border: sshResult.isValid ? '1px solid rgba(16, 185, 129, 0.3)' : '1px solid rgba(239, 68, 68, 0.3)'
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      {sshResult.isValid ? (
                        <CheckCircle2 size={16} color="var(--accent-emerald)" />
                      ) : (
                        <AlertTriangle size={16} color="var(--accent-crimson)" />
                      )}
                      <span style={{ fontWeight: 600, fontSize: '0.8125rem', color: '#F8FAFC' }}>
                        {sshResult.isValid ? 'Valid Public Key Detected' : 'Verification Failed'}
                      </span>
                    </div>

                    {sshResult.isValid && (
                      <div className="font-mono" style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', marginTop: '6px', display: 'flex', flexDirection: 'column', gap: '4px' }}>
                        <div><strong>Key Type:</strong> {sshResult.keyType} ({sshResult.keyBits} bits)</div>
                        <div><strong>SHA256 Fingerprint:</strong> {sshResult.fingerprint}</div>
                        <div><strong>Comment:</strong> {sshResult.comment || 'N/A'}</div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
```


## `src/components/FileInspectorModal.tsx`

**389 lines** · rendered a FileInstance assembled in the browser from an ingest response plus invented fields (s3Uri built by string concatenation, runId from Date.now()). Replaced by the artifact detail pane, which reads /artifacts/{id}.

```tsx
import React, { useState } from 'react';
import { FileInstance, Partner, FileContract } from '../types/financial';
import { X, ShieldAlert, CheckCircle, Terminal, Copy, Check } from 'lucide-react';

interface FileInspectorModalProps {
  fileInstance: FileInstance;
  partner?: Partner;
  contract?: FileContract;
  rawContent?: string;
  onClose: () => void;
  onLaunchAiTriage: (file: FileInstance) => void;
}

export const FileInspectorModal: React.FC<FileInspectorModalProps> = ({
  fileInstance,
  partner,
  contract,
  rawContent = '',
  onClose,
  onLaunchAiTriage
}) => {
  const [activeTab, setActiveTab] = useState<'findings' | 'raw' | 'controls'>('findings');
  const [copiedHash, setCopiedHash] = useState(false);

  const res = fileInstance.validationResult;
  const isQuarantined = fileInstance.state === 'QUARANTINED';

  const handleCopyHash = () => {
    navigator.clipboard.writeText(fileInstance.sha256Hash);
    setCopiedHash(true);
    setTimeout(() => setCopiedHash(false), 2000);
  };

  const rawLines = rawContent.split(/\r?\n/).filter(l => l.length > 0);

  return (
    <div style={{
      position: 'fixed',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      background: 'rgba(5, 8, 14, 0.85)',
      backdropFilter: 'blur(8px)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 100,
      padding: '24px'
    }}>
      <div style={{
        background: 'var(--bg-secondary)',
        border: '1px solid var(--border-medium)',
        borderRadius: '12px',
        width: '100%',
        maxWidth: '960px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 20px 40px rgba(0, 0, 0, 0.8)',
        overflow: 'hidden'
      }}>
        {/* Modal Header */}
        <div style={{
          padding: '16px 24px',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'var(--bg-card)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div style={{
              width: '32px',
              height: '32px',
              borderRadius: '6px',
              background: isQuarantined ? 'var(--accent-crimson-dim)' : 'var(--accent-emerald-dim)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              border: `1px solid ${isQuarantined ? 'rgba(239, 68, 68, 0.4)' : 'rgba(16, 185, 129, 0.4)'}`
            }}>
              {isQuarantined ? <ShieldAlert size={18} color="var(--accent-crimson)" /> : <CheckCircle size={18} color="var(--accent-emerald)" />}
            </div>
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <h2 style={{ fontSize: '1.0625rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  {fileInstance.filename}
                </h2>
                <span className={`badge ${isQuarantined ? 'badge-danger' : 'badge-success'}`}>
                  {fileInstance.state}
                </span>
              </div>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Partner: <strong style={{ color: 'var(--text-secondary)' }}>{partner?.name || 'Unknown'}</strong> | Contract: {contract?.name || 'Standard ACH'}
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <button 
              className="btn btn-primary"
              onClick={() => onLaunchAiTriage(fileInstance)}
              style={{ fontSize: '0.8125rem', padding: '6px 14px' }}
            >
              <Terminal size={14} />
              <span>Launch Astra Copilot</span>
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

        {/* Cryptographic Lineage Header Bar */}
        <div style={{
          padding: '10px 24px',
          background: 'var(--bg-primary)',
          borderBottom: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: '0.75rem',
          color: 'var(--text-secondary)'
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <span style={{ color: 'var(--text-muted)' }}>SHA-256 Digest:</span>
            <span className="font-mono" style={{ color: 'var(--accent-cyan)' }}>
              {fileInstance.sha256Hash}
            </span>
            <button 
              onClick={handleCopyHash}
              style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center' }}
              title="Copy hash"
            >
              {copiedHash ? <Check size={12} color="var(--accent-emerald)" /> : <Copy size={12} />}
            </button>
          </div>
          <div>
            <span>Size: <strong>{fileInstance.byteSize.toLocaleString()} bytes</strong></span>
            <span style={{ margin: '0 8px', color: 'var(--border-subtle)' }}>|</span>
            <span>Received: <strong>{fileInstance.receivedAtUtc.substring(11, 19)} UTC</strong></span>
          </div>
        </div>

        {/* Tabs Bar */}
        <div style={{
          display: 'flex',
          gap: '8px',
          padding: '8px 24px',
          background: 'var(--bg-secondary)',
          borderBottom: '1px solid var(--border-subtle)'
        }}>
          <button
            onClick={() => setActiveTab('findings')}
            className={`btn ${activeTab === 'findings' ? 'btn-secondary' : ''}`}
            style={{
              padding: '6px 12px',
              fontSize: '0.8125rem',
              background: activeTab === 'findings' ? 'var(--bg-tertiary)' : 'transparent'
            }}
          >
            Deterministic Findings ({res?.findings.length || 0})
          </button>
          <button
            onClick={() => setActiveTab('controls')}
            className={`btn ${activeTab === 'controls' ? 'btn-secondary' : ''}`}
            style={{
              padding: '6px 12px',
              fontSize: '0.8125rem',
              background: activeTab === 'controls' ? 'var(--bg-tertiary)' : 'transparent'
            }}
          >
            Control Record Balancer
          </button>
          <button
            onClick={() => setActiveTab('raw')}
            className={`btn ${activeTab === 'raw' ? 'btn-secondary' : ''}`}
            style={{
              padding: '6px 12px',
              fontSize: '0.8125rem',
              background: activeTab === 'raw' ? 'var(--bg-tertiary)' : 'transparent'
            }}
          >
            Raw 94-Char Record Inspector
          </button>
        </div>

        {/* Tab Content */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', flex: 1 }}>
          {activeTab === 'findings' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {res?.findings.length === 0 ? (
                <div style={{
                  padding: '30px',
                  textAlign: 'center',
                  background: 'var(--accent-emerald-dim)',
                  border: '1px solid rgba(16, 185, 129, 0.3)',
                  borderRadius: '8px',
                  color: 'var(--accent-emerald)'
                }}>
                  <CheckCircle size={32} style={{ margin: '0 auto 8px auto' }} />
                  <p style={{ fontWeight: 600 }}>Pre-Flight Verification Succeeded</p>
                  <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', marginTop: '4px' }}>
                    Zero syntax, block alignment, entry hash, or arithmetic discrepancies detected.
                  </p>
                </div>
              ) : (
                res?.findings.map(finding => (
                  <div 
                    key={finding.id}
                    style={{
                      background: 'var(--bg-primary)',
                      border: `1px solid ${finding.severity === 'FATAL' || finding.severity === 'ERROR' || finding.severity === 'CRITICAL' ? 'rgba(239, 68, 68, 0.3)' : 'rgba(245, 158, 11, 0.3)'}`,
                      borderRadius: '8px',
                      padding: '14px 16px'
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                        <span className={`badge ${finding.severity === 'BLOCKING' ? 'badge-danger' : finding.severity === 'WARNING' ? 'badge-warning' : 'badge-neutral'}`}>
                          {finding.code}
                        </span>
                        {finding.lineNumber && (
                          <span className="badge badge-neutral">Line {finding.lineNumber}</span>
                        )}
                        {finding.byteOffset !== undefined && (
                          <span className="badge badge-neutral">Byte {finding.byteOffset}</span>
                        )}
                        {finding.fieldStart ? (
                          <span className="badge badge-neutral">
                            Chars {finding.fieldStart}-{finding.fieldEnd}
                          </span>
                        ) : null}
                      </div>
                      <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                        {finding.code} v{finding.ruleVersion}
                        {finding.provenance === 'UNVERIFIED_REQUIRES_LICENSED_RULES' && ' · unverified'}
                      </span>
                    </div>

                    <p style={{ fontSize: '0.875rem', color: 'var(--text-primary)', marginTop: '4px' }}>
                      {finding.message}
                    </p>

                    {(finding.expected || finding.actual) && (
                      <div style={{ marginTop: '8px', display: 'flex', gap: '16px', fontSize: '0.75rem' }}>
                        <span style={{ color: 'var(--text-muted)' }}>
                          Declared: <span className="font-mono" style={{ color: 'var(--text-secondary)' }}>{finding.expected}</span>
                        </span>
                        <span style={{ color: 'var(--text-muted)' }}>
                          Computed: <span className="font-mono" style={{ color: 'var(--accent-cyan)' }}>{finding.actual}</span>
                        </span>
                      </div>
                    )}
                    {finding.evidence && (
                      <div style={{ marginTop: '8px' }}>
                        <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                          REDACTED FIELD EXCERPT (digits masked at the server):
                        </span>
                        <div className="log-viewer" style={{ marginTop: '2px' }}>
                          {finding.evidence}
                        </div>
                      </div>
                    )}
                  </div>
                ))
              )}
            </div>
          )}

          {activeTab === 'controls' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
              {/* Balances Card */}
              <div className="glass-panel" style={{ padding: '16px' }}>
                <h3 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
                  Dollar Amount Reconciliation
                </h3>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '0.8125rem' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Total Debits:</span>
                    <span className="font-mono" style={{ fontWeight: 600, color: 'var(--accent-crimson)' }}>
                      ${((res?.totalDebitsMinor || 0) / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                    </span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Total Credits:</span>
                    <span className="font-mono" style={{ fontWeight: 600, color: 'var(--accent-emerald)' }}>
                      ${((res?.totalCreditsMinor || 0) / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                    </span>
                  </div>
                  {/*
                    Stated as arithmetic, not as a verdict. Whether a file must
                    balance is a term of the feed contract: a credit-only
                    payroll file never balances and is entirely correct. The
                    badge this replaces said "Settlement State" and rendered
                    "Balanced (Zero-Net)" as a success, which asserted both a
                    settlement concept this product does not have and a
                    correctness claim the file cannot support on its own.
                  */}
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Debits vs credits:</span>
                    <span className="badge badge-neutral">
                      {res && res.totalDebitsMinor === res.totalCreditsMinor ? 'Equal' : 'Not equal'}
                    </span>
                  </div>
                </div>
              </div>

              {/* Decision provenance */}
              <div className="glass-panel" style={{ padding: '16px' }}>
                <h3 style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '12px' }}>
                  Decision provenance
                </h3>
                {/*
                  The card this replaces showed a calculated and a declared
                  entry hash and then a badge reading "Verified Modulo 10^10"
                  that was a constant: it rendered as verified whether the two
                  values matched or not. Hash disagreements are now reported as
                  findings, with both sides shown, by the rule that detected
                  them.
                */}
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '0.8125rem' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Release policy:</span>
                    <span className="font-mono" style={{ fontWeight: 600, color: 'var(--accent-cyan)' }}>
                      {res?.policyVersion || 'not recorded'}
                    </span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                    <span style={{ color: 'var(--text-muted)' }}>Feed contract:</span>
                    <span className="font-mono" style={{ fontWeight: 600, color: 'var(--text-secondary)' }}>
                      {res?.contractId || 'none applied'}
                    </span>
                  </div>
                  {res?.notCheckedRuleIds && res.notCheckedRuleIds.length > 0 && (
                    <div style={{ padding: '6px 0' }}>
                      <span style={{ color: 'var(--text-muted)' }}>
                        {res.notCheckedRuleIds.length} rule(s) not checked — no licensed rule source:
                      </span>
                      <div style={{ marginTop: '4px', display: 'flex', flexWrap: 'wrap', gap: '4px' }}>
                        {res.notCheckedRuleIds.map(id => (
                          <span key={id} className="badge badge-warning" style={{ fontSize: '0.65rem' }}>{id}</span>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}

          {activeTab === 'raw' && (
            <div className="log-viewer" style={{ maxHeight: '400px' }}>
              {rawLines.map((line, idx) => {
                const lineNum = idx + 1;
                const hasErrorOnLine = res?.findings.some(f => f.lineNumber === lineNum);

                return (
                  <div 
                    key={lineNum}
                    className={hasErrorOnLine ? 'log-line-error' : ''}
                    style={{ display: 'flex', gap: '12px', padding: '2px 0' }}
                  >
                    <span style={{ color: 'var(--text-dim)', width: '36px', textAlign: 'right', userSelect: 'none' }}>
                      {lineNum}
                    </span>
                    <span style={{ whiteSpace: 'pre', color: hasErrorOnLine ? '#FCA5A5' : 'var(--text-secondary)' }}>
                      {line}
                    </span>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
```


## `src/components/ContractConfigModal.tsx`

**365 lines** · called fetch against a hardcoded http://localhost:8080 outside the typed client, so it carried no credentials, no tenant selector and no CSRF token, and swallowed every failure with console.warn("Config fetch notice"). An unreachable gateway rendered as an empty contract list. Replaced by ContractsScreen.

```tsx
import React, { useState, useEffect } from 'react';
import { 
  Building2, 
  FileSpreadsheet, 
  X, 
  Plus, 
  CheckCircle2, 
  Clock
} from 'lucide-react';

interface ContractConfigModalProps {
  onClose: () => void;
}

export const ContractConfigModal: React.FC<ContractConfigModalProps> = ({ onClose }) => {
  const [partners, setPartners] = useState<any[]>([]);
  const [contracts, setContracts] = useState<any[]>([]);
  const [activeTab, setActiveTab] = useState<'PARTNERS' | 'CONTRACTS' | 'NEW_PARTNER'>('CONTRACTS');
  const [newPartnerName, setNewPartnerName] = useState<string>('');
  const [newPartnerRouting, setNewPartnerRouting] = useState<string>('');
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [successMsg, setSuccessMsg] = useState<string>('');

  const fetchConfig = async () => {
    try {
      const [pRes, cRes] = await Promise.all([
        fetch('http://localhost:8080/api/v1/partners').then(r => r.json()),
        fetch('http://localhost:8080/api/v1/contracts').then(r => r.json())
      ]);
      setPartners(pRes || []);
      setContracts(cRes || []);
    } catch (e) {
      console.warn('Config fetch notice:', e);
    }
  };

  useEffect(() => {
    fetchConfig();
  }, []);

  const handleCreatePartner = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newPartnerName || !newPartnerRouting) return;
    setIsSubmitting(true);
    try {
      const res = await fetch('http://localhost:8080/api/v1/partners', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newPartnerName, routingNumber: newPartnerRouting })
      });
      if (res.ok) {
        setSuccessMsg(`Partner ${newPartnerName} successfully registered in Go Gateway ledger!`);
        setNewPartnerName('');
        setNewPartnerRouting('');
        await fetchConfig();
        setTimeout(() => setSuccessMsg(''), 3000);
      }
    } catch (err: any) {
      alert(`Error creating partner: ${err.message}`);
    } finally {
      setIsSubmitting(false);
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
        maxWidth: '850px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.7)',
        overflow: 'hidden'
      }}>
        {/* Header */}
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
              background: 'rgba(2, 132, 199, 0.2)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              <Building2 size={18} color="var(--accent-cyan)" />
            </div>
            <div>
              <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                Counterparty Partners & File Contracts Manager
              </h3>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                Configure expected arrival windows, filename regex patterns, and SLA grace periods.
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
            onClick={() => setActiveTab('CONTRACTS')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'CONTRACTS' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'CONTRACTS' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <FileSpreadsheet size={14} />
            <span>Active File Contracts ({contracts.length})</span>
          </button>

          <button
            onClick={() => setActiveTab('PARTNERS')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'PARTNERS' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'PARTNERS' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Building2 size={14} />
            <span>Authorized Counterparties ({partners.length})</span>
          </button>

          <button
            onClick={() => setActiveTab('NEW_PARTNER')}
            style={{
              padding: '12px 16px',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'NEW_PARTNER' ? '2px solid var(--accent-cyan)' : '2px solid transparent',
              color: activeTab === 'NEW_PARTNER' ? 'var(--accent-cyan)' : 'var(--text-muted)',
              fontWeight: 600,
              fontSize: '0.8125rem',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px'
            }}
          >
            <Plus size={14} />
            <span>Register New Counterparty</span>
          </button>
        </div>

        {/* Content Area */}
        <div style={{ padding: '24px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {successMsg && (
            <div style={{
              background: 'rgba(16, 185, 129, 0.15)',
              border: '1px solid rgba(16, 185, 129, 0.4)',
              borderRadius: '8px',
              padding: '12px 16px',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              color: 'var(--accent-emerald)',
              fontSize: '0.8125rem',
              fontWeight: 600
            }}>
              <CheckCircle2 size={16} />
              <span>{successMsg}</span>
            </div>
          )}

          {activeTab === 'CONTRACTS' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {contracts.map(c => (
                <div
                  key={c.id}
                  style={{
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '14px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between'
                  }}
                >
                  <div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                        {c.name}
                      </span>
                      <span className="badge badge-cyan" style={{ fontSize: '0.65rem' }}>{c.direction}</span>
                      <span className="badge badge-neutral" style={{ fontSize: '0.65rem' }}>{c.timezone}</span>
                    </div>
                    <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                      Counterparty: <strong style={{ color: 'var(--text-secondary)' }}>{c.partnerName}</strong> | Pattern: <code>{c.filenamePattern}</code>
                    </div>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                    <div style={{ textAlign: 'right' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '4px', justifyContent: 'flex-end', fontSize: '0.75rem', color: 'var(--accent-amber)', fontWeight: 600 }}>
                        <Clock size={12} />
                        <span>Cutoff: {c.expectedTime}</span>
                      </div>
                      <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Grace: +{c.gracePeriodMinutes}m</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}

          {activeTab === 'PARTNERS' && (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
              {partners.map(p => (
                <div
                  key={p.id}
                  style={{
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '8px',
                    padding: '14px'
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
                    <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                      {p.name}
                    </span>
                    <span className="badge badge-success" style={{ fontSize: '0.65rem' }}>ACTIVE</span>
                  </div>
                  <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                    Fed Routing (ABA): <strong className="font-mono" style={{ color: 'var(--accent-cyan)' }}>{p.routingNumber}</strong>
                  </div>
                  <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)', marginTop: '4px' }}>
                    Registered: {p.createdAt?.substring(0, 10)}
                  </div>
                </div>
              ))}
            </div>
          )}

          {activeTab === 'NEW_PARTNER' && (
            <form onSubmit={handleCreatePartner} style={{ display: 'flex', flexDirection: 'column', gap: '14px', maxWidth: '500px' }}>
              <div>
                <label style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                  Institution / Counterparty Name:
                </label>
                <input
                  type="text"
                  placeholder="e.g. Citadel Securities Clearing LLC"
                  value={newPartnerName}
                  onChange={(e) => setNewPartnerName(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '6px',
                    color: 'var(--text-primary)',
                    fontSize: '0.8125rem'
                  }}
                  required
                />
              </div>

              <div>
                <label style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                  Federal Reserve ABA Routing Number (9 Digits):
                </label>
                <input
                  type="text"
                  placeholder="e.g. 021000021"
                  maxLength={9}
                  value={newPartnerRouting}
                  onChange={(e) => setNewPartnerRouting(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '10px 12px',
                    background: 'var(--bg-primary)',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: '6px',
                    color: 'var(--text-primary)',
                    fontFamily: 'var(--font-mono)',
                    fontSize: '0.8125rem'
                  }}
                  required
                />
              </div>

              <button
                type="submit"
                className="btn btn-primary"
                disabled={isSubmitting}
                style={{ marginTop: '8px', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px' }}
              >
                <Plus size={14} />
                <span>{isSubmitting ? 'Registering Counterparty...' : 'Register Counterparty in Ledger'}</span>
              </button>
            </form>
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


## `src/components/DemoDataBanner.tsx`

**112 lines** · superseded by the console's DemoProfileBanner, which reports the profile the *server* named rather than a constant compiled into the bundle.

```tsx
import React, { useEffect, useState } from 'react';
import { AlertTriangle, WifiOff, CheckCircle2 } from 'lucide-react';
import { SentinelApi } from '../services/api';

/**
 * DemoDataBanner states two facts an operator cannot otherwise see.
 *
 * 1. The expectation board, partners and contracts on this screen are a local
 *    synthetic corpus (src/mockData/syntheticCorpus.ts). They are NOT read from
 *    the gateway. Before Prompt 01 nothing on screen said so, and synthetic
 *    state was visually identical to real state.
 *
 * 2. Whether the gateway is actually reachable. Previously a backend outage was
 *    invisible: the API client caught every error, logged "using local mock
 *    state" to a console nobody watches, and returned an empty array, so an
 *    outage rendered as a healthy, empty board.
 *
 * Prompt 12 rebuilds these screens on authenticated server data, at which point
 * the demo half of this banner is deleted and the connectivity half becomes a
 * real per-query degraded state.
 */

type GatewayState = 'checking' | 'reachable' | 'unreachable' | 'unauthorized';

export const DemoDataBanner: React.FC = () => {
  const [gateway, setGateway] = useState<GatewayState>('checking');
  const [detail, setDetail] = useState<string>('');

  useEffect(() => {
    let cancelled = false;

    const poll = async () => {
      const res = await SentinelApi.checkHealth();
      if (cancelled) return;
      if (res.state === 'ok') {
        setGateway('reachable');
        setDetail('');
      } else if (res.state === 'unauthorized') {
        setGateway('unauthorized');
        setDetail(res.error);
      } else {
        setGateway('unreachable');
        setDetail(res.error);
      }
    };

    poll();
    const id = setInterval(poll, 15000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  const gatewayLine = () => {
    switch (gateway) {
      case 'checking':
        return { icon: <WifiOff size={14} />, text: 'Checking gateway…', color: 'var(--text-muted)' };
      case 'reachable':
        return {
          icon: <CheckCircle2 size={14} />,
          text: 'Gateway reachable. Uploads and triage use live server validation.',
          color: 'var(--accent-emerald)',
        };
      case 'unauthorized':
        return {
          icon: <WifiOff size={14} />,
          text: `Gateway reachable but rejected this client: ${detail}`,
          color: 'var(--accent-amber)',
        };
      case 'unreachable':
        return {
          icon: <WifiOff size={14} />,
          text: `Gateway UNAVAILABLE — ${detail} Nothing on this screen reflects live server state.`,
          color: 'var(--accent-red, #ef4444)',
        };
    }
  };

  const g = gatewayLine();

  return (
    <div
      role="status"
      aria-live="polite"
      className="glass-panel"
      style={{
        padding: '12px 16px',
        border: '1px solid var(--accent-amber)',
        background: 'rgba(245, 158, 11, 0.08)',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <AlertTriangle size={16} color="var(--accent-amber)" />
        <strong style={{ fontSize: '0.8125rem', letterSpacing: '0.04em', color: 'var(--accent-amber)' }}>
          DEMO DATA
        </strong>
        <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
          Expectations, partners and contracts shown below come from a local synthetic corpus, not
          from the gateway. Do not read them as operational state.
        </span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: g.color, fontSize: '0.75rem' }}>
        {g.icon}
        <span>{g.text}</span>
      </div>
    </div>
  );
};
```


## `src/scheduler/deadlineEngine.ts`

**102 lines** · the browser copy of deadline derivation. internal/schedule is the implementation, including the Federal Reserve calendar and the DST resolution this file did not attempt.

```tsx
import { ExpectationOccurrence, FileContract, Incident, Partner } from '../types/financial';

export interface SlaAssessment {
  occurrence: ExpectationOccurrence;
  minutesUntilDue: number;
  minutesUntilGraceExpiry: number;
  isBreached: boolean;
  breachProbability: number; // 0.0 to 1.0 (from historical baseline)
  statusLabel: string;
  badgeVariant: 'success' | 'warning' | 'danger' | 'neutral';
}

/**
 * Assesses the SLA status of an expectation occurrence given the current UTC time.
 */
export function assessOccurrenceSla(
  occurrence: ExpectationOccurrence,
  nowUtc: Date = new Date()
): SlaAssessment {
  const dueTime = new Date(occurrence.dueAtUtc).getTime();
  const graceTime = new Date(occurrence.graceExpiresAtUtc).getTime();
  const nowTime = nowUtc.getTime();

  const minutesUntilDue = Math.round((dueTime - nowTime) / (1000 * 60));
  const minutesUntilGraceExpiry = Math.round((graceTime - nowTime) / (1000 * 60));

  let isBreached = false;
  let breachProbability = 0.05;
  let statusLabel = 'On Schedule';
  let badgeVariant: SlaAssessment['badgeVariant'] = 'success';

  if (occurrence.status === 'VALID' || occurrence.status === 'RELEASED') {
    statusLabel = 'Delivered & Validated';
    badgeVariant = 'success';
    breachProbability = 0.0;
  } else if (occurrence.status === 'QUARANTINED') {
    statusLabel = 'Quarantined (Invalid)';
    badgeVariant = 'danger';
    isBreached = true;
    breachProbability = 1.0;
  } else if (occurrence.status === 'RECEIVED' || occurrence.status === 'VALIDATING') {
    statusLabel = 'Processing In-Flight';
    badgeVariant = 'warning';
    breachProbability = 0.15;
  } else if (nowTime > graceTime) {
    isBreached = true;
    statusLabel = 'SLA Breached (Missing File)';
    badgeVariant = 'danger';
    breachProbability = 1.0;
  } else if (nowTime > dueTime) {
    statusLabel = 'In Grace Period (Imminent Breach)';
    badgeVariant = 'danger';
    breachProbability = 0.85;
  } else if (minutesUntilDue <= 30) {
    statusLabel = `Due Soon (${minutesUntilDue}m remaining)`;
    badgeVariant = 'warning';
    breachProbability = 0.45;
  } else {
    statusLabel = `Expected in ${Math.round(minutesUntilDue / 60)}h ${minutesUntilDue % 60}m`;
    badgeVariant = 'neutral';
    breachProbability = 0.08;
  }

  return {
    occurrence,
    minutesUntilDue,
    minutesUntilGraceExpiry,
    isBreached,
    breachProbability,
    statusLabel,
    badgeVariant
  };
}

/**
 * Generates an automated Missing File Incident if overdue.
 */
export function evaluateMissingFileIncident(
  occurrence: ExpectationOccurrence,
  contract: FileContract,
  partner: Partner,
  nowUtc: Date = new Date()
): Incident | null {
  const assessment = assessOccurrenceSla(occurrence, nowUtc);
  
  if (assessment.isBreached && occurrence.status !== 'VALID' && occurrence.status !== 'RELEASED') {
    return {
      id: `INC-MISSING-${occurrence.id}`,
      type: 'MISSING_FILE_DEADLINE',
      severity: 'CRITICAL',
      title: `Missing Inbound File: ${contract.name} (${partner.name})`,
      occurrenceId: occurrence.id,
      partnerId: partner.id,
      status: 'OPEN',
      openedAtUtc: nowUtc.toISOString(),
      slaDeadlineUtc: occurrence.dueAtUtc,
      resolutionNote: `Expected by ${occurrence.dueAtUtc} (Grace expired at ${occurrence.graceExpiresAtUtc}). No matching transmission detected over SFTP.`
    };
  }

  return null;
}
```


## `src/audit/hashChain.ts`

**132 lines** · the browser copy of the evidence chain. internal/ledger is the implementation; a chain built and verified in the same browser tab is not tamper evidence.

```tsx
import { DomainEvent } from '../types/financial';

/**
 * Fast SHA-256 string hash calculation (using Web Crypto API with fallback)
 */
export async function calculateSha256(text: string): Promise<string> {
  if (typeof crypto !== 'undefined' && crypto.subtle) {
    const msgUint8 = new TextEncoder().encode(text);
    const hashBuffer = await crypto.subtle.digest('SHA-256', msgUint8);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
  }
  
  // Deterministic fallback hash for non-crypto environments
  let h1 = 0xdeadbeef, h2 = 0x41c6ce57;
  for (let i = 0; i < text.length; i++) {
    const ch = text.charCodeAt(i);
    h1 = Math.imul(h1 ^ ch, 2654435761);
    h2 = Math.imul(h2 ^ ch, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507);
  h1 ^= Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507);
  h2 ^= Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(16).padStart(64, '0');
}

export class TamperEvidentEventStore {
  private events: DomainEvent[] = [];
  private latestHash = '0000000000000000000000000000000000000000000000000000000000000000';

  public async appendEvent(
    tenantId: string,
    aggregateId: string,
    aggregateType: DomainEvent['aggregateType'],
    eventType: string,
    actor: string,
    payload: Record<string, unknown>,
    correlationId: string,
    causationId?: string
  ): Promise<DomainEvent> {
    const timestampUtc = new Date().toISOString();
    const previousHash = this.latestHash;

    const eventContentForHash = JSON.stringify({
      previousHash,
      tenantId,
      aggregateId,
      aggregateType,
      eventType,
      timestampUtc,
      actor,
      correlationId,
      payload
    });

    const currentHash = await calculateSha256(eventContentForHash);

    const event: DomainEvent = {
      id: `EVT-${Date.now()}-${Math.random().toString(36).substring(2, 7)}`,
      tenantId,
      aggregateId,
      aggregateType,
      eventType,
      timestampUtc,
      actor,
      correlationId,
      causationId,
      payload,
      previousHash,
      currentHash
    };

    this.events.push(event);
    this.latestHash = currentHash;
    return event;
  }

  public getEvents(): DomainEvent[] {
    return [...this.events];
  }

  public async verifyIntegrity(): Promise<{
    isValid: boolean;
    totalEvents: number;
    tamperedEventIndex?: number;
    error?: string;
  }> {
    let expectedPreviousHash = '0000000000000000000000000000000000000000000000000000000000000000';

    for (let i = 0; i < this.events.length; i++) {
      const event = this.events[i];
      if (event.previousHash !== expectedPreviousHash) {
        return {
          isValid: false,
          totalEvents: this.events.length,
          tamperedEventIndex: i,
          error: `Broken chain link at index ${i}: expected previousHash '${expectedPreviousHash}', found '${event.previousHash}'.`
        };
      }

      const contentForHash = JSON.stringify({
        previousHash: event.previousHash,
        tenantId: event.tenantId,
        aggregateId: event.aggregateId,
        aggregateType: event.aggregateType,
        eventType: event.eventType,
        timestampUtc: event.timestampUtc,
        actor: event.actor,
        correlationId: event.correlationId,
        payload: event.payload
      });

      const recalculate = await calculateSha256(contentForHash);
      if (recalculate !== event.currentHash) {
        return {
          isValid: false,
          totalEvents: this.events.length,
          tamperedEventIndex: i,
          error: `Payload tamper detected at event index ${i} ('${event.id}'): calculated hash '${recalculate}' does not match stored hash '${event.currentHash}'.`
        };
      }

      expectedPreviousHash = event.currentHash;
    }

    return {
      isValid: true,
      totalEvents: this.events.length
    };
  }
}
```


## `src/ai/exceptionAnalyst.ts`

**202 lines** · the deterministic matcher used as a fallback when AI triage failed. Its output was rendered as an analysis. Prompt 15 builds the real read-only analyst; until then the correct behaviour on an unreachable AI tier is to say so.

```tsx
import { AgentRun, Incident, ValidationResult, FileInstance, FileContract, Partner } from '../types/financial';

export interface ApprovedRunbook {
  id: string;
  title: string;
  category: 'ACH_VALIDATION' | 'MISSING_FILE' | 'DUPLICATE_INGESTION' | 'SECURITY';
  applicableFindingCodes: string[];
  summary: string;
  recommendedSteps: string[];
  requiresDualApproval: boolean;
}

export const APPROVED_RUNBOOKS: ApprovedRunbook[] = [
  {
    id: 'RB-ACH-01',
    title: 'NACHA Entry Hash Mismatch Triage & Remediation',
    category: 'ACH_VALIDATION',
    applicableFindingCodes: ['NACHA.MATH.BATCH_ENTRY_HASH', 'NACHA.MATH.FILE_ENTRY_HASH'],
    summary: 'Occurs when the sum of routing numbers in Entry Detail records does not match the 10-digit hash declared in Batch/File Control records.',
    recommendedSteps: [
      '1. Verify if any Entry Detail (Type 6) records were dropped or truncated during SFTP transmission.',
      '2. Inspect if the originating banking application truncated the 10-digit hash accumulator.',
      '3. Issue a format remediation notice to the partner file originator citing Nacha Appendix B.',
      '4. If verified as an originator math glitch with valid payment details, submit an exceptional release request for dual-control human approval.'
    ],
    requiresDualApproval: true
  },
  {
    id: 'RB-ACH-02',
    title: 'NACHA Debit/Credit Control Imbalance Triage',
    category: 'ACH_VALIDATION',
    applicableFindingCodes: ['NACHA.MATH.BATCH_DEBIT_TOTAL', 'NACHA.MATH.BATCH_CREDIT_TOTAL', 'NACHA.MATH.FILE_DEBIT_TOTAL', 'NACHA.MATH.FILE_CREDIT_TOTAL'],
    summary: 'Occurs when the calculated dollar sum of individual payment records does not match the batch or file trailer totals.',
    recommendedSteps: [
      '1. Isolate the specific batch number with the arithmetic discrepancy.',
      '2. Redline the declared trailer record against the calculated sum of Type 6 detail amounts.',
      '3. Check whether the file is balanced vs. unbalanced per partner File Contract agreement.',
      '4. Under no circumstances edit payment dollar amounts. Request an authorized re-drop from partner.'
    ],
    requiresDualApproval: true
  },
  {
    id: 'RB-MISS-01',
    title: 'Missing Counterparty File Cutoff Escalation',
    category: 'MISSING_FILE',
    applicableFindingCodes: ['MISSING_FILE_DEADLINE'],
    summary: 'Triggered when a scheduled delivery fails to arrive within the contract window plus grace period.',
    recommendedSteps: [
      '1. Check SFTPGo ingress transport logs to confirm whether the partner attempted an SSH connection.',
      '2. Check whether upstream Federal Reserve or central bank holiday schedules apply.',
      '3. Dispatch urgent missing-file advisory to partner treasury operations desk.',
      '4. Assess liquidity impact on downstream FedACH / Fedwire settlement windows.'
    ],
    requiresDualApproval: false
  },
  {
    id: 'RB-SEC-01',
    title: '0-Byte or Truncated File Ingestion Guard',
    category: 'SECURITY',
    applicableFindingCodes: ['NACHA.STRUCT.EMPTY', 'NACHA.STRUCT.NO_FILE_CONTROL', 'NACHA.STRUCT.TRUNCATED'],
    summary: 'Prevents downstream core database ingestion of incomplete or empty transmissions.',
    recommendedSteps: [
      '1. Confirm file was fully closed by SFTP client (check transport completion event).',
      '2. Quarantine the transmission immediately to prevent race conditions with downstream jobs.',
      '3. Notify partner support engineer of EOF truncation.'
    ],
    requiresDualApproval: false
  }
];

export function runExceptionAnalyst(
  incident: Incident,
  fileInstance?: FileInstance,
  contract?: FileContract,
  partner?: Partner,
  validationResult?: ValidationResult
): AgentRun {
  const startTime = performance.now();
  const findingCodes = validationResult?.findings.map(f => f.code) || [incident.type];

  // Match relevant approved runbooks
  const matchedRunbooks = APPROVED_RUNBOOKS.filter(rb =>
    rb.applicableFindingCodes.some(code => findingCodes.includes(code)) ||
    (incident.type === 'MISSING_FILE_DEADLINE' && rb.id === 'RB-MISS-01')
  );

  const citedFindingCodes = validationResult?.findings.map(f => f.code) || [incident.type];
  const citedRunbookSections = matchedRunbooks.map(rb => `${rb.id}: ${rb.title}`);
  const citedEventIds = [
    `EVT-${incident.id}`,
    fileInstance ? `EVT-SRC-${fileInstance.sourceEventId}` : `EVT-EXP-${incident.occurrenceId}`
  ];

  let findingsSummary = '';
  const hypotheses: AgentRun['hypotheses'] = [];
  const proposedActionPlan: AgentRun['proposedActionPlan'] = [];

  if (incident.type === 'MISSING_FILE_DEADLINE') {
    findingsSummary = `Expected transmission '${contract?.name || 'Inbound File'}' from counterparty '${partner?.name || 'Partner'}' did not arrive by scheduled deadline ${incident.slaDeadlineUtc}. Grace period has expired with zero observed SFTP arrival events.`;
    
    hypotheses.push({
      hypothesis: 'Originating partner batch job failed to initiate or hung on upstream file generation.',
      confidence: 'HIGH',
      supportingEvidence: [
        `Zero SSH upload completion events logged in SFTPGo since window start.`,
        `Contract deadline was ${incident.slaDeadlineUtc} with a ${contract?.gracePeriodMinutes || 15}-minute grace period.`
      ]
    });

    hypotheses.push({
      hypothesis: 'Network firewall or DNS resolution issue between partner SFTP client and ingress gateway.',
      confidence: 'MEDIUM',
      supportingEvidence: [
        `No TCP connection resets or authentication failures observed on port 22.`
      ]
    });

    proposedActionPlan.push(
      { step: 1, action: 'Inspect SFTPGo daemon connection logs for partial handshake attempts.', authorityTier: 0, requiresHumanApproval: false },
      { step: 2, action: 'Draft priority SLA breach notification to partner primary contact.', authorityTier: 2, requiresHumanApproval: false },
      { step: 3, action: 'If partner confirms delayed run, request temporary 45-minute SLA waiver.', authorityTier: 3, requiresHumanApproval: true }
    );
  } else {
    const blockingFindings = validationResult?.findings.filter(f => f.severity === 'BLOCKING') || [];
    const errorCount = blockingFindings.length;
    findingsSummary = `File '${fileInstance?.filename}' was quarantined at the transfer boundary. Pre-flight inspection identified ${errorCount} deterministic violation(s) preventing safe downstream release.`;

    const hashFindings = blockingFindings.filter(f =>
      f.code === 'NACHA.MATH.BATCH_ENTRY_HASH' || f.code === 'NACHA.MATH.FILE_ENTRY_HASH');
    if (hashFindings.length > 0) {
      hypotheses.push({
        hypothesis: 'Originating core system calculated Entry Hash using a non-standard 10-digit truncation or omitted an entry record from the trailer accumulator.',
        confidence: 'HIGH',
        // The evidence comes from the finding the server raised, which carries
        // both sides of the disagreement. The previous version read two
        // top-level fields the response no longer has, and cited "Nacha 2025
        // Chapter 3" -- a licensed source this system does not have and
        // therefore cannot cite.
        supportingEvidence: hashFindings.map(f =>
          `${f.code} v${f.ruleVersion} at record ${f.lineNumber ?? '?'}: declared '${f.expected ?? 'n/a'}', computed '${f.actual ?? 'n/a'}'.`
        )
      });
    }

    if (findingCodes.includes('NACHA.ROUTING.CHECK_DIGIT')) {
      hypotheses.push({
        hypothesis: 'Individual payment record contains an invalid ABA routing number failing Federal Reserve Modulo 10 verification.',
        confidence: 'HIGH',
        supportingEvidence: [
          `Validation finding caught check digit discrepancy at record detail line.`,
          `Releasing this payment downstream would cause bank return code R03/R04.`
        ]
      });
    }

    proposedActionPlan.push(
      { step: 1, action: 'Generate cryptographic evidence bundle with exact line-item finding citations.', authorityTier: 0, requiresHumanApproval: false },
      { step: 2, action: 'Draft format rejection notice for partner with redacted error excerpt.', authorityTier: 2, requiresHumanApproval: false },
      { step: 3, action: 'Hold file in quarantine until corrected file drop is received.', authorityTier: 0, requiresHumanApproval: false }
    );
  }

  const partnerNotice = `
SUBJECT: [URGENT] Sentinel Flow Validation Notice: ${contract?.name || 'File Delivery'} (${incident.id})

Dear ${partner?.primaryContact.name || 'Partner Operations Team'},

Sentinel Flow quarantined incoming file '${fileInstance?.filename || contract?.filenamePattern}' received at ${fileInstance?.receivedAtUtc || new Date().toISOString()}.

Deterministic Findings:
${validationResult?.findings.map(f => ` • [${f.code}] Line ${f.lineNumber || 'N/A'}: ${f.message}`).join('\n') || ' • Scheduled file arrival deadline was breached with no arrival recorded.'}

Required Action:
Please review the attached diagnostic report, verify against Nacha Operating Rules, and submit a corrected file. Original file remains in quarantined state (SHA-256: ${fileInstance?.sha256Hash || 'N/A'}).

Sentinel Flow Financial Gateway
`.trim();

  const durationMs = Math.round(performance.now() - startTime);

  return {
    id: `AGENT-RUN-${Date.now()}`,
    incidentId: incident.id,
    agentVersion: 'Sentinel-Analyst-v1.2 (Astra-RRR-Constrained)',
    modelIdentifier: 'claude-3-5-sonnet / gpt-4o-mini (Deterministic Hybrid)',
    ranAtUtc: new Date().toISOString(),
    inputDigest: `SHA256:${Date.now().toString(16)}`,
    citedEventIds,
    citedFindingCodes,
    citedRunbookSections,
    findingsSummary,
    hypotheses,
    proposedActionPlan,
    draftExternalPartnerNotice: partnerNotice,
    metrics: {
      durationMs,
      inputTokens: 1420,
      outputTokens: 460,
      estimatedCostUsd: 0.0038
    }
  };
}
```


## `src/mockData/syntheticCorpus.ts`

**171 lines** · SYNTHETIC_PARTNERS, SYNTHETIC_CONTRACTS, generateInitialOccurrences and the two sample NACHA constants. Nothing in the production path may read these; the sample-file generator that remains asks the *server* for a fixture.

```tsx
import { Partner, FileContract, ExpectationOccurrence } from '../types/financial';

export const SYNTHETIC_PARTNERS: Partner[] = [
  {
    id: 'PARTNER-MERIDIAN-01',
    tenantId: 'TENANT-DEFAULT',
    name: 'Meridian Treasury Services',
    leiCode: '549300V5L2CGV15S5P38',
    status: 'ACTIVE',
    allowedChannelIdentities: ['sftp://meridian-ingest.sentinel.internal:22', 'sftp-user-meridian-prod'],
    sshKeyFingerprints: ['SHA256:4a3F9cKz8eL1mQxWpTvBn9Rt6sDuFhJ2kLm8qWeRt7y'],
    pgpKeyIds: ['0x9E8C7B6A5F4E3D2C'],
    primaryContact: {
      name: 'Michael Sterling (Treasury Ops Lead)',
      email: 'm.sterling@meridian.mock.com',
      phone: '+1 (212) 635-1000'
    }
  },
  {
    id: 'PARTNER-APEX-02',
    tenantId: 'TENANT-DEFAULT',
    name: 'Apex Clearing & Settlement Network',
    leiCode: '8I5D0TI0ER51YZ7JPI37',
    status: 'ACTIVE',
    allowedChannelIdentities: ['sftp://apex-gateway.sentinel.internal:22', 'sftp-user-apex-clearing'],
    sshKeyFingerprints: ['SHA256:7uK2xP9qLm4nRt6sDuFhJ2kLm8qWeRt7y4a3F9cKz8e'],
    pgpKeyIds: ['0x1A2B3C4D5E6F7A8B'],
    primaryContact: {
      name: 'Sarah Chen (Settlement Engineering)',
      email: 's.chen@apexclearing.mock.com',
      phone: '+1 (212) 270-6000'
    }
  },
  {
    id: 'PARTNER-ATLANTIC-03',
    tenantId: 'TENANT-DEFAULT',
    name: 'Atlantic Custody & Trust Services',
    leiCode: '5714005EN0VJ0SD75X48',
    status: 'ACTIVE',
    allowedChannelIdentities: ['sftp://atlantic-mft.sentinel.internal:22'],
    sshKeyFingerprints: ['SHA256:9qWeRt7y4a3F9cKz8eL1mQxWpTvBn9Rt6sDuFhJ2kLm'],
    pgpKeyIds: ['0x8F7E6D5C4B3A2910'],
    primaryContact: {
      name: 'David Vance (Custody Operations)',
      email: 'd.vance@atlantictrust.mock.com',
      phone: '+1 (617) 786-3000'
    }
  }
];

export const SYNTHETIC_CONTRACTS: FileContract[] = [
  {
    id: 'CONTRACT-MERIDIAN-ACH-01',
    partnerId: 'PARTNER-MERIDIAN-01',
    name: 'Meridian Inbound Commercial ACH Batch',
    format: 'NACHA_ACH',
    parserVersion: 'NachaEngine-v1.4',
    rulePackVersion: 'Nacha2025-Q4',
    filenamePattern: '^MERIDIAN_ACH_COMMERCIAL_\\d{8}_\\d{4}\\.txt$',
    timezone: 'America/New_York',
    businessCalendar: 'US_FED_RESERVE',
    expectedDueTimeUtc: '16:45:00', // 4:45 PM Central Settlement Window
    gracePeriodMinutes: 15,
    expectedSizeBounds: { minBytes: 940, maxBytes: 50000000 },
    expectedRecordBounds: { minRecords: 10, maxRecords: 100000 },
    allowUnbalancedAch: false,
    releasePolicy: 'AUTOMATIC_ON_VALID'
  },
  {
    id: 'CONTRACT-APEX-SWEEP-02',
    partnerId: 'PARTNER-APEX-02',
    name: 'Apex End-of-Day Cash Sweep Batch',
    format: 'NACHA_ACH',
    parserVersion: 'NachaEngine-v1.4',
    rulePackVersion: 'Nacha2025-Q4',
    filenamePattern: '^APEX_SWEEP_\\d{8}\\.ach$',
    timezone: 'America/New_York',
    businessCalendar: 'US_FED_RESERVE',
    expectedDueTimeUtc: '17:30:00',
    gracePeriodMinutes: 10,
    expectedSizeBounds: { minBytes: 940, maxBytes: 25000000 },
    expectedRecordBounds: { minRecords: 10, maxRecords: 50000 },
    allowUnbalancedAch: true,
    releasePolicy: 'AUTOMATIC_ON_VALID'
  },
  {
    id: 'CONTRACT-ATLANTIC-TRADE-03',
    partnerId: 'PARTNER-ATLANTIC-03',
    name: 'Atlantic Custody Trade Settlement',
    format: 'NACHA_ACH',
    parserVersion: 'NachaEngine-v1.4',
    rulePackVersion: 'Nacha2025-Q4',
    filenamePattern: '^ATLANTIC_SETTLE_\\d{8}\\.txt$',
    timezone: 'America/New_York',
    businessCalendar: 'US_FED_RESERVE',
    expectedDueTimeUtc: '18:15:00',
    gracePeriodMinutes: 20,
    expectedSizeBounds: { minBytes: 940, maxBytes: 40000000 },
    expectedRecordBounds: { minRecords: 10, maxRecords: 75000 },
    allowUnbalancedAch: false,
    releasePolicy: 'MANUAL_APPROVAL_REQUIRED'
  }
];

export function generateInitialOccurrences(): ExpectationOccurrence[] {
  const todayStr = new Date().toISOString().split('T')[0];

  return [
    {
      id: 'EXP-MERIDIAN-TODAY',
      contractId: 'CONTRACT-MERIDIAN-ACH-01',
      partnerId: 'PARTNER-MERIDIAN-01',
      windowStartUtc: `${todayStr}T15:00:00.000Z`,
      windowEndUtc: `${todayStr}T17:00:00.000Z`,
      dueAtUtc: `${todayStr}T16:45:00.000Z`,
      graceExpiresAtUtc: `${todayStr}T17:00:00.000Z`,
      status: 'EXPECTED',
      expectedDescription: 'Daily 4:45 PM Clearing Window (Meridian Commercial Payroll & Vendor Feeds)'
    },
    {
      id: 'EXP-APEX-TODAY',
      contractId: 'CONTRACT-APEX-SWEEP-02',
      partnerId: 'PARTNER-APEX-02',
      windowStartUtc: `${todayStr}T16:00:00.000Z`,
      windowEndUtc: `${todayStr}T17:40:00.000Z`,
      dueAtUtc: `${todayStr}T17:30:00.000Z`,
      graceExpiresAtUtc: `${todayStr}T17:40:00.000Z`,
      status: 'EXPECTED',
      expectedDescription: 'EOD Corporate Treasury Concentration Sweep'
    },
    {
      id: 'EXP-ATLANTIC-TODAY',
      contractId: 'CONTRACT-ATLANTIC-TRADE-03',
      partnerId: 'PARTNER-ATLANTIC-03',
      windowStartUtc: `${todayStr}T17:00:00.000Z`,
      windowEndUtc: `${todayStr}T18:35:00.000Z`,
      dueAtUtc: `${todayStr}T18:15:00.000Z`,
      graceExpiresAtUtc: `${todayStr}T18:35:00.000Z`,
      status: 'EXPECTED',
      expectedDescription: 'Institutional Securities Settle & Net Asset Value (NAV) Feed'
    }
  ];
}

/**
 * Pre-constructed Valid 10-line NACHA ACH File (Balanced, Valid Hash, Valid Check Digits)
 */
export const SAMPLE_VALID_NACHA = `101 021000021 1234567892608141645A094101MERIDIAN CUSTODY        SENTINEL FLOW          00000001
5200MERIDIAN PAYROLL DISCRETIONARY       1234567890PPDVENDOR PAY260814260814   1021000020000001
62202100002112345678901      0000150000ID-10042        JOHN DOE              00021000020000001
62702100002198765432109      0000150000ID-10043        ACME CORP             00021000020000002
820000000200042000040000001500000000001500001234567890                         021000020000001
9000001000001000000020004200004000000150000000000150000                                       
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999`;

/**
 * Pre-constructed Malformed NACHA ACH File (Out of Balance + Hash Mismatch + Invalid ABA Check Digit)
 */
export const SAMPLE_CORRUPTED_NACHA = `101 021000021 1234567892608141645A094101MERIDIAN CUSTODY        SENTINEL FLOW          00000001
5200MERIDIAN PAYROLL DISCRETIONARY       1234567890PPDVENDOR PAY260814260814   1021000020000001
62202100002912345678901      0000450000ID-10042        CORRUPT ENTRY         00021000020000001
62702100002198765432109      0000150000ID-10043        ACME CORP             00021000020000002
820000000200099999990000001500000000004500001234567890                         021000020000001
9000001000001000000020009999999000000150000000000450000                                       
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999`;
```
