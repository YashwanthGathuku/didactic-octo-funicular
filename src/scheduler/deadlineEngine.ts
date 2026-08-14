import { ExpectationOccurrence, FileContract, Incident, Partner } from '../types/financial';

export interface SlaAssessment {
  occurrence: ExpectationOccurrence;
  minutesUntilDue: number;
  minutesUntilGraceExpiry: number;
  isBreached: boolean;
  breachProbability: number; // 0.0 to 1.0 (from historical baseline)
  statusLabel: string;
  badgeVariant: 'success' | 'warning' | 'danger' | 'neutral';
}

/**
 * Assesses the SLA status of an expectation occurrence given the current UTC time.
 */
export function assessOccurrenceSla(
  occurrence: ExpectationOccurrence,
  nowUtc: Date = new Date()
): SlaAssessment {
  const dueTime = new Date(occurrence.dueAtUtc).getTime();
  const graceTime = new Date(occurrence.graceExpiresAtUtc).getTime();
  const nowTime = nowUtc.getTime();

  const minutesUntilDue = Math.round((dueTime - nowTime) / (1000 * 60));
  const minutesUntilGraceExpiry = Math.round((graceTime - nowTime) / (1000 * 60));

  let isBreached = false;
  let breachProbability = 0.05;
  let statusLabel = 'On Schedule';
  let badgeVariant: SlaAssessment['badgeVariant'] = 'success';

  if (occurrence.status === 'VALID' || occurrence.status === 'RELEASED') {
    statusLabel = 'Delivered & Validated';
    badgeVariant = 'success';
    breachProbability = 0.0;
  } else if (occurrence.status === 'QUARANTINED') {
    statusLabel = 'Quarantined (Invalid)';
    badgeVariant = 'danger';
    isBreached = true;
    breachProbability = 1.0;
  } else if (occurrence.status === 'RECEIVED' || occurrence.status === 'VALIDATING') {
    statusLabel = 'Processing In-Flight';
    badgeVariant = 'warning';
    breachProbability = 0.15;
  } else if (nowTime > graceTime) {
    isBreached = true;
    statusLabel = 'SLA Breached (Missing File)';
    badgeVariant = 'danger';
    breachProbability = 1.0;
  } else if (nowTime > dueTime) {
    statusLabel = 'In Grace Period (Imminent Breach)';
    badgeVariant = 'danger';
    breachProbability = 0.85;
  } else if (minutesUntilDue <= 30) {
    statusLabel = `Due Soon (${minutesUntilDue}m remaining)`;
    badgeVariant = 'warning';
    breachProbability = 0.45;
  } else {
    statusLabel = `Expected in ${Math.round(minutesUntilDue / 60)}h ${minutesUntilDue % 60}m`;
    badgeVariant = 'neutral';
    breachProbability = 0.08;
  }

  return {
    occurrence,
    minutesUntilDue,
    minutesUntilGraceExpiry,
    isBreached,
    breachProbability,
    statusLabel,
    badgeVariant
  };
}

/**
 * Generates an automated Missing File Incident if overdue.
 */
export function evaluateMissingFileIncident(
  occurrence: ExpectationOccurrence,
  contract: FileContract,
  partner: Partner,
  nowUtc: Date = new Date()
): Incident | null {
  const assessment = assessOccurrenceSla(occurrence, nowUtc);
  
  if (assessment.isBreached && occurrence.status !== 'VALID' && occurrence.status !== 'RELEASED') {
    return {
      id: `INC-MISSING-${occurrence.id}`,
      type: 'MISSING_FILE_DEADLINE',
      severity: 'CRITICAL',
      title: `Missing Inbound File: ${contract.name} (${partner.name})`,
      occurrenceId: occurrence.id,
      partnerId: partner.id,
      status: 'OPEN',
      openedAtUtc: nowUtc.toISOString(),
      slaDeadlineUtc: occurrence.dueAtUtc,
      resolutionNote: `Expected by ${occurrence.dueAtUtc} (Grace expired at ${occurrence.graceExpiresAtUtc}). No matching transmission detected over SFTP.`
    };
  }

  return null;
}
