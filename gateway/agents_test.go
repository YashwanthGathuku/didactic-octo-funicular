package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
	"sentinel-gateway/internal/auth"
)

func TestAgentRegistry(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, err = db.Exec(`
		INSERT OR IGNORE INTO tenants (id, name) VALUES ('TENANT-DEFAULT', 'Default Tenant');
	`)
	if err != nil {
		t.Fatalf("seed tenants: %v", err)
	}

	cfg := &Config{
		Profile:       ProfileLocalDemo,
		AllowedOrigin: "http://localhost:3000",
	}

	verifier := &auth.Verifier{}
	router := NewRouterWithStore(db, cfg, verifier, nil)

	t.Run("Register New Agent With Tool Scopes", func(t *testing.T) {
		reqBody := map[string]any{
			"id":          "compliance-agent-v1",
			"agentType":   "COMPLIANCE",
			"displayName": "NACHA Compliance Specialist",
			"description": "Expert in NACHA operating rules and ACH formatting",
			"modelId":     "gemini-2.5-flash",
			"version":     "1.0.0",
			"toolScopes":  []string{"lookup_finding", "lookup_nacha_rule"},
		}
		raw, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewReader(raw))
		req.Header.Set("X-Sentinel-Tenant", "TENANT-DEFAULT")
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var res map[string]any
		_ = json.NewDecoder(rec.Body).Decode(&res)
		if res["status"] != "REGISTERED" || res["agentId"] != "compliance-agent-v1" {
			t.Errorf("unexpected register response: %v", res)
		}
		if res["configHash"] == "" {
			t.Errorf("expected non-empty configHash for drift detection")
		}
	})

	t.Run("List Registered Agents For Tenant", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents", nil)
		req.Header.Set("X-Sentinel-Tenant", "TENANT-DEFAULT")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var res struct {
			Agents []AgentRecord `json:"agents"`
			Count  int           `json:"count"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("decode agents: %v", err)
		}

		if res.Count != 1 {
			t.Errorf("expected 1 agent, got %d", res.Count)
		}
		if res.Agents[0].ID != "compliance-agent-v1" || res.Agents[0].ModelID != "gemini-2.5-flash" {
			t.Errorf("unexpected agent data: %+v", res.Agents[0])
		}
	})

	t.Run("Query Agent Invocations", func(t *testing.T) {
		// Insert an invocation directly
		_, err = db.Exec(`
			INSERT INTO agent_invocations (id, agent_id, tenant_id, started_at, status, input_hash, token_count, latency_ms, model_armor_verdict)
			VALUES ('inv-001', 'compliance-agent-v1', 'TENANT-DEFAULT', CURRENT_TIMESTAMP, 'COMPLETED', 'hash123', 450, 850, 'ALLOWED');
		`)
		if err != nil {
			t.Fatalf("insert invocation: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/v1/agents/compliance-agent-v1/invocations", nil)
		req.Header.Set("X-Sentinel-Tenant", "TENANT-DEFAULT")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var res struct {
			AgentID     string                  `json:"agentId"`
			Invocations []AgentInvocationRecord `json:"invocations"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("decode invocations: %v", err)
		}

		if len(res.Invocations) != 1 {
			t.Errorf("expected 1 invocation, got %d", len(res.Invocations))
		}
		if res.Invocations[0].ModelArmorVerdict != "ALLOWED" {
			t.Errorf("expected modelArmorVerdict ALLOWED, got %s", res.Invocations[0].ModelArmorVerdict)
		}
	})
}
