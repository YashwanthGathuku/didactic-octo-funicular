/**
 * Judge/demo proof surface.
 *
 * Local implementation/test evidence and live Google managed-service evidence
 * are intentionally separate. A managed capability defaults to NOT_RUN until
 * real cloud proof explicitly upgrades its VITE_SENTINEL_*_PROOF value.
 */

import React from 'react';
import {
  Activity,
  BadgeCheck,
  BrainCircuit,
  CheckCircle2,
  CircleDashed,
  Cloud,
  DatabaseZap,
  FileLock2,
  Fingerprint,
  GitBranch,
  Globe,
  History,
  Network,
  ScanSearch,
  ShieldAlert,
  ShieldCheck,
  ShieldX,
  UsersRound,
} from 'lucide-react';

type ProofState = 'TESTED' | 'IMPLEMENTED' | 'PASS_LIVE' | 'NOT_RUN' | 'FAIL';

type Proof = {
  label: string;
  state: ProofState;
  detail: string;
  Icon: React.ElementType;
};

const readProof = (name: string, fallback: ProofState = 'NOT_RUN'): ProofState => {
  const value = String(import.meta.env[name] ?? '').trim().toUpperCase();
  return ['TESTED', 'IMPLEMENTED', 'PASS_LIVE', 'NOT_RUN', 'FAIL'].includes(value)
    ? (value as ProofState)
    : fallback;
};

const LOCAL_PROOFS: Proof[] = [
  {
    label: 'Google ADK topology',
    state: 'TESTED',
    detail: 'Fixed bounded specialist roster and orchestration are exercised by local tests/evals.',
    Icon: GitBranch,
  },
  {
    label: 'Gemini 3.5 Flash path',
    state: readProof('VITE_SENTINEL_GEMINI_PROOF', 'IMPLEMENTED'),
    detail: 'Governed provider path is implemented; PASS_LIVE requires a real provider invocation.',
    Icon: BrainCircuit,
  },
  {
    label: 'Deterministic control plane',
    state: 'TESTED',
    detail: 'Go owns parser, policy, Tool Gateway, candidate derivation, verification, review and release state.',
    Icon: ShieldCheck,
  },
  {
    label: 'Data Sovereignty (SF-SAFE-007)',
    state: 'TESTED',
    detail: 'Layer 20 boot invariant prevents cross-border model and memory bank invocations without silent fallback.',
    Icon: Globe,
  },
  {
    label: 'Tool Poisoning Containment',
    state: 'TESTED',
    detail: 'Evaluated against descriptor injection, capability escalation, schema poisoning, and output poisoning attacks.',
    Icon: ShieldAlert,
  },
];

const MANAGED_PROOFS: Proof[] = [
  {
    label: 'Agent Runtime',
    state: readProof('VITE_SENTINEL_AGENT_RUNTIME_PROOF'),
    detail: 'Deployment packaging exists; managed runtime resource evidence is tracked separately.',
    Icon: Cloud,
  },
  {
    label: 'Agent Identity',
    state: readProof('VITE_SENTINEL_AGENT_IDENTITY_PROOF'),
    detail: 'Managed ingress is designed to consume system-attested identity; application-fabricated managed identity is forbidden.',
    Icon: Fingerprint,
  },
  {
    label: 'Agent Gateway',
    state: readProof('VITE_SENTINEL_AGENT_GATEWAY_PROOF'),
    detail: 'Managed network/auth ingress stays separate from SentinelFlow Tool Gateway business authorization.',
    Icon: Network,
  },
  {
    label: 'Agent Registry',
    state: readProof('VITE_SENTINEL_AGENT_REGISTRY_PROOF'),
    detail: 'Registry is inventory/discovery only and cannot expand the fixed application roster.',
    Icon: DatabaseZap,
  },
  {
    label: 'Memory Bank',
    state: readProof('VITE_SENTINEL_MEMORY_BANK_PROOF'),
    detail: 'Managed memory remains advisory; authoritative Go source resolution is required before factual use.',
    Icon: History,
  },
  {
    label: 'Model Armor service',
    state: readProof('VITE_SENTINEL_MODEL_ARMOR_PROOF'),
    detail: 'Managed prompt/response screening is defense-in-depth. ModelArmorPass is never authorization.',
    Icon: ShieldCheck,
  },
  {
    label: 'Cloud Trace / managed observability',
    state: readProof('VITE_SENTINEL_OBSERVABILITY_PROOF'),
    detail: 'Instrumentation excludes raw financial payloads; live managed trace evidence is a separate gate.',
    Icon: Activity,
  },
];

const STORY = [
  {
    title: 'Original preserved',
    detail: 'The received payment artifact is hashed and preserved before agent reasoning begins.',
    Icon: FileLock2,
  },
  {
    title: 'Deterministic quarantine',
    detail: 'Go parsing and validation establish financial truth and quarantine invalid input.',
    Icon: ScanSearch,
  },
  {
    title: 'Bounded investigation',
    detail: 'Commander delegates explanation and advisory reasoning to a fixed ADK roster.',
    Icon: BrainCircuit,
  },
  {
    title: 'Memory resolves to sources',
    detail: 'Historical recall can locate prior context; memory itself cannot satisfy an evidence requirement.',
    Icon: History,
  },
  {
    title: 'Derived candidate only',
    detail: 'The model proposes allowlisted intent; Go creates a new candidate from the original artifact.',
    Icon: GitBranch,
  },
  {
    title: 'Independent verification',
    detail: 'Go re-reads candidate bytes, checks derivation integrity, and reruns deterministic validation.',
    Icon: BadgeCheck,
  },
  {
    title: 'Human authorization',
    detail: 'VERIFIED is not APPROVED or RELEASED. Distinct authorized humans control irreversible release.',
    Icon: UsersRound,
  },
];

const stateClass = (state: ProofState): string => {
  switch (state) {
    case 'PASS_LIVE': return 'badge-emerald';
    case 'TESTED': return 'badge-sky';
    case 'IMPLEMENTED': return 'badge-indigo';
    case 'FAIL': return 'badge-rose';
    default: return 'badge-slate';
  }
};

const StateIcon: React.FC<{ state: ProofState }> = ({ state }) => {
  if (state === 'PASS_LIVE') return <CheckCircle2 className="h-3.5 w-3.5" aria-hidden />;
  if (state === 'FAIL') return <ShieldX className="h-3.5 w-3.5" aria-hidden />;
  return <CircleDashed className="h-3.5 w-3.5" aria-hidden />;
};

export const SubmissionProofScreen: React.FC = () => {
  const managedLive = MANAGED_PROOFS.filter((proof) => proof.state === 'PASS_LIVE').length;

  return (
    <section aria-labelledby="submission-proof-heading" className="space-y-4">
      <div className="surface-panel overflow-hidden">
        <div className="grid gap-0 xl:grid-cols-[minmax(0,1fr)_300px]">
          <div className="p-5 sm:p-6 lg:p-7">
            <div className="flex items-center gap-2 text-[11px] font-semibold text-slate-500">
              <ShieldCheck className="h-4 w-4" aria-hidden />
              Fortified Enterprise Fleet · authority proof
            </div>
            <h2 id="submission-proof-heading" className="mt-3 max-w-3xl text-2xl font-bold tracking-[-0.04em] text-slate-950 sm:text-[28px]">
              Autonomous remediation under deterministic financial control
            </h2>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-slate-600">
              SentinelFlow gives Gemini room to investigate and propose without transferring financial authority. The deterministic Go control plane decides what is valid and executable; humans authorize irreversible release.
            </p>

            <div className="mt-6 flex flex-wrap items-center gap-x-2 gap-y-2 text-xs font-semibold text-slate-800" aria-label="Authority chain">
              <span className="rounded-md border border-slate-300 bg-white px-2.5 py-1.5">AI proposes</span>
              <span className="text-slate-300" aria-hidden>→</span>
              <span className="rounded-md border border-slate-900 bg-slate-950 px-2.5 py-1.5 text-white">Deterministic core decides</span>
              <span className="text-slate-300" aria-hidden>→</span>
              <span className="rounded-md border border-slate-300 bg-white px-2.5 py-1.5">Humans authorize</span>
            </div>
          </div>

          <aside className="border-t border-slate-200 bg-slate-50/80 p-5 xl:border-l xl:border-t-0" aria-label="Managed Google proof status">
            <p className="text-[10px] font-semibold tracking-wide text-slate-500">LIVE MANAGED PROOF</p>
            <p className="mt-2 text-3xl font-bold tracking-[-0.05em] text-slate-950 tabular-nums">
              {managedLive}<span className="text-sm font-medium text-slate-400"> / {MANAGED_PROOFS.length}</span>
            </p>
            <p className="mt-1 text-xs leading-5 text-slate-600">
              Managed services stay <strong className="font-semibold text-slate-800">NOT_RUN</strong> until real Google Cloud evidence is captured. Local code or mocks never upgrade this count.
            </p>
          </aside>
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(360px,0.85fr)]">
        <section className="surface-panel overflow-hidden" aria-labelledby="authority-path-heading">
          <div className="border-b border-slate-200 px-4 py-3.5 sm:px-5">
            <h3 id="authority-path-heading" className="text-sm font-semibold text-slate-950">Demo authority path</h3>
            <p className="mt-0.5 text-xs text-slate-500">Each transition narrows authority instead of moving it into the model.</p>
          </div>

          <ol className="divide-y divide-slate-100">
            {STORY.map(({ title, detail, Icon }, index) => (
              <li key={title} className="grid gap-3 px-4 py-3.5 sm:grid-cols-[34px_minmax(0,1fr)] sm:px-5">
                <div className="flex h-8 w-8 items-center justify-center rounded-md border border-slate-200 bg-slate-50 font-mono text-[11px] font-semibold text-slate-600">
                  {String(index + 1).padStart(2, '0')}
                </div>
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <Icon className="h-4 w-4 shrink-0 text-slate-500" aria-hidden />
                    <p className="text-xs font-semibold text-slate-900">{title}</p>
                  </div>
                  <p className="mt-1 text-xs leading-5 text-slate-600">{detail}</p>
                </div>
              </li>
            ))}
          </ol>

          <div className="border-t border-slate-200 bg-slate-50 px-4 py-3 text-xs font-semibold text-slate-700 sm:px-5">
            VERIFIED ≠ APPROVED ≠ RELEASED
          </div>
        </section>

        <div className="space-y-4">
          <ProofLedger title="Local implementation / test evidence" proofs={LOCAL_PROOFS} />
          <ProofLedger title="Google managed-service evidence" proofs={MANAGED_PROOFS} />

          <div className="border-l-2 border-amber-500 bg-amber-50 px-4 py-3">
            <p className="text-xs font-semibold text-amber-950">Return intelligence prioritizes; it does not authorize.</p>
            <p className="mt-1 text-xs leading-5 text-amber-900">
              The return-risk score is reproducibly computed in Go. ReturnRiskAgent may explain that score but cannot change it, reject a file, satisfy deterministic verification, or authorize release.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
};

const ProofLedger: React.FC<{ title: string; proofs: Proof[] }> = ({ title, proofs }) => (
  <section className="surface-panel overflow-hidden">
    <div className="border-b border-slate-200 px-4 py-3">
      <h3 className="text-xs font-semibold text-slate-900">{title}</h3>
    </div>
    <div className="divide-y divide-slate-100">
      {proofs.map(({ label, state, detail, Icon }) => (
        <div key={label} className="px-4 py-3">
          <div className="flex items-start justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2">
              <Icon className="h-4 w-4 shrink-0 text-slate-500" aria-hidden />
              <span className="text-xs font-semibold text-slate-900">{label}</span>
            </div>
            <span className={`inline-flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 font-mono text-[9px] font-semibold ${stateClass(state)}`}>
              <StateIcon state={state} />
              {state}
            </span>
          </div>
          <p className="mt-1.5 pl-6 text-[11px] leading-4 text-slate-500">{detail}</p>
        </div>
      ))}
    </div>
  </section>
);
