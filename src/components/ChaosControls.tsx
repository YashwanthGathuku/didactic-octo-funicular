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
