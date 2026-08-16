package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/secrets"

	"github.com/golang-jwt/jwt/v5"
)

// Router-level acceptance for Prompt 04: the API must fail closed when
// authentication is missing or misconfigured, and no security decision may
// depend on the caller's cooperation.

func prodConfig() *Config {
	return &Config{
		Profile:       ProfileProduction,
		AllowedOrigin: "https://ops.example.com",
		OIDCIssuer:    "https://idp.example.com/",
		OIDCAudience:  "sentinel-flow-api",
	}
}

// The headline acceptance criterion. A production router built without a
// verifier must refuse every business route rather than serving them openly.
func TestProductionRouterFailsClosedWithoutVerifier(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	router := NewRouter(db, prodConfig(), nil) // no verifier configured

	for _, path := range []string{
		"/api/v1/health",
		"/api/v1/sla-board",
		"/api/v1/partners",
		"/api/v1/incidents",
		"/api/v1/ledger",
		"/api/v1/compliance/export",
	} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusOK {
			t.Errorf("GET %s served 200 with no authentication configured; body=%s",
				path, rec.Body.String())
		}
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s: expected 503 authentication_not_configured, got %d", path, rec.Code)
		}
	}
}

// An unauthenticated request against a properly configured production router.
func TestProductionRouterRejectsAnonymousRequests(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	v, err := auth.NewVerifier(auth.VerifierConfig{
		Issuer:   "https://idp.example.com/",
		Audience: "sentinel-flow-api",
		Keys:     map[string]*rsa.PublicKey{"k1": &key.PublicKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(db, prodConfig(), v)

	cases := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"empty bearer", "Bearer "},
		{"not a bearer", "Basic dXNlcjpwYXNz"},
		{"garbage token", "Bearer not.a.jwt"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/incidents", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d (body %s)", rec.Code, rec.Body.String())
			}
			// The rejection must not explain itself: a caller probing for the
			// difference between "expired" and "forged" learns nothing.
			if strings.Contains(rec.Body.String(), "expired") ||
				strings.Contains(rec.Body.String(), "signature") ||
				strings.Contains(rec.Body.String(), "issuer") {
				t.Errorf("rejection leaked the reason: %s", rec.Body.String())
			}
		})
	}
}

// The forged-actor vulnerability, at the route level.
//
// Before Prompt 04 this endpoint read `actor` from the body and defaulted it to
// "TREASURY_SUPERVISOR_01", so the audit ledger recorded whatever the caller
// claimed. The recorded actor must now be the token subject regardless of what
// the body says.
func TestApprovalRecordsVerifiedActorNotRequestBody(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	cfg := &Config{Profile: ProfileLocalDemo, AllowedOrigin: "http://localhost:3000"}
	router := NewRouter(db, cfg, nil) // demo principal: demo-operator@local

	body := `{"actor":"TREASURY_SUPERVISOR_01","supervisorId":"someone-else","justification":"looks fine to me"}`
	req := httptest.NewRequest("POST", "/api/v1/incidents/1/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("approve returned %d: %s", rec.Code, rec.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["actor"] != "demo-operator@local" {
		t.Errorf("response actor = %v; must be the verified subject", out["actor"])
	}

	// And the audit ledger must agree.
	var actor string
	err := db.QueryRow(
		`SELECT actor FROM audit_events WHERE event_type = 'INCIDENT_RESOLVED_BY_REVIEWER' ORDER BY id DESC LIMIT 1`,
	).Scan(&actor)
	if err != nil {
		t.Fatalf("no audit event recorded: %v", err)
	}
	if actor == "TREASURY_SUPERVISOR_01" || actor == "someone-else" {
		t.Fatalf("the audit ledger recorded a caller-supplied actor: %q", actor)
	}
	if actor != "demo-operator@local" {
		t.Errorf("audit actor = %q; must be the verified subject", actor)
	}
}

// A decision with no stated reason is not a decision anyone can review later.
func TestApprovalRequiresJustification(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()
	router := NewRouter(db, &Config{Profile: ProfileLocalDemo, AllowedOrigin: "x"}, nil)

	for _, body := range []string{`{}`, `{"justification":""}`, `{"justification":"   "}`} {
		req := httptest.NewRequest("POST", "/api/v1/incidents/1/approve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", body, rec.Code)
		}
	}
}

// CSRF: a cookie-authenticated mutation without a matching token is refused.
func TestCsrfRequiredForCookieAuthenticatedMutations(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()
	router := NewRouter(db, &Config{Profile: ProfileLocalDemo, AllowedOrigin: "x"}, nil)

	// With a session cookie and no CSRF header: refused.
	req := httptest.NewRequest("POST", "/api/v1/incidents/1/approve",
		strings.NewReader(`{"justification":"cross-site attempt"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "sentinel_session", Value: "session-value"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("a cookie-authenticated mutation with no CSRF token returned %d, want 403", rec.Code)
	}

	// A mismatched token is equally refused.
	req = httptest.NewRequest("POST", "/api/v1/incidents/1/approve",
		strings.NewReader(`{"justification":"cross-site attempt"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "wrong-value")
	req.AddCookie(&http.Cookie{Name: "sentinel_session", Value: "session-value"})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a mismatched CSRF token returned %d, want 403", rec.Code)
	}

	// The matching token succeeds.
	req = httptest.NewRequest("POST", "/api/v1/incidents/1/approve",
		strings.NewReader(`{"justification":"legitimate operator action"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "session-value")
	req.AddCookie(&http.Cookie{Name: "sentinel_session", Value: "session-value"})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("a matching CSRF token was refused: %d %s", rec.Code, rec.Body.String())
	}

	// A GET is never CSRF-checked.
	req = httptest.NewRequest("GET", "/api/v1/incidents", nil)
	req.AddCookie(&http.Cookie{Name: "sentinel_session", Value: "session-value"})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Errorf("a safe method was CSRF-rejected")
	}
}

// A token minted for another audience must not be replayable here, end to end
// through the router rather than only in the verifier's unit tests.
func TestRouterRejectsTokenForAnotherAudience(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	v, _ := auth.NewVerifier(auth.VerifierConfig{
		Issuer:   "https://idp.example.com/",
		Audience: "sentinel-flow-api",
		Keys:     map[string]*rsa.PublicKey{"k1": &key.PublicKey},
	})
	router := NewRouter(db, prodConfig(), v)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://idp.example.com/",
		"aud": "a-different-service", // valid token, wrong service
		"sub": "user-mallory",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "k1"
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/v1/incidents", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("a token for another audience was accepted: %d", rec.Code)
	}
}

// Gap 1: /metrics is registered outside /api/v1 and was reachable anonymously.
func TestMetricsRequiresCredentialInProduction(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	cfg := prodConfig()
	metricsToken := strings.Repeat("m", 40)
	token, err := secrets.New(metricsToken)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MetricsToken = token
	router := NewRouter(db, cfg, nil)

	// Anonymous.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous /metrics returned %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "sentinel_") {
		t.Errorf("anonymous /metrics leaked metric names: %s", rec.Body.String())
	}

	// Wrong credential.
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 40))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong metrics credential returned %d, want 401", rec.Code)
	}

	// Correct credential.
	req = httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+metricsToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("correct metrics credential returned %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "sentinel_uptime_seconds") {
		t.Errorf("authorised scrape did not receive metrics")
	}
}

// In the demo profile the process binds loopback only, which is the guard.
func TestMetricsIsOpenOnlyInTheDemoProfile(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()
	router := NewRouter(db, &Config{Profile: ProfileLocalDemo, AllowedOrigin: "x"}, nil)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("demo profile /metrics returned %d; loopback binding is the guard there", rec.Code)
	}
}
