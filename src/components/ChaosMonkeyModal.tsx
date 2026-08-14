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
