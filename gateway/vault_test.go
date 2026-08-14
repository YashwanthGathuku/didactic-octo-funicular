package main

import (
	"strings"
	"testing"
)

func TestTokenizationAndMasking(t *testing.T) {
	// Vault fails closed without key material; provision test keys.
	t.Setenv("SENTINEL_VAULT_HMAC_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("SENTINEL_VAULT_AES_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

	// 1. Test ABA Routing Number Tokenization
	routingRecord, _ := TokenizeField("TENANT-MERIDIAN-PROD", "ROUTING_NUMBER", "021000021")
	if routingRecord.MaskedValue != "0210****1" {
		t.Errorf("Expected masked routing 0210****1, got %s", routingRecord.MaskedValue)
	}
	if !strings.HasPrefix(routingRecord.TokenKey, "TOK-ROU-") {
		t.Errorf("Expected token key prefix TOK-ROU-, got %s", routingRecord.TokenKey)
	}

	// 2. Test Account Number Tokenization
	acctRecord, _ := TokenizeField("TENANT-MERIDIAN-PROD", "ACCOUNT_NUMBER", "12345678901842")
	if !strings.HasSuffix(acctRecord.MaskedValue, "1842") || !strings.HasPrefix(acctRecord.MaskedValue, "*") {
		t.Errorf("Expected format-preserving account mask, got %s", acctRecord.MaskedValue)
	}

	// 3. Test Individual Name Redaction
	nameRecord, _ := TokenizeField("TENANT-MERIDIAN-PROD", "INDIVIDUAL_NAME", "Johnathan Alexander Doe")
	if !strings.Contains(nameRecord.MaskedValue, "J***") {
		t.Errorf("Expected initial character preserved with asterisk mask, got %s", nameRecord.MaskedValue)
	}
}

func TestInstantPaymentFedNowValidation(t *testing.T) {
	sampleFedNowXml := `<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08">
  <FIToFICstmrCdtTrf>
    <GrpHdr>
      <MsgId>FEDNOW-2026-MSG-001</MsgId>
      <CreDtTm>2026-08-14T10:00:00Z</CreDtTm>
      <NbOfTxs>1</NbOfTxs>
      <SttlmInf><SttlmMtd>CLRG</SttlmMtd></SttlmInf>
    </GrpHdr>
    <CdtTrfTxInf>
      <PmtId><EndToEndId>E2E-FEDNOW-8891</EndToEndId></PmtId>
      <IntrBkSttlmAmt Ccy="USD">150000.00</IntrBkSttlmAmt>
    </CdtTrfTxInf>
  </FIToFICstmrCdtTrf>
</Document>`

	tx, _ := ValidateInstantPaymentXml(sampleFedNowXml)

	if tx.Network != NetFedNow {
		t.Errorf("Expected FEDNOW network, got %s", tx.Network)
	}

	if tx.ValidationLatencyMs > tx.SlaThresholdMs {
		t.Errorf("Validation latency %.2f ms exceeded SLA threshold %.2f ms", tx.ValidationLatencyMs, tx.SlaThresholdMs)
	}
}

func TestDisasterRecoveryFailoverSimulation(t *testing.T) {
	result := SimulateCrossRegionFailover()

	if !result.IsScriptedDemo || result.Disclaimer == "" {
		t.Errorf("Failover result must be explicitly marked as a scripted demo, got %+v", result)
	}
	if result.RpoSecondsTarget != 0.00 {
		t.Errorf("Expected RPO TARGET = 0.00, got %.2f", result.RpoSecondsTarget)
	}

	if result.DataLossTransactionCount != 0 {
		t.Errorf("Expected 0 lost transactions in simulated failover, got %d", result.DataLossTransactionCount)
	}

	if result.StandbyHealthStatus != "NOT_PROVISIONED" {
		t.Errorf("Expected standby status NOT_PROVISIONED (no replica exists), got %s", result.StandbyHealthStatus)
	}
}
