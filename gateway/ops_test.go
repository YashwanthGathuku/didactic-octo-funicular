package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The read API the operations UI is built on.
//
// The requirement these tests exist for is the guide's "large paginated dataset
// does not load all rows into browser memory". That is not a property of the
// browser -- a UI cannot avoid holding what the server sends it. It is a
// property of the server: the list endpoints must refuse to return everything.

func opsGet(t *testing.T, handler http.Handler, path string, out any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if out != nil && rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s: %v (body %s)", path, err, rec.Body.String())
		}
	}
	return rec
}

func TestAListEndpointRefusesToReturnEverything(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)

	// Enough artifacts that an unpaged response would be visibly wrong.
	for i := range 40 {
		rec, _ := doUpload(t, handler,
			uploadRequest(t, fmt.Sprintf("bulk-%03d.ach", i),
				[]byte(fmt.Sprintf("bulk artifact number %d", i)), nil))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("upload %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}

	// A client asking for everything is given a page, not everything. The
	// ceiling is enforced rather than negotiated: a limit the caller chooses is
	// a way to exhaust the server with one authenticated request.
	var p page[artifactSummary]
	opsGet(t, handler, "/api/v1/artifacts?limit=100000", &p)
	if p.Limit > maxPageLimit {
		t.Errorf("server honoured a limit of 100000 (returned limit %d)", p.Limit)
	}
	if len(p.Items) > maxPageLimit {
		t.Errorf("server returned %d rows, above its own ceiling of %d",
			len(p.Items), maxPageLimit)
	}

	// And the page is genuinely a page: walking the cursor reaches every row
	// exactly once, which is the property that makes a bounded page usable
	// rather than merely small.
	seen := map[int64]bool{}
	cursor := ""
	pages := 0
	for {
		path := "/api/v1/artifacts?limit=7"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		var pg page[artifactSummary]
		opsGet(t, handler, path, &pg)
		if len(pg.Items) > 7 {
			t.Fatalf("page carried %d items for a limit of 7", len(pg.Items))
		}
		for _, a := range pg.Items {
			if seen[a.ID] {
				t.Errorf("artifact %d appeared on two pages", a.ID)
			}
			seen[a.ID] = true
		}
		pages++
		if !pg.HasMore || pages > 30 {
			break
		}
		if pg.NextCursor == "" {
			t.Fatal("hasMore was true and no cursor was issued; the list has no next page")
		}
		cursor = pg.NextCursor
	}
	if len(seen) < 40 {
		t.Errorf("paging reached %d artifacts across %d pages, want at least the 40 uploaded",
			len(seen), pages)
	}
	t.Logf("%d artifacts across %d pages of 7, none repeated", len(seen), pages)
}

func TestAForgedCursorIsRefused(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)

	for _, cursor := range []string{
		"1",                // not base64 of a versioned cursor
		"bm90LWEtY3Vyc29y", // base64 of "not-a-cursor"
		"c2ZwMTotMQ",       // sfp1:-1
		"c2ZwMToxLDIsMw",   // sfp1:1,2,3 -- too many components
	} {
		rec := opsGet(t, handler, "/api/v1/artifacts?cursor="+cursor, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("cursor %q returned %d, want 400 (a bad cursor must not silently "+
				"restart the list at the head)", cursor, rec.Code)
		}
	}
}

func TestAnUnknownFilterValueIsRefusedRatherThanIgnored(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)

	// Ignoring it would return the unfiltered list, which reads to the
	// operator as "there are no quarantined files".
	rec := opsGet(t, handler, "/api/v1/artifacts?status=DEFINITELY_NOT_A_STATUS", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown status returned %d, want 400: %s", rec.Code, rec.Body.String())
	}
	rec = opsGet(t, handler, "/api/v1/sla-board?status=NOPE", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown expectation status returned %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestSessionReportsWhatTheCallerMayActuallyDo(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)

	var s sessionResponse
	rec := opsGet(t, handler, "/api/v1/session", &s)
	if rec.Code != http.StatusOK {
		t.Fatalf("session: %d %s", rec.Code, rec.Body.String())
	}
	if s.Subject == "" || s.TenantID == "" {
		t.Fatalf("session carried no identity: %+v", s)
	}
	if !s.Demo {
		t.Error("the demo profile did not announce itself; a demo build that looks like " +
			"production is the failure this rehabilitation started from")
	}

	// The demo principal holds reviewer but not release_supervisor, so it may
	// approve and may not override. If the session ever claimed otherwise the
	// UI would offer a control the server refuses.
	has := func(p string) bool {
		for _, got := range s.Permissions {
			if got == p {
				return true
			}
		}
		return false
	}
	if !has("release:approve") {
		t.Error("session did not report release:approve for a principal holding reviewer")
	}
	if has("release:override") {
		t.Error("session reported release:override for a principal that holds no " +
			"release_supervisor membership")
	}
}

func TestServiceHealthNeverReportsAnUnmeasuredDependencyAsHealthy(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)

	var h serviceHealthResponse
	rec := opsGet(t, handler, "/api/v1/service-health", &h)
	if rec.Code != http.StatusOK {
		t.Fatalf("service-health: %d %s", rec.Code, rec.Body.String())
	}
	if h.Database.Status != "OK" || !h.Database.Measured {
		t.Errorf("database health = %+v, want a measured OK", h.Database)
	}
	// Neither is configured in this environment. Reporting either as OK would
	// be the constant-metric defect returning through the health screen.
	for name, c := range map[string]componentHealth{
		"objectStore": h.ObjectStore,
		"aiTier":      h.AITier,
	} {
		if c.Status == "OK" {
			t.Errorf("%s reported OK without being probed: %+v", name, c)
		}
		if c.Measured {
			t.Errorf("%s claimed to be measured and nothing measured it: %+v", name, c)
		}
	}
	if h.Queue == nil {
		t.Error("queue depth was not reported; a health screen with no queue state is " +
			"the state it exists to show")
	}
}

func TestArtifactDetailCarriesTheRunAndItsFindings(t *testing.T) {
	_, handler, _ := newIngestTestEnv(t)

	rec, up := doUpload(t, handler,
		uploadRequest(t, "detail.ach", []byte("not a valid nacha file"), nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}

	var d artifactDetail
	got := opsGet(t, handler, fmt.Sprintf("/api/v1/artifacts/%d", up.ArtifactID), &d)
	if got.Code != http.StatusOK {
		t.Fatalf("artifact detail: %d %s", got.Code, got.Body.String())
	}
	if d.ID != up.ArtifactID || d.SHA256 != up.SHA256 {
		t.Errorf("detail described a different artifact: %+v", d)
	}
	// An artifact whose validation has not run yet reports no run. That is a
	// distinct state from "validated with no findings", and the screen has to
	// be able to tell them apart.
	if d.Run == nil {
		t.Logf("no validation run yet for artifact %d; the detail says so rather than "+
			"implying a clean result", d.ID)
	}
	if d.Findings == nil {
		t.Error("findings was null rather than an empty list; null and empty read " +
			"differently in a UI and only one of them is true here")
	}
}

func TestArtifactDetailRefusesAnotherTenantsArtifact(t *testing.T) {
	db, handler, _ := newIngestTestEnv(t)

	other := "TENANT-OTHER-OPS"
	if _, err := db.Exec(`INSERT INTO tenants (id, name) VALUES (?, ?)`, other, "Other"); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRow(`
		INSERT INTO file_instances
			(tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (?, 'theirs.ach', 'mem://theirs', 10, 'other-tenant-hash', 'VALIDATED', CURRENT_TIMESTAMP)
		RETURNING id`, other).Scan(&id); err != nil {
		t.Fatal(err)
	}

	rec := opsGet(t, handler, fmt.Sprintf("/api/v1/artifacts/%d", id), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("reading another tenant's artifact returned %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "theirs.ach") {
		t.Fatal("the refusal disclosed the other tenant's filename")
	}
}

func TestOperationsRoutesRefuseAnUnauthorizedCaller(t *testing.T) {
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })
	handler := NewRouter(db, &Config{Profile: ProfileProduction}, nil)

	for _, path := range []string{
		"/api/v1/session",
		"/api/v1/artifacts",
		"/api/v1/artifacts/1",
		"/api/v1/contracts/1/versions",
		"/api/v1/evidence",
		"/api/v1/service-health",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s returned 200 for an unauthenticated caller", path)
		}
	}
}
