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
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "modernc.org/sqlite"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/candidate"
	"sentinel-gateway/internal/objectstore"
)

// setupAdversarialTestEnvironment sets up mock IAP server, SQLite memory DB, object store, and router.
func setupAdversarialTestEnvironment(t *testing.T) (*sql.DB, objectstore.ObjectStore, http.Handler, *ecdsa.PrivateKey, string, string, func()) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	kid := "adv-iap-kid"
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

	aud := "/projects/999888777/global/backendServices/111222333"
	projectID := "adv-project-999"
	runtimeSubject := "principal://agents.adv-runtime-subject"

	os.Setenv("SENTINEL_MANAGED_AGENT_INGRESS", "true")
	os.Setenv("GOOGLE_CLOUD_PROJECT", projectID)
	os.Setenv("SENTINEL_IAP_EXPECTED_AUDIENCE", aud)
	os.Setenv("SENTINEL_AGENT_RUNTIME_SUBJECT", runtimeSubject)
	os.Setenv("SENTINEL_IAP_JWK_URL", server.URL)

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

	cleanup := func() {
		server.Close()
		db.Close()
		os.Unsetenv("SENTINEL_MANAGED_AGENT_INGRESS")
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("SENTINEL_IAP_EXPECTED_AUDIENCE")
		os.Unsetenv("SENTINEL_AGENT_RUNTIME_SUBJECT")
		os.Unsetenv("SENTINEL_IAP_JWK_URL")
	}

	return db, store, router, key, kid, aud, cleanup
}

func signAdvIAPToken(t *testing.T, key *ecdsa.PrivateKey, kid, aud, subject string) string {
	t.Helper()
	claims := testIAPClaims{
		Email: "adversarial-tester@sentinel.bank",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.IAPIssuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{aud},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
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

// -------------------------------------------------------------
// ADVERSARIAL TEST SUITE 1: R2 Governed Remediation Ingress
// -------------------------------------------------------------

func TestAdversarial_R2_TenantSpoofing(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupAdversarialTestEnvironment(t)
	defer cleanup()

	legitTenant := "TENANT-REAL-001"
	spoofedTenant := "TENANT-ATTACKER-999"
	workflowID := "wf-adv-spoof-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, legitTenant, workflowID)

	token := signAdvIAPToken(t, key, kid, aud, "agents.adv-runtime-subject")

	t.Run("Attack: Forged X-Sentinel-Tenant Header Must Return 403", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-adv-spoof-header")
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":      "RemediationAgent",
			"tool_name":       "remediation.candidate.create",
			"tool_args":       json.RawMessage(argsBytes),
			"idempotency_key": "ik-adv-spoof-header-1",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req.Header.Set("X-Workflow-ID", workflowID)
		req.Header.Set("X-Sentinel-Tenant", spoofedTenant) // FORGED TENANT HEADER

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("ADVERSARIAL FAIL: Expected 403 Forbidden on forged tenant header, got %d. Body: %s", rec.Code, rec.Body.String())
		}
		var errResp map[string]string
		_ = json.NewDecoder(rec.Body).Decode(&errResp)
		if errResp["error"] != "tenant_context_mismatch" {
			t.Errorf("expected error 'tenant_context_mismatch', got %q", errResp["error"])
		}
	})

	t.Run("Attack: Forged tenant_id in Payload Must Be Ignored and Overridden Server-Side", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-adv-spoof-payload")
		toolArgs["tenant_id"] = spoofedTenant // FORGED IN PAYLOAD
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":      "RemediationAgent",
			"tool_name":       "remediation.candidate.create",
			"tool_args":       json.RawMessage(argsBytes),
			"idempotency_key": "ik-adv-spoof-payload-1",
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

		// Verify candidate was saved ONLY under legitTenant
		var attackerArtifactCount int
		_ = db.QueryRow(`SELECT COUNT(*) FROM file_instances WHERE tenant_id = ?`, spoofedTenant).Scan(&attackerArtifactCount)
		if attackerArtifactCount != 0 {
			t.Fatalf("ADVERSARIAL LEAK: Artifact saved under spoofed tenant (%s) count=%d", spoofedTenant, attackerArtifactCount)
		}

		var legitArtifactCount int
		_ = db.QueryRow(`SELECT COUNT(*) FROM file_instances WHERE tenant_id = ? AND status = 'CANDIDATE'`, legitTenant).Scan(&legitArtifactCount)
		if legitArtifactCount != 1 {
			t.Fatalf("expected candidate created in legit tenant (%s), count=%d", legitTenant, legitArtifactCount)
		}
	})
}

func TestAdversarial_R2_PreconditionViolations(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupAdversarialTestEnvironment(t)
	defer cleanup()

	tenantID := "TENANT-PRECOND-99"
	workflowID := "wf-adv-precond-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	token := signAdvIAPToken(t, key, kid, aud, "agents.adv-runtime-subject")

	t.Run("Precondition: Stale Expected Artifact SHA -> 412 Precondition Failed", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-adv-stale-sha")
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":               "RemediationAgent",
			"tool_name":                "remediation.candidate.create",
			"tool_args":                json.RawMessage(argsBytes),
			"idempotency_key":          "ik-adv-stale-sha-1",
			"expected_artifact_sha256": "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
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
			t.Fatalf("ADVERSARIAL FAIL: Expected 412 Precondition Failed on stale SHA, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Precondition: Wrong Row Version -> 412 Precondition Failed", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-adv-wrong-version")
		argsBytes, _ := json.Marshal(toolArgs)
		wrongVer := 42

		body := map[string]interface{}{
			"agent_name":           "RemediationAgent",
			"tool_name":            "remediation.candidate.create",
			"tool_args":            json.RawMessage(argsBytes),
			"idempotency_key":      "ik-adv-wrong-version-1",
			"expected_row_version": &wrongVer,
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
			t.Fatalf("ADVERSARIAL FAIL: Expected 412 Precondition Failed on wrong row version, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Precondition: Invalid Expected Workflow State -> 412 Precondition Failed", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-adv-wrong-state")
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":              "RemediationAgent",
			"tool_name":               "remediation.candidate.create",
			"tool_args":               json.RawMessage(argsBytes),
			"idempotency_key":         "ik-adv-wrong-state-1",
			"expected_workflow_state": "QUARANTINED", // Actual is REMEDIATING
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
			t.Fatalf("ADVERSARIAL FAIL: Expected 412 Precondition Failed on invalid workflow state, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestAdversarial_R2_IdempotencyContract(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupAdversarialTestEnvironment(t)
	defer cleanup()

	tenantID := "TENANT-IDEM-99"
	workflowID := "wf-adv-idem-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	token := signAdvIAPToken(t, key, kid, aud, "agents.adv-runtime-subject")

	t.Run("Missing Idempotency Key -> 400 Bad Request", func(t *testing.T) {
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-adv-no-idem")
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
			t.Fatalf("ADVERSARIAL FAIL: Expected 400 Bad Request on missing idempotency key, got %d. Body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Identical Idempotency Replay Returns Cached Response and Conflicting Payload Returns 409", func(t *testing.T) {
		planHash := "plan-adv-idem-replay-001"
		toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, planHash)
		argsBytes, _ := json.Marshal(toolArgs)

		body := map[string]interface{}{
			"agent_name":      "RemediationAgent",
			"tool_name":       "remediation.candidate.create",
			"tool_args":       json.RawMessage(argsBytes),
			"idempotency_key": "ik-adv-replay-key-001",
		}
		bodyBytes, _ := json.Marshal(body)

		// 1. Initial invocation
		req1 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req1.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req1.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req1.Header.Set("X-Workflow-ID", workflowID)

		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK {
			t.Fatalf("initial call failed: %d: %s", rec1.Code, rec1.Body.String())
		}

		var res1 struct {
			Output json.RawMessage `json:"output"`
		}
		_ = json.NewDecoder(rec1.Body).Decode(&res1)
		var cand1 candidate.CandidateResult
		_ = json.Unmarshal(res1.Output, &cand1)

		// 2. Replay with identical payload and key
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-Goog-IAP-JWT-Assertion", token)
		req2.Header.Set("X-Sentinel-Agent-Name", "RemediationAgent")
		req2.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
		req2.Header.Set("X-Workflow-ID", workflowID)

		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Fatalf("replay call failed: %d: %s", rec2.Code, rec2.Body.String())
		}

		var res2 struct {
			Output json.RawMessage `json:"output"`
		}
		_ = json.NewDecoder(rec2.Body).Decode(&res2)
		var cand2 candidate.CandidateResult
		_ = json.Unmarshal(res2.Output, &cand2)

		if cand1.CandidateSHA256 != cand2.CandidateSHA256 || cand1.CandidateArtifactID != cand2.CandidateArtifactID {
			t.Fatalf("ADVERSARIAL FAIL: Replay did not return identical candidate result: %v vs %v", cand1, cand2)
		}

		// 3. Conflict: Same idempotency key with conflicting payload
		toolArgsConflict := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-adv-conflicting-hash")
		argsBytesConflict, _ := json.Marshal(toolArgsConflict)

		bodyConflict := map[string]interface{}{
			"agent_name":      "RemediationAgent",
			"tool_name":       "remediation.candidate.create",
			"tool_args":       json.RawMessage(argsBytesConflict),
			"idempotency_key": "ik-adv-replay-key-001", // REUSED KEY WITH DIFFERENT PAYLOAD
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
			t.Fatalf("ADVERSARIAL FAIL: Expected 409 Conflict on idempotency key reuse with different payload, got %d. Body: %s", recConflict.Code, recConflict.Body.String())
		}
	})
}

func TestAdversarial_R2_RBAC(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupAdversarialTestEnvironment(t)
	defer cleanup()

	tenantID := "TENANT-RBAC-ADV"
	workflowID := "wf-adv-rbac-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	token := signAdvIAPToken(t, key, kid, aud, "agents.adv-runtime-subject")

	unauthorizedFleet := []string{
		"IncidentCommanderAgent",
		"DiagnosisAgent",
		"PolicySLAAgent",
		"MemoryAgent",
		"VerifierAgent",
		"ReturnRiskAgent",
		"MaliciousCustomAgent",
	}

	for _, rogueAgent := range unauthorizedFleet {
		t.Run(fmt.Sprintf("Agent '%s' forbidden from candidate create", rogueAgent), func(t *testing.T) {
			toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, fmt.Sprintf("plan-adv-rbac-%s", rogueAgent))
			argsBytes, _ := json.Marshal(toolArgs)

			body := map[string]interface{}{
				"agent_name":      rogueAgent,
				"tool_name":       "remediation.candidate.create",
				"tool_args":       json.RawMessage(argsBytes),
				"idempotency_key": fmt.Sprintf("ik-adv-rbac-%s", rogueAgent),
			}
			bodyBytes, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/agent-tools", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Goog-IAP-JWT-Assertion", token)
			req.Header.Set("X-Sentinel-Agent-Name", rogueAgent)
			req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
			req.Header.Set("X-Workflow-ID", workflowID)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("ADVERSARIAL FAIL: Rogue agent '%s' succeeded in calling candidate create (status=%d). Body: %s", rogueAgent, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdversarial_R2_ParentArtifactImmutability(t *testing.T) {
	db, store, router, key, kid, aud, cleanup := setupAdversarialTestEnvironment(t)
	defer cleanup()

	tenantID := "TENANT-IMMUT-ADV"
	workflowID := "wf-adv-immut-001"
	parentArtifactID, incidentID, parentSHA := seedTestWorkflow(t, db, store, tenantID, workflowID)

	var parentStoragePath string
	_ = db.QueryRow(`SELECT storage_path FROM file_instances WHERE id = ?`, parentArtifactID).Scan(&parentStoragePath)

	// Step 1: Read raw original bytes from ObjectStore before candidate creation
	rcBefore, err := store.Get(context.Background(), parentStoragePath)
	if err != nil {
		t.Fatalf("read parent before: %v", err)
	}
	bytesBefore, _ := io.ReadAll(rcBefore)
	rcBefore.Close()

	shaBefore := sha256.Sum256(bytesBefore)
	shaBeforeHex := hex.EncodeToString(shaBefore[:])

	token := signAdvIAPToken(t, key, kid, aud, "agents.adv-runtime-subject")

	toolArgs := buildRemediationArgs(parentArtifactID, incidentID, parentSHA, "plan-adv-immut-001")
	argsBytes, _ := json.Marshal(toolArgs)

	body := map[string]interface{}{
		"agent_name":      "RemediationAgent",
		"tool_name":       "remediation.candidate.create",
		"tool_args":       json.RawMessage(argsBytes),
		"idempotency_key": "ik-adv-immut-001",
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

	// Step 2: Read raw parent bytes from ObjectStore AFTER candidate creation
	rcAfter, err := store.Get(context.Background(), parentStoragePath)
	if err != nil {
		t.Fatalf("read parent after: %v", err)
	}
	bytesAfter, _ := io.ReadAll(rcAfter)
	rcAfter.Close()

	shaAfter := sha256.Sum256(bytesAfter)
	shaAfterHex := hex.EncodeToString(shaAfter[:])

	// Invariant Verification
	if !bytes.Equal(bytesBefore, bytesAfter) {
		t.Fatalf("CRITICAL SAFETY BREACH: Parent artifact bytes were MUTATED during candidate generation!")
	}
	if shaBeforeHex != shaAfterHex {
		t.Fatalf("CRITICAL SAFETY BREACH: Parent artifact SHA-256 changed from %s to %s!", shaBeforeHex, shaAfterHex)
	}
	if shaAfterHex != parentSHA {
		t.Fatalf("CRITICAL SAFETY BREACH: Parent artifact SHA-256 %s != initial SHA-256 %s", shaAfterHex, parentSHA)
	}
}

// -------------------------------------------------------------
// ADVERSARIAL TEST SUITE 2: R4 PostgreSQL Parity Deep Audit
// -------------------------------------------------------------

func TestAdversarial_R4_PostgreSQLDeepAudit(t *testing.T) {
	pgDir := filepath.Join("migrations_postgres")
	entries, err := os.ReadDir(pgDir)
	if err != nil {
		t.Fatalf("failed to read migrations_postgres: %v", err)
	}

	var sqlFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}

	if len(sqlFiles) != 24 {
		t.Fatalf("ADVERSARIAL FAIL: Expected 24 PostgreSQL migration files, found %d", len(sqlFiles))
	}

	tableRegex := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z0-9_]+)\s*\(([\s\S]*?)\);`)

	totalTenantTables := 0
	totalRLSPolicies := 0

	for _, fname := range sqlFiles {
		contentBytes, err := os.ReadFile(filepath.Join(pgDir, fname))
		if err != nil {
			t.Fatalf("read %s: %v", fname, err)
		}
		content := string(contentBytes)

		tables := tableRegex.FindAllStringSubmatch(content, -1)
		for _, m := range tables {
			tblName := strings.ToLower(m[1])
			tblBody := m[2]

			// If table has tenant_id, assert strict RLS and tenant_isolation policy
			if strings.Contains(strings.ToLower(tblBody), "tenant_id") {
				totalTenantTables++

				enableRLS := regexp.MustCompile(fmt.Sprintf(`(?i)ALTER\s+TABLE\s+%s\s+ENABLE\s+ROW\s+LEVEL\s+SECURITY`, tblName))
				if !enableRLS.MatchString(content) {
					t.Fatalf("ADVERSARIAL FAIL in %s: Table %s has tenant_id but lacks ENABLE ROW LEVEL SECURITY", fname, tblName)
				}

				forceRLS := regexp.MustCompile(fmt.Sprintf(`(?i)ALTER\s+TABLE\s+%s\s+FORCE\s+ROW\s+LEVEL\s+SECURITY`, tblName))
				if !forceRLS.MatchString(content) {
					t.Fatalf("ADVERSARIAL FAIL in %s: Table %s has tenant_id but lacks FORCE ROW LEVEL SECURITY", fname, tblName)
				}

				policyRegex := regexp.MustCompile(fmt.Sprintf(`(?i)CREATE\s+POLICY\s+tenant_isolation_%s\s+ON\s+%s`, tblName, tblName))
				if !policyRegex.MatchString(content) {
					t.Fatalf("ADVERSARIAL FAIL in %s: Table %s lacks CREATE POLICY tenant_isolation_%s", fname, tblName, tblName)
				}
				totalRLSPolicies++
			}
		}
	}

	if totalTenantTables == 0 || totalTenantTables != totalRLSPolicies {
		t.Fatalf("ADVERSARIAL FAIL: total tenant tables (%d) != total RLS policies (%d)", totalTenantTables, totalRLSPolicies)
	}
}
