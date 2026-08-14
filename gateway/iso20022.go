package main

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Minimal ISO 20022 pacs.008 schema models
type Iso20022Document struct {
	XMLName        xml.Name       `xml:"Document"`
	FIToFICstmrCdt *FIToFICstmrCdt `xml:"FIToFICstmrCdtTrf"`
	BkToCstmrStmt  *BkToCstmrStmt  `xml:"BkToCstmrStmt"`
}

type FIToFICstmrCdt struct {
	GrpHdr      GrpHdr       `xml:"GrpHdr"`
	CdtTrfTxInf []CdtTrfTxInf `xml:"CdtTrfTxInf"`
}

type BkToCstmrStmt struct {
	GrpHdr GrpHdr `xml:"GrpHdr"`
	Stmt   []Stmt `xml:"Stmt"`
}

type GrpHdr struct {
	MsgId             string  `xml:"MsgId"`
	CreDtTm           string  `xml:"CreDtTm"`
	NbOfTxs           int     `xml:"NbOfTxs"`
	TtlIntrBkSttlmAmt float64 `xml:"TtlIntrBkSttlmAmt"`
	IntrBkSttlmDt     string  `xml:"IntrBkSttlmDt"`
}

type CdtTrfTxInf struct {
	PmtId      PmtId      `xml:"PmtId"`
	IntrBkSttlmAmt float64 `xml:"IntrBkSttlmAmt"`
	Dbtr       Party      `xml:"Dbtr"`
	Cdtr       Party      `xml:"Cdtr"`
}

type Stmt struct {
	Id      string    `xml:"Id"`
	CreDtTm string    `xml:"CreDtTm"`
	Acct    Account   `xml:"Acct"`
	Bal     []Balance `xml:"Bal"`
}

type PmtId struct {
	EndToEndId string `xml:"EndToEndId"`
	TxId       string `xml:"TxId"`
}

type Party struct {
	Nm string `xml:"Nm"`
}

type Account struct {
	Id AccountId `xml:"Id"`
}

type AccountId struct {
	IBAN string `xml:"IBAN"`
	Othr string `xml:"Othr"`
}

type Balance struct {
	Tp  string  `xml:"Tp>CdOrPrtry>Cd"`
	Amt float64 `xml:"Amt"`
	CdtDbtInd string `xml:"CdtDbtInd"`
}

// ValidateIso20022Xml parses and validates an ISO 20022 XML payload.
func ValidateIso20022Xml(content []byte) ([]ValidationFindingRecord, float64, float64, int, bool) {
	findings := make([]ValidationFindingRecord, 0)
	var totalDebits float64 = 0
	var totalCredits float64 = 0
	var recordCount = 0

	var doc Iso20022Document
	if err := xml.Unmarshal(content, &doc); err != nil {
		findings = append(findings, ValidationFindingRecord{
			Code:          "ISO_ERR_0001_MALFORMED_XML_SCHEMA",
			Description:   fmt.Sprintf("Failed to parse ISO 20022 XML document: %v", err),
			Severity:      "FATAL",
			LineNumber:    1,
			RuleReference: "ISO 20022 Financial Services Schema Standard",
		})
		return findings, 0, 0, 0, false
	}

	// 1. Validate pacs.008 (Credit Transfer)
	if doc.FIToFICstmrCdt != nil {
		hdr := doc.FIToFICstmrCdt.GrpHdr
		if strings.TrimSpace(hdr.MsgId) == "" {
			findings = append(findings, ValidationFindingRecord{
				Code:          "ISO_ERR_0002_MANDATORY_TAG_MISSING",
				Description:   "Mandatory Group Header Message Identification (<MsgId>) is missing or empty.",
				Severity:      "CRITICAL",
				LineNumber:    3,
				RuleReference: "ISO 20022 pacs.008.001.08 Section 1.1: Group Header Specifications",
			})
		}

		if strings.TrimSpace(hdr.IntrBkSttlmDt) == "" {
			findings = append(findings, ValidationFindingRecord{
				Code:          "ISO_ERR_0003_SETTLEMENT_DATE_MISSING",
				Description:   "Mandatory Interbank Settlement Date (<IntrBkSttlmDt>) is missing.",
				Severity:      "ERROR",
				LineNumber:    6,
				RuleReference: "ISO 20022 pacs.008.001.08 Section 1.2",
			})
		}

		recordCount = len(doc.FIToFICstmrCdt.CdtTrfTxInf)
		var sumTxAmt float64 = 0

		for idx, tx := range doc.FIToFICstmrCdt.CdtTrfTxInf {
			if strings.TrimSpace(tx.PmtId.EndToEndId) == "" {
				findings = append(findings, ValidationFindingRecord{
					Code:          "ISO_ERR_0004_END_TO_END_ID_MISSING",
					Description:   fmt.Sprintf("Transaction at index %d is missing mandatory <EndToEndId> tag.", idx+1),
					Severity:      "CRITICAL",
					LineNumber:    12 + (idx * 8),
					RuleReference: "ISO 20022 FedNow / CHIPS Operating Rules 2025",
				})
			}
			sumTxAmt += tx.IntrBkSttlmAmt
			totalCredits += tx.IntrBkSttlmAmt
		}

		// Arithmetic reconciliation
		if hdr.TtlIntrBkSttlmAmt > 0 && hdr.TtlIntrBkSttlmAmt != sumTxAmt {
			findings = append(findings, ValidationFindingRecord{
				Code:          "ISO_ERR_0005_CONTROL_TOTAL_MISMATCH",
				Description:   fmt.Sprintf("Declared <TtlIntrBkSttlmAmt> (%.2f) does not match sum of individual transactions (%.2f).", hdr.TtlIntrBkSttlmAmt, sumTxAmt),
				Severity:      "CRITICAL",
				LineNumber:    5,
				RuleReference: "ISO 20022 Validation Rule VR-088",
			})
		}
	} else if doc.BkToCstmrStmt != nil {
		// 2. Validate camt.053 (Statement)
		recordCount = len(doc.BkToCstmrStmt.Stmt)
		for _, stmt := range doc.BkToCstmrStmt.Stmt {
			for _, bal := range stmt.Bal {
				if bal.CdtDbtInd == "CRDT" {
					totalCredits += bal.Amt
				} else {
					totalDebits += bal.Amt
				}
			}
		}
	} else {
		findings = append(findings, ValidationFindingRecord{
			Code:          "ISO_ERR_0099_UNKNOWN_MESSAGE_ROOT",
			Description:   "XML document is missing valid ISO 20022 root elements (<FIToFICstmrCdtTrf> or <BkToCstmrStmt>).",
			Severity:      "FATAL",
			LineNumber:    1,
			RuleReference: "ISO 20022 Message Definitions 2025",
		})
	}

	isBalanced := (totalDebits == totalCredits)
	return findings, totalDebits, totalCredits, recordCount, isBalanced
}
