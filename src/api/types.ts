/**
 * Server response shapes.
 *
 * Hand-written to match the Go structs one-for-one, and deliberately not
 * inferred from what a running server happens to return: a type derived from a
 * sample response encodes whatever that sample contained, including fields the
 * server only sometimes sends and the absence of ones it sends rarely.
 *
 * Where a Go field is a pointer or has `omitempty`, the field here is optional.
 * Where the Go side can return null meaningfully -- `validationRun` on an
 * artifact that has not been validated -- the type is `T | null` rather than
 * optional, because "absent" and "explicitly nothing" are different answers and
 * only one of them means "not validated yet".
 */

// ---------------------------------------------------------------------------
// Paging
// ---------------------------------------------------------------------------

/**
 * The envelope every list endpoint returns.
 *
 * `partial` is the state the guide calls partial-data: the server assembled the
 * page but could not read every row. A list that silently omitted an unreadable
 * quarantined artifact would be worse than one that says it is incomplete, so
 * the flag exists and the UI has to render it.
 */
export interface Page<T> {
  items: T[];
  nextCursor?: string;
  hasMore: boolean;
  limit: number;
  partial?: boolean;
  partialReason?: string;
}

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

export type Permission =
  | 'tenant:read'
  | 'evidence:read'
  | 'artifact:upload'
  | 'release:approve'
  | 'release:override'
  | 'contract:manage'
  | 'release_policy:manage'
  | 'secret:manage';

export interface Session {
  subject: string;
  issuer: string;
  email?: string;
  tenantId: string;
  tenants: string[];
  roles: string[];
  /**
   * What this caller may do, answered by the same Authorize call the handlers
   * use. A presentation hint only: every route re-authorizes, so a client that
   * lies to itself about this list gains nothing but a worse error message.
   */
  permissions: Permission[];
  profile: string;
  demo: boolean;
  /** The server's clock. The browser's is settable by the person reading it. */
  serverNow: string;
}

// ---------------------------------------------------------------------------
// Artifacts and validation
// ---------------------------------------------------------------------------

export type ArtifactStatus =
  | 'RECEIVED'
  | 'VALIDATING'
  | 'VALIDATED'
  | 'QUARANTINED'
  | 'APPROVED'
  | 'RELEASED'
  | 'REJECTED';

export interface ArtifactSummary {
  id: number;
  filename: string;
  sha256: string;
  sizeBytes: number;
  status: ArtifactStatus;
  receivedAt: string;
  updatedAt: string;
  expectationId?: number;
  contractId?: string;
  partnerName?: string;
  blockingFindings: number;
  warningFindings: number;
  decisionId?: number;
  decisionState?: string;
}

export type Severity = 'INFO' | 'WARNING' | 'BLOCKING';

export interface Finding {
  id: number;
  code: string;
  ruleVersion: string;
  provenance: string;
  description: string;
  severity: Severity;
  lineNumber: number;
  byteOffset: number;
  fieldStart: number;
  fieldEnd: number;
  /**
   * Redacted by the server where it was written. There is no un-redacted form
   * to leak and no masking for this client to remember to apply -- which is the
   * point of masking by API policy rather than by CSS.
   */
  evidence?: string;
  expected?: string;
  actual?: string;
}

export interface ValidationRun {
  id: number;
  parserName: string;
  parserVersion: string;
  rulePackVersion: string;
  policyVersion: string;
  contractId?: string;
  contractVersion?: string;
  outcome: string;
  parserOk: boolean;
  findingsDigest: string;
  blockingRuleIds?: string;
  recordsParsed: number;
  totalDebitsMinor: number;
  totalCreditsMinor: number;
  startedAt: string;
  completedAt?: string;
}

export interface Transition {
  fromState: string;
  toState: string;
  actorId: string;
  reason?: string;
  occurredAt: string;
}

export interface ArtifactDetail extends ArtifactSummary {
  storagePath?: string;
  rowVersion: number;
  /** null means not validated. It does not mean validated with no findings. */
  validationRun: ValidationRun | null;
  findings: Finding[];
  history: Transition[];
  notCheckedRuleIds?: string[];
}

// ---------------------------------------------------------------------------
// Expectations (the feed/status board)
// ---------------------------------------------------------------------------

export type ExpectationStatus =
  | 'PENDING'
  | 'DUE'
  | 'OVERDUE'
  | 'BREACHED'
  | 'ARRIVED'
  | 'WAIVED';

export interface Expectation {
  id: number;
  contractId: number;
  partnerName: string;
  partnerRouting: string;
  contractName: string;
  filenamePattern: string;
  expectedTime: string;
  gracePeriodMinutes: number;
  /** UTC instants. */
  deliveryStart: string;
  deliveryEnd: string;
  status: ExpectationStatus;
  businessDate?: string;
  feedId?: string;
  /** The deadline as the partner's agreement states it, in their zone. */
  dueLocal?: string;
  timezone?: string;
  breachesAt?: string;
  /** Why this deadline is not simply the contracted time. */
  scheduleNote?: string;
  reviewRequired?: boolean;
}

// ---------------------------------------------------------------------------
// Incidents
// ---------------------------------------------------------------------------

export interface Incident {
  id: number;
  expectationId: number;
  fileInstanceId?: number;
  type: string;
  severity: string;
  status: 'OPEN' | 'INVESTIGATING' | 'RESOLVED' | 'CLOSED';
  createdAt: string;
  partnerName: string;
  filename: string;
  sha256: string;
  findings?: Finding[];
}

// ---------------------------------------------------------------------------
// Release decisions
// ---------------------------------------------------------------------------

export interface Vote {
  actorId: string;
  role: string;
  choice: 'APPROVE' | 'REJECT';
  reason: string;
  at: string;
}

export interface Decision {
  id: number;
  artifactId: number;
  validationRunId: number;
  state: 'PROPOSED' | 'APPROVED' | 'REJECTED' | 'EXPIRED';
  outcome: string;
  policyVersion: string;
  rulePackVersion: string;
  artifactSha256: string;
  /**
   * The digest the decision currently stands on. A vote cast against a
   * different one does not count, which is how an approval expires when the
   * artifact, findings, policy, rule pack, contract or run change.
   */
  integrityDigest: string;
  findingsDigest: string;
  proposedBy: string;
  proposedAt: string;
  requiredApprovals: number;
  separationOfDuties: boolean;
  votes: Vote[];
  expiredReason?: string;
  releasedAt?: string;
  releasedBy?: string;
  reason?: string;
  /** Optimistic concurrency. A mutation against a stale value is refused. */
  rowVersion: number;
}

export interface ReleasePolicy {
  minApprovals: number;
  separationOfDuties: boolean;
  overrideAllowed: boolean;
}

export interface OverrideRecord {
  id: number;
  decisionId: number;
  artifactId: number;
  actorId: string;
  role: string;
  reason: string;
  /** What was bypassed, in the system's own terms. */
  bypassed: string;
  approvalsHeld: number;
  approvalsRequired: number;
  blockingRuleIds?: string[];
  createdAt: string;
}

// ---------------------------------------------------------------------------
// Contracts
// ---------------------------------------------------------------------------

export interface Contract {
  id: number;
  partnerId: number;
  name: string;
  filenamePattern: string;
  format: string;
  expectedTime: string;
  timezone: string;
  gracePeriodMinutes: number;
  [key: string]: unknown;
}

export interface ContractVersion {
  id: number;
  version: number;
  filenamePattern: string;
  format: string;
  expectedLocal: string;
  timezone: string;
  graceMinutes: number;
  calendarId?: string;
  balancedMode: string;
  effectiveFrom: string;
  effectiveTo?: string;
  createdAt: string;
  /** Decided by the server against its own clock, not recomputed here. */
  current: boolean;
}

// ---------------------------------------------------------------------------
// Evidence
// ---------------------------------------------------------------------------

export interface EvidenceEntry {
  id: number;
  sequenceNo: number;
  eventType: string;
  actor: string;
  objectType?: string;
  objectId?: number;
  correlationId?: string;
  payload: unknown;
  previousHash: string;
  currentHash: string;
  createdAt: string;
}

// ---------------------------------------------------------------------------
// Service health
// ---------------------------------------------------------------------------

/**
 * NOT_CONFIGURED and UNKNOWN are separate from OK on purpose. A dependency
 * nobody configured is not healthy, and one nobody measured is not healthy
 * either.
 */
export type ComponentStatus =
  | 'OK'
  | 'DEGRADED'
  | 'UNAVAILABLE'
  | 'NOT_CONFIGURED'
  | 'UNKNOWN';

export interface ComponentHealth {
  status: ComponentStatus;
  detail?: string;
  /** False means nothing probed this. The UI must not render it as green. */
  measured: boolean;
  latencyMs?: number;
}

export interface QueueHealth {
  queued: number;
  leased: number;
  running: number;
  retryable: number;
  dead: number;
  /** A depth of 3 that has not moved in two hours is worse than 300 draining. */
  oldestQueuedAgeSeconds: number | null;
}

export interface OutboxHealth {
  undelivered: number;
  dead: number;
  oldestUndeliveredAgeSeconds: number | null;
}

export interface ServiceHealth {
  profile: string;
  demo: boolean;
  serverNow: string;
  database: ComponentHealth;
  queue: QueueHealth | null;
  outbox: OutboxHealth | null;
  scheduler: ComponentHealth;
  objectStore: ComponentHealth;
  aiTier: ComponentHealth;
}

// ---------------------------------------------------------------------------
// Event stream
// ---------------------------------------------------------------------------

export interface StreamEvent {
  id: number;
  eventType: string;
  subjectType: string;
  subjectId: number;
  payload: unknown;
  createdAt: string;
}

export interface StreamHello {
  cursor: number;
  head: number;
  tenantId: string;
  replay: boolean;
  /**
   * True when the requested cursor predates the retained window. The client
   * must reload rather than assume it has a continuous view: a stream with an
   * undetected hole in it is worse than one that admits the hole.
   */
  gap: boolean;
  serverNow: string;
}
