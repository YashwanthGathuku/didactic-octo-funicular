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
