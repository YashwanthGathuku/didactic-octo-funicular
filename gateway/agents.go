package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/objectstore"
)

// AgentRecord represents a registered specialist agent.
type AgentRecord struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenantId"`
	AgentType   string    `json:"agentType"`
	DisplayName string    `json:"displayName"`
	Description string    `json:"description"`
	ModelID     string    `json:"modelId"`
	Version     string    `json:"version"`
	Status      string    `json:"status"` // ACTIVE, DEPRECATED, DISABLED
	ToolScopes  []string  `json:"toolScopes"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	ConfigHash  string    `json:"configHash"`
}

// AgentInvocationRecord tracks an execution of an agent against an incident or workflow.
type AgentInvocationRecord struct {
	ID                string     `json:"id"`
	AgentID           string     `json:"agentId"`
	IncidentID        *int64     `json:"incidentId,omitempty"`
	TenantID          string     `json:"tenantId"`
	StartedAt         time.Time  `json:"startedAt"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
	Status            string     `json:"status"` // RUNNING, COMPLETED, FAILED, TIMEOUT
	InputHash         string     `json:"inputHash"`
	OutputHash        *string    `json:"outputHash,omitempty"`
	TokenCount        int        `json:"tokenCount"`
	LatencyMs         int        `json:"latencyMs"`
	ModelArmorVerdict string     `json:"modelArmorVerdict,omitempty"` // ALLOWED, BLOCKED, FLAGGED
}

func registerAgentRoutes(r chi.Router, db *sql.DB, store objectstore.ObjectStore) {
	// P17 managed cloud ingress deliberately lives under the existing /api/v1
	// router so it shares request-size, recovery, correlation and transport
	// middleware. Browser/API bearer authentication lets ONLY this exact route
	// through when an IAP assertion is present; registerManagedAgentToolRoute
	// then cryptographically verifies that assertion before decoding tool input.
	// If the operator explicitly enables the route but its security configuration
	// is incomplete, startup fails closed rather than exposing a weaker endpoint.
	if err := registerManagedAgentToolRoute(r, db, store); err != nil {
		panic(fmt.Sprintf("managed agent tool ingress configuration: %v", err))
	}

	r.Route("/agents", func(r chi.Router) {
		// GET /api/v1/agents — List all active and registered specialist agents
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}

			rows, err := db.Query(`
				SELECT id, tenant_id, agent_type, display_name, description,
				       model_id, version, status, tool_scopes, created_at, updated_at, config_hash
				FROM agent_registry
				WHERE tenant_id = ? OR tenant_id = 'system'
				ORDER BY agent_type ASC, version DESC
			`, scope.TenantID())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			agents := make([]AgentRecord, 0)
			for rows.Next() {
				var (
					a          AgentRecord
					toolsJSON  string
					createdStr string
					updatedStr string
				)
				if err := rows.Scan(&a.ID, &a.TenantID, &a.AgentType, &a.DisplayName,
					&a.Description, &a.ModelID, &a.Version, &a.Status, &toolsJSON,
					&createdStr, &updatedStr, &a.ConfigHash); err == nil {
					_ = json.Unmarshal([]byte(toolsJSON), &a.ToolScopes)
					if a.ToolScopes == nil {
						a.ToolScopes = []string{}
					}
					a.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
					a.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
					agents = append(agents, a)
				}
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"agents": agents,
				"count":  len(agents),
			})
		})

		// GET /api/v1/agents/{id}/invocations — Paged execution history of an agent
		r.Get("/{id}/invocations", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}
			agentID := chi.URLParam(r, "id")

			rows, err := db.Query(`
				SELECT id, agent_id, incident_id, tenant_id, started_at, completed_at,
				       status, input_hash, output_hash, token_count, latency_ms,
				       COALESCE(model_armor_verdict, 'ALLOWED')
				FROM agent_invocations
				WHERE agent_id = ? AND tenant_id = ?
				ORDER BY started_at DESC
				LIMIT 50
			`, agentID, scope.TenantID())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			invocations := make([]AgentInvocationRecord, 0)
			for rows.Next() {
				var (
					inv        AgentInvocationRecord
					startedStr string
					compStr    sql.NullString
				)
				if err := rows.Scan(&inv.ID, &inv.AgentID, &inv.IncidentID, &inv.TenantID,
					&startedStr, &compStr, &inv.Status, &inv.InputHash, &inv.OutputHash,
					&inv.TokenCount, &inv.LatencyMs, &inv.ModelArmorVerdict); err == nil {
					inv.StartedAt, _ = time.Parse(time.RFC3339, startedStr)
					if compStr.Valid {
						t, _ := time.Parse(time.RFC3339, compStr.String)
						inv.CompletedAt = &t
					}
					invocations = append(invocations, inv)
				}
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"agentId":     agentID,
				"invocations": invocations,
			})
		})

		// POST /api/v1/agents — Register or update an agent lifecycle status
		r.Post("/", func(w http.ResponseWriter, r *http.Request) {
			scope, serr := resolveScope(r, auth.PermManageContract) // Admin scope
			if serr != nil {
				serr.write(w)
				return
			}

			var req struct {
				ID          string   `json:"id"`
				AgentType   string   `json:"agentType"`
				DisplayName string   `json:"displayName"`
				Description string   `json:"description"`
				ModelID     string   `json:"modelId"`
				Version     string   `json:"version"`
				ToolScopes  []string `json:"toolScopes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
				return
			}

			if req.ID == "" || req.AgentType == "" || req.Version == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id, agentType, and version required"})
				return
			}

			toolsJSON, _ := json.Marshal(req.ToolScopes)
			configRaw := fmt.Sprintf("%s:%s:%s:%s", req.AgentType, req.ModelID, req.Version, string(toolsJSON))
			h := sha256.Sum256([]byte(configRaw))
			configHash := hex.EncodeToString(h[:])

			_, err := db.Exec(`
				INSERT INTO agent_registry (id, tenant_id, agent_type, display_name, description, model_id, version, status, tool_scopes, config_hash)
				VALUES (?, ?, ?, ?, ?, ?, ?, 'ACTIVE', ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					display_name = excluded.display_name,
					description = excluded.description,
					model_id = excluded.model_id,
					version = excluded.version,
					tool_scopes = excluded.tool_scopes,
					config_hash = excluded.config_hash,
					updated_at = CURRENT_TIMESTAMP
			`, req.ID, scope.TenantID(), req.AgentType, req.DisplayName, req.Description, req.ModelID, req.Version, string(toolsJSON), configHash)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"status":     "REGISTERED",
				"agentId":    req.ID,
				"configHash": configHash,
			})
		})
	})
}
