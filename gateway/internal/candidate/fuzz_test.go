package candidate

import (
	"encoding/json"
	"strings"
	"testing"
)

func FuzzRemediationPlanDecoding(f *testing.F) {
	f.Add([]byte(`{"operation_type": "RECOMPUTE_BATCH_CONTROL_TOTAL", "target_ref": "BATCH-1"}`))
	f.Add([]byte(`{"operation_type": "UNKNOWN_OP", "target_ref": "BATCH-999"}`))
	f.Add([]byte(`{malformed_json...`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var op RemediationOperation
		_ = json.Unmarshal(data, &op)

		var req CandidateCreationRequest
		_ = json.Unmarshal(data, &req)
	})
}

func FuzzSemanticTargetResolution(f *testing.F) {
	f.Add("BATCH-1")
	f.Add("BATCH-999")
	f.Add("FILE_CONTROL")
	f.Add("BATCH-0")
	f.Add("BATCH--1")
	f.Add("MALFORMED")
	f.Add("")

	f.Fuzz(func(t *testing.T, targetRef string) {
		svc := &Service{}
		nacha := "101 121000358 0210000212603011200A094101" + strings.Repeat(" ", 54) + "\n" +
			"5200" + strings.Repeat(" ", 70) + "021000020000001\n" +
			"8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000\n" +
			"9000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000\n"

		ops := []RemediationOperation{
			{OperationType: OpRecomputeBatchControlTotal, TargetRef: targetRef},
		}

		_, _, _, _ = svc.applyDeterministicOperations([]byte(nacha), ops)
	})
}

func FuzzOperationValidation(f *testing.F) {
	f.Add("RECOMPUTE_BATCH_CONTROL_TOTAL", "BATCH-1")
	f.Add("RECOMPUTE_FILE_CONTROL_TOTAL", "FILE_CONTROL")
	f.Add("DELETE_BATCH", "BATCH-1")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, opType, targetRef string) {
		svc := &Service{}
		nacha := "101 121000358 0210000212603011200A094101" + strings.Repeat(" ", 54) + "\n" +
			"5200" + strings.Repeat(" ", 70) + "021000020000001\n" +
			"8200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000\n" +
			"9000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000\n"

		ops := []RemediationOperation{
			{OperationType: opType, TargetRef: targetRef},
		}

		_, _, _, _ = svc.applyDeterministicOperations([]byte(nacha), ops)
	})
}
