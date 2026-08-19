package main

import (
	"os"
	"strings"
	"testing"
)

func TestReleaseGate_AuditMatrixCompleteness(t *testing.T) {
	matrixData, err := os.ReadFile("../docs/engineering/RELEASE_READINESS_MATRIX.md")
	if err != nil {
		t.Fatalf("failed to read RELEASE_READINESS_MATRIX.md: %v", err)
	}
	content := string(matrixData)

	requiredDimensions := []string{
		"Clean Build & Dependency Locks",
		"Migrations & Restart Persistence",
		"Auth, AuthZ, Tenant Isolation & CSRF",
		"Secret Handling & Egress Restrictions",
		"Upload/Object Immutability & Redaction",
		"NACHA Fail-Closed Validation Fixtures",
		"State-Machine & Dual-Control Rules",
		"Job Idempotency, Concurrency & Retries",
		"Audit-Chain Concurrency & Verification",
		"Scheduling, Timezone & Calendar Cases",
		"UI Degraded, Permission & Stale States",
		"Telemetry Correctness & Benchmarks",
		"Failure-Recovery Scenarios & Runbooks",
		"AI Read-Only Boundary & Evals",
		"Connector SSRF, SQL & Conformance",
		"Dependency, License, SBOM & Provenance",
		"Backup/Restore & Retention Behavior",
		"Threat-Model Residual-Risk Review",
		"README & Demo Claim Traceability",
	}

	for _, dim := range requiredDimensions {
		if !strings.Contains(content, dim) {
			t.Errorf("release readiness matrix is missing audit dimension: %s", dim)
		}
	}
}

func TestReleaseGate_ResidualRisksDocumented(t *testing.T) {
	matrixData, err := os.ReadFile("../docs/engineering/RELEASE_READINESS_MATRIX.md")
	if err != nil {
		t.Fatalf("failed to read RELEASE_READINESS_MATRIX.md: %v", err)
	}
	content := string(matrixData)

	if !strings.Contains(content, "Top 5 Residual Risks") {
		t.Error("release readiness matrix is missing Top 5 Residual Risks section")
	}

	if !strings.Contains(content, "Final Release Candidate Qualification Sign-Off") {
		t.Error("release readiness matrix is missing qualification sign-off section")
	}
}
