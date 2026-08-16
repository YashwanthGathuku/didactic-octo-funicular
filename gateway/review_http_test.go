package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/review"
)

// The release workflow over HTTP.
//
// The property that matters most here is the one the old
// POST /incidents/{id}/approve got wrong: it read `actor` from the request body
// and defaulted it to the literal "TREASURY_SUPERVISOR_01", so any caller could
// record a decision under a name that looked like a real supervisor.

func TestAForgedActorInTheBodyIsIgnored(t *testing.T) {
	// The full ingest environment, because the artifact a decision is about has
	// to have actually been stored.
	db, handler, _ := newIngestTestEnv(t)

	// Find the tenant the demo principal resolves to.
	_, probe := doUpload(t, handler, uploadRequest(t, "probe.ach", []byte("probe"), nil))
	var tenantID string
	if err := db.QueryRow(`SELECT tenant_id FROM file_instances WHERE id = ?`,
		probe.ArtifactID).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}

	store, err := reviewStoreFor(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// A decision to vote on.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.ProposeTx(ctx, tx, tenantID, "system:validation-worker", review.Subject{
		ArtifactID: probe.ArtifactID, ArtifactSHA256: probe.SHA256,
		PolicyVersion: "release-policy/1.0.0", RulePackVersion: "nacha-rules/1.0.0",
		Outcome: "VALIDATED", ParserName: "internal/nacha", ParserVersion: "1",
	}, review.Policy{MinApprovals: 2, SeparationOfDuties: true, OverrideAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Vote, claiming to be someone else.
	body, _ := json.Marshal(map[string]string{
		"reason": "checked the control totals against the partner statement",
		"actor":  "TREASURY_SUPERVISOR_01",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/decisions/"+itoa64(id)+"/approve", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}

	d, err := store.Get(ctx, tenantID, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Votes) != 1 {
		t.Fatalf("votes = %d, want 1", len(d.Votes))
	}
	if d.Votes[0].ActorID == "TREASURY_SUPERVISOR_01" {
		t.Fatal("the request body's actor was recorded; identity must come from the verified " +
			"principal and from nowhere else")
	}
	if d.Votes[0].ActorID == "" {
		t.Error("the vote was recorded with no actor")
	}
	t.Logf("vote recorded against the verified principal %q, not the body's claim",
		d.Votes[0].ActorID)
}

// The role matrix must keep the three authorities apart.
func TestApprovingOverridingAndConfiguringAreDifferentAuthorities(t *testing.T) {
	for _, tc := range []struct {
		role                   auth.Role
		approve, override, cfg bool
	}{
		{auth.RoleViewer, false, false, false},
		{auth.RoleOperator, false, false, false},
		{auth.RoleReviewer, true, false, false},
		{auth.RoleTenantAdmin, false, false, true},
		{auth.RoleReleaseSupervisor, true, true, false},
	} {
		p := &auth.Principal{
			Subject: "u", Issuer: "test",
			Memberships: []auth.Membership{{TenantID: "T", Roles: []auth.Role{tc.role}}},
		}
		check := func(perm auth.Permission) bool { return p.Authorize("T", perm) == nil }

		if got := check(auth.PermApproveRelease); got != tc.approve {
			t.Errorf("%s approve = %v, want %v", tc.role, got, tc.approve)
		}
		if got := check(auth.PermOverrideRelease); got != tc.override {
			t.Errorf("%s override = %v, want %v", tc.role, got, tc.override)
		}
		if got := check(auth.PermManageReleasePolicy); got != tc.cfg {
			t.Errorf("%s configure policy = %v, want %v", tc.role, got, tc.cfg)
		}
	}

	// The specific collapse the design refuses: nobody may both configure the
	// threshold and step around it, because "lower the threshold, then meet
	// it" would be an alternative to writing a justification.
	for _, role := range []auth.Role{
		auth.RoleViewer, auth.RoleOperator, auth.RoleReviewer,
		auth.RoleTenantAdmin, auth.RoleReleaseSupervisor,
	} {
		p := &auth.Principal{
			Subject: "u", Issuer: "test",
			Memberships: []auth.Membership{{TenantID: "T", Roles: []auth.Role{role}}},
		}
		canConfigure := p.Authorize("T", auth.PermManageReleasePolicy) == nil
		canOverride := p.Authorize("T", auth.PermOverrideRelease) == nil
		if canConfigure && canOverride {
			t.Errorf("%s can both configure dual control and override it", role)
		}
	}
}

// A route that decides a release must refuse a caller without the permission.
func TestReleaseRoutesRefuseAnUnauthorizedCaller(t *testing.T) {
	db := setupTestDb(t)
	t.Cleanup(func() { db.Close() })

	// The production profile with no configuration refuses every request, which
	// is the fail-closed behaviour Prompt 04 established. Here it stands in for
	// "a caller who is not authorized".
	handler := NewRouter(db, &Config{Profile: ProfileProduction}, nil)

	for _, path := range []string{
		"/api/v1/review-queue",
		"/api/v1/decisions/1/approve",
		"/api/v1/decisions/1/reject",
		"/api/v1/decisions/1/release",
		"/api/v1/decisions/1/override",
		"/api/v1/release-overrides",
		"/api/v1/release-policy",
	} {
		method := http.MethodGet
		if strings.Contains(path, "decisions") {
			method = http.MethodPost
		}
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
			t.Errorf("%s %s returned %d for an unauthenticated caller", method, path, rec.Code)
		}
	}
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
