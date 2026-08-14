import React, { useState, useEffect } from 'react';
import { Header } from './components/Header';
import { ChaosControls } from './components/ChaosControls';
import { SlaBoard } from './components/SlaBoard';
import { FileInspectorModal } from './components/FileInspectorModal';
import { AiAnalystPanel } from './components/AiAnalystPanel';
import { AuditLedgerModal } from './components/AuditLedgerModal';
import { UploadModal } from './components/UploadModal';
import { BenchmarkModal } from './components/BenchmarkModal';
import { ExecutiveDeckModal } from './components/ExecutiveDeckModal';
import { ContractConfigModal } from './components/ContractConfigModal';
import { FileDiffModal } from './components/FileDiffModal';
import { ChaosMonkeyModal } from './components/ChaosMonkeyModal';
import { SqlConsoleModal } from './components/SqlConsoleModal';
import { IntegrationHubModal } from './components/IntegrationHubModal';
import { AgentSwarmModal } from './components/AgentSwarmModal';
import { SelfHealingModal } from './components/SelfHealingModal';
import { VaultAndInstantPaymentsModal } from './components/VaultAndInstantPaymentsModal';
import { InfrastructureConfigModal } from './components/InfrastructureConfigModal';

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

import { parseAndValidateNacha } from './parsers/nachaParser';
import { evaluateMissingFileIncident } from './scheduler/deadlineEngine';
import { runExceptionAnalyst } from './ai/exceptionAnalyst';
import { TamperEvidentEventStore, calculateSha256 } from './audit/hashChain';
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
  const [showBenchmarkModal, setShowBenchmarkModal] = useState<boolean>(false);
  const [showDeckModal, setShowDeckModal] = useState<boolean>(false);
  const [showContractsModal, setShowContractsModal] = useState<boolean>(false);
  const [showDiffModal, setShowDiffModal] = useState<boolean>(false);
  const [showChaosMonkeyModal, setShowChaosMonkeyModal] = useState<boolean>(false);
  const [showSqlConsoleModal, setShowSqlConsoleModal] = useState<boolean>(false);
  const [showIntegrationHubModal, setShowIntegrationHubModal] = useState<boolean>(false);
  const [showSwarmModal, setShowSwarmModal] = useState<boolean>(false);
  const [showSelfHealingModal, setShowSelfHealingModal] = useState<boolean>(false);
  const [showVaultModal, setShowVaultModal] = useState<boolean>(false);
  const [showInfraConfigModal, setShowInfraConfigModal] = useState<boolean>(false);
  const [isSimulating, setIsSimulating] = useState<boolean>(false);

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
      state: apiResult.status === 'RELEASED' ? 'VALID' : 'QUARANTINED',
      receivedAtUtc: new Date().toISOString(),
      validationResult: {
        runId: `VAL-RUN-${Date.now()}`,
        parserVersion: 'Moov-ACH-v1.63',
        rulePackVersion: 'Nacha2025.1',
        startedAtUtc: new Date().toISOString(),
        completedAtUtc: new Date().toISOString(),
        outcome: apiResult.status === 'RELEASED' ? 'VALID' : 'QUARANTINED',
        totalRecordsParsed: apiResult.totalRecordsParsed,
        totalDebitsUsd: apiResult.totalDebitsUsd,
        totalCreditsUsd: apiResult.totalCreditsUsd,
        calculatedEntryHash: apiResult.calculatedHash,
        expectedEntryHash: apiResult.expectedHash,
        isBalanced: apiResult.isBalanced,
        findings: apiResult.findings.map(f => ({
          id: `FND-${f.id}`,
          code: f.code,
          severity: f.severity as any,
          lineNumber: f.lineNumber,
          message: f.description,
          ruleReference: f.ruleReference,
          rawSampleRedacted: f.rawData
        })),
        resourceMetrics: {
          streamDurationMs: 4.2,
          peakMemoryMb: 1.8,
          bytesPerSecond: 28500000
        }
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
        onOpenBenchmark={() => setShowBenchmarkModal(true)}
        onOpenDeck={() => setShowDeckModal(true)}
        onOpenContracts={() => setShowContractsModal(true)}
        onOpenDiff={() => setShowDiffModal(true)}
        onOpenChaosDaemon={() => setShowChaosMonkeyModal(true)}
        onOpenSqlConsole={() => setShowSqlConsoleModal(true)}
        onOpenIntegrationHub={() => setShowIntegrationHubModal(true)}
        onOpenAgentSwarm={() => setShowSwarmModal(true)}
        onOpenSelfHealing={() => setShowSelfHealingModal(true)}
        onOpenVault={() => setShowVaultModal(true)}
        onOpenInfraConfig={() => setShowInfraConfigModal(true)}
        openIncidentsCount={openIncidentsCount}
        quarantinedCount={quarantinedCount}
      />

      {/* Main Operations Cockpit Content */}
      <main style={{ padding: '24px', display: 'flex', flexDirection: 'column', gap: '20px', flex: 1 }}>
        {/* Chaos & Simulation Bar */}
        <ChaosControls 
          onSimulateMissingFile={handleSimulateMissingFile}
          onSimulateMalformedNacha={handleSimulateMalformedNacha}
          onSimulateValidNacha={handleSimulateValidNacha}
          onSimulateWorkerCrashRecovery={handleSimulateWorkerCrashRecovery}
          onResetAll={handleResetAll}
          isSimulating={isSimulating}
        />

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

      {/* Performance Benchmark & AI Adversarial Eval Modal */}
      {showBenchmarkModal && (
        <BenchmarkModal 
          onClose={() => setShowBenchmarkModal(false)}
        />
      )}

      {/* Executive Architecture & Briefing Deck Modal */}
      {showDeckModal && (
        <ExecutiveDeckModal 
          onClose={() => setShowDeckModal(false)}
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

      {/* Autonomous Chaos Monkey Daemon & Outbound Webhook Modal */}
      {showChaosMonkeyModal && (
        <ChaosMonkeyModal 
          onClose={() => setShowChaosMonkeyModal(false)}
          onTriggerScenario={(scenario) => {
            if (scenario === 'MISSING_FILE') {
              handleSimulateMissingFile();
            } else if (scenario === 'WORKER_CRASH') {
              handleSimulateWorkerCrashRecovery();
            } else {
              handleSimulateMalformedNacha();
            }
          }}
        />
      )}

      {/* Institutional Read-Only SQL Audit Console Modal */}
      {showSqlConsoleModal && (
        <SqlConsoleModal 
          onClose={() => setShowSqlConsoleModal(false)}
        />
      )}

      {/* Secure Integration Hub & Visual Lineage DAG Modal */}
      {showIntegrationHubModal && (
        <IntegrationHubModal 
          onClose={() => setShowIntegrationHubModal(false)}
        />
      )}

      {/* Astra Multi-Agent Swarm Cockpit Modal */}
      {showSwarmModal && (
        <AgentSwarmModal 
          onClose={() => setShowSwarmModal(false)}
        />
      )}

      {/* Infrastructure Config Modal */}
      {showInfraConfigModal && (
        <InfrastructureConfigModal
          onClose={() => setShowInfraConfigModal(false)}
        />
      )}

      {/* Autonomous Self-Healing & Drift Inspector Modal */}
      {showSelfHealingModal && (
        <SelfHealingModal 
          onClose={() => setShowSelfHealingModal(false)}
        />
      )}

      {/* Zero-Knowledge Vault & FedNow Instant Gateway Modal */}
      {showVaultModal && (
        <VaultAndInstantPaymentsModal 
          onClose={() => setShowVaultModal(false)}
        />
      )}
    </div>
  );
};
export default App;
