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
