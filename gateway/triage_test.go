package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"sentinel-gateway/internal/auth"
)

func TestTriageTenantIsolation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert DefaultTenantID and secondary tenant data
	_, err = db.Exec(`
		INSERT OR IGNORE INTO tenants (id, name) VALUES ('TENANT-DEFAULT', 'Tenant Alpha'), ('tenant-b', 'Tenant Bravo');
		INSERT INTO partners (tenant_id, name, routing_number) VALUES ('TENANT-DEFAULT', 'Partner A', '011000015'), ('tenant-b', 'Partner B', '011000028');
		INSERT INTO file_contracts (tenant_id, partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
		VALUES ('TENANT-DEFAULT', 1, 'Contract A', 'INBOUND', 'A_*.ach', '14:00', 30, 'UTC'),
		       ('tenant-b', 2, 'Contract B', 'INBOUND', 'B_*.ach', '14:00', 30, 'UTC');
		INSERT INTO file_instances (id, tenant_id, filename, storage_path, sha256_hash, size_bytes, status, received_at)
		VALUES (101, 'TENANT-DEFAULT', 'A_test.ach', 's3://bucket/A_test.ach', '1111111111111111111111111111111111111111111111111111111111111111', 100, 'QUARANTINED', CURRENT_TIMESTAMP),
		       (202, 'tenant-b', 'B_test.ach', 's3://bucket/B_test.ach', '2222222222222222222222222222222222222222222222222222222222222222', 100, 'QUARANTINED', CURRENT_TIMESTAMP);
		INSERT INTO validation_runs (id, tenant_id, file_instance_id, parser_name, parser_version, rule_pack_version, parser_ok, records_parsed, started_at)
		VALUES (501, 'TENANT-DEFAULT', 101, 'nacha', '1.0', '1.0', 0, 10, CURRENT_TIMESTAMP),
		       (502, 'tenant-b', 202, 'nacha', '1.0', '1.0', 0, 10, CURRENT_TIMESTAMP);
		INSERT INTO policy_decisions (tenant_id, file_instance_id, validation_run_id, policy_version, state, outcome, artifact_sha256)
		VALUES ('TENANT-DEFAULT', 101, 501, 'policy-v1', 'APPROVED', 'QUARANTINED', '1111111111111111111111111111111111111111111111111111111111111111'),
		       ('tenant-b', 202, 502, 'policy-v2', 'APPROVED', 'QUARANTINED', '2222222222222222222222222222222222222222222222222222222222222222');
		INSERT INTO incidents (id, tenant_id, file_instance_id, type, severity, status)
		VALUES (1001, 'TENANT-DEFAULT', 101, 'VALIDATION_FAILED', 'CRITICAL', 'OPEN'),
		       (2002, 'tenant-b', 202, 'VALIDATION_FAILED', 'CRITICAL', 'OPEN');
		INSERT INTO validation_findings (id, tenant_id, file_instance_id, code, rule_version, description, severity, evidence_redacted)
		VALUES (1, 'TENANT-DEFAULT', 101, '0802', 'v1', 'Hash mismatch for Tenant A', 'BLOCKING', 'ACC-XXXX-1234'),
		       (2, 'tenant-b', 202, '0602', 'v1', 'Routing error for Tenant B', 'BLOCKING', 'ROUTING-INVALID');
	`)
	if err != nil {
		t.Fatalf("seed test data: %v", err)
	}

	cfg := &Config{
		Profile:       ProfileLocalDemo,
		AllowedOrigin: "http://localhost:3000",
		AITierURL:     "", // Unset -> should return 503 NOT_CONFIGURED when accessible
	}

	verifier := &auth.Verifier{}
	router := NewRouterWithStore(db, cfg, verifier, nil)

	t.Run("Cross-Tenant Triage Returns 404", func(t *testing.T) {
		// Principal in TENANT-DEFAULT requests triage on tenant-b's incident (2002)
		req := httptest.NewRequest("POST", "/api/v1/incidents/2002/triage", nil)
		req.Header.Set("X-Sentinel-Tenant", "TENANT-DEFAULT")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 NotFound for cross-tenant triage attempt, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Same-Tenant Triage Reaches AI Boundary", func(t *testing.T) {
		// Principal in TENANT-DEFAULT requests triage on own incident (1001)
		req := httptest.NewRequest("POST", "/api/v1/incidents/1001/triage", nil)
		req.Header.Set("X-Sentinel-Tenant", "TENANT-DEFAULT")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// Since AI_TIER_URL is unset, it should safely return 503 NOT_CONFIGURED, proving it passed tenant check!
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503 ServiceUnavailable (AI not configured), got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["status"] != "NOT_CONFIGURED" {
			t.Errorf("expected status NOT_CONFIGURED, got %v", body["status"])
		}
	})
}

func TestAgentContextEnvelopeInvariantsAndDelivery(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO partners (tenant_id, name, routing_number) VALUES ('TENANT-DEFAULT', 'Partner Alpha', '011000015');
		INSERT INTO file_contracts (tenant_id, partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
		VALUES ('TENANT-DEFAULT', 1, 'Contract Alpha', 'INBOUND', 'ALPHA_*.ach', '14:00', 30, 'UTC');
		INSERT INTO file_instances (id, tenant_id, filename, storage_path, sha256_hash, size_bytes, status, received_at)
		VALUES (777, 'TENANT-DEFAULT', 'ALPHA_batch.ach', 's3://bucket/ALPHA_batch.ach', 'abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890', 4096, 'QUARANTINED', CURRENT_TIMESTAMP);
		INSERT INTO validation_runs (id, tenant_id, file_instance_id, parser_name, parser_version, rule_pack_version, parser_ok, records_parsed, started_at)
		VALUES (888, 'TENANT-DEFAULT', 777, 'nacha', '1.0', '1.0', 0, 20, CURRENT_TIMESTAMP);
		INSERT INTO policy_decisions (tenant_id, file_instance_id, validation_run_id, policy_version, state, outcome, artifact_sha256)
		VALUES ('TENANT-DEFAULT', 777, 888, 'nacha-v2.1', 'APPROVED', 'QUARANTINED', 'abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890');
		INSERT INTO incidents (id, tenant_id, file_instance_id, type, severity, status)
		VALUES (999, 'TENANT-DEFAULT', 777, 'VALIDATION_FAILED', 'CRITICAL', 'OPEN');
		INSERT INTO validation_findings (id, tenant_id, file_instance_id, code, rule_version, description, severity, evidence_redacted)
		VALUES (101, 'TENANT-DEFAULT', 777, '0802', 'v1', 'Batch entry hash mismatch', 'BLOCKING', 'ACC-XXXX-9999');
	`)
	if err != nil {
		t.Fatalf("seed data: %v", err)
	}

	var capturedEnv AgentContextEnvelope
	var receivedHeaders http.Header

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&capturedEnv); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AnalystResponse{
			IncidentID: capturedEnv.IncidentID,
			TenantID:   capturedEnv.TenantID,
			FileID:     capturedEnv.ArtifactID,
			Summary:    "Analyzed with zero raw data",
			Statement:  "The AI incident analyst operates in a read-only capacity and has made no system state changes.",
		})
	}))
	defer mockServer.Close()

	cfg := &Config{
		Profile:       ProfileLocalDemo,
		AllowedOrigin: "http://localhost:3000",
		AITierURL:     mockServer.URL,
	}

	verifier := &auth.Verifier{}
	router := NewRouterWithStore(db, cfg, verifier, nil)

	req := httptest.NewRequest("POST", "/api/v1/incidents/999/triage", nil)
	req.Header.Set("X-Sentinel-Tenant", "TENANT-DEFAULT")
	req.Header.Set("X-Correlation-ID", "corr-test-12345")
	req.Header.Set("X-Trace-ID", "trace-test-67890")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify header propagation
	if receivedHeaders.Get("X-Correlation-ID") != "corr-test-12345" {
		t.Errorf("expected X-Correlation-ID header, got %q", receivedHeaders.Get("X-Correlation-ID"))
	}
	if receivedHeaders.Get("X-Sentinel-Tenant") != "TENANT-DEFAULT" {
		t.Errorf("expected X-Sentinel-Tenant header, got %q", receivedHeaders.Get("X-Sentinel-Tenant"))
	}

	// Verify Envelope Invariants
	if capturedEnv.TenantID != "TENANT-DEFAULT" {
		t.Errorf("expected TenantID 'TENANT-DEFAULT', got %q", capturedEnv.TenantID)
	}
	if capturedEnv.IncidentID != 999 {
		t.Errorf("expected IncidentID 999, got %d", capturedEnv.IncidentID)
	}
	if capturedEnv.ArtifactID != 777 {
		t.Errorf("expected ArtifactID 777 (distinct from incident), got %d", capturedEnv.ArtifactID)
	}
	if capturedEnv.ArtifactSHA256 != "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Errorf("expected ArtifactSHA256 sha, got %q", capturedEnv.ArtifactSHA256)
	}
	if capturedEnv.ValidationRunID != "888" {
		t.Errorf("expected ValidationRunID '888', got %q", capturedEnv.ValidationRunID)
	}
	if capturedEnv.PolicyVersion != "nacha-v2.1" {
		t.Errorf("expected PolicyVersion 'nacha-v2.1', got %q", capturedEnv.PolicyVersion)
	}
	if len(capturedEnv.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(capturedEnv.Findings))
	}
	if capturedEnv.Findings[0].ID != "FINDING-101" {
		t.Errorf("expected Finding ID 'FINDING-101', got %q", capturedEnv.Findings[0].ID)
	}
}

func TestAIClientBoundedPolicyAndTimeout(t *testing.T) {
	t.Run("Timeout Enforced on Slow AI Tier", func(t *testing.T) {
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond) // exceeds short timeout
			w.WriteHeader(http.StatusOK)
		}))
		defer slowServer.Close()

		client := NewAIClient(AIClientConfig{
			BaseURL:          slowServer.URL,
			Timeout:          50 * time.Millisecond,
			MaxRequestBytes:  1024,
			MaxResponseBytes: 1024,
			MaxRetries:       0,
		})

		env := &AgentContextEnvelope{
			SchemaVersion:  "1.0",
			TenantID:       "TENANT-1",
			IncidentID:     1,
			ArtifactID:     2,
			ArtifactSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
			CorrelationID:  "corr-1",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := client.TriageIncident(ctx, env)
		if err == nil {
			t.Fatal("expected timeout error from bounded AI client, got nil")
		}
	})

	t.Run("Fails Closed When AI Tier Returns 500", func(t *testing.T) {
		failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "internal error"}`))
		}))
		defer failingServer.Close()

		client := NewAIClient(AIClientConfig{
			BaseURL:          failingServer.URL,
			Timeout:          1 * time.Second,
			MaxRequestBytes:  1024,
			MaxResponseBytes: 1024,
			MaxRetries:       1,
		})

		env := &AgentContextEnvelope{
			SchemaVersion:  "1.0",
			TenantID:       "TENANT-1",
			IncidentID:     1,
			ArtifactID:     2,
			ArtifactSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
			CorrelationID:  "corr-1",
		}

		_, err := client.TriageIncident(context.Background(), env)
		if err == nil {
			t.Fatal("expected error on 500 status from AI tier, got nil")
		}
	})

	t.Run("Response Size Limit Enforced", func(t *testing.T) {
		hugeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Return 2000 bytes when ceiling is 500
			_, _ = w.Write(make([]byte, 2000))
		}))
		defer hugeServer.Close()

		client := NewAIClient(AIClientConfig{
			BaseURL:          hugeServer.URL,
			Timeout:          1 * time.Second,
			MaxRequestBytes:  1024,
			MaxResponseBytes: 500,
			MaxRetries:       0,
		})

		env := &AgentContextEnvelope{
			SchemaVersion:  "1.0",
			TenantID:       "TENANT-1",
			IncidentID:     1,
			ArtifactID:     2,
			ArtifactSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
			CorrelationID:  "corr-1",
		}

		_, err := client.TriageIncident(context.Background(), env)
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("expected ErrResponseTooLarge, got %v", err)
		}
	})

	t.Run("Propagates Idempotency Header And Handles 403", func(t *testing.T) {
		var receivedIdempotencyKey string
		authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedIdempotencyKey = r.Header.Get("X-Idempotency-Key")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"detail":"Tenant mismatch"}`))
		}))
		defer authServer.Close()

		client := NewAIClient(DefaultAIClientConfig(authServer.URL))
		env := &AgentContextEnvelope{
			SchemaVersion:  "1.0",
			TenantID:       "TENANT-1",
			IncidentID:     1,
			ArtifactID:     2,
			ArtifactSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
			CorrelationID:  "corr-idempotent-999",
		}

		_, err := client.TriageIncident(context.Background(), env)
		if !errors.Is(err, ErrAITierBadRequest) {
			t.Fatalf("expected ErrAITierBadRequest on 403, got %v", err)
		}
		if receivedIdempotencyKey != "corr-idempotent-999" {
			t.Errorf("expected X-Idempotency-Key 'corr-idempotent-999', got %q", receivedIdempotencyKey)
		}
	})
}
