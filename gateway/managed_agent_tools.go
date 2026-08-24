package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/toolgateway"
)

// managedAgentToolRequest is the only managed-agent business ingress exposed by
// P17. The workload is authenticated by signed IAP assertion before this body is
// decoded. Tenant authority is NOT accepted from this body or from a model
// header; it is derived from the durable workflow row.
type managedAgentToolRequest struct {
	AgentName      string          `json:"agent_name"`
	ToolName       string          `json:"tool_name"`
	ToolVersion    string          `json:"tool_version,omitempty"`
	ToolArgs       json.RawMessage `json:"tool_args"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

type managedAgentDataProvider struct {
	db *sql.DB
}

func (p *managedAgentDataProvider) GetIncident(ctx context.Context, tenantID, incidentID string) (map[string]interface{}, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(incidentID), 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid incident_id")
	}
	var status, created string
	var expectationID int64
	var artifactID sql.NullInt64
	err = p.db.QueryRowContext(ctx, `
		SELECT status, created_at, expectation_id, file_instance_id
		FROM incidents
		WHERE tenant_id = ? AND id = ?
	`, tenantID, id).Scan(&status, &created, &expectationID, &artifactID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("incident not found")
	}
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{
		"incident_id":         incidentID,
		"tenant_id":           tenantID,
		"status":              status,
		"expectation_id":      expectationID,
		"data_classification": string(toolgateway.ClassificationMetadataOnly),
		"created_at":          created,
	}
	if artifactID.Valid {
		out["artifact_id"] = strconv.FormatInt(artifactID.Int64, 10)
	}
	return out, nil
}

func (p *managedAgentDataProvider) ListRedactedFindings(ctx context.Context, tenantID, artifactID string) ([]map[string]interface{}, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(artifactID), 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid artifact_id")
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT code, severity, description, line_number, evidence_redacted,
		       expected_value, actual_value, rule_version
		FROM validation_findings
		WHERE tenant_id = ? AND file_instance_id = ?
		ORDER BY id ASC
	`, tenantID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0)
	for rows.Next() {
		var code, severity, description string
		var line sql.NullInt64
		var evidence, expected, actual, ruleVersion sql.NullString
		if err := rows.Scan(&code, &severity, &description, &line, &evidence, &expected, &actual, &ruleVersion); err != nil {
			return nil, err
		}
		item := map[string]interface{}{
			"finding_code":        code,
			"severity":            severity,
			"message":             description,
			"tenant_id":           tenantID,
			"artifact_id":         artifactID,
			"data_classification": string(toolgateway.ClassificationRedactedFindings),
		}
		if line.Valid {
			item["line_number"] = line.Int64
		}
		if evidence.Valid {
			item["evidence_redacted"] = evidence.String
		}
		if expected.Valid {
			item["expected_value"] = expected.String
		}
		if actual.Valid {
			item["actual_value"] = actual.String
		}
		if ruleVersion.Valid {
			item["rule_version"] = ruleVersion.String
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (p *managedAgentDataProvider) GetArtifactMetadata(ctx context.Context, tenantID, artifactID string) (map[string]interface{}, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(artifactID), 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid artifact_id")
	}
	var sha, status, created string
	var filename sql.NullString
	var size sql.NullInt64
	// Existing schemas use file_instances as the durable artifact metadata row.
	err = p.db.QueryRowContext(ctx, `
		SELECT sha256_hash, status, created_at, filename, byte_count
		FROM file_instances
		WHERE tenant_id = ? AND id = ?
	`, tenantID, id).Scan(&sha, &status, &created, &filename, &size)
	if err != nil {
		// Older migrations may not expose byte_count. Fall back to the stable
		// fields rather than widening the managed-agent SQL surface.
		err = p.db.QueryRowContext(ctx, `
			SELECT sha256_hash, status, created_at, filename
			FROM file_instances
			WHERE tenant_id = ? AND id = ?
		`, tenantID, id).Scan(&sha, &status, &created, &filename)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("artifact not found")
	}
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{
		"artifact_id":         artifactID,
		"tenant_id":           tenantID,
		"state":               status,
		"artifact_sha256":     sha,
		"data_classification": string(toolgateway.ClassificationMetadataOnly),
		"created_at":          created,
	}
	if filename.Valid {
		out["filename"] = filename.String
	}
	if size.Valid {
		out["byte_count"] = size.Int64
	}
	return out, nil
}

func (p *managedAgentDataProvider) GetWorkflow(ctx context.Context, tenantID, workflowID string) (map[string]interface{}, error) {
	var state, updated string
	var rowVersion, attempts int
	err := p.db.QueryRowContext(ctx, `
		SELECT state, row_version, updated_at,
		       COALESCE((SELECT MAX(attempt_number) FROM artifact_derivations d
		                 WHERE d.tenant_id = w.tenant_id AND d.workflow_id = w.id), 0)
		FROM agent_workflows w
		WHERE tenant_id = ? AND id = ?
	`, tenantID, workflowID).Scan(&state, &rowVersion, &updated, &attempts)
	if err != nil {
		// The workflow read must remain available even if a pre-P07 schema does
		// not have artifact_derivations yet.
		err = p.db.QueryRowContext(ctx, `
			SELECT state, row_version, updated_at
			FROM agent_workflows
			WHERE tenant_id = ? AND id = ?
		`, tenantID, workflowID).Scan(&state, &rowVersion, &updated)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("workflow not found")
	}
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"workflow_id":         workflowID,
		"tenant_id":           tenantID,
		"state":               state,
		"row_version":         rowVersion,
		"attempt_count":       attempts,
		"data_classification": string(toolgateway.ClassificationMetadataOnly),
		"updated_at":          updated,
	}, nil
}

func buildManagedReadToolGateway(db *sql.DB) (*toolgateway.ToolGatewayService, error) {
	reg := toolgateway.NewRegistry()
	if err := toolgateway.RegisterDefaultTools(reg, &managedAgentDataProvider{db: db}); err != nil {
		return nil, fmt.Errorf("register managed tools: %w", err)
	}
	engine := policy.NewEngineWithDefaults()
	if engine == nil {
		return nil, errors.New("default deterministic policy engine unavailable")
	}
	return toolgateway.NewToolGatewayService(reg, engine, toolgateway.NewToolStore(db)), nil
}

func capabilitiesForManagedAgent(identity *auth.RegisteredAgentIdentity) []toolgateway.ToolCapability {
	if identity == nil {
		return nil
	}
	seen := map[toolgateway.ToolCapability]bool{}
	out := make([]toolgateway.ToolCapability, 0, 5)
	for _, toolID := range identity.AllowedCapabilities {
		var cap toolgateway.ToolCapability
		switch toolID {
		case toolgateway.ToolIncidentGet:
			cap = toolgateway.CapIncidentRead
		case toolgateway.ToolValidationFindingsList:
			cap = toolgateway.CapFindingsReadRedacted
		case toolgateway.ToolArtifactMetadataGet:
			cap = toolgateway.CapArtifactMetadataRead
		case toolgateway.ToolWorkflowGet:
			cap = toolgateway.CapWorkflowRead
		case toolgateway.ToolRemediationCandidateCreate:
			cap = toolgateway.CapCandidateCreate
		default:
			continue
		}
		if !seen[cap] {
			seen[cap] = true
			out = append(out, cap)
		}
	}
	return out
}

// deriveManagedWorkflowContext makes the durable workflow row authoritative for
// tenant/artifact context. X-Sentinel-Tenant is checked only for mismatch
// detection; it can never create or switch the tenant scope.
func deriveManagedWorkflowContext(ctx context.Context, db *sql.DB, workflowID string) (tenantID, artifactID, artifactSHA, workflowState string, rowVersion int, startedAt time.Time, err error) {
	if strings.TrimSpace(workflowID) == "" {
		err = errors.New("X-Workflow-ID is required")
		return
	}
	var artifact int64
	var started sql.NullTime
	err = db.QueryRowContext(ctx, `
		SELECT tenant_id, artifact_id, artifact_sha256, state, row_version, started_at
		FROM agent_workflows
		WHERE id = ?
	`, workflowID).Scan(&tenantID, &artifact, &artifactSHA, &workflowState, &rowVersion, &started)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = errors.New("workflow not found")
		}
		return
	}
	artifactID = strconv.FormatInt(artifact, 10)
	if started.Valid {
		startedAt = started.Time.UTC()
	}
	return
}

func managedToolErrorStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, toolgateway.ErrUnauthorizedCapability),
		errors.Is(err, toolgateway.ErrToolNotInAllowlist),
		errors.Is(err, toolgateway.ErrPolicyDenial),
		errors.Is(err, toolgateway.ErrRequireHumanReview),
		errors.Is(err, toolgateway.ErrAgentExecutionDisabled),
		errors.Is(err, toolgateway.ErrAgentBudgetExhausted),
		errors.Is(err, toolgateway.ErrAgentDeadlineExceeded):
		return http.StatusForbidden
	case errors.Is(err, toolgateway.ErrIdempotencyConflict):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

func registerManagedAgentToolRoute(r chi.Router, db *sql.DB) error {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("SENTINEL_MANAGED_AGENT_INGRESS")), "true") &&
		strings.TrimSpace(os.Getenv("SENTINEL_MANAGED_AGENT_INGRESS")) != "1" {
		return nil
	}

	projectID := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT"))
	audience := strings.TrimSpace(os.Getenv("SENTINEL_IAP_EXPECTED_AUDIENCE"))
	runtimeSubject := strings.TrimSpace(os.Getenv("SENTINEL_AGENT_RUNTIME_SUBJECT"))
	if projectID == "" || audience == "" || runtimeSubject == "" {
		return errors.New("managed agent ingress requires GOOGLE_CLOUD_PROJECT, SENTINEL_IAP_EXPECTED_AUDIENCE, and SENTINEL_AGENT_RUNTIME_SUBJECT")
	}
	if !strings.HasPrefix(runtimeSubject, "principal://agents.") && !strings.HasPrefix(runtimeSubject, "agents.") {
		return errors.New("SENTINEL_AGENT_RUNTIME_SUBJECT must be the observed Agent Runtime identity")
	}
	// IAP JWT subject is the bare agents.* subject; IAM bindings use principal://.
	jwtSubject := strings.TrimPrefix(runtimeSubject, "principal://")

	gateway, err := buildManagedReadToolGateway(db)
	if err != nil {
		return err
	}
	identityValidator := auth.NewAgentIdentityValidator(projectID)
	iapVerifier := auth.NewIAPJWTVerifier(audience)

	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		identity, ok := auth.AgentIdentityFromContext(req.Context())
		if !ok || identity == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "managed_identity_missing"})
			return
		}

		var body managedAgentToolRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 256*1024))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_tool_request"})
			return
		}
		if body.AgentName != "" && body.AgentName != identity.AgentName {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent_metadata_mismatch"})
			return
		}
		if body.ToolName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool_name_required"})
			return
		}
		if err := identityValidator.AuthorizeCapability(identity, body.ToolName); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent_capability_denied"})
			return
		}

		workflowID := strings.TrimSpace(req.Header.Get("X-Workflow-ID"))
		tenantID, artifactID, artifactSHA, workflowState, rowVersion, startedAt, err := deriveManagedWorkflowContext(req.Context(), db, workflowID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_workflow_context"})
			return
		}
		if suppliedTenant := strings.TrimSpace(req.Header.Get("X-Sentinel-Tenant")); suppliedTenant != "" && suppliedTenant != tenantID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "tenant_context_mismatch"})
			return
		}

		idem := strings.TrimSpace(body.IdempotencyKey)
		if idem == "" {
			idem = strings.TrimSpace(req.Header.Get("Idempotency-Key"))
		}
		if idem == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "idempotency_key_required"})
			return
		}

		requestID := telemetryRequestID(req)
		now := time.Now().UTC()
		execCtx := &toolgateway.TrustedExecutionContext{
			RequestID:              requestID,
			IdempotencyKey:         idem,
			CorrelationID:          requestID,
			TenantID:               tenantID,
			CallerType:             toolgateway.CallerTypeAgent,
			CallerID:               identity.AgentName,
			CallerCapabilities:     capabilitiesForManagedAgent(identity),
			CallerAutonomyLevel:    map[auth.AutonomyLevel]int{auth.AutonomyA1: 1, auth.AutonomyA2: 2}[identity.AutonomyLevel],
			WorkflowID:             workflowID,
			WorkflowState:          workflowState,
			ArtifactID:             artifactID,
			ArtifactSHA256:         artifactSHA,
			ResourceVersion:        rowVersion,
			AllowedTools:           append([]string(nil), identity.AllowedCapabilities...),
			ExecutionMode:          "LIVE",
			Timestamp:              now,
			WorkflowStartedAt:      startedAt,
			MaxWorkflowDurationSec: 120,
		}

		// The route currently exposes only read-only managed proof tools. Even
		// though RemediationAgent's roster permits candidate creation elsewhere,
		// the managed cloud endpoint cannot create candidates until the production
		// candidate service is deliberately wired here in a later controlled step.
		if body.ToolName == toolgateway.ToolRemediationCandidateCreate {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "managed_candidate_creation_not_enabled"})
			return
		}

		toolReq := &toolgateway.ToolRequest{
			ToolID:         body.ToolName,
			ToolVersion:    body.ToolVersion,
			Args:           body.ToolArgs,
			IdempotencyKey: idem,
			ResourcePreconditions: &toolgateway.ResourcePreconditions{
				ExpectedArtifactSHA256: artifactSHA,
				ExpectedRowVersion:     rowVersion,
				ExpectedWorkflowState:  workflowState,
			},
		}
		resp, execErr := gateway.Execute(req.Context(), execCtx, toolReq, nil)
		if execErr != nil {
			writeJSON(w, managedToolErrorStatus(execErr), map[string]interface{}{
				"error":       "tool_execution_denied",
				"error_type":  fmt.Sprintf("%T", execErr),
				"workflow_id": workflowID,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":               resp.Status,
			"tool_id":              resp.ToolID,
			"tool_version":         resp.ToolVersion,
			"invocation_id":        resp.InvocationID,
			"output":               json.RawMessage(resp.Output),
			"manifest_hash":        resp.ManifestHash,
			"policy_decision_hash": resp.PolicyDecisionHash,
			"policy_bundle_hash":   resp.PolicyBundleHash,
			"output_hash":          resp.OutputHash,
			"workflow_id":          workflowID,
			"tenant_scope_source":  "GO_WORKFLOW_REPOSITORY",
		})
	})

	managed := identityValidator.ManagedAgentIdentityMiddleware(iapVerifier, jwtSubject, handler)
	r.With(func(next http.Handler) http.Handler { return managed }).Post("/internal/agent-tools", func(w http.ResponseWriter, req *http.Request) {
		// ManagedAgentIdentityMiddleware terminates in handler and intentionally
		// ignores the placeholder next handler. This closure is unreachable.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "managed_route_misconfigured"})
	})
	return nil
}

// telemetryRequestID avoids importing the telemetry package into the managed
// route only to read context. The correlation middleware always sets the header
// on responses, but request headers may not carry it, so prefer an explicit
// request ID header and otherwise derive a non-secret time-based identifier.
func telemetryRequestID(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get("X-Request-ID")); id != "" && len(id) <= 128 {
		return id
	}
	return fmt.Sprintf("managed-%d", time.Now().UTC().UnixNano())
}
