package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "modernc.org/sqlite"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/candidate"
	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/repository"
	"sentinel-gateway/internal/verification"
)

type testIAPClaims struct {
	Email string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

func setupTestIAPServer(t *testing.T) (*ecdsa.PrivateKey, string, string, func()) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	kid := "test-iap-kid"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "EC",
				"crv": "P-256",
				"alg": "ES256",
				"use": "sig",
				"kid": kid,
				"x":   base64.RawURLEncoding.EncodeToString(key.X.Bytes()),
				"y":   base64.RawURLEncoding.EncodeToString(key.Y.Bytes()),
			}},
		})
	}))

	aud := "/projects/123456789/global/backendServices/987654321"
	projectID := "test-project-123"
	runtimeSubject := "principal://agents.test-runtime-subject"

	os.Setenv("SENTINEL_MANAGED_AGENT_INGRESS", "true")
	os.Setenv("GOOGLE_CLOUD_PROJECT", projectID)
	os.Setenv("SENTINEL_IAP_EXPECTED_AUDIENCE", aud)
	os.Setenv("SENTINEL_AGENT_RUNTIME_SUBJECT", runtimeSubject)
	os.Setenv("SENTINEL_IAP_JWK_URL", server.URL)

	cleanup := func() {
		server.Close()
		os.Unsetenv("SENTINEL_MANAGED_AGENT_INGRESS")
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("SENTINEL_IAP_EXPECTED_AUDIENCE")
		os.Unsetenv("SENTINEL_AGENT_RUNTIME_SUBJECT")
		os.Unsetenv("SENTINEL_IAP_JWK_URL")
	}

	return key, kid, aud, cleanup
}

func signTestIAPToken(t *testing.T, key *ecdsa.PrivateKey, kid, aud, subject string) string {
	t.Helper()
	claims := testIAPClaims{
		Email: "managed-agent@test.invalid",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.IAPIssuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{aud},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func testPadNum(n int64, width int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) > width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

func testPadText(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func generateManagedTestNACHA(corruptDebits bool) string {
	routingOrigin := "021000021"
	routingRDFI := "121000358"
	routingRDFI2 := "011000015"

	header := "101 " + routingRDFI + " " + routingOrigin + "2603011200A094101" +
		testPadText("DESTINATION BANK", 23) + testPadText("ORIGIN COMPANY", 23) + testPadText("", 8)

	bh := "5200" + testPadText("ORIGIN COMPANY", 16) + testPadText("", 20) +
		testPadText("1234567890", 10) + testPadText("PPD", 3) + testPadText("PAYROLL", 10) +
		"260301260302   1" + routingOrigin[:8] + testPadNum(1, 7)

	ed1 := "622" + routingRDFI + testPadText("1234567890", 17) +
		testPadNum(15000, 10) + testPadText("INDIVID001", 15) +
		testPadText("EMPLOYEE ONE", 22) + "  0" +
		routingOrigin[:8] + testPadNum(1, 7)

	ed2 := "627" + routingRDFI2 + testPadText("0987654321", 17) +
		testPadNum(15000, 10) + testPadText("INDIVID002", 15) +
		testPadText("EMPLOYEE TWO", 22) + "  0" +
		routingOrigin[:8] + testPadNum(2, 7)

	declaredDebits := int64(15000)
	if corruptDebits {
		declaredDebits = int64(0)
	}

	bc := "8200" + testPadNum(2, 6) + testPadNum(13200036, 10) +
		testPadNum(declaredDebits, 12) + testPadNum(15000, 12) +
		testPadText("1234567890", 10) + testPadText("", 19) + testPadText("", 6) +
		routingOrigin[:8] + testPadNum(1, 7)

	declaredFileDebits := int64(15000)
	if corruptDebits {
		declaredFileDebits = int64(0)
	}

	fc := "9" + testPadNum(1, 6) + testPadNum(1, 6) +
		testPadNum(2, 8) + testPadNum(13200036, 10) +
		testPadNum(declaredFileDebits, 12) + testPadNum(15000, 12) + testPadText("", 39)

	records := []string{header, bh, ed1, ed2, bc, fc}
	return strings.Join(records, "\n") + "\n"
}

func setupManagedTestGateway(t *testing.T) (*sql.DB, objectstore.ObjectStore, http.Handler, *ecdsa.PrivateKey, string, string, func()) {
	t.Helper()
	key, kid, aud, cleanup := setupTestIAPServer(t)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	store, err := objectstore.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatalf("create filesystem objectstore: %v", err)
	}

	cfg := &Config{
		Profile:       ProfileProduction,
		AllowedOrigin: "http://localhost:3000",
		ObjectStore:   store,
	}

	router := NewRouterWithStore(db, cfg, nil, store)

	fullCleanup := func() {
		cleanup()
		db.Close()
	}

	return db, store, router, key, kid, aud, fullCleanup
}

func seedTestWorkflow(t *testing.T, db *sql.DB, store objectstore.ObjectStore, tenantID, workflowID string) (int64, int64, string) {
	t.Helper()
	ctx := context.Background()

	_, _ = db.Exec(`INSERT OR IGNORE INTO tenants (id, name) VALUES (?, ?)`, tenantID, tenantID)

	nachaText := generateManagedTestNACHA(true) // corrupt batch debit control
	nachaBytes := []byte(nachaText)
	parentSHABytes := sha256.Sum256(nachaBytes)
	parentSHAHex := hex.EncodeToString(parentSHABytes[:])

	parentKey, _ := objectstore.NewKey(tenantID, time.Now().UTC())
	_, err := store.Put(ctx, parentKey, bytes.NewReader(nachaBytes), int64(len(nachaBytes)))
	if err != nil {
		t.Fatalf("store parent artifact: %v", err)
	}

	res, err := db.Exec(`
		INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (?, 'orig.ach', ?, ?, ?, 'QUARANTINED', CURRENT_TIMESTAMP)
	`, tenantID, parentKey, len(nachaBytes), parentSHAHex)
	if err != nil {
		t.Fatalf("insert file_instance: %v", err)
	}
	parentArtifactID, _ := res.LastInsertId()

	resInc, err := db.Exec(`
		INSERT INTO incidents (tenant_id, file_instance_id, type, severity, status)
		VALUES (?, ?, 'VALIDATION_FAILURE', 'HIGH', 'OPEN')
	`, tenantID, parentArtifactID)
	if err != nil {
		t.Fatalf("insert incident: %v", err)
	}
	incidentID, _ := resInc.LastInsertId()

	_, err = db.Exec(`
		INSERT INTO agent_workflows (
			id, tenant_id, incident_id, artifact_id, artifact_sha256,
			state, agent_name, agent_version, workflow_type,
			correlation_id, row_version, policy_bundle_hash, created_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?,
			'REMEDIATING', 'RemediationAgent', '1.0.0', 'TRIAGE_AND_REMEDIATION',
			'corr-123', 1, 'pol-bundle-hash-1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
	`, workflowID, tenantID, incidentID, parentArtifactID, parentSHAHex)
	if err != nil {
		t.Fatalf("insert agent_workflow: %v", err)
	}

	return parentArtifactID, incidentID, parentSHAHex
}

func buildRemediationArgs(parentArtifactID, incidentID int64, parentSHA string, planHash string) map[string]interface{} {
	return map[string]interface{}{
		"incident_id":            incidentID,
		"parent_artifact_id":     parentArtifactID,
		"expected_parent_sha256": parentSHA,
		"attempt_number":         1,
		"plan_hash":              planHash,
		"confidence":             "HIGH",
		"agent_name":             "RemediationAgent",
		"agent_version":          "1.0.0",
		"operations": []map[string]interface{}{
			{
				"operation_type": candidate.OpRecomputeBatchControlTotal,
				"target_ref":     "batch:1",
				"rationale":      "Recompute batch debit control accumulator",
			},
			{
				"operation_type": candidate.OpRecomputeFileControlTotal,
				"target_ref":     "file:control",
				"rationale":      "Recompute file debit control accumulator",
			},
		},
	}
}

func TestManagedAgentTools_CandidateCreate_SuccessAndIndependentVerification(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupManagedTestGateway(t)
	defer cleanup()

	tenantID := "TENANT-MANAGED-01"
	workflowID := "wf-managed-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	token := signTestIAPToken(t, key, kid, aud, "agents.test-runtime-subject")

	planHash := "plan-hash-valid-001"
	toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, planHash)
	argsBytes, _ := json.Marshal(toolArgs)

	body := map[string]interface{}{
		"agent_name":      "RemediationAgent",
		"tool_name":       "remediation.candidate.create",
		"tool_version":    "1.0.0",
		"tool_args":       json.RawMessage(argsBytes),
		"idempotency_key": "ik-managed-cand-001",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
	req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
	req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
	req.Header.Set("X-Workflow-ID", workflowID)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status            string          `json:"status"`
		ToolID            string          `json:"tool_id"`
		Output            json.RawMessage `json:"output"`
		WorkflowID        string          `json:"workflow_id"`
		TenantScopeSource string          `json:"tenant_scope_source"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Status != "SUCCEEDED" {
		t.Errorf("expected status SUCCEEDED, got %s", resp.Status)
	}
	if resp.ToolID != "remediation.candidate.create" {
		t.Errorf("expected tool_id remediation.candidate.create, got %s", resp.ToolID)
	}
	if resp.TenantScopeSource != "GO_WORKFLOW_REPOSITORY" {
		t.Errorf("expected tenant_scope_source GO_WORKFLOW_REPOSITORY, got %s", resp.TenantScopeSource)
	}

	var candRes candidate.CandidateResult
	if err := json.Unmarshal(resp.Output, &candRes); err != nil {
		t.Fatalf("decode candidate output: %v", err)
	}

	if candRes.ValidationOutcome != "VALIDATION_PASSED" {
		t.Errorf("expected validation outcome VALIDATION_PASSED, got %s", candRes.ValidationOutcome)
	}
	if candRes.BlockingFindingsCount != 0 {
		t.Errorf("expected 0 blocking findings, got %d", candRes.BlockingFindingsCount)
	}
	if candRes.CandidateSHA256 == "" || candRes.CandidateSHA256 == parentSHA {
		t.Errorf("candidate SHA-256 (%s) must be non-empty and differ from parent SHA (%s)", candRes.CandidateSHA256, parentSHA)
	}

	// 1. Fail-closed Quarantine Check: file_instances record has status = 'CANDIDATE'
	var dbStatus, dbDerivedFrom, dbAgentID string
	var dbSize int64
	err := db.QueryRow(`
		SELECT status, derived_from, derivation_agent_id, size_bytes
		FROM file_instances
		WHERE id = ? AND tenant_id = ?
	`, candRes.CandidateArtifactID, tenantID).Scan(&dbStatus, &dbDerivedFrom, &dbAgentID, &dbSize)
	if err != nil {
		t.Fatalf("query candidate file_instance: %v", err)
	}
	if dbStatus != "CANDIDATE" {
		t.Errorf("expected candidate file_instance status 'CANDIDATE', got %q", dbStatus)
	}
	if dbDerivedFrom != fmt.Sprintf("%d", parentArtifactID) {
		t.Errorf("expected derived_from %d, got %s", parentArtifactID, dbDerivedFrom)
	}
	if dbAgentID != "RemediationAgent" {
		t.Errorf("expected derivation_agent_id RemediationAgent, got %s", dbAgentID)
	}

	// 2. Independent Verification Gate: Candidate passes independent verification
	p := &auth.Principal{
		Subject: "verifier-sub",
		Memberships: []auth.Membership{
			{TenantID: tenantID, Roles: []auth.Role{auth.RoleOperator}},
		},
	}
	scope, _ := repository.NewScope(p, tenantID, auth.PermReadTenant)
	engine := policy.NewEngineWithDefaults()
	verifierSvc := verification.NewService(db, store, engine)

	verReq := &verification.VerificationRequest{
		TenantID:                tenantID,
		WorkflowID:              workflowID,
		DerivationID:            candRes.DerivationID,
		CandidateArtifactID:     candRes.CandidateArtifactID,
		ExpectedCandidateSHA256: candRes.CandidateSHA256,
		ExpectedParentSHA256:    parentSHA,
		CallerID:                "VerifierAgent",
	}
	verRes, err := verifierSvc.VerifyCandidate(context.Background(), scope, verReq)
	if err != nil {
		t.Fatalf("independent verification failed: %v", err)
	}
	if verRes.DeterministicOutcome != verification.OutcomePass {
		t.Errorf("expected independent verification OutcomePass, got %s", verRes.DeterministicOutcome)
	}
}

func TestManagedAgentTools_TenantSpoofingPrevention(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupManagedTestGateway(t)
	defer cleanup()

	legitTenant := "TENANT-LEGIT-01"
	attackerTenant := "TENANT-ATTACKER-02"
	workflowID := "wf-managed-spoof-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, legitTenant, workflowID)

	token := signTestIAPToken(t, key, kid, aud, "agents.test-runtime-subject")

	// Case 1: Attacker supplies X-Sentinel-Tenant header mismatch -> 403 Forbidden
	planHash := "plan-hash-spoof-001"
	toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, planHash)
	argsBytes, _ := json.Marshal(toolArgs)

	body := map[string]interface{}{
		"agent_name":      "RemediationAgent",
		"tool_name":       "remediation.candidate.create",
		"tool_args":       json.RawMessage(argsBytes),
		"idempotency_key": "ik-spoof-header-001",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
	req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
	req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
	req.Header.Set("X-Workflow-ID", workflowID)
	req.Header.Set("X-Sentinel-Tenant", attackerTenant) // Header mismatch!

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on tenant header mismatch, got %d. Body: %s", rec.Code, rec.Body.String())
	}
	var errResp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp["error"] != "tenant_context_mismatch" {
		t.Errorf("expected error 'tenant_context_mismatch', got %q", errResp["error"])
	}

	// Case 2: Attacker puts tenant_id in tool_args payload -> overridden by server-side workflow row
	toolArgs["tenant_id"] = attackerTenant
	argsBytes2, _ := json.Marshal(toolArgs)
	body2 := map[string]interface{}{
		"agent_name":      "RemediationAgent",
		"tool_name":       "remediation.candidate.create",
		"tool_args":       json.RawMessage(argsBytes2),
		"idempotency_key": "ik-spoof-payload-002",
	}
	bodyBytes2, _ := json.Marshal(body2)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Goog-IAP-JWT-Assertion", token)
	req2.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
	req2.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
	req2.Header.Set("X-Workflow-ID", workflowID)

	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", rec2.Code, rec2.Body.String())
	}

	// Verify candidate was created under legitTenant, NOT attackerTenant
	var candCountAttacker, candCountLegit int
	_ = db.QueryRow(`SELECT COUNT(*) FROM file_instances WHERE tenant_id = ? AND status = 'CANDIDATE'`, attackerTenant).Scan(&candCountAttacker)
	_ = db.QueryRow(`SELECT COUNT(*) FROM file_instances WHERE tenant_id = ? AND status = 'CANDIDATE'`, legitTenant).Scan(&candCountLegit)

	if candCountAttacker != 0 {
		t.Errorf("expected 0 candidates in attacker tenant, got %d", candCountAttacker)
	}
	if candCountLegit != 1 {
		t.Errorf("expected 1 candidate in legit tenant, got %d", candCountLegit)
	}
}

func TestManagedAgentTools_PreconditionFailures(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupManagedTestGateway(t)
	defer cleanup()

	tenantID := "TENANT-PRECOND-01"
	workflowID := "wf-managed-precond-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	token := signTestIAPToken(t, key, kid, aud, "agents.test-runtime-subject")

	t.Run("Stale SHA Precondition", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-hash-stale-sha")
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":               "RemediationAgent",
			"tool_name":                "remediation.candidate.create",
			"tool_args":                json.RawMessage(argsBytes),
			"idempotency_key":          "ik-precond-stale-sha",
			"expected_artifact_sha256": "0000000000000000000000000000000000000000000000000000000000000000",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusPreconditionFailed {
			t.Fatalf("expected 412 Precondition Failed, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Stale Row Version Precondition", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-hash-stale-version")
		argsBytes, _ := json.Marshal(toolArgs)
		staleVer := 999

		body := map[string]interface{}{
			"agent_name":           "RemediationAgent",
			"tool_name":            "remediation.candidate.create",
			"tool_args":            json.RawMessage(argsBytes),
			"idempotency_key":      "ik-precond-stale-ver",
			"expected_row_version": &staleVer,
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusPreconditionFailed {
			t.Fatalf("expected 412 Precondition Failed, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Wrong Workflow State Precondition", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-hash-wrong-state")
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":              "RemediationAgent",
			"tool_name":               "remediation.candidate.create",
			"tool_args":               json.RawMessage(argsBytes),
			"idempotency_key":         "ik-precond-wrong-state",
			"expected_workflow_state": "COMPLETED",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusPreconditionFailed {
			t.Fatalf("expected 412 Precondition Failed, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Nonexistent Workflow Context", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-hash-nonexistent-wf")
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":      "RemediationAgent",
			"tool_name":       "remediation.candidate.create",
			"tool_args":       json.RawMessage(argsBytes),
			"idempotency_key": "ik-precond-no-wf",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", "wf-does-not-exist")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request on invalid workflow, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestManagedAgentTools_Idempotency(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupManagedTestGateway(t)
	defer cleanup()

	tenantID := "TENANT-IDEM-01"
	workflowID := "wf-managed-idem-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	token := signTestIAPToken(t, key, kid, aud, "agents.test-runtime-subject")

	t.Run("Missing Idempotency Key Rejected", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-hash-no-idem")
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name": "RemediationAgent",
			"tool_name":  "remediation.candidate.create",
			"tool_args":  json.RawMessage(argsBytes),
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request on missing idempotency key, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Identical Replay Returns Cached Candidate", func(t *testing.T) {
		planHash := "plan-hash-idem-001"
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, planHash)
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":      "RemediationAgent",
			"tool_name":       "remediation.candidate.create",
			"tool_args":       json.RawMessage(argsBytes),
			"idempotency_key": "ik-idem-replay-001",
		}
		bodyBytes, _ := json.Marshal(body)

		// 1st invocation
		req1 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req1.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req1.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req1.Header.Set("X-Workflow-ID", workflowID)

		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK {
			t.Fatalf("first call failed: %d: %s", rec1.Code, rec1.Body.String())
		}

		var res1 struct {
			Output json.RawMessage `json:"output"`
		}
		_ = json.NewDecoder(rec1.Body).Decode(&res1)
		var cand1 candidate.CandidateResult
		_ = json.Unmarshal(res1.Output, &cand1)

		// 2nd invocation with exact same idempotency key and payload
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req2.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req2.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req2.Header.Set("X-Workflow-ID", workflowID)

		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Fatalf("second call replay failed: %d: %s", rec2.Code, rec2.Body.String())
		}

		var res2 struct {
			Output json.RawMessage `json:"output"`
		}
		_ = json.NewDecoder(rec2.Body).Decode(&res2)
		var cand2 candidate.CandidateResult
		_ = json.Unmarshal(res2.Output, &cand2)

		if cand1.CandidateSHA256 != cand2.CandidateSHA256 {
			t.Errorf("candidate SHA mismatch on replay: %s vs %s", cand1.CandidateSHA256, cand2.CandidateSHA256)
		}
		if cand1.CandidateArtifactID != cand2.CandidateArtifactID {
			t.Errorf("candidate artifact ID mismatch on replay: %d vs %d", cand1.CandidateArtifactID, cand2.CandidateArtifactID)
		}
	})

	t.Run("Conflict With Same Idempotency Key Returns 409", func(t *testing.T) {
		// Send different payload with the same idempotency key as above
		toolArgsDiff := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-hash-different-002")
		argsBytesDiff, _ := json.Marshal(toolArgsDiff)

		bodyConflict := map[string]interface{}{
			"agent_name":      "RemediationAgent",
			"tool_name":       "remediation.candidate.create",
			"tool_args":       json.RawMessage(argsBytesDiff),
			"idempotency_key": "ik-idem-replay-001", // Same key as above!
		}
		bodyBytesConflict, _ := json.Marshal(bodyConflict)

		reqConflict := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytesConflict))
		reqConflict.Header.Set("Content-Type", "application/json")
		reqConflict.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		reqConflict.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		reqConflict.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		reqConflict.Header.Set("X-Workflow-ID", workflowID)

		recConflict := httptest.NewRecorder()
		router.ServeHTTP(recConflict, reqConflict)

		if recConflict.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d. Body: %s", recConflict.Code, recConflict.Body.String())
		}
	})
}

func TestManagedAgentTools_RBAC(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupManagedTestGateway(t)
	defer cleanup()

	tenantID := "TENANT-RBAC-01"
	workflowID := "wf-managed-rbac-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	token := signTestIAPToken(t, key, kid, aud, "agents.test-runtime-subject")

	nonRemediationAgents := []string{
		"IncidentCommanderAgent",
		"DiagnosisAgent",
		"PolicySLAAgent",
		"MemoryAgent",
		"VerifierAgent",
		"ReturnRiskAgent",
	}

	for _, agentName := range nonRemediationAgents {
		t.Run(fmt.Sprintf("%s Denied Candidate Create", agentName), func(t *testing.T) {
			toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, fmt.Sprintf("plan-hash-rbac-%s", agentName))
			argsBytes, _ := json.Marshal(toolArgs)

			body := map[string]interface{}{
				"agent_name":      agentName,
				"tool_name":       "remediation.candidate.create",
				"tool_args":       json.RawMessage(argsBytes),
				"idempotency_key": fmt.Sprintf("ik-rbac-%s", agentName),
			}
			bodyBytes, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
			req.Header.Set("X-Sentinel-Agent-Name", agentName)
			req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
			req.Header.Set("X-Workflow-ID", workflowID)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 Forbidden for agent %s, got %d. Body: %s", agentName, rec.Code, rec.Body.String())
			}
		})
	}

	t.Run("RemediationAgent Allowed Candidate Create", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-hash-rbac-remed")
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":      "RemediationAgent",
			"tool_name":       "remediation.candidate.create",
			"tool_args":       json.RawMessage(argsBytes),
			"idempotency_key": "ik-rbac-remed-allowed",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for RemediationAgent, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestManagedAgentTools_OriginalImmutability(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupManagedTestGateway(t)
	defer cleanup()

	tenantID := "TENANT-IMMUTABLE-01"
	workflowID := "wf-managed-immut-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	var parentStoragePath string
	_ = db.QueryRow(`SELECT storage_path FROM file_instances WHERE id = ?`, parentArtifactID).Scan(&parentStoragePath)

	// Read parent bytes before candidate creation
	rcBefore, err := store.Get(context.Background(), parentStoragePath)
	if err != nil {
		t.Fatalf("get parent before: %v", err)
	}
	parentBytesBefore, _ := io.ReadAll(rcBefore)
	rcBefore.Close()

	token := signTestIAPToken(t, key, kid, aud, "agents.test-runtime-subject")

	toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-hash-immut-001")
	argsBytes, _ := json.Marshal(toolArgs)

	body := map[string]interface{}{
		"agent_name":      "RemediationAgent",
		"tool_name":       "remediation.candidate.create",
		"tool_args":       json.RawMessage(argsBytes),
		"idempotency_key": "ik-immut-001",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
	req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
	req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
	req.Header.Set("X-Workflow-ID", workflowID)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("candidate creation failed: %d: %s", rec.Code, rec.Body.String())
	}

	// Re-read parent bytes after candidate creation
	rcAfter, err := store.Get(context.Background(), parentStoragePath)
	if err != nil {
		t.Fatalf("get parent after: %v", err)
	}
	parentBytesAfter, _ := io.ReadAll(rcAfter)
	rcAfter.Close()

	if !bytes.Equal(parentBytesBefore, parentBytesAfter) {
		t.Fatalf("CRITICAL INVARIANT VIOLATION: parent artifact bytes in ObjectStore were mutated during candidate generation")
	}

	hAfter := sha256.Sum256(parentBytesAfter)
	if hex.EncodeToString(hAfter[:]) != parentSHA {
		t.Fatalf("CRITICAL INVARIANT VIOLATION: parent artifact SHA-256 changed in ObjectStore")
	}
}

func TestManagedAgentTools_ReadOnlyTools(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupManagedTestGateway(t)
	defer cleanup()

	tenantID := "TENANT-READ-01"
	workflowID := "wf-managed-read-001"
	parentArtifactID, incidentID, _ := seedTestWorkflow(t, db, store, tenantID, workflowID)

	token := signTestIAPToken(t, key, kid, aud, "agents.test-runtime-subject")

	t.Run("incident.get", func(t *testing.T) {
		args, _ := json.Marshal(map[string]interface{}{"incident_id": fmt.Sprintf("%d", incidentID)})
		body, _ := json.Marshal(map[string]interface{}{
			"agent_name":      "IncidentCommanderAgent",
			"tool_name":       "incident.get",
			"tool_args":       json.RawMessage(args),
			"idempotency_key": "ik-read-inc-01",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "IncidentCommanderAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("incident.get failed: %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("artifact.metadata.get", func(t *testing.T) {
		args, _ := json.Marshal(map[string]interface{}{"artifact_id": fmt.Sprintf("%d", parentArtifactID)})
		body, _ := json.Marshal(map[string]interface{}{
			"agent_name":      "IncidentCommanderAgent",
			"tool_name":       "artifact.metadata.get",
			"tool_args":       json.RawMessage(args),
			"idempotency_key": "ik-read-art-01",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "IncidentCommanderAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("artifact.metadata.get failed: %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("workflow.get", func(t *testing.T) {
		args, _ := json.Marshal(map[string]interface{}{"workflow_id": workflowID})
		body, _ := json.Marshal(map[string]interface{}{
			"agent_name":      "IncidentCommanderAgent",
			"tool_name":       "workflow.get",
			"tool_args":       json.RawMessage(args),
			"idempotency_key": "ik-read-wf-01",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "IncidentCommanderAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("workflow.get failed: %d: %s", rec.Code, rec.Body.String())
		}
	})
}
