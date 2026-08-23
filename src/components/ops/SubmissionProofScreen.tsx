/**
 * Submission Proof Screen
 *
 * A judge/demo-oriented view of SentinelFlow's authority boundaries and the
 * evidence still required from live Google managed services.  The component is
 * intentionally conservative: managed-cloud capabilities default to NOT_RUN
 * unless a build-time VITE_SENTINEL_*_PROOF value is explicitly supplied after
 * real proof is captured.
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
  History,
  Network,
  ScanSearch,
  ShieldCheck,
  ShieldX,
  UsersRound,
} from 'lucide-react';

type ProofState = 'TESTED' | 'IMPLEMENTED' | 'PASS_LIVE' | 'NOT_RUN' | 'FAIL';

type ManagedProof = {
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

const MANAGED_PROOFS: ManagedProof[] = [
  {
    label: 'Gemini 3.5 Flash',
    state: readProof('VITE_SENTINEL_GEMINI_PROOF', 'IMPLEMENTED'),
    detail: 'Governed model path; live status upgrades only after a real provider invocation.',
    Icon: BrainCircuit,
  },
  {
    label: 'Google ADK',
    state: 'TESTED',
    detail: 'Fixed seven-agent topology with bounded A1/A2 specialists.',
    Icon: GitBranch,
  },
  {
    label: 'Agent Runtime',
    state: readProof('VITE_SENTINEL_AGENT_RUNTIME_PROOF'),
    detail: 'Real AdkApp deployment path exists; managed resource proof is tracked separately.',
    Icon: Cloud,
  },
  {
    label: 'Agent Identity',
    state: readProof('VITE_SENTINEL_AGENT_IDENTITY_PROOF'),
    detail: 'Managed ingress consumes system-attested identity; application-generated identity is forbidden.',
    Icon: Fingerprint,
  },
  {
    label: 'Agent Gateway',
    state: readProof('VITE_SENTINEL_AGENT_GATEWAY_PROOF'),
    detail: 'Network reachability is separate from SentinelFlow Tool Gateway business authority.',
    Icon: Network,
  },
  {
    label: 'Agent Registry',
    state: readProof('VITE_SENTINEL_AGENT_REGISTRY_PROOF'),
    detail: 'Registry is inventory/discovery; it never expands the fixed application roster.',
    Icon: DatabaseZap,
  },
  {
    label: 'Memory Bank',
    state: readProof('VITE_SENTINEL_MEMORY_BANK_PROOF'),
    detail: 'Managed memory is advisory; Go source resolution is required before evidence is minted.',
    Icon: History,
  },
  {
    label: 'Model Armor',
    state: readProof('VITE_SENTINEL_MODEL_ARMOR_PROOF', 'IMPLEMENTED'),
    detail: 'Prompt/response content boundary; a pass is never authorization.',
    Icon: ShieldCheck,
  },
  {
    label: 'Agent Observability',
    state: readProof('VITE_SENTINEL_OBSERVABILITY_PROOF'),
    detail: 'OpenTelemetry instrumentation excludes raw financial payloads by design.',
    Icon: Activity,
  },
];

const STORY = [
  {
    title: 'Immutable ingress',
    detail: 'Original payment file is hashed and preserved before any agent reasoning.',
    Icon: FileLock2,
  },
  {
    title: 'Deterministic quarantine',
    detail: 'Go parser and validators establish financial truth and quarantine invalid input.',
    Icon: ScanSearch,
  },
  {
    title: 'Bounded agent fleet',
    detail: 'Commander delegates diagnosis, policy/SLA, memory and return-risk reasoning to a fixed roster.',
    Icon: BrainCircuit,
  },
  {
    title: 'Memory as a pointer',
    detail: 'Historical recall may locate prior evidence, but memory itself cannot become proof.',
    Icon: History,
  },
  {
    title: 'Derived candidate only',
    detail: 'RemediationAgent proposes intent; Go creates a new immutable candidate from the original.',
    Icon: GitBranch,
  },
  {
    title: 'Independent verification',
    detail: 'Go re-reads bytes, hashes derivation, and reruns deterministic validation before VERIFIED.',
    Icon: BadgeCheck,
  },
  {
    title: 'Human dual control',
    detail: 'Verified is not approved or released. Distinct human reviewers authorize final release.',
    Icon: UsersRound,
  },
];

const stateClass = (state: ProofState): string => {
  switch (state) {
    case 'PASS_LIVE':
      return 'border-emerald-200 bg-emerald-50 text-emerald-800';
    case 'TESTED':
      return 'border-sky-200 bg-sky-50 text-sky-800';
    case 'IMPLEMENTED':
      return 'border-indigo-200 bg-indigo-50 text-indigo-800';
    case 'FAIL':
      return 'border-rose-200 bg-rose-50 text-rose-800';
    default:
      return 'border-slate-200 bg-slate-50 text-slate-600';
  }
};

const StateIcon: React.FC<{ state: ProofState }> = ({ state }) => {
  if (state === 'PASS_LIVE') return <CheckCircle2 className="h-3.5 w-3.5" aria-hidden />;
  if (state === 'FAIL') return <ShieldX className="h-3.5 w-3.5" aria-hidden />;
  return <CircleDashed className="h-3.5 w-3.5" aria-hidden />;
};

export const SubmissionProofScreen: React.FC = () => {
  const liveCount = MANAGED_PROOFS.filter((proof) => proof.state === 'PASS_LIVE').length;

  return (
    <section aria-labelledby="submission-proof-heading" className="space-y-5">
      <div className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
        <div className="border-b border-slate-200 bg-gradient-to-r from-indigo-50 via-white to-emerald-50 px-5 py-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="max-w-3xl">
              <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-indigo-200 bg-white px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.16em] text-indigo-700">
                <ShieldCheck className="h-3.5 w-3.5" aria-hidden />
                Fortified Enterprise Fleet
              </div>
              <h2 id="submission-proof-heading" className="text-xl font-black tracking-tight text-slate-950">
                Autonomous remediation under deterministic financial control
              </h2>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600">
                Gemini and Google ADK can investigate, remember, explain and propose. Go owns financial truth,
                executable capability, verification, evidence, approvals and release.
              </p>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white px-4 py-3 text-right shadow-xs">
              <p className="text-[10px] font-bold uppercase tracking-wider text-slate-500">Managed live proofs</p>
              <p className="mt-1 font-mono text-2xl font-black text-slate-900">
                {liveCount}/{MANAGED_PROOFS.length}
              </p>
              <p className="text-[10px] text-slate-500">Defaults to NOT_RUN until real cloud evidence exists</p>
            </div>
          </div>
        </div>

        <div className="grid gap-px bg-slate-200 md:grid-cols-3">
          <AuthorityCard
            title="AI may propose"
            detail="Diagnosis, memory recall, SLA interpretation, return-risk explanation and allowlisted remediation intent."
            tone="indigo"
          />
          <AuthorityCard
            title="Deterministic systems decide"
            detail="Parser, policy, Tool Gateway, candidate generation, source resolution and independent verification stay authoritative."
            tone="emerald"
          />
          <AuthorityCard
            title="Humans authorize"
            detail="Irreversible release remains behind identity-bound separation of duties and exact artifact/policy integrity binding."
            tone="amber"
          />
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-[1.25fr_1fr]">
        <div className="fintech-card p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-bold text-slate-900">Killer demo authority path</h3>
              <p className="text-xs text-slate-500">Every step narrows authority instead of transferring it to the model.</p>
            </div>
            <span className="rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-[10px] font-bold uppercase tracking-wider text-emerald-700">
              Verified ≠ Approved ≠ Released
            </span>
          </div>
          <ol className="space-y-2">
            {STORY.map(({ title, detail, Icon }, index) => (
              <li key={title} className="flex gap-3 rounded-xl border border-slate-200 bg-slate-50/60 p-3">
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-white font-mono text-xs font-black text-indigo-700 shadow-xs ring-1 ring-slate-200">
                  {index + 1}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <Icon className="h-4 w-4 text-indigo-600" aria-hidden />
                    <p className="text-xs font-bold text-slate-900">{title}</p>
                  </div>
                  <p className="mt-1 text-xs leading-5 text-slate-600">{detail}</p>
                </div>
              </li>
            ))}
          </ol>
        </div>

        <div className="space-y-4">
          <div className="fintech-card p-4">
            <div className="mb-3">
              <h3 className="text-sm font-bold text-slate-900">Google managed proof ledger</h3>
              <p className="text-xs text-slate-500">
                Local implementation and managed live proof are deliberately different states.
              </p>
            </div>
            <div className="space-y-2">
              {MANAGED_PROOFS.map(({ label, state, detail, Icon }) => (
                <div key={label} className="rounded-lg border border-slate-200 bg-white p-2.5">
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex min-w-0 items-center gap-2">
                      <Icon className="h-4 w-4 shrink-0 text-slate-600" aria-hidden />
                      <span className="truncate text-xs font-semibold text-slate-900">{label}</span>
                    </div>
                    <span className={`inline-flex shrink-0 items-center gap-1 rounded-md border px-1.5 py-0.5 font-mono text-[9px] font-bold ${stateClass(state)}`}>
                      <StateIcon state={state} />
                      {state}
                    </span>
                  </div>
                  <p className="mt-1 pl-6 text-[11px] leading-4 text-slate-500">{detail}</p>
                </div>
              ))}
            </div>
          </div>

          <div className="rounded-xl border border-amber-200 bg-amber-50 p-4">
            <div className="flex items-start gap-3">
              <ShieldCheck className="mt-0.5 h-5 w-5 shrink-0 text-amber-700" aria-hidden />
              <div>
                <h3 className="text-xs font-black uppercase tracking-wider text-amber-900">Return intelligence is prioritization, not authority</h3>
                <p className="mt-1 text-xs leading-5 text-amber-900/80">
                  P12 computes a reproducible 0–100 score in Go from seven score-bearing features. ReturnRiskAgent may explain the score but cannot change it, reject a file, or authorize release.
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

const AuthorityCard: React.FC<{
  title: string;
  detail: string;
  tone: 'indigo' | 'emerald' | 'amber';
}> = ({ title, detail, tone }) => {
  const classes = {
    indigo: 'bg-indigo-50 text-indigo-700',
    emerald: 'bg-emerald-50 text-emerald-700',
    amber: 'bg-amber-50 text-amber-700',
  }[tone];

  return (
    <div className="bg-white p-4">
      <div className={`inline-flex rounded-lg p-2 ${classes}`}>
        <ShieldCheck className="h-4 w-4" aria-hidden />
      </div>
      <p className="mt-2 text-xs font-black uppercase tracking-wider text-slate-900">{title}</p>
      <p className="mt-1 text-xs leading-5 text-slate-600">{detail}</p>
    </div>
  );
};
