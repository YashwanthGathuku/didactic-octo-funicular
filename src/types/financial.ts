export type FormatID = 'NACHA_ACH' | 'BAI2' | 'ISO20022_XML' | 'SWIFT_MT';

export type OccurrenceStatus = 
  | 'EXPECTED'
  | 'DUE_SOON'
  | 'OVERDUE'
  | 'RECEIVED'
  | 'RECEIVED_LATE'
  | 'VALIDATING'
  | 'VALID'
  | 'QUARANTINED'
  | 'RELEASED'
  | 'SUPERSEDED'
  | 'WAIVED';

export type FileState = 
  | 'IN_FLIGHT'
  | 'RECEIVED'
  | 'PARSING'
  | 'VALID'
  | 'QUARANTINED'
  | 'RELEASED'
  | 'REJECTED';

export type IncidentType = 
  | 'MISSING_FILE_DEADLINE'
  | 'NACHA_CONTROL_OUT_OF_BALANCE'
  | 'NACHA_ENTRY_HASH_MISMATCH'
  | 'NACHA_RECORD_ALIGNMENT_CORRUPTION'
  | 'SUSPECTED_DUPLICATE_BATCH'
  | 'UNAUTHORIZED_COUNTERPARTY_KEY'
  | 'PAYLOAD_ZERO_BYTE_DROP'
  | 'ISO20022_MANDATORY_TAG_MISSING';

export type Severity = 'FATAL' | 'ERROR' | 'WARNING' | 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW' | 'INFO';

export interface Partner {
  id: string;
  tenantId: string;
  name: string;
  leiCode?: string;
  status: 'ACTIVE' | 'SUSPENDED' | 'MAINTENANCE';
  allowedChannelIdentities: string[];
  sshKeyFingerprints: string[];
  pgpKeyIds: string[];
  primaryContact: {
    name: string;
    email: string;
    phone: string;
  };
}

export interface FileContract {
  id: string;
  partnerId: string;
  name: string;
  format: FormatID;
  parserVersion: string;
  rulePackVersion: string;
  filenamePattern: string; // regex or glob
  timezone: string; // e.g. "America/New_York"
  businessCalendar: 'US_FED_RESERVE' | 'TARGET2' | 'UK_BANK';
  expectedDueTimeUtc: string; // "16:45:00"
  gracePeriodMinutes: number;
  expectedSizeBounds: {
    minBytes: number;
    maxBytes: number;
  };
  expectedRecordBounds: {
    minRecords: number;
    maxRecords: number;
  };
  allowUnbalancedAch: boolean;
  releasePolicy: 'AUTOMATIC_ON_VALID' | 'MANUAL_APPROVAL_REQUIRED';
}

export interface ExpectationOccurrence {
  id: string;
  contractId: string;
  partnerId: string;
  windowStartUtc: string;
  windowEndUtc: string;
  dueAtUtc: string;
  graceExpiresAtUtc: string;
  status: OccurrenceStatus;
  matchedFileInstanceId?: string;
  incidentId?: string;
  expectedDescription: string;
}

export interface SourceEvent {
  id: string;
  source: 'SFTPGO_WEBHOOK' | 'AS2_INBOUND' | 'INTERNAL_TRANSFER';
  externalEventId: string;
  actor: string;
  clientIp: string;
  sshFingerprint?: string;
  objectBucket: string;
  objectKey: string;
  objectVersion: string;
  receivedAtUtc: string;
  payloadDigestSha256: string;
  status: 'ACCEPTED' | 'REJECTED' | 'DUPLICATE';
}

export interface ValidationFinding {
  id: string;
  code: string; // e.g. "ACH_ERR_0802_HASH_MISMATCH"
  severity: Severity;
  lineNumber?: number;
  recordType?: string; // "1", "5", "6", "8", "9"
  fieldName?: string;
  characterOffset?: number;
  message: string;
  rawSampleRedacted?: string;
  ruleReference: string; // "Nacha Operating Rules 2025, Section 3.2.1"
}

export interface ValidationResult {
  runId: string;
  parserVersion: string;
  rulePackVersion: string;
  startedAtUtc: string;
  completedAtUtc: string;
  outcome: 'VALID' | 'QUARANTINED';
  totalRecordsParsed: number;
  totalDebitsUsd: number;
  totalCreditsUsd: number;
  calculatedEntryHash: string;
  expectedEntryHash?: string;
  isBalanced: boolean;
  findings: ValidationFinding[];
  resourceMetrics: {
    streamDurationMs: number;
    peakMemoryMb: number;
    bytesPerSecond: number;
  };
}

export interface FileInstance {
  id: string;
  sourceEventId: string;
  partnerId: string;
  contractId: string;
  occurrenceId?: string;
  filename: string;
  byteSize: number;
  sha256Hash: string;
  s3Uri: string;
  state: FileState;
  receivedAtUtc: string;
  validationResult?: ValidationResult;
  quarantineReason?: string;
  approvalId?: string;
}

export interface Incident {
  id: string;
  type: IncidentType;
  severity: Severity;
  title: string;
  occurrenceId?: string;
  fileInstanceId?: string;
  partnerId: string;
  status: 'OPEN' | 'IN_INVESTIGATION' | 'MITIGATED' | 'RESOLVED' | 'WAIVED';
  openedAtUtc: string;
  slaDeadlineUtc: string;
  acknowledgedBy?: string;
  resolvedAtUtc?: string;
  resolutionNote?: string;
  assignedAnalystAgentRunId?: string;
}

export interface Approval {
  id: string;
  incidentId: string;
  actionType: 'EXCEPTIONAL_RELEASE' | 'FORCE_REVALIDATE' | 'WAIVE_MISSING_FILE' | 'APPLY_PAYLOAD_PATCH';
  proposedPayloadDigest: string;
  requesterActor: string;
  requestedAtUtc: string;
  approverActor?: string;
  approvedAtUtc?: string;
  status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'EXPIRED';
  justificationReason: string;
  expiresAtUtc: string;
}

export interface AgentRun {
  id: string;
  incidentId: string;
  agentVersion: string;
  modelIdentifier: string;
  ranAtUtc: string;
  inputDigest: string;
  citedEventIds: string[];
  citedFindingCodes: string[];
  citedRunbookSections: string[];
  findingsSummary: string;
  hypotheses: {
    hypothesis: string;
    confidence: 'HIGH' | 'MEDIUM' | 'LOW';
    supportingEvidence: string[];
  }[];
  proposedActionPlan: {
    step: number;
    action: string;
    authorityTier: 0 | 1 | 2 | 3;
    requiresHumanApproval: boolean;
  }[];
  draftExternalPartnerNotice?: string;
  metrics: {
    durationMs: number;
    inputTokens: number;
    outputTokens: number;
    estimatedCostUsd: number;
  };
}

export interface DomainEvent {
  id: string;
  tenantId: string;
  aggregateId: string;
  aggregateType: 'OCCURRENCE' | 'FILE' | 'INCIDENT' | 'APPROVAL';
  eventType: string;
  timestampUtc: string;
  actor: string;
  correlationId: string;
  causationId?: string;
  payload: Record<string, unknown>;
  previousHash: string;
  currentHash: string;
}
