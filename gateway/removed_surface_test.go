package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These tests enforce the Prompt 01 acceptance criteria: removed routes must be
// gone from the served surface, and no production response may carry a fixed
// success, security, compliance, or performance value.
//
// They are deliberately behavioural. A deleted handler already fails to
// compile, but that proves nothing about what the router still serves.

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })
	// Empty apiToken: these tests assert routing, not authorization. The
	// authentication gap is tracked for Prompt 04.
	return NewRouter(db, "")
}

func TestRemovedRoutesReturn404(t *testing.T) {
	removed := []struct {
		method string
		path   string
		reason string
	}{
		{"GET", "/api/v1/hub/connections", "Integration Hub was an in-memory literal"},
		{"GET", "/api/v1/hub/assets", "Integration Hub catalog"},
		{"GET", "/api/v1/hub/assets/ASSET-001/sample", "masked preview literals"},
		{"GET", "/api/v1/hub/lineage", "lineage literals"},
		{"POST", "/api/v1/hub/edge/sync", "returned mTLSVerified:true over plain HTTP"},
		{"POST", "/api/v1/instant-payments/validate", "fabricated SETTLED_INSTANT"},
		{"GET", "/api/v1/instant-payments/metrics", "fixed 1.42ms / 99.998% / 12500 TPS"},
		{"POST", "/api/v1/vault/tokenize", "vault removed from v1 scope"},
		{"GET", "/api/v1/vault/policies", "vault removed from v1 scope"},
		{"POST", "/api/v1/vault/detokenize", "vault removed from v1 scope"},
		{"POST", "/api/v1/swarm/deliberate", "scripted transcript"},
		{"GET", "/api/v1/swarm/sessions", "scripted transcript"},
		{"POST", "/api/v1/healing/propose", "hardcoded 0.995 confidence"},
		{"POST", "/api/v1/healing/apply", "accepted caller-supplied supervisor identity"},
		{"POST", "/api/v1/chaos/failover/simulate", "scripted DR simulation"},
		{"GET", "/api/v1/analytics/drift", "hardcoded report envelope"},
		{"GET", "/api/v1/analytics/anomalies", "hardcoded baseline and inputs"},
		{"POST", "/api/v1/sql/query", "arbitrary SQL console leaked secrets"},
		{"GET", "/api/v1/webhooks", "returned plaintext secrets"},
		{"POST", "/api/v1/webhooks", "returned plaintext secrets"},
		{"POST", "/api/v1/webhooks/test", "unrestricted SSRF sink"},
		{"POST", "/api/v1/chaos/trigger", "scripted incident injection"},
		{"GET", "/api/v1/benchmark/run", "benchmark is test-only until Prompt 13"},
	}

	router := testRouter(t)

	for _, tc := range removed {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s returned %d, expected 404 (removed: %s)",
					tc.method, tc.path, rec.Code, tc.reason)
			}
		})
	}
}

func TestSurvivingRoutesStillServe(t *testing.T) {
	// Counterweight: the deletion must not have taken the real surface with it.
	router := testRouter(t)

	for _, path := range []string{
		"/api/v1/health",
		"/api/v1/sla-board",
		"/api/v1/partners",
		"/api/v1/contracts",
		"/api/v1/incidents",
		"/api/v1/ledger",
		"/metrics",
	} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Errorf("GET %s returned 404; a surviving route was removed by mistake", path)
		}
	}
}

// TestNoFabricatedValuesInProductionResponses scans the responses of every
// surviving GET route for the specific constants the Prompt 00 audit found
// being served as if they were measurements or verified state.
func TestNoFabricatedValuesInProductionResponses(t *testing.T) {
	banned := []struct {
		needle string
		why    string
	}{
		{"mTLSVerified", "security state must derive from verified transport state"},
		{"SETTLED_INSTANT", "settlement is not a state in this product"},
		{"99.998", "fabricated SLA compliance rate"},
		{"12500", "fabricated throughput"},
		{"1.42 ms", "fabricated latency"},
		{"breachRiskPct", "predictive breach risk is a v1 non-goal"},
		{"countdownMinutes", "fixed countdown presented as live"},
		{"sentinel_worker_pool_active", "gauge reported a literal 8 with no worker pool"},
		{"sentinel_merkle_chain_height", "misnomer: this is a linear hash chain"},
		{"SEC Rule 17a-4", "unsupported regulatory claim"},
		{"SOX 404", "unsupported regulatory claim"},
		{"FINRA", "unsupported regulatory claim"},
		{"passRatePct", "fabricated evaluation pass rate"},
		{"Eliza 2.0", "fabricated AI analyst identity"},
		{"whsec_", "webhook secrets must never appear in a response"},
	}

	router := testRouter(t)

	for _, path := range []string{
		"/api/v1/health",
		"/api/v1/sla-board",
		"/api/v1/partners",
		"/api/v1/contracts",
		"/api/v1/incidents",
		"/api/v1/ledger",
		"/api/v1/compliance/export",
		"/metrics",
	} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		body := rec.Body.String()

		for _, b := range banned {
			if strings.Contains(body, b.needle) {
				t.Errorf("GET %s response contains %q (%s)", path, b.needle, b.why)
			}
		}
	}
}

// TestAiTierUnavailableIsNotSuccess pins the removal of the fabricated
// fallbacks. With no AI tier running, these routes must report that they could
// not run -- not invent a result. Before Prompt 01 the eval route returned
// passRatePct 100.0 and the triage route invented a summary with citations.
func TestAiTierUnavailableIsNotSuccess(t *testing.T) {
	router := testRouter(t)

	req := httptest.NewRequest("GET", "/api/v1/evals/run", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("evals route returned 200 with no evaluator running; body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "passRatePct") {
		t.Errorf("evals route emitted a pass rate with no evaluator running")
	}
	if !strings.Contains(rec.Body.String(), "NOT_RUN") {
		t.Errorf("expected NOT_RUN status, got %s", rec.Body.String())
	}
}
