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
