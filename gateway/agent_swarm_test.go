package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMultiAgentSwarmDeliberation(t *testing.T) {
	r := chi.NewRouter()
	RegisterSwarmRoutes(r, nil)

	payload := `{"incidentId": 101, "fileId": 501, "findings": ["INVALID_MOD10_ROUTING"], "rawData": "6220210000218420000245000999888800John Doe"}`
	req := httptest.NewRequest("POST", "/swarm/deliberate", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200 from /swarm/deliberate, got %d", rr.Code)
	}

	var session SwarmSession
	if err := json.Unmarshal(rr.Body.Bytes(), &session); err != nil {
		t.Fatalf("Failed to parse swarm session: %v", err)
	}

	if session.Status != "CONSENSUS_REACHED" {
		t.Errorf("Expected status CONSENSUS_REACHED, got %s", session.Status)
	}

	if len(session.Messages) < 5 {
		t.Errorf("Expected at least 5 inter-agent messages, got %d", len(session.Messages))
	}

	if session.ConfidenceScore < 0.90 {
		t.Errorf("Expected confidence >= 0.90, got %.2f", session.ConfidenceScore)
	}

	// Verify all 4 agent roles participated
	rolesFound := make(map[AgentRole]bool)
	for _, m := range session.Messages {
		rolesFound[m.AgentRole] = true
	}

	expectedRoles := []AgentRole{RoleLeadSupervisor, RoleFormatValidator, RoleLineageRecon, RoleAuditCompliance}
	for _, er := range expectedRoles {
		if !rolesFound[er] {
			t.Errorf("Agent role %s did not participate in swarm deliberation", er)
		}
	}
}
