/**
 * Sentinel Flow - Multi-Scenario Synthetic NACHA Generator
 * Generates custom institutional NACHA test fixtures on demand for live validation testing.
 */

export type GeneratorPresetKey =
  | 'BALANCED_PPD_PAYROLL'
  | 'UNBALANCED_CCD'
  | 'CORRUPTED_ENTRY_HASH'
  | 'INVALID_ABA_ROUTING'
  | 'RECORD_ALIGNMENT_ERROR'
  | 'ZERO_BYTE_DROP';

export interface PresetInfo {
  key: GeneratorPresetKey;
  label: string;
  category: 'NOMINAL' | 'CONTRACT_ALLOWABLE' | 'PRE_FLIGHT_REJECT';
  description: string;
}

export const PRESET_OPTIONS: PresetInfo[] = [
  {
    key: 'BALANCED_PPD_PAYROLL',
    label: 'Balanced PPD Corporate Payroll ($5,000)',
    category: 'NOMINAL',
    description: 'Valid 94-char NACHA batch with equal debits & credits, valid Mod10 ABA check digits, and verified Entry Hash sum.'
  },
  {
    key: 'UNBALANCED_CCD',
    label: 'Unbalanced CCD Cash Concentration ($2,000 Credits)',
    category: 'CONTRACT_ALLOWABLE',
    description: 'Valid unbalanced ACH disbursement allowed under bilateral contract terms with verified batch control totals.'
  },
  {
    key: 'CORRUPTED_ENTRY_HASH',
    label: 'Corrupted Batch Entry Hash (Trailer Mismatch)',
    category: 'PRE_FLIGHT_REJECT',
    description: 'Trailer declares 0999999999 instead of actual entry routing sum 0143100026. Halts at pre-flight validation.'
  },
  {
    key: 'INVALID_ABA_ROUTING',
    label: 'Invalid ABA Routing Number (Mod10 Checksum Failure)',
    category: 'PRE_FLIGHT_REJECT',
    description: 'Entry detail contains invalid routing number 999999999 failing the Federal Reserve Modulo-10 check digit test.'
  },
  {
    key: 'RECORD_ALIGNMENT_ERROR',
    label: 'Record Alignment Truncation (<94 chars)',
    category: 'PRE_FLIGHT_REJECT',
    description: 'Line 2 truncated to 48 characters, violating Nacha standard 94-character fixed-width record specification.'
  },
  {
    key: 'ZERO_BYTE_DROP',
    label: 'Zero-Byte Empty Transmission Drop',
    category: 'PRE_FLIGHT_REJECT',
    description: 'Simulates counterparty SFTP drop creating a 0-byte ghost file.'
  }
];
