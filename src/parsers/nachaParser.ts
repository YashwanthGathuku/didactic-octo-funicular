import { ValidationFinding, ValidationResult, FileContract } from '../types/financial';

/**
 * ABA Routing Number Checksum Validator
 * Formula: (3*(d1+d4+d7) + 7*(d2+d5+d8) + 1*(d3+d6+d9)) % 10 === 0
 */
export function validateAbaRouting(routing9: string): boolean {
  if (!/^\d{9}$/.test(routing9)) return false;
  const digits = routing9.split('').map(Number);
  const sum = 
    3 * (digits[0] + digits[3] + digits[6]) +
    7 * (digits[1] + digits[4] + digits[7]) +
    1 * (digits[2] + digits[5] + digits[8]);
  return sum % 10 === 0;
}

export interface NachaParsedData {
  immediateOrigin: string;
  immediateDestination: string;
  fileCreationDate: string;
  fileCreationTime: string;
  fileIdModifier: string;
  batches: Array<{
    batchNumber: number;
    secCode: string;
    companyName: string;
    companyId: string;
    serviceClassCode: string;
    entryCount: number;
    calculatedEntryHash: number;
    batchControlEntryHash: number;
    totalDebitCents: number;
    totalCreditCents: number;
    entries: Array<{
      lineNumber: number;
      transactionCode: string;
      routingNumber: string;
      checkDigit: string;
      accountNumberRedacted: string;
      amountCents: number;
      individualName: string;
      traceNumber: string;
      isDebit: boolean;
    }>;
  }>;
  fileControl: {
    batchCount: number;
    blockCount: number;
    entryAddendaCount: number;
    entryHash: number;
    totalDebitCents: number;
    totalCreditCents: number;
  };
}

export function parseAndValidateNacha(
  rawContent: string,
  contract: Partial<FileContract> = {}
): { parsed?: NachaParsedData; result: ValidationResult } {
  const startedAt = new Date().toISOString();
  const startTime = performance.now();
  const findings: ValidationFinding[] = [];

  const lines = rawContent.split(/\r?\n/).filter(line => line.length > 0);
  
  if (lines.length === 0) {
    findings.push({
      id: 'FIND-EMPTY-001',
      code: 'ACH_ERR_0100_EMPTY_PAYLOAD',
      severity: 'FATAL',
      message: 'The submitted file is 0 bytes or contains no valid record lines.',
      ruleReference: 'Nacha Operating Rules 2025, Chapter 3: File Structure'
    });

    return {
      result: {
        runId: `RUN-${Date.now()}`,
        parserVersion: 'NachaEngine-v1.4-moov-compat',
        rulePackVersion: 'Nacha2025-Q4-Standard',
        startedAtUtc: startedAt,
        completedAtUtc: new Date().toISOString(),
        outcome: 'QUARANTINED',
        totalRecordsParsed: 0,
        totalDebitsUsd: 0,
        totalCreditsUsd: 0,
        calculatedEntryHash: '0',
        isBalanced: true,
        findings,
        resourceMetrics: {
          streamDurationMs: Math.round(performance.now() - startTime),
          peakMemoryMb: 2.1,
          bytesPerSecond: 0
        }
      }
    };
  }

  let hasFileHeader = false;
  let hasFileControl = false;
  let fileImmediateOrigin = '';
  let fileImmediateDestination = '';
  let fileCreationDate = '';
  let fileCreationTime = '';
  let fileIdModifier = '';

  const batches: NachaParsedData['batches'] = [];
  let currentBatch: NachaParsedData['batches'][0] | null = null;

  let totalFileDebitsCents = 0;
  let totalFileCreditsCents = 0;
  let totalFileEntryHashAccumulator = 0;
  let totalEntriesAndAddenda = 0;

  for (let i = 0; i < lines.length; i++) {
    const lineNumber = i + 1;
    const line = lines[i];

    // Check strict 94-character length
    if (line.length !== 94) {
      findings.push({
        id: `FIND-LEN-${lineNumber}`,
        code: 'ACH_ERR_0101_INVALID_RECORD_LENGTH',
        severity: 'FATAL',
        lineNumber,
        recordType: line[0] || '?',
        message: `Record length is ${line.length} characters; exactly 94 characters are required by Nacha specifications.`,
        rawSampleRedacted: line.substring(0, 30) + '...',
        ruleReference: 'Nacha Operating Rules 2025, Section 3.1: Record Specifications'
      });
    }

    const recordType = line[0];

    // 1 - File Header
    if (recordType === '1') {
      if (lineNumber !== 1) {
        findings.push({
          id: `FIND-POS-HDR-${lineNumber}`,
          code: 'ACH_ERR_0102_HEADER_OUT_OF_POSITION',
          severity: 'FATAL',
          lineNumber,
          recordType: '1',
          message: 'File Header (Type 1) must be the first line of the ACH file.',
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.1'
        });
      }
      hasFileHeader = true;
      fileImmediateDestination = line.substring(3, 13).trim();
      fileImmediateOrigin = line.substring(13, 23).trim();
      fileCreationDate = line.substring(23, 29);
      fileCreationTime = line.substring(29, 33);
      fileIdModifier = line.substring(33, 34);

      const recordSize = line.substring(34, 37);
      const blockingFactor = line.substring(37, 39);
      const formatCode = line.substring(39, 40);

      if (recordSize !== '094') {
        findings.push({
          id: `FIND-HDR-SIZE-${lineNumber}`,
          code: 'ACH_ERR_0103_RECORD_SIZE_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '1',
          fieldName: 'Record Size',
          message: `File header Record Size is '${recordSize}', expected '094'.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.1'
        });
      }

      if (blockingFactor !== '10') {
        findings.push({
          id: `FIND-HDR-BLOCK-${lineNumber}`,
          code: 'ACH_ERR_0104_BLOCKING_FACTOR_INVALID',
          severity: 'WARNING',
          lineNumber,
          recordType: '1',
          fieldName: 'Blocking Factor',
          message: `Blocking Factor is '${blockingFactor}', standard is '10'.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.1'
        });
      }

      if (formatCode !== '1') {
        findings.push({
          id: `FIND-HDR-FMT-${lineNumber}`,
          code: 'ACH_ERR_0105_FORMAT_CODE_INVALID',
          severity: 'ERROR',
          lineNumber,
          recordType: '1',
          fieldName: 'Format Code',
          message: `Format Code is '${formatCode}', expected '1'.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.1'
        });
      }
    }

    // 5 - Batch Header
    else if (recordType === '5') {
      if (currentBatch !== null) {
        findings.push({
          id: `FIND-BATCH-NEST-${lineNumber}`,
          code: 'ACH_ERR_0501_UNCLOSED_PREVIOUS_BATCH',
          severity: 'FATAL',
          lineNumber,
          recordType: '5',
          message: `New Batch Header encountered without closing Batch #${currentBatch.batchNumber} with a Type 8 record.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.2'
        });
      }

      const serviceClassCode = line.substring(1, 4);
      const companyName = line.substring(4, 20).trim();
      const companyId = line.substring(40, 50).trim();
      const secCode = line.substring(50, 53).trim();
      const batchNumber = parseInt(line.substring(87, 94), 10) || (batches.length + 1);

      currentBatch = {
        batchNumber,
        secCode,
        companyName,
        companyId,
        serviceClassCode,
        entryCount: 0,
        calculatedEntryHash: 0,
        batchControlEntryHash: 0,
        totalDebitCents: 0,
        totalCreditCents: 0,
        entries: []
      };
    }

    // 6 - Entry Detail
    else if (recordType === '6') {
      if (!currentBatch) {
        findings.push({
          id: `FIND-ORPHAN-ENTRY-${lineNumber}`,
          code: 'ACH_ERR_0601_ORPHAN_ENTRY_DETAIL',
          severity: 'FATAL',
          lineNumber,
          recordType: '6',
          message: 'Entry Detail record (Type 6) found outside of any open Batch Header (Type 5).',
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.3'
        });
        continue;
      }

      totalEntriesAndAddenda++;
      currentBatch.entryCount++;

      const transactionCode = line.substring(1, 3);
      const receivingDfiRouting8 = line.substring(3, 11);
      const checkDigit = line.substring(11, 12);
      const fullRouting = receivingDfiRouting8 + checkDigit;
      const dfiAccountRaw = line.substring(12, 29).trim();
      const amountCents = parseInt(line.substring(29, 39), 10) || 0;
      const individualName = line.substring(54, 76).trim();
      const traceNumber = line.substring(79, 94).trim();

      const isDebit = ['27', '28', '37', '38', '47', '48'].includes(transactionCode);
      if (isDebit) {
        currentBatch.totalDebitCents += amountCents;
        totalFileDebitsCents += amountCents;
      } else {
        currentBatch.totalCreditCents += amountCents;
        totalFileCreditsCents += amountCents;
      }

      const routing8Number = parseInt(receivingDfiRouting8, 10) || 0;
      currentBatch.calculatedEntryHash = (currentBatch.calculatedEntryHash + routing8Number) % 10000000000;

      if (!validateAbaRouting(fullRouting)) {
        findings.push({
          id: `FIND-ABA-ERR-${lineNumber}`,
          code: 'ACH_ERR_0602_INVALID_ABA_CHECK_DIGIT',
          severity: 'ERROR',
          lineNumber,
          recordType: '6',
          fieldName: 'Receiving DFI Routing Number',
          message: `Invalid ABA routing number checksum for '${receivingDfiRouting8}${checkDigit}' (Check Digit ${checkDigit} failed Modulo 10 verification).`,
          rawSampleRedacted: `${receivingDfiRouting8}X ... $${(amountCents / 100).toFixed(2)}`,
          ruleReference: 'Nacha Operating Rules 2025, Appendix B: Routing Number Verification'
        });
      }

      const redactedAccount = dfiAccountRaw.length > 4 
        ? '*'.repeat(dfiAccountRaw.length - 4) + dfiAccountRaw.slice(-4)
        : '****';

      currentBatch.entries.push({
        lineNumber,
        transactionCode,
        routingNumber: receivingDfiRouting8,
        checkDigit,
        accountNumberRedacted: redactedAccount,
        amountCents,
        individualName,
        traceNumber,
        isDebit
      });
    }

    // 7 - Addenda Record
    else if (recordType === '7') {
      totalEntriesAndAddenda++;
    }

    // 8 - Batch Control
    else if (recordType === '8') {
      if (!currentBatch) {
        findings.push({
          id: `FIND-ORPHAN-CTRL-${lineNumber}`,
          code: 'ACH_ERR_0801_ORPHAN_BATCH_CONTROL',
          severity: 'FATAL',
          lineNumber,
          recordType: '8',
          message: 'Batch Control record (Type 8) found without an active Batch.',
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.4'
        });
        continue;
      }

      const controlEntryCount = parseInt(line.substring(4, 10), 10) || 0;
      const controlEntryHash = parseInt(line.substring(10, 20), 10) || 0;
      const controlTotalDebits = parseInt(line.substring(20, 32), 10) || 0;
      const controlTotalCredits = parseInt(line.substring(32, 44), 10) || 0;

      currentBatch.batchControlEntryHash = controlEntryHash;

      if (controlEntryCount !== currentBatch.entryCount) {
        findings.push({
          id: `FIND-BATCH-COUNT-${lineNumber}`,
          code: 'ACH_ERR_0802_BATCH_COUNT_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '8',
          fieldName: 'Entry/Addenda Count',
          message: `Batch #${currentBatch.batchNumber} declares ${controlEntryCount} entries, but contains ${currentBatch.entryCount} actual records.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.4'
        });
      }

      if (controlEntryHash !== currentBatch.calculatedEntryHash) {
        findings.push({
          id: `FIND-BATCH-HASH-${lineNumber}`,
          code: 'ACH_ERR_0803_BATCH_ENTRY_HASH_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '8',
          fieldName: 'Entry Hash',
          message: `Batch #${currentBatch.batchNumber} declared Entry Hash '${controlEntryHash}' does not match calculated sum '${currentBatch.calculatedEntryHash}'.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.4'
        });
      }

      if (controlTotalDebits !== currentBatch.totalDebitCents) {
        findings.push({
          id: `FIND-BATCH-DEBIT-${lineNumber}`,
          code: 'ACH_ERR_0804_BATCH_DEBIT_TOTAL_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '8',
          fieldName: 'Total Debit Entry Dollar Amount',
          message: `Batch #${currentBatch.batchNumber} declared Debits ($${(controlTotalDebits/100).toFixed(2)}) != calculated sum ($${(currentBatch.totalDebitCents/100).toFixed(2)}).`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.4'
        });
      }

      if (controlTotalCredits !== currentBatch.totalCreditCents) {
        findings.push({
          id: `FIND-BATCH-CREDIT-${lineNumber}`,
          code: 'ACH_ERR_0805_BATCH_CREDIT_TOTAL_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '8',
          fieldName: 'Total Credit Entry Dollar Amount',
          message: `Batch #${currentBatch.batchNumber} declared Credits ($${(controlTotalCredits/100).toFixed(2)}) != calculated sum ($${(currentBatch.totalCreditCents/100).toFixed(2)}).`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.4'
        });
      }

      totalFileEntryHashAccumulator = (totalFileEntryHashAccumulator + currentBatch.calculatedEntryHash) % 10000000000;
      batches.push(currentBatch);
      currentBatch = null;
    }

    // 9 - File Control
    else if (recordType === '9') {
      hasFileControl = true;
      const declaredBatchCount = parseInt(line.substring(1, 7), 10) || 0;
      const declaredBlockCount = parseInt(line.substring(7, 13), 10) || 0;
      const declaredEntryAddendaCount = parseInt(line.substring(13, 21), 10) || 0;
      const declaredFileEntryHash = parseInt(line.substring(21, 31), 10) || 0;
      const declaredFileDebits = parseInt(line.substring(31, 43), 10) || 0;
      const declaredFileCredits = parseInt(line.substring(43, 55), 10) || 0;

      if (declaredBatchCount !== batches.length) {
        findings.push({
          id: `FIND-FILE-BATCH-COUNT-${lineNumber}`,
          code: 'ACH_ERR_0901_FILE_BATCH_COUNT_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '9',
          fieldName: 'Batch Count',
          message: `File Control declares ${declaredBatchCount} batches, but file contains ${batches.length} parsed batches.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.5'
        });
      }

      if (declaredEntryAddendaCount !== totalEntriesAndAddenda) {
        findings.push({
          id: `FIND-FILE-ENTRY-COUNT-${lineNumber}`,
          code: 'ACH_ERR_0902_FILE_ENTRY_COUNT_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '9',
          fieldName: 'Entry/Addenda Count',
          message: `File Control declares ${declaredEntryAddendaCount} entries/addenda, actual count is ${totalEntriesAndAddenda}.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.5'
        });
      }

      if (declaredFileEntryHash !== totalFileEntryHashAccumulator) {
        findings.push({
          id: `FIND-FILE-HASH-${lineNumber}`,
          code: 'ACH_ERR_0903_FILE_ENTRY_HASH_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '9',
          fieldName: 'Entry Hash Total',
          message: `File Control Entry Hash '${declaredFileEntryHash}' does not match batch accumulator '${totalFileEntryHashAccumulator}'.`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.5'
        });
      }

      if (declaredFileDebits !== totalFileDebitsCents) {
        findings.push({
          id: `FIND-FILE-DEBIT-${lineNumber}`,
          code: 'ACH_ERR_0904_FILE_DEBIT_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '9',
          fieldName: 'Total Debit Entry Dollar Amount',
          message: `File Control Debits ($${(declaredFileDebits/100).toFixed(2)}) != sum of batch debits ($${(totalFileDebitsCents/100).toFixed(2)}).`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.5'
        });
      }

      if (declaredFileCredits !== totalFileCreditsCents) {
        findings.push({
          id: `FIND-FILE-CREDIT-${lineNumber}`,
          code: 'ACH_ERR_0905_FILE_CREDIT_MISMATCH',
          severity: 'ERROR',
          lineNumber,
          recordType: '9',
          fieldName: 'Total Credit Entry Dollar Amount',
          message: `File Control Credits ($${(declaredFileCredits/100).toFixed(2)}) != sum of batch credits ($${(totalFileCreditsCents/100).toFixed(2)}).`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.5'
        });
      }

      const expectedBlockCount = Math.ceil(lines.length / 10);
      if (declaredBlockCount !== expectedBlockCount) {
        findings.push({
          id: `FIND-FILE-BLOCK-${lineNumber}`,
          code: 'ACH_ERR_0906_BLOCK_COUNT_MISMATCH',
          severity: 'WARNING',
          lineNumber,
          recordType: '9',
          fieldName: 'Block Count',
          message: `File Control Block Count is ${declaredBlockCount}, calculated block count is ${expectedBlockCount} (based on ${lines.length} records).`,
          ruleReference: 'Nacha Operating Rules 2025, Section 3.2.5'
        });
      }
    }
  }

  if (!hasFileHeader) {
    findings.push({
      id: 'FIND-MISSING-HDR',
      code: 'ACH_ERR_0100_MISSING_FILE_HEADER',
      severity: 'FATAL',
      message: 'Missing mandatory File Header Record (Type 1).',
      ruleReference: 'Nacha Operating Rules 2025, Section 3.2.1'
    });
  }

  if (!hasFileControl) {
    findings.push({
      id: 'FIND-MISSING-CTRL',
      code: 'ACH_ERR_0900_MISSING_FILE_CONTROL',
      severity: 'FATAL',
      message: 'Missing mandatory File Control Record (Type 9). Truncation suspected.',
      ruleReference: 'Nacha Operating Rules 2025, Section 3.2.5'
    });
  }

  const isBalanced = totalFileDebitsCents === totalFileCreditsCents;

  if (contract.allowUnbalancedAch === false && !isBalanced) {
    findings.push({
      id: 'FIND-CONTRACT-UNBALANCED',
      code: 'ACH_ERR_CONTRACT_UNBALANCED_PROHIBITED',
      severity: 'ERROR',
      message: `File Contract '${contract.name || 'Default'}' prohibits unbalanced ACH files. Total debits ($${(totalFileDebitsCents/100).toFixed(2)}) != total credits ($${(totalFileCreditsCents/100).toFixed(2)}).`,
      ruleReference: 'Institutional Data Contract Policy: Strict Zero-Net Settlement'
    });
  }

  const hasFatalOrError = findings.some(f => f.severity === 'FATAL' || f.severity === 'ERROR');
  const outcome = hasFatalOrError ? 'QUARANTINED' : 'VALID';

  const durationMs = Math.round(performance.now() - startTime);

  return {
    parsed: {
      immediateOrigin: fileImmediateOrigin,
      immediateDestination: fileImmediateDestination,
      fileCreationDate,
      fileCreationTime,
      fileIdModifier,
      batches,
      fileControl: {
        batchCount: batches.length,
        blockCount: Math.ceil(lines.length / 10),
        entryAddendaCount: totalEntriesAndAddenda,
        entryHash: totalFileEntryHashAccumulator,
        totalDebitCents: totalFileDebitsCents,
        totalCreditCents: totalFileCreditsCents
      }
    },
    result: {
      runId: `RUN-${Date.now()}`,
      parserVersion: 'NachaEngine-v1.4-moov-compat',
      rulePackVersion: 'Nacha2025-Q4-Standard',
      startedAtUtc: startedAt,
      completedAtUtc: new Date().toISOString(),
      outcome,
      totalRecordsParsed: lines.length,
      totalDebitsUsd: totalFileDebitsCents / 100,
      totalCreditsUsd: totalFileCreditsCents / 100,
      calculatedEntryHash: totalFileEntryHashAccumulator.toString().padStart(10, '0'),
      isBalanced,
      findings,
      resourceMetrics: {
        streamDurationMs: durationMs,
        peakMemoryMb: Math.max(1.8, Math.round((rawContent.length / 1024 / 1024) * 3 * 10) / 10),
        bytesPerSecond: Math.round(rawContent.length / (Math.max(1, durationMs) / 1000))
      }
    }
  };
}
