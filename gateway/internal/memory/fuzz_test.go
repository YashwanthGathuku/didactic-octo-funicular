package memory

import (
	"encoding/json"
	"testing"
	"time"
)

func FuzzComputeMemoryHash(f *testing.F) {
	f.Add("mem-001", "tenant-1", "PARTNER-01", "{\"status\":\"ok\"}", "ref-1", "hash-1")
	f.Add("", "", "", "", "", "")
	f.Add("invalid\x00byte", "tenant\twith\nnewlines", "subj", "{\"nested\":[1,2,3]}", "ref-x", "hash-y")

	f.Fuzz(func(t *testing.T, memID, tenantID, subjRef, structVal, srcRef, srcHash string) {
		rec := &OperationalMemoryRecord{
			MemoryID:               memID,
			TenantID:               tenantID,
			MemoryType:             MemoryTypeOperationalFact,
			SubjectType:            SubjectTypePartner,
			SubjectRef:             subjRef,
			FactType:               FactTypeVerifiedRemediationSuccess,
			StructuredValue:        json.RawMessage(structVal),
			SourceRefs:             []string{srcRef},
			SourceHashes:           []string{srcHash},
			SourceVerificationRefs: []string{"verif-001"},
			ConfidenceSource:       ConfidenceSourceVerifiedWorkflow,
			Classification:         ClassificationInternal,
			ValidFrom:              time.Now().UTC(),
			CreatedAt:              time.Now().UTC(),
			CreatedBy:              "fuzzer",
		}

		// ComputeMemoryHash may return an error if structVal is invalid JSON, but MUST NOT panic
		_, _ = ComputeMemoryHash(rec)
	})
}

func FuzzScanForDisallowedContent(f *testing.F) {
	f.Add("{\"status\": \"ok\", \"count\": 10}")
	f.Add("{\"token\": \"sk_live_1234567890abcdef1234\"}")
	f.Add("6221210003581234567890          00000500000918273645JOHN DOE              0121000350000001")
	f.Add("{\"bearer\": \"bearer abcdefghijklmnopqrstuvwxyz1234567890\"}")

	f.Fuzz(func(t *testing.T, rawPayload string) {
		_ = scanForDisallowedContent(json.RawMessage(rawPayload))
	})
}

func FuzzMemoryPayloadDecoding(f *testing.F) {
	f.Add("{\"workflow_id\":\"wf-1\",\"incident_id\":10,\"applied_operations\":[\"OP_1\"]}")
	f.Add("{\"partner_id\":\"P-1\",\"file_standard\":\"NACHA_CCD_2025\",\"max_batch_count\":10}")
	f.Add("{\"invalid\":true}")

	f.Fuzz(func(t *testing.T, jsonPayload string) {
		var remFact RemediationSuccessFact
		_ = json.Unmarshal([]byte(jsonPayload), &remFact)

		var tolFact PartnerFormatToleranceFact
		_ = json.Unmarshal([]byte(jsonPayload), &tolFact)

		var slaFact OperationalSLABreachFact
		_ = json.Unmarshal([]byte(jsonPayload), &slaFact)
	})
}
