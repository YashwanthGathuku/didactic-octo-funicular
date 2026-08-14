import { Partner, FileContract, ExpectationOccurrence } from '../types/financial';

export const SYNTHETIC_PARTNERS: Partner[] = [
  {
    id: 'PARTNER-MERIDIAN-01',
    tenantId: 'TENANT-DEFAULT',
    name: 'Meridian Treasury Services',
    leiCode: '549300V5L2CGV15S5P38',
    status: 'ACTIVE',
    allowedChannelIdentities: ['sftp://meridian-ingest.sentinel.internal:22', 'sftp-user-meridian-prod'],
    sshKeyFingerprints: ['SHA256:4a3F9cKz8eL1mQxWpTvBn9Rt6sDuFhJ2kLm8qWeRt7y'],
    pgpKeyIds: ['0x9E8C7B6A5F4E3D2C'],
    primaryContact: {
      name: 'Michael Sterling (Treasury Ops Lead)',
      email: 'm.sterling@meridian.mock.com',
      phone: '+1 (212) 635-1000'
    }
  },
  {
    id: 'PARTNER-APEX-02',
    tenantId: 'TENANT-DEFAULT',
    name: 'Apex Clearing & Settlement Network',
    leiCode: '8I5D0TI0ER51YZ7JPI37',
    status: 'ACTIVE',
    allowedChannelIdentities: ['sftp://apex-gateway.sentinel.internal:22', 'sftp-user-apex-clearing'],
    sshKeyFingerprints: ['SHA256:7uK2xP9qLm4nRt6sDuFhJ2kLm8qWeRt7y4a3F9cKz8e'],
    pgpKeyIds: ['0x1A2B3C4D5E6F7A8B'],
    primaryContact: {
      name: 'Sarah Chen (Settlement Engineering)',
      email: 's.chen@apexclearing.mock.com',
      phone: '+1 (212) 270-6000'
    }
  },
  {
    id: 'PARTNER-ATLANTIC-03',
    tenantId: 'TENANT-DEFAULT',
    name: 'Atlantic Custody & Trust Services',
    leiCode: '5714005EN0VJ0SD75X48',
    status: 'ACTIVE',
    allowedChannelIdentities: ['sftp://atlantic-mft.sentinel.internal:22'],
    sshKeyFingerprints: ['SHA256:9qWeRt7y4a3F9cKz8eL1mQxWpTvBn9Rt6sDuFhJ2kLm'],
    pgpKeyIds: ['0x8F7E6D5C4B3A2910'],
    primaryContact: {
      name: 'David Vance (Custody Operations)',
      email: 'd.vance@atlantictrust.mock.com',
      phone: '+1 (617) 786-3000'
    }
  }
];

export const SYNTHETIC_CONTRACTS: FileContract[] = [
  {
    id: 'CONTRACT-MERIDIAN-ACH-01',
    partnerId: 'PARTNER-MERIDIAN-01',
    name: 'Meridian Inbound Commercial ACH Batch',
    format: 'NACHA_ACH',
    parserVersion: 'NachaEngine-v1.4',
    rulePackVersion: 'Nacha2025-Q4',
    filenamePattern: '^MERIDIAN_ACH_COMMERCIAL_\\d{8}_\\d{4}\\.txt$',
    timezone: 'America/New_York',
    businessCalendar: 'US_FED_RESERVE',
    expectedDueTimeUtc: '16:45:00', // 4:45 PM Central Settlement Window
    gracePeriodMinutes: 15,
    expectedSizeBounds: { minBytes: 940, maxBytes: 50000000 },
    expectedRecordBounds: { minRecords: 10, maxRecords: 100000 },
    allowUnbalancedAch: false,
    releasePolicy: 'AUTOMATIC_ON_VALID'
  },
  {
    id: 'CONTRACT-APEX-SWEEP-02',
    partnerId: 'PARTNER-APEX-02',
    name: 'Apex End-of-Day Cash Sweep Batch',
    format: 'NACHA_ACH',
    parserVersion: 'NachaEngine-v1.4',
    rulePackVersion: 'Nacha2025-Q4',
    filenamePattern: '^APEX_SWEEP_\\d{8}\\.ach$',
    timezone: 'America/New_York',
    businessCalendar: 'US_FED_RESERVE',
    expectedDueTimeUtc: '17:30:00',
    gracePeriodMinutes: 10,
    expectedSizeBounds: { minBytes: 940, maxBytes: 25000000 },
    expectedRecordBounds: { minRecords: 10, maxRecords: 50000 },
    allowUnbalancedAch: true,
    releasePolicy: 'AUTOMATIC_ON_VALID'
  },
  {
    id: 'CONTRACT-ATLANTIC-TRADE-03',
    partnerId: 'PARTNER-ATLANTIC-03',
    name: 'Atlantic Custody Trade Settlement',
    format: 'NACHA_ACH',
    parserVersion: 'NachaEngine-v1.4',
    rulePackVersion: 'Nacha2025-Q4',
    filenamePattern: '^ATLANTIC_SETTLE_\\d{8}\\.txt$',
    timezone: 'America/New_York',
    businessCalendar: 'US_FED_RESERVE',
    expectedDueTimeUtc: '18:15:00',
    gracePeriodMinutes: 20,
    expectedSizeBounds: { minBytes: 940, maxBytes: 40000000 },
    expectedRecordBounds: { minRecords: 10, maxRecords: 75000 },
    allowUnbalancedAch: false,
    releasePolicy: 'MANUAL_APPROVAL_REQUIRED'
  }
];

export function generateInitialOccurrences(): ExpectationOccurrence[] {
  const todayStr = new Date().toISOString().split('T')[0];

  return [
    {
      id: 'EXP-MERIDIAN-TODAY',
      contractId: 'CONTRACT-MERIDIAN-ACH-01',
      partnerId: 'PARTNER-MERIDIAN-01',
      windowStartUtc: `${todayStr}T15:00:00.000Z`,
      windowEndUtc: `${todayStr}T17:00:00.000Z`,
      dueAtUtc: `${todayStr}T16:45:00.000Z`,
      graceExpiresAtUtc: `${todayStr}T17:00:00.000Z`,
      status: 'EXPECTED',
      expectedDescription: 'Daily 4:45 PM Clearing Window (Meridian Commercial Payroll & Vendor Feeds)'
    },
    {
      id: 'EXP-APEX-TODAY',
      contractId: 'CONTRACT-APEX-SWEEP-02',
      partnerId: 'PARTNER-APEX-02',
      windowStartUtc: `${todayStr}T16:00:00.000Z`,
      windowEndUtc: `${todayStr}T17:40:00.000Z`,
      dueAtUtc: `${todayStr}T17:30:00.000Z`,
      graceExpiresAtUtc: `${todayStr}T17:40:00.000Z`,
      status: 'EXPECTED',
      expectedDescription: 'EOD Corporate Treasury Concentration Sweep'
    },
    {
      id: 'EXP-ATLANTIC-TODAY',
      contractId: 'CONTRACT-ATLANTIC-TRADE-03',
      partnerId: 'PARTNER-ATLANTIC-03',
      windowStartUtc: `${todayStr}T17:00:00.000Z`,
      windowEndUtc: `${todayStr}T18:35:00.000Z`,
      dueAtUtc: `${todayStr}T18:15:00.000Z`,
      graceExpiresAtUtc: `${todayStr}T18:35:00.000Z`,
      status: 'EXPECTED',
      expectedDescription: 'Institutional Securities Settle & Net Asset Value (NAV) Feed'
    }
  ];
}

/**
 * Pre-constructed Valid 10-line NACHA ACH File (Balanced, Valid Hash, Valid Check Digits)
 */
export const SAMPLE_VALID_NACHA = `101 021000021 1234567892608141645A094101MERIDIAN CUSTODY        SENTINEL FLOW          00000001
5200MERIDIAN PAYROLL DISCRETIONARY       1234567890PPDVENDOR PAY260814260814   1021000020000001
62202100002112345678901      0000150000ID-10042        JOHN DOE              00021000020000001
62702100002198765432109      0000150000ID-10043        ACME CORP             00021000020000002
820000000200042000040000001500000000001500001234567890                         021000020000001
9000001000001000000020004200004000000150000000000150000                                       
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999`;

/**
 * Pre-constructed Malformed NACHA ACH File (Out of Balance + Hash Mismatch + Invalid ABA Check Digit)
 */
export const SAMPLE_CORRUPTED_NACHA = `101 021000021 1234567892608141645A094101MERIDIAN CUSTODY        SENTINEL FLOW          00000001
5200MERIDIAN PAYROLL DISCRETIONARY       1234567890PPDVENDOR PAY260814260814   1021000020000001
62202100002912345678901      0000450000ID-10042        CORRUPT ENTRY         00021000020000001
62702100002198765432109      0000150000ID-10043        ACME CORP             00021000020000002
820000000200099999990000001500000000004500001234567890                         021000020000001
9000001000001000000020009999999000000150000000000450000                                       
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999
9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999`;
