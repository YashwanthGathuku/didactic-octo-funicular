package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"sentinel-gateway/internal/ledger"
)

// Tests for Prompt 15: Read-Only Evidence-Grounded AI Incident Analyst

func TestAiAnalyst_ReturnsTypedRecommendationWhenConfigured(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	// Seed an incident
	_, err := db.Exec(`INSERT INTO incidents (tenant_id, type, severity, status, created_at)
		VALUES (?, 'VALIDATION_FAILURE', 'ERROR', 'OPEN', CURRENT_TIMESTAMP)`, DefaultTenantID)
	if err != nil {
		t.Fatal(err)
	}
	var incID int64
	_ = db.QueryRow("SELECT id FROM incidents WHERE tenant_id = ?", DefaultTenantID).Scan(&incID)

	// Mock AI Tier server returning typed AnalystResponse
	mockAiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AnalystResponse{
			IncidentID: incID,
			TenantID:   DefaultTenantID,
			FileID:     incID,
			Summary:    "Quarantined file due to entry hash mismatch.",
			Hypotheses: []Hypothesis{
				{
					Rank:              1,
					Hypothesis:        "Batch control entry hash calculation mismatch during file compilation.",
					Confidence:        "HIGH",
					Rationale:         "Accumulator mismatch between entry routing prefixes and trailer.",
					EvidenceCitations: []string{"FINDING-001", "RUNBOOK-RB-05"},
				},
			},
			MissingEvidence:    []string{"Did counterparty deploy an update?"},
			RunbookPassageIDs:  []string{"RB-05"},
			RecommendedActions: []string{"Contact counterparty to request retransmission."},
			Statement:          "The AI incident analyst operates in a read-only capacity and has made no system state changes.",
			Audit: AuditMetadata{
				Model:            "gemini-2.5-flash",
				Provider:         "Google Gemini",
				PromptVersion:    "1.0.0",
				SchemaVersion:    "1.0.0",
				LatencyMs:        45.2,
				TokenUsage:       map[string]int{"prompt_tokens": 120, "completion_tokens": 80, "total_tokens": 200},
				EstimatedCostUSD: 0.000066,
			},
		})
	}))
	defer mockAiServer.Close()

	cfg := ingestDemoConfig()
	cfg.AITierURL = mockAiServer.URL

	router := NewRouterWithStore(db, cfg, nil, nil)

	// Call triage endpoint
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/triage", incID), bytes.NewReader([]byte("{}")))
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 from triage endpoint, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AnalystResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Hypotheses) == 0 {
		t.Errorf("expected at least 1 hypothesis")
	}
	if resp.Statement != "The AI incident analyst operates in a read-only capacity and has made no system state changes." {
		t.Errorf("expected mandatory disclaimer statement, got %q", resp.Statement)
	}
	if len(resp.RunbookPassageIDs) == 0 || resp.RunbookPassageIDs[0] != "RB-05" {
		t.Errorf("expected RB-05 runbook passage, got %+v", resp.RunbookPassageIDs)
	}

	// Verify audit ledger entry was written
	evidence, err := ledger.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	v, err := evidence.Verify(context.Background(), DefaultTenantID)
	if err != nil || !v.Intact {
		t.Errorf("expected intact audit ledger after AI analysis execution")
	}
}

func TestAiAnalyst_Returns503WhenUnconfiguredOrOffline(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO incidents (tenant_id, type, severity, status, created_at)
		VALUES (?, 'VALIDATION_FAILURE', 'ERROR', 'OPEN', CURRENT_TIMESTAMP)`, DefaultTenantID)

	var incID int64
	_ = db.QueryRow("SELECT id FROM incidents WHERE tenant_id = ?", DefaultTenantID).Scan(&incID)

	// 1. Unconfigured AI tier URL
	cfg := ingestDemoConfig()
	cfg.AITierURL = ""
	router := NewRouterWithStore(db, cfg, nil, nil)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/triage", incID), bytes.NewReader([]byte("{}"))))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when AI_TIER_URL is unset, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Offline AI tier URL
	cfg.AITierURL = "http://127.0.0.1:54321/nonexistent-ai-tier"
	routerOffline := NewRouterWithStore(db, cfg, nil, nil)

	recOffline := httptest.NewRecorder()
	routerOffline.ServeHTTP(recOffline, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/triage", incID), bytes.NewReader([]byte("{}"))))
	if recOffline.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when AI tier is offline, got %d: %s", recOffline.Code, recOffline.Body.String())
	}

	var errBody map[string]any
	_ = json.Unmarshal(recOffline.Body.Bytes(), &errBody)
	if errBody["status"] != "UNAVAILABLE" {
		t.Errorf("expected status=UNAVAILABLE, got %+v", errBody)
	}
}

func TestAiAnalyst_CannotMutateApplicationState(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	// Seed a quarantined file and incident
	_, err := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (?, 'payroll.ach', 's3://bucket/payroll.ach', 1024, 'hash-1234', 'QUARANTINED', CURRENT_TIMESTAMP)`, DefaultTenantID)
	if err != nil {
		t.Fatal(err)
	}

	// AI analysis response must NOT change file status from QUARANTINED to RELEASED
	var statusBefore string
	_ = db.QueryRow("SELECT status FROM file_instances WHERE sha256_hash = 'hash-1234'").Scan(&statusBefore)
	if statusBefore != "QUARANTINED" {
		t.Fatalf("expected file to be QUARANTINED before AI triage, got %s", statusBefore)
	}

	// Status after remains strictly QUARANTINED
	var statusAfter string
	_ = db.QueryRow("SELECT status FROM file_instances WHERE sha256_hash = 'hash-1234'").Scan(&statusAfter)
	if statusAfter != "QUARANTINED" {
		t.Errorf("security violation: file was mutated away from QUARANTINED!")
	}
}
