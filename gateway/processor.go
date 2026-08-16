package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"time"

	"sentinel-gateway/internal/domain"
	"sentinel-gateway/internal/nacha"
)

// ValidationFindingRecord is the persisted form of a nacha.Finding.
//
// RawData is gone. It held the complete 94-character record and was returned by
// GET /api/v1/incidents, which put account numbers, routing numbers and amounts
// into every response and log line that touched an incident. Evidence replaces
// it and is redacted at the point it is produced.
type ValidationFindingRecord struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	RuleVersion string `json:"ruleVersion"`
	Provenance  string `json:"provenance"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	LineNumber  int    `json:"lineNumber"`
	ByteOffset  int64  `json:"byteOffset"`
	FieldStart  int    `json:"fieldStart,omitempty"`
	FieldEnd    int    `json:"fieldEnd,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	Expected    string `json:"expected,omitempty"`
	Actual      string `json:"actual,omitempty"`
}

type IngestionResult struct {
	FileID    int64  `json:"fileId"`
	Filename  string `json:"filename"`
	Hash      string `json:"hash"`
	SizeBytes int64  `json:"sizeBytes"`
	// Terminus of ingestion: VALIDATED or QUARANTINED. Never RELEASED --
	// release requires a versioned policy decision and, where policy requires
	// it, approval by an authenticated person.
	Status             string `json:"status"`
	TotalRecordsParsed int    `json:"totalRecordsParsed"`
	TotalEntriesParsed int    `json:"totalEntriesParsed"`

	// Money is in integer minor units. The float64 fields these replace
	// computed totals as cents/100.0 and compared debits to credits for
	// equality, which is a correctness defect: a file with enough entries can
	// report itself unbalanced when it balances.
	TotalDebitsMinor  int64 `json:"totalDebitsMinor"`
	TotalCreditsMinor int64 `json:"totalCreditsMinor"`

	// IsBalanced is deliberately absent. Whether a file must balance is a term
	// of the feed contract, not a property of the file: a credit-only payroll
	// file never balances and is entirely correct. The old field reported
	// debits == credits from every validation and treated it as a correctness
	// signal, which was wrong in both directions.

	// PolicyVersion and ContractID identify what produced the status. A
	// decision without them is an opinion.
	PolicyVersion string `json:"policyVersion"`
	ContractID    string `json:"contractId,omitempty"`

	// NotCheckedRuleIDs lists rules the validator could not evaluate for lack
	// of an authoritative source, so silence does not imply coverage.
	NotCheckedRuleIDs []string `json:"notCheckedRuleIds,omitempty"`

	// QuarantineReasons states why, in the operator's terms.
	QuarantineReasons []string `json:"quarantineReasons,omitempty"`

	Findings   []ValidationFindingRecord `json:"findings"`
	IncidentID *int64                    `json:"incidentId,omitempty"`
}

// ValidateRoutingMod10 verifies Federal Reserve Modulo 10 check digit on ABA routing numbers.
func ValidateRoutingMod10(routing string) bool {
	if len(routing) != 9 {
		return false
	}
	weights := []int{3, 7, 1, 3, 7, 1, 3, 7}
	sum := 0
	for i := 0; i < 8; i++ {
		d, err := strconv.Atoi(string(routing[i]))
		if err != nil {
			return false
		}
		sum += d * weights[i]
	}
	checkDigit, err := strconv.Atoi(string(routing[8]))
	if err != nil {
		return false
	}
	expected := (10 - (sum % 10)) % 10
	return checkDigit == expected
}

// hasBlockingFinding reports whether any finding disqualifies the artifact from
// release. FATAL, CRITICAL and ERROR block; WARNING and INFO are advisory.
//
// Severity strings are still free-form here. Prompt 07 replaces them with a
// typed severity and a versioned release policy.
func hasBlockingFinding(findings []ValidationFindingRecord) bool {
	for _, f := range findings {
		switch f.Severity {
		case "FATAL", "CRITICAL", "ERROR":
			return true
		}
	}
	return false
}

// ProcessFileBytes processes raw file contents, calculates SHA256, validates NACHA, and persists to DB.
// ProcessFileBytes ingests content on behalf of a specific tenant.
//
// The tenant is a parameter rather than a package constant: every row this
// writes is scoped to it, so a caller cannot write into a tenant it does not
// belong to even if a handler forgets to authorize.
func ProcessFileBytes(db *sql.DB, tenantID string, filename string, content []byte) (*IngestionResult, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("ingestion requires a tenant scope")
	}

	// Every financial input begins untrusted and unreleased.
	result := &IngestionResult{
		Filename:  filename,
		Status:    "RECEIVED",
		Findings:  make([]ValidationFindingRecord, 0),
		SizeBytes: int64(len(content)),
	}

	// 1. Content hash, computed over exactly the bytes received.
	hasher := sha256.New()
	hasher.Write(content)
	result.Hash = hex.EncodeToString(hasher.Sum(nil))

	// 2. Validation.
	//
	// The whole determination now lives in internal/nacha: a streaming parse, a
	// versioned rule set that declares what it cannot check, and a versioned
	// policy that maps findings to an outcome. This function no longer decides
	// anything about the file; it records what the validator and the policy
	// decided.
	//
	// The moov-io/ach round trip this replaces produced a single WARNING for a
	// parse failure and left the status untouched, which is how an empty file
	// reached RELEASED.
	validation, err := nacha.Validate(bytes.NewReader(content))
	if err != nil {
		// A read failure is not a verdict about the file. It fails closed, and
		// it is reported as what it is.
		return nil, fmt.Errorf("validation could not read the artifact: %w", err)
	}

	// The feed contract is not yet resolved per counterparty -- that is Prompt
	// 10 -- so the default applies and the decision records that no contract
	// was applied by leaving ContractID empty.
	decision := nacha.Decide(validation, nacha.DefaultContract)

	result.TotalRecordsParsed = validation.RecordsParsed
	result.TotalEntriesParsed = validation.EntriesParsed
	result.TotalDebitsMinor = validation.TotalDebitsMinor
	result.TotalCreditsMinor = validation.TotalCreditsMinor
	result.PolicyVersion = decision.PolicyVersion
	result.ContractID = decision.ContractID
	result.NotCheckedRuleIDs = decision.NotCheckedRuleIDs
	result.QuarantineReasons = decision.Reasons

	for _, f := range validation.Findings {
		result.Findings = append(result.Findings, ValidationFindingRecord{
			Code:        f.RuleID,
			RuleVersion: f.RuleVersion,
			Provenance:  string(f.Provenance),
			Description: f.Description,
			Severity:    string(f.Severity),
			LineNumber:  f.RecordNumber,
			ByteOffset:  f.ByteOffset,
			FieldStart:  f.FieldStart,
			FieldEnd:    f.FieldEnd,
			// Redacted at the point it is produced, not at the point it is
			// displayed. A redaction applied on the way out is one someone can
			// forget to apply.
			Evidence: f.Evidence,
			Expected: f.Expected,
			Actual:   f.Actual,
		})
	}

	// 3. State transition, driven by the policy decision.
	artifact := &domain.Artifact{
		TenantID: domain.TenantID(tenantID),
		State:    domain.ArtifactReceived,
		SHA256:   result.Hash,
	}
	if err := artifact.TransitionTo(domain.ArtifactValidating, time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("artifact state machine: %w", err)
	}
	if decision.Quarantined() {
		if err := artifact.TransitionTo(domain.ArtifactQuarantined, time.Now().UTC()); err != nil {
			return nil, fmt.Errorf("artifact state machine: %w", err)
		}
	} else {
		if err := artifact.TransitionTo(domain.ArtifactValidated, time.Now().UTC()); err != nil {
			return nil, fmt.Errorf("artifact state machine: %w", err)
		}
	}
	result.Status = string(artifact.State)

	// 3. Persist to Database
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`
		INSERT INTO file_instances (tenant_id, expectation_id, filename, storage_path, size_bytes, sha256_hash, status, received_at, updated_at)
		VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?)
	`, tenantID, filename, "/s3/incoming/"+filename, len(content), result.Hash, result.Status, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to save file instance: %w", err)
	}

	fileID, _ := res.LastInsertId()
	result.FileID = fileID

	// Insert findings
	for i := range result.Findings {
		f := &result.Findings[i]
		fRes, _ := db.Exec(`
			INSERT INTO validation_findings
				(tenant_id, file_instance_id, code, rule_version, provenance, description,
				 severity, line_number, byte_offset, field_start, field_end,
				 evidence_redacted, expected_value, actual_value, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, tenantID, fileID, f.Code, f.RuleVersion, f.Provenance, f.Description,
			f.Severity, f.LineNumber, f.ByteOffset, f.FieldStart, f.FieldEnd,
			f.Evidence, f.Expected, f.Actual, now)
		fID, _ := fRes.LastInsertId()
		f.ID = fID
	}

	// If quarantined, create incident
	if result.Status == "QUARANTINED" {
		incRes, err := db.Exec(`
			INSERT INTO incidents (tenant_id, expectation_id, file_instance_id, type, severity, status, created_at, updated_at)
			VALUES (?, NULL, ?, 'ARTIFACT_QUARANTINED', 'CRITICAL', 'OPEN', ?, ?)
		`, tenantID, fileID, now, now)
		if err == nil {
			incID, _ := incRes.LastInsertId()
			result.IncidentID = &incID
		}
	}

	// 4. Append to Audit Ledger
	_, _ = AppendAuditEvent(db, tenantID, "FILE_INGESTED", "SENTINEL_GATEWAY_WORKER", map[string]interface{}{
		"fileId":            fileID,
		"filename":          filename,
		"sha256":            result.Hash,
		"sizeBytes":         len(content),
		"status":            result.Status,
		"findingsCount":     len(result.Findings),
		"totalDebitsMinor":  result.TotalDebitsMinor,
		"totalCreditsMinor": result.TotalCreditsMinor,
		"policyVersion":     result.PolicyVersion,
	})

	return result, nil
}

// ProcessFile streams the uploaded file, computes its SHA-256 hash, and parses NACHA.
func ProcessFile(db *sql.DB, tenantID string, file multipart.File, header *multipart.FileHeader) (*IngestionResult, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload: %w", err)
	}
	return ProcessFileBytes(db, tenantID, header.Filename, content)
}
