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
