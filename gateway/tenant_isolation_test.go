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

	"github.com/golang-jwt/jwt/v5"
)

// The Prompt 04 acceptance criterion, proven over HTTP rather than at the
// repository layer alone:
//
//   two test tenants cannot read, infer, update, or enumerate each other's
//   records
//
// Every request below carries a real signed token. Nothing here uses the demo
// principal or the DefaultTenantID constant.

const (
	isoIssuer   = "https://idp.example.com/"
	isoAudience = "sentinel-flow-api"
	isoKID      = "iso-key"
	isoClaim    = "https://sentinelflow.dev/tenants"
)

func newIsolationEnv(t *testing.T) (http.Handler, *rsa.PrivateKey) {
	t.Helper()

	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(
		`INSERT INTO tenants (id,name) VALUES ('TENANT-ALPHA','Alpha'),('TENANT-BETA','Beta')`,
	); err != nil {
		t.Fatalf("create tenants: %v", err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.NewVerifier(auth.VerifierConfig{
		Issuer:      isoIssuer,
		Audience:    isoAudience,
		Keys:        map[string]*rsa.PublicKey{isoKID: &key.PublicKey},
		TenantClaim: isoClaim,
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Profile:       ProfileProduction,
		AllowedOrigin: "https://ops.example.com",
		OIDCIssuer:    isoIssuer,
		OIDCAudience:  isoAudience,
	}
	return NewRouter(db, cfg, v), key
}

// tokenFor mints a real token granting the named roles in the named tenant.
func tokenFor(t *testing.T, key *rsa.PrivateKey, subject, tenant string, roles ...string) string {
	t.Helper()
	anyRoles := make([]any, 0, len(roles))
	for _, r := range roles {
		anyRoles = append(anyRoles, r)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":    isoIssuer,
		"aud":    isoAudience,
		"sub":    subject,
		"exp":    time.Now().Add(time.Hour).Unix(),
		isoClaim: map[string]any{tenant: anyRoles},
	})
	tok.Header["kid"] = isoKID
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func do(t *testing.T, router http.Handler, method, path, token, body string, hdrs map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Authorization", "Bearer "+token)
	for k, v := range hdrs {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	return rec
}

// Each tenant ingests a file, then neither can see the other's.
func TestTwoTenantsCannotReadEachOthersArtifacts(t *testing.T) {
	router, key := newIsolationEnv(t)

	alpha := tokenFor(t, key, "alice@alpha", "TENANT-ALPHA", "operator", "viewer")
	beta := tokenFor(t, key, "bob@beta", "TENANT-BETA", "operator", "viewer")

	valid := GenerateNachaScenario(PresetBalancedPayroll)

	recA := do(t, router, "POST", "/api/v1/files/ingest-raw", alpha,
		`{"filename":"alpha-secret.ach","content":`+jsonString(valid.Content)+`}`, nil)
	if recA.Code != http.StatusOK {
		t.Fatalf("alpha ingest failed: %d %s", recA.Code, recA.Body.String())
	}
	recB := do(t, router, "POST", "/api/v1/files/ingest-raw", beta,
		`{"filename":"beta-secret.ach","content":""}`, nil)
	if recB.Code != http.StatusOK {
		t.Fatalf("beta ingest failed: %d %s", recB.Code, recB.Body.String())
	}

	// Alpha's incident list must not mention beta's file, and vice versa.
	listA := do(t, router, "GET", "/api/v1/incidents", alpha, "", nil)
	if listA.Code != http.StatusOK {
		t.Fatalf("alpha incidents: %d %s", listA.Code, listA.Body.String())
	}
	if strings.Contains(listA.Body.String(), "beta-secret.ach") {
		t.Errorf("TENANT-ALPHA can see TENANT-BETA's filename:\n%s", listA.Body.String())
	}

	listB := do(t, router, "GET", "/api/v1/incidents", beta, "", nil)
	if strings.Contains(listB.Body.String(), "alpha-secret.ach") {
		t.Errorf("TENANT-BETA can see TENANT-ALPHA's filename:\n%s", listB.Body.String())
	}
}

// The audit ledger is per tenant. One tenant's actions must not appear in
// another's evidence export.
func TestLedgerAndEvidenceExportAreTenantScoped(t *testing.T) {
	router, key := newIsolationEnv(t)
	alpha := tokenFor(t, key, "alice@alpha", "TENANT-ALPHA", "operator", "viewer")
	beta := tokenFor(t, key, "bob@beta", "TENANT-BETA", "operator", "viewer")

	do(t, router, "POST", "/api/v1/files/ingest-raw", alpha,
		`{"filename":"alpha-only.ach","content":""}`, nil)

	// Beta's ledger must be empty; it has done nothing.
	recB := do(t, router, "GET", "/api/v1/ledger", beta, "", nil)
	if recB.Code != http.StatusOK {
		t.Fatalf("beta ledger: %d %s", recB.Code, recB.Body.String())
	}
	var ledgerB struct {
		TotalEvents int `json:"totalEvents"`
	}
	if err := json.Unmarshal(recB.Body.Bytes(), &ledgerB); err != nil {
		t.Fatal(err)
	}
	if ledgerB.TotalEvents != 0 {
		t.Errorf("TENANT-BETA's ledger shows %d events from another tenant's activity", ledgerB.TotalEvents)
	}
	if strings.Contains(recB.Body.String(), "alpha-only.ach") {
		t.Errorf("TENANT-BETA's ledger leaked TENANT-ALPHA's filename")
	}

	// Alpha's ledger must show its own event.
	recA := do(t, router, "GET", "/api/v1/ledger", alpha, "", nil)
	var ledgerA struct {
		TotalEvents int `json:"totalEvents"`
	}
	_ = json.Unmarshal(recA.Body.Bytes(), &ledgerA)
	if ledgerA.TotalEvents == 0 {
		t.Errorf("TENANT-ALPHA's own ledger is empty; scoping is too aggressive")
	}

	// Enumeration through the evidence export: counts must not include the
	// other tenant's volume.
	expB := do(t, router, "GET", "/api/v1/compliance/export", beta, "", nil)
	if strings.Contains(expB.Body.String(), "alpha-only.ach") {
		t.Errorf("evidence export leaked another tenant's filename")
	}
	var pkg struct {
		ValidationSummary map[string]any `json:"validationSummary"`
	}
	if err := json.Unmarshal(expB.Body.Bytes(), &pkg); err != nil {
		t.Fatal(err)
	}
	if n, ok := pkg.ValidationSummary["totalTransmissionsIngested"].(float64); ok && n != 0 {
		t.Errorf("TENANT-BETA's export counts %v transmissions; it ingested none", n)
	}
}

// Naming another tenant in the selector header is a denial, not an escalation.
func TestTenantHeaderCannotSelectAnotherTenant(t *testing.T) {
	router, key := newIsolationEnv(t)
	alpha := tokenFor(t, key, "alice@alpha", "TENANT-ALPHA", "operator", "viewer")

	for _, path := range []string{
		"/api/v1/incidents",
		"/api/v1/ledger",
		"/api/v1/partners",
		"/api/v1/contracts",
		"/api/v1/sla-board",
		"/api/v1/compliance/export",
	} {
		rec := do(t, router, "GET", path, alpha, "", map[string]string{
			TenantHeader: "TENANT-BETA",
		})
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET %s with a spoofed tenant header returned %d, want 403 (body %s)",
				path, rec.Code, rec.Body.String())
		}
	}

	// And writing into another tenant is refused too.
	rec := do(t, router, "POST", "/api/v1/files/ingest-raw", alpha,
		`{"filename":"planted.ach","content":""}`, map[string]string{TenantHeader: "TENANT-BETA"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-tenant write returned %d, want 403", rec.Code)
	}
}

// A nonexistent tenant must be refused the same way an existing one the caller
// does not belong to is, so the header cannot be used to probe which tenants
// exist.
func TestUnknownAndForeignTenantsAreIndistinguishable(t *testing.T) {
	router, key := newIsolationEnv(t)
	alpha := tokenFor(t, key, "alice@alpha", "TENANT-ALPHA", "viewer")

	foreign := do(t, router, "GET", "/api/v1/incidents", alpha, "", map[string]string{
		TenantHeader: "TENANT-BETA", // exists
	})
	unknown := do(t, router, "GET", "/api/v1/incidents", alpha, "", map[string]string{
		TenantHeader: "TENANT-DOES-NOT-EXIST", // does not
	})

	if foreign.Code != unknown.Code {
		t.Errorf("status differs: existing-but-foreign=%d unknown=%d; this confirms tenant existence",
			foreign.Code, unknown.Code)
	}
	if foreign.Body.String() != unknown.Body.String() {
		t.Errorf("body differs between foreign and unknown tenant:\n  foreign: %s\n  unknown: %s",
			foreign.Body.String(), unknown.Body.String())
	}
}

// Role enforcement over HTTP, not only in the matrix unit test.
func TestViewerCannotUploadOrApproveOverHttp(t *testing.T) {
	router, key := newIsolationEnv(t)
	viewer := tokenFor(t, key, "vera@alpha", "TENANT-ALPHA", "viewer")

	rec := do(t, router, "POST", "/api/v1/files/ingest-raw", viewer,
		`{"filename":"x.ach","content":""}`, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a viewer uploaded an artifact: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, router, "POST", "/api/v1/incidents/1/approve", viewer,
		`{"justification":"I should not be able to do this"}`, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a viewer approved a release: %d %s", rec.Code, rec.Body.String())
	}
}

// tenant_admin administers; it does not approve releases.
func TestTenantAdminCannotApproveOverHttp(t *testing.T) {
	router, key := newIsolationEnv(t)
	admin := tokenFor(t, key, "adam@alpha", "TENANT-ALPHA", "tenant_admin")

	rec := do(t, router, "POST", "/api/v1/incidents/1/approve", admin,
		`{"justification":"administering is not approving"}`, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("tenant_admin approved a release: %d %s", rec.Code, rec.Body.String())
	}
}

// A principal in two tenants must say which one it means, rather than having
// one silently chosen.
func TestMultiTenantPrincipalMustSelectATenant(t *testing.T) {
	router, key := newIsolationEnv(t)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": isoIssuer, "aud": isoAudience, "sub": "multi@example",
		"exp": time.Now().Add(time.Hour).Unix(),
		isoClaim: map[string]any{
			"TENANT-ALPHA": []any{"viewer"},
			"TENANT-BETA":  []any{"viewer"},
		},
	})
	tok.Header["kid"] = isoKID
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	rec := do(t, router, "GET", "/api/v1/incidents", raw, "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a multi-tenant principal had a tenant chosen for it: %d %s", rec.Code, rec.Body.String())
	}

	// With an explicit, legitimate selection it works.
	rec = do(t, router, "GET", "/api/v1/incidents", raw, "", map[string]string{TenantHeader: "TENANT-BETA"})
	if rec.Code != http.StatusOK {
		t.Errorf("an explicit legitimate tenant selection was refused: %d %s", rec.Code, rec.Body.String())
	}
}

// jsonString quotes a string for embedding in a JSON body.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
