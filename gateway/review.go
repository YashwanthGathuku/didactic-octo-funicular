package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/jobs"
	"sentinel-gateway/internal/nacha"
	"sentinel-gateway/internal/review"
)

// The gateway's adapter onto internal/review.
//
// Nothing here decides whether a release is permitted. That is entirely
// internal/review's job, and it is kept there so there is one place that
// answers the question -- a handler that made its own judgement would be a
// second place, and the two would eventually disagree.

// reviewStoreFor builds the review store on the application's database.
var (
	reviewOnce  sync.Once
	reviewStore *review.Store
	reviewErr   error
)

func reviewStoreFor(db *sql.DB) (*review.Store, error) {
	reviewOnce.Do(func() {
		reviewStore, reviewErr = review.NewStore(db, "sqlite")
	})
	return reviewStore, reviewErr
}

// ledgerReleaseSink writes release-workflow events to the evidence chain and
// the outbox.
//
// Both, and in the transaction that produced the event. The ledger is the
// tamper-evident record; the outbox is what lets anything downstream react. A
// release that reached one and not the other would be a release the rest of the
// system did not know about, or one with no evidence.
type ledgerReleaseSink struct {
	db    *sql.DB
	queue *jobs.Queue
}

func (s *ledgerReleaseSink) ReleaseEvent(ctx context.Context, tx *sql.Tx, ev review.Event) error {
	if _, err := AppendAuditEvent(s.db, ev.TenantID, ev.Action, ev.Actor, ev.Payload); err != nil {
		return err
	}
	if s.queue == nil {
		return nil
	}
	return s.queue.PublishTx(ctx, tx, jobs.OutboxEvent{
		TenantID:    ev.TenantID,
		EventType:   ev.Action,
		SubjectType: "decision",
		SubjectID:   ev.DecisionID,
		// Keyed by decision and action, so a retried transition publishes one
		// event rather than two.
		DedupeKey: ev.Action + "-" + strconv.FormatInt(ev.DecisionID, 10),
		Payload:   ev.Payload,
	})
}

// subjectForArtifact rebuilds what a decision is about, from what is stored.
//
// Every caller that votes or releases derives the subject this way, so a
// decision is always compared against the artifact's current state rather than
// against whatever the caller believed. That is what makes staleness detectable
// instead of assumed.
func subjectForArtifact(ctx context.Context, db *sql.DB, tenantID string, artifactID int64) (review.Subject, error) {
	var s review.Subject
	s.ArtifactID = artifactID

	if err := db.QueryRowContext(ctx,
		`SELECT sha256_hash FROM file_instances WHERE tenant_id = ? AND id = ?`,
		tenantID, artifactID).Scan(&s.ArtifactSHA256); err != nil {
		return s, err
	}

	// The most recent run is the one that governs. An older run's findings
	// describe bytes that may since have been revalidated.
	err := db.QueryRowContext(ctx, `
		SELECT id, policy_version, rule_pack_version, contract_id, contract_version, outcome
		FROM validation_runs
		WHERE tenant_id = ? AND file_instance_id = ?
		ORDER BY started_at DESC, id DESC LIMIT 1`, tenantID, artifactID).
		Scan(&s.ValidationRunID, &s.PolicyVersion, &s.RulePackVersion,
			&s.ContractID, &s.ContractVersion, &s.Outcome)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT code, severity FROM validation_findings
		WHERE tenant_id = ? AND file_instance_id = ?`, tenantID, artifactID)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var f review.Finding
		if err := rows.Scan(&f.RuleID, &f.Severity); err != nil {
			return s, err
		}
		s.Findings = append(s.Findings, f)
	}
	return s, rows.Err()
}

// registerReviewRoutes mounts the review queue and the decision actions.
func registerReviewRoutes(r chi.Router, db *sql.DB) {
	r.Get("/review-queue", listReviewQueue(db))
	r.Get("/release-policy", getReleasePolicy(db))
	r.Put("/release-policy", setReleasePolicy(db))
	r.Post("/decisions/{id}/approve", voteOnDecision(db, true))
	r.Post("/decisions/{id}/reject", voteOnDecision(db, false))
	r.Post("/decisions/{id}/release", releaseDecision(db))
	r.Post("/decisions/{id}/override", overrideDecision(db))
	r.Get("/release-overrides", listOverrides(db))
}

func listReviewQueue(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		store, err := reviewStoreFor(db)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "review_unavailable"})
			return
		}

		req, perr := parsePage(r)
		if perr != nil {
			writeJSON(w, http.StatusBadRequest,
				map[string]any{"error": "bad_page", "detail": perr.Error()})
			return
		}

		queue, err := store.QueuePage(r.Context(), scope.TenantID(), req.After, req.Limit+1)
		if err != nil {
			log.Printf("review: queue failed for tenant %s: %v", scope.TenantID(), err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		writeJSON(w, http.StatusOK,
			finish(queue, req, func(d *review.Decision) int64 { return d.ID }))
	}
}

// voteOnDecision records an approval or a rejection.
//
// The actor comes from the verified principal and from nowhere else. Any actor
// field in the body is ignored -- not rejected with an error, ignored -- so a
// client that sends one gets the same behaviour as one that does not, and there
// is no code path in which a request-supplied name reaches the record.
func voteOnDecision(db *sql.DB, approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermApproveRelease)
		if serr != nil {
			serr.write(w)
			return
		}
		principal := auth.FromContext(r.Context())
		if principal == nil || principal.ActorID() == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}

		store, id, ok := decisionTarget(w, r, db)
		if !ok {
			return
		}

		var body struct {
			Reason string `json:"reason"`
			// Actor is declared so the field is visibly ignored rather than
			// silently absent from the struct. A reader of this handler should
			// be able to see that a client can send one and that it does
			// nothing.
			Actor string `json:"actor"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body)

		current, err := subjectForArtifactOfDecision(r.Context(), db, store, scope.TenantID(), id)
		if err != nil {
			writeReviewError(w, err)
			return
		}

		d, err := store.Vote(r.Context(), scope.TenantID(), principal.ActorID(),
			rolesOf(principal, scope.TenantID()), current,
			review.VoteRequest{DecisionID: id, Approve: approve, Reason: body.Reason})
		if err != nil {
			writeReviewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

func releaseDecision(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermApproveRelease)
		if serr != nil {
			serr.write(w)
			return
		}
		principal := auth.FromContext(r.Context())
		if principal == nil || principal.ActorID() == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}

		store, id, ok := decisionTarget(w, r, db)
		if !ok {
			return
		}
		current, err := subjectForArtifactOfDecision(r.Context(), db, store, scope.TenantID(), id)
		if err != nil {
			writeReviewError(w, err)
			return
		}

		res, err := store.Release(r.Context(), scope.TenantID(), principal.ActorID(), id, current)
		if err != nil {
			writeReviewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func overrideDecision(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// An override needs its own authority. Approving is a reviewer's job;
		// stepping around the controls is a supervisor's, and configuring the
		// controls is an administrator's -- three different people.
		scope, serr := resolveScope(r, auth.PermOverrideRelease)
		if serr != nil {
			serr.write(w)
			return
		}
		principal := auth.FromContext(r.Context())
		if principal == nil || principal.ActorID() == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}

		store, id, ok := decisionTarget(w, r, db)
		if !ok {
			return
		}

		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body)

		current, err := subjectForArtifactOfDecision(r.Context(), db, store, scope.TenantID(), id)
		if err != nil {
			writeReviewError(w, err)
			return
		}

		res, err := store.Override(r.Context(), scope.TenantID(), principal.ActorID(), current,
			review.OverrideRequest{
				DecisionID: id, Reason: body.Reason,
				Role: rolesOf(principal, scope.TenantID()),
			})
		if err != nil {
			writeReviewError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func listOverrides(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadEvidence)
		if serr != nil {
			serr.write(w)
			return
		}
		store, err := reviewStoreFor(db)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "review_unavailable"})
			return
		}
		req, perr := parsePage(r)
		if perr != nil {
			writeJSON(w, http.StatusBadRequest,
				map[string]any{"error": "bad_page", "detail": perr.Error()})
			return
		}
		records, err := store.OverridesPage(r.Context(), scope.TenantID(), req.After, req.Limit+1)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		writeJSON(w, http.StatusOK,
			finish(records, req, func(o review.OverrideRecord) int64 { return o.ID }))
	}
}

func getReleasePolicy(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		store, err := reviewStoreFor(db)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "review_unavailable"})
			return
		}
		p, err := store.Policy(r.Context(), scope.TenantID())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func setReleasePolicy(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermManageReleasePolicy)
		if serr != nil {
			serr.write(w)
			return
		}
		principal := auth.FromContext(r.Context())
		if principal == nil || principal.ActorID() == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		store, err := reviewStoreFor(db)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "review_unavailable"})
			return
		}

		var body struct {
			MinApprovals       int  `json:"minApprovals"`
			SeparationOfDuties bool `json:"separationOfDuties"`
			OverrideAllowed    bool `json:"overrideAllowed"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
			return
		}

		p := review.Policy{
			TenantID:           scope.TenantID(),
			MinApprovals:       body.MinApprovals,
			SeparationOfDuties: body.SeparationOfDuties,
			OverrideAllowed:    body.OverrideAllowed,
		}
		if err := store.SetPolicy(r.Context(), principal.ActorID(), p); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid_policy", "message": err.Error()})
			return
		}
		// Changing dual control is itself a control change, so it is evidence.
		if _, err := AppendAuditEvent(db, scope.TenantID(), "RELEASE_POLICY_CHANGED",
			principal.ActorID(), map[string]any{
				"minApprovals":       p.MinApprovals,
				"separationOfDuties": p.SeparationOfDuties,
				"overrideAllowed":    p.OverrideAllowed,
			}); err != nil {
			log.Printf("review: could not record the policy change for tenant %s: %v",
				scope.TenantID(), err)
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func decisionTarget(w http.ResponseWriter, r *http.Request, db *sql.DB) (*review.Store, int64, bool) {
	store, err := reviewStoreFor(db)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "review_unavailable"})
		return nil, 0, false
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		// The same shape as a decision belonging to another tenant, so a
		// malformed id and a foreign id are indistinguishable.
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "decision_not_found"})
		return nil, 0, false
	}
	return store, id, true
}

func subjectForArtifactOfDecision(ctx context.Context, db *sql.DB, store *review.Store, tenantID string, id int64) (review.Subject, error) {
	d, err := store.Get(ctx, tenantID, id)
	if err != nil {
		return review.Subject{}, err
	}
	return subjectForArtifact(ctx, db, tenantID, d.ArtifactID)
}

func writeReviewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, review.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": "decision_not_found", "message": "decision not found in this tenant"})
	case errors.Is(err, review.ErrStale):
		// 409: the request was well formed and the world moved. The message
		// names what changed, because "the approval expired" is not actionable.
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "decision_stale", "message": err.Error()})
	case errors.Is(err, review.ErrSelfApproval):
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "separation_of_duties", "message": err.Error()})
	case errors.Is(err, review.ErrAlreadyVoted):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "already_voted", "message": err.Error()})
	case errors.Is(err, review.ErrNotApproved):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "insufficient_approvals", "message": err.Error()})
	case errors.Is(err, review.ErrOverrideNotAllowed):
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "override_forbidden", "message": err.Error()})
	case errors.Is(err, review.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "conflict", "message": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "decision_rejected", "message": err.Error()})
	}
}

// proposeRelease records the validation run and the decision resting on it.
//
// Called from the validation worker inside the job's transaction, so the run,
// the decision and the artifact's new state commit together.
func proposeRelease(
	ctx context.Context, tx *sql.Tx, store *review.Store,
	tenantID string, artifactID int64, sha string,
	result *nacha.Result, decision nacha.Decision, policy review.Policy,
) error {
	subject := review.Subject{
		ArtifactID:      artifactID,
		ArtifactSHA256:  sha,
		PolicyVersion:   decision.PolicyVersion,
		RulePackVersion: nacha.PolicyVersion,
		ContractID:      decision.ContractID,
		ContractVersion: decision.ContractVersion,
		Outcome:         string(decision.Outcome),
		ParserName:      "internal/nacha",
		ParserVersion:   nacha.PolicyVersion,
		RecordsParsed:   result.RecordsParsed,
	}
	for _, f := range result.Findings {
		subject.Findings = append(subject.Findings, review.Finding{
			RuleID: f.RuleID, Severity: string(f.Severity),
		})
	}

	// The proposer is the worker, not a person, so separation of duties does
	// not exclude any human reviewer. A file nobody proposed is a file everyone
	// may review.
	_, err := store.ProposeTx(ctx, tx, tenantID, "system:validation-worker", subject, policy)
	return err
}

// rolesOf renders the roles a principal holds in a tenant, for the record.
//
// All of them, joined, rather than a single "primary" role. A person acting as
// both reviewer and supervisor did so with both authorities, and picking one to
// record would understate what they were permitted to do.
func rolesOf(p *auth.Principal, tenantID string) string {
	roles := p.RolesIn(tenantID)
	parts := make([]string, 0, len(roles))
	for _, r := range roles {
		parts = append(parts, string(r))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}
