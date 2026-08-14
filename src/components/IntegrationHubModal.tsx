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
