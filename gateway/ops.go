package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
)

// The read API the operations UI is built on.
//
// Everything here is a projection of stored state. Nothing in this file decides
// anything: no status is computed that the database does not already hold, no
// field is synthesised to fill a gap, and no handler returns a plausible value
// when a query fails. A screen that showed a value this file invented would be
// the exact defect Prompt 01 removed, arriving back through the read path.

func registerOperationsRoutes(r chi.Router, db *sql.DB, cfg *Config) {
	r.Get("/session", describeSession(cfg))
	r.Get("/artifacts", listArtifacts(db))
	r.Get("/artifacts/{id}", getArtifact(db))
	r.Get("/contracts/{id}/versions", listContractVersions(db))
	r.Get("/evidence", listEvidence(db))
	r.Get("/service-health", serviceHealth(db, cfg))
}

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

// uiPermissions is the set the operations UI asks about.
//
// Enumerated rather than derived from the role table so that adding a
// permission does not silently start advertising it to every browser. What the
// UI is told it may do is a deliberate list.
var uiPermissions = []auth.Permission{
	auth.PermReadTenant,
	auth.PermReadEvidence,
	auth.PermUploadArtifact,
	auth.PermApproveRelease,
	auth.PermOverrideRelease,
	auth.PermManageContract,
	auth.PermManageReleasePolicy,
	auth.PermManageSecret,
}

type sessionResponse struct {
	Subject string `json:"subject"`
	Issuer  string `json:"issuer"`
	Email   string `json:"email,omitempty"`

	TenantID string   `json:"tenantId"`
	Tenants  []string `json:"tenants"`
	Roles    []string `json:"roles"`

	// Permissions is what this caller actually holds in this tenant, answered
	// by the same Authorize call the handlers use.
	//
	// The UI uses it to render a control as unavailable-with-a-reason instead
	// of offering it and reporting a 403 afterwards. It is a *presentation*
	// hint and nothing more: every route re-authorizes, so a client that lies
	// to itself about this list gains nothing.
	Permissions []string `json:"permissions"`

	Profile string `json:"profile"`
	// Demo is surfaced so the UI can label itself. A demo build that looked
	// like production is the failure mode this whole rehabilitation started
	// from.
	Demo      bool   `json:"demo"`
	ServerNow string `json:"serverNow"`
}

func describeSession(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		p := auth.FromContext(r.Context())
		if p == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}

		out := sessionResponse{
			Subject:  p.Subject,
			Issuer:   p.Issuer,
			Email:    p.Email,
			TenantID: scope.TenantID(),
			Tenants:  p.Tenants(),
			Profile:  string(cfg.Profile),
			Demo:     cfg.IsDemo(),
			// The server's clock, so the UI can say how stale a view is
			// without trusting the browser's clock -- which is settable by the
			// person reading the screen.
			ServerNow: time.Now().UTC().Format(time.RFC3339),
		}
		for _, role := range p.RolesIn(scope.TenantID()) {
			out.Roles = append(out.Roles, string(role))
		}
		for _, perm := range uiPermissions {
			if p.Authorize(scope.TenantID(), perm) == nil {
				out.Permissions = append(out.Permissions, string(perm))
			}
		}
		if out.Roles == nil {
			out.Roles = []string{}
		}
		if out.Permissions == nil {
			out.Permissions = []string{}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// ---------------------------------------------------------------------------
// Artifacts
// ---------------------------------------------------------------------------

type artifactSummary struct {
	ID        int64  `json:"id"`
	Filename  string `json:"filename"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
	Status    string `json:"status"`

	ReceivedAt string `json:"receivedAt"`
	UpdatedAt  string `json:"updatedAt"`

	ExpectationID *int64 `json:"expectationId,omitempty"`
	ContractID    string `json:"contractId,omitempty"`
	PartnerName   string `json:"partnerName,omitempty"`

	// Counts by severity, so the list can show that a file has blocking
	// findings without shipping the findings themselves to a list view.
	BlockingFindings int `json:"blockingFindings"`
	WarningFindings  int `json:"warningFindings"`

	// DecisionID is present when a release decision exists for this artifact.
	DecisionID    *int64 `json:"decisionId,omitempty"`
	DecisionState string `json:"decisionState,omitempty"`
}

// artifactStatuses is the allowlist for the status filter.
//
// A filter value that reached the query would be a way to ask the database
// questions the API does not offer, so the parameter selects from a fixed set
// and an unknown value is refused rather than ignored. Ignoring it would return
// the unfiltered list, which reads as "there are no quarantined files" when the
// operator asked for exactly those.
var artifactStatuses = map[string]bool{
	"RECEIVED": true, "VALIDATING": true, "VALIDATED": true,
	"QUARANTINED": true, "APPROVED": true, "RELEASED": true, "REJECTED": true,
}

func listArtifacts(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		req, err := parsePage(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest,
				map[string]any{"error": "bad_page", "detail": err.Error()})
			return
		}

		where := []string{"f.tenant_id = ?"}
		args := []any{scope.TenantID()}

		if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
			// Several statuses may be named. Each is checked; one bad value
			// fails the request.
			var placeholders []string
			for _, s := range strings.Split(raw, ",") {
				s = strings.ToUpper(strings.TrimSpace(s))
				if !artifactStatuses[s] {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error": "unknown_status", "detail": "no such artifact status: " + s})
					return
				}
				placeholders = append(placeholders, "?")
				args = append(args, s)
			}
			where = append(where, "f.status IN ("+strings.Join(placeholders, ",")+")")
		}

		// Filename search. Bound the pattern and escape the wildcards, so a
		// query of "%" does not become "match every row" and a query of "_"
		// does not silently match one character of anything.
		if q := strings.TrimSpace(r.URL.Query().Get("filename")); q != "" {
			if len(q) > 128 {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "filter_too_long", "detail": "filename filter is limited to 128 characters"})
				return
			}
			esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
			where = append(where, `f.filename LIKE ? ESCAPE '\'`)
			args = append(args, "%"+esc+"%")
		}

		if req.After > 0 {
			where = append(where, "f.id < ?")
			args = append(args, req.After)
		}
		args = append(args, req.Limit+1)

		rows, err := db.QueryContext(r.Context(), `
			SELECT f.id, f.filename, f.sha256_hash, f.size_bytes, f.status,
			       f.received_at, f.updated_at, f.expectation_id,
			       COALESCE(c.id, ''), COALESCE(p.name, ''),
			       (SELECT COUNT(*) FROM validation_findings vf
			          WHERE vf.tenant_id = f.tenant_id AND vf.file_instance_id = f.id
			            AND vf.severity = 'BLOCKING'),
			       (SELECT COUNT(*) FROM validation_findings vf
			          WHERE vf.tenant_id = f.tenant_id AND vf.file_instance_id = f.id
			            AND vf.severity = 'WARNING'),
			       (SELECT d.id FROM policy_decisions d
			          WHERE d.tenant_id = f.tenant_id AND d.file_instance_id = f.id
			          ORDER BY d.id DESC LIMIT 1),
			       COALESCE((SELECT d.state FROM policy_decisions d
			          WHERE d.tenant_id = f.tenant_id AND d.file_instance_id = f.id
			          ORDER BY d.id DESC LIMIT 1), '')
			FROM file_instances f
			LEFT JOIN expectations e ON e.id = f.expectation_id AND e.tenant_id = f.tenant_id
			LEFT JOIN file_contracts c ON c.id = e.contract_id AND c.tenant_id = f.tenant_id
			LEFT JOIN partners p ON p.id = c.partner_id AND p.tenant_id = f.tenant_id
			WHERE `+strings.Join(where, " AND ")+`
			ORDER BY f.id DESC
			LIMIT ?`, args...)
		if err != nil {
			log.Printf("ops: artifact list failed for tenant %s: %v", scope.TenantID(), err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		defer rows.Close()

		var items []artifactSummary
		var skipped int
		for rows.Next() {
			var (
				a        artifactSummary
				expID    sql.NullInt64
				decID    sql.NullInt64
				received time.Time
				updated  time.Time
				contract string
				partner  string
				blocking int
				warning  int
				decState string
			)
			if err := rows.Scan(&a.ID, &a.Filename, &a.SHA256, &a.SizeBytes, &a.Status,
				&received, &updated, &expID, &contract, &partner,
				&blocking, &warning, &decID, &decState); err != nil {
				// Counted, not dropped in silence. The response says the page
				// is partial, because a list that quietly omits a quarantined
				// artifact is worse than one that admits it is incomplete.
				log.Printf("ops: unreadable artifact row for tenant %s: %v", scope.TenantID(), err)
				skipped++
				continue
			}
			a.ReceivedAt = received.UTC().Format(time.RFC3339)
			a.UpdatedAt = updated.UTC().Format(time.RFC3339)
			if expID.Valid {
				a.ExpectationID = &expID.Int64
			}
			if decID.Valid {
				a.DecisionID = &decID.Int64
			}
			a.ContractID, a.PartnerName = contract, partner
			a.BlockingFindings, a.WarningFindings = blocking, warning
			a.DecisionState = decState
			items = append(items, a)
		}
		if err := rows.Err(); err != nil {
			log.Printf("ops: artifact list iteration failed for tenant %s: %v", scope.TenantID(), err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}

		out := finish(items, req, func(a artifactSummary) int64 { return a.ID })
		if skipped > 0 {
			out.Partial = true
			out.PartialReason = strconv.Itoa(skipped) + " row(s) on this page could not be read"
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type artifactDetail struct {
	artifactSummary

	StoragePath string `json:"storagePath,omitempty"`
	RowVersion  int64  `json:"rowVersion"`

	Run      *validationRunView `json:"validationRun"`
	Findings []findingView      `json:"findings"`
	History  []transitionView   `json:"history"`

	// NotCheckedRuleIDs is why silence does not imply coverage. Carried
	// through from the run so the detail screen can say what was not evaluated.
	NotCheckedRuleIDs []string `json:"notCheckedRuleIds,omitempty"`
}

type validationRunView struct {
	ID              int64  `json:"id"`
	ParserName      string `json:"parserName"`
	ParserVersion   string `json:"parserVersion"`
	RulePackVersion string `json:"rulePackVersion"`
	PolicyVersion   string `json:"policyVersion"`
	ContractID      string `json:"contractId,omitempty"`
	ContractVersion string `json:"contractVersion,omitempty"`
	Outcome         string `json:"outcome"`
	ParserOK        bool   `json:"parserOk"`
	FindingsDigest  string `json:"findingsDigest"`
	BlockingRuleIDs string `json:"blockingRuleIds,omitempty"`

	RecordsParsed     int    `json:"recordsParsed"`
	TotalDebitsMinor  int64  `json:"totalDebitsMinor"`
	TotalCreditsMinor int64  `json:"totalCreditsMinor"`
	StartedAt         string `json:"startedAt"`
	CompletedAt       string `json:"completedAt,omitempty"`
}

type findingView struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	RuleVersion string `json:"ruleVersion"`
	Provenance  string `json:"provenance"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	LineNumber  int    `json:"lineNumber"`
	ByteOffset  int    `json:"byteOffset"`
	FieldStart  int    `json:"fieldStart"`
	FieldEnd    int    `json:"fieldEnd"`
	// Redacted where it was written. There is no un-redacted form to leak.
	Evidence string `json:"evidence,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

type transitionView struct {
	FromState  string `json:"fromState"`
	ToState    string `json:"toState"`
	ActorID    string `json:"actorId"`
	Reason     string `json:"reason,omitempty"`
	OccurredAt string `json:"occurredAt"`
}

func getArtifact(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_artifact_id"})
			return
		}

		var (
			d        artifactDetail
			expID    sql.NullInt64
			received time.Time
			updated  time.Time
		)
		// The tenant filter is in the WHERE clause, not applied after the read.
		// A not-found and a belongs-to-another-tenant are the same response for
		// the same reason a scope denial is: distinguishing them enumerates
		// what exists elsewhere.
		err = db.QueryRowContext(r.Context(), `
			SELECT f.id, f.filename, f.sha256_hash, f.size_bytes, f.status, f.storage_path,
			       f.row_version, f.received_at, f.updated_at, f.expectation_id,
			       COALESCE(c.id, ''), COALESCE(p.name, '')
			FROM file_instances f
			LEFT JOIN expectations e ON e.id = f.expectation_id AND e.tenant_id = f.tenant_id
			LEFT JOIN file_contracts c ON c.id = e.contract_id AND c.tenant_id = f.tenant_id
			LEFT JOIN partners p ON p.id = c.partner_id AND p.tenant_id = f.tenant_id
			WHERE f.tenant_id = ? AND f.id = ?`, scope.TenantID(), id).
			Scan(&d.ID, &d.Filename, &d.SHA256, &d.SizeBytes, &d.Status, &d.StoragePath,
				&d.RowVersion, &received, &updated, &expID, &d.ContractID, &d.PartnerName)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
			return
		}
		if err != nil {
			log.Printf("ops: artifact %d read failed for tenant %s: %v", id, scope.TenantID(), err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		d.ReceivedAt = received.UTC().Format(time.RFC3339)
		d.UpdatedAt = updated.UTC().Format(time.RFC3339)
		if expID.Valid {
			d.ExpectationID = &expID.Int64
		}

		d.Run = latestRun(r.Context(), db, scope.TenantID(), id)
		d.Findings = findingsFor(r.Context(), db, scope.TenantID(), id)
		for _, f := range d.Findings {
			switch f.Severity {
			case "BLOCKING":
				d.BlockingFindings++
			case "WARNING":
				d.WarningFindings++
			}
		}
		d.History = historyFor(r.Context(), db, scope.TenantID(), "artifact", id)

		var decID sql.NullInt64
		var decState sql.NullString
		_ = db.QueryRowContext(r.Context(),
			`SELECT id, state FROM policy_decisions
			 WHERE tenant_id = ? AND file_instance_id = ? ORDER BY id DESC LIMIT 1`,
			scope.TenantID(), id).Scan(&decID, &decState)
		if decID.Valid {
			d.DecisionID = &decID.Int64
			d.DecisionState = decState.String
		}

		writeJSON(w, http.StatusOK, d)
	}
}

func latestRun(ctx context.Context, db *sql.DB, tenantID string, artifactID int64) *validationRunView {
	var (
		v           validationRunView
		parserOK    int
		contractID  sql.NullString
		contractVer sql.NullString
		blocking    sql.NullString
		started     time.Time
		completed   sql.NullTime
	)
	err := db.QueryRowContext(ctx, `
		SELECT id, parser_name, parser_version, rule_pack_version, parser_ok,
		       records_parsed, total_debits_minor, total_credits_minor,
		       policy_version, contract_id, contract_version, outcome,
		       findings_digest, blocking_rule_ids, started_at, completed_at
		FROM validation_runs
		WHERE tenant_id = ? AND file_instance_id = ?
		ORDER BY started_at DESC, id DESC LIMIT 1`, tenantID, artifactID).
		Scan(&v.ID, &v.ParserName, &v.ParserVersion, &v.RulePackVersion, &parserOK,
			&v.RecordsParsed, &v.TotalDebitsMinor, &v.TotalCreditsMinor,
			&v.PolicyVersion, &contractID, &contractVer, &v.Outcome,
			&v.FindingsDigest, &blocking, &started, &completed)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("ops: validation run read failed for artifact %d: %v", artifactID, err)
		}
		// nil is a state the UI renders: "not validated yet" is different from
		// "validated with no findings", and the difference is the whole point
		// of the screen.
		return nil
	}
	v.ParserOK = parserOK != 0
	v.ContractID, v.ContractVersion = contractID.String, contractVer.String
	v.BlockingRuleIDs = blocking.String
	v.StartedAt = started.UTC().Format(time.RFC3339)
	if completed.Valid {
		v.CompletedAt = completed.Time.UTC().Format(time.RFC3339)
	}
	return &v
}

func findingsFor(ctx context.Context, db *sql.DB, tenantID string, artifactID int64) []findingView {
	out := []findingView{}
	rows, err := db.QueryContext(ctx, `
		SELECT id, code, rule_version, provenance, description, severity,
		       COALESCE(line_number, 0), byte_offset, field_start, field_end,
		       evidence_redacted, expected_value, actual_value
		FROM validation_findings
		WHERE tenant_id = ? AND file_instance_id = ?
		ORDER BY
		  CASE severity WHEN 'BLOCKING' THEN 0 WHEN 'WARNING' THEN 1 ELSE 2 END,
		  COALESCE(line_number, 0), id
		LIMIT ?`, tenantID, artifactID, maxPageLimit)
	if err != nil {
		log.Printf("ops: findings read failed for artifact %d: %v", artifactID, err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var f findingView
		if err := rows.Scan(&f.ID, &f.Code, &f.RuleVersion, &f.Provenance, &f.Description,
			&f.Severity, &f.LineNumber, &f.ByteOffset, &f.FieldStart, &f.FieldEnd,
			&f.Evidence, &f.Expected, &f.Actual); err != nil {
			continue
		}
		out = append(out, f)
	}
	return out
}

func historyFor(ctx context.Context, db *sql.DB, tenantID, objectType string, objectID int64) []transitionView {
	out := []transitionView{}
	rows, err := db.QueryContext(ctx, `
		SELECT from_state, to_state, actor_id, COALESCE(reason, ''), occurred_at
		FROM status_history
		WHERE tenant_id = ? AND object_type = ? AND object_id = ?
		ORDER BY occurred_at ASC, id ASC
		LIMIT ?`, tenantID, objectType, objectID, maxPageLimit)
	if err != nil {
		log.Printf("ops: history read failed for %s %d: %v", objectType, objectID, err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var t transitionView
		var at time.Time
		if err := rows.Scan(&t.FromState, &t.ToState, &t.ActorID, &t.Reason, &at); err != nil {
			continue
		}
		t.OccurredAt = at.UTC().Format(time.RFC3339)
		out = append(out, t)
	}
	return out
}

// ---------------------------------------------------------------------------
// Contract version history
// ---------------------------------------------------------------------------

type contractVersionView struct {
	ID              int64  `json:"id"`
	Version         int    `json:"version"`
	FilenamePattern string `json:"filenamePattern"`
	Format          string `json:"format"`
	ExpectedLocal   string `json:"expectedLocal"`
	Timezone        string `json:"timezone"`
	GraceMinutes    int    `json:"graceMinutes"`
	CalendarID      string `json:"calendarId,omitempty"`
	BalancedMode    string `json:"balancedMode"`
	EffectiveFrom   string `json:"effectiveFrom"`
	EffectiveTo     string `json:"effectiveTo,omitempty"`
	CreatedAt       string `json:"createdAt"`
	// Current says this version governs now. Derived from the effective window
	// against the server's clock, because a UI computing it from the browser's
	// clock would disagree with the scheduler that actually used it.
	Current bool `json:"current"`
}

func listContractVersions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		contractID := strings.TrimSpace(chi.URLParam(r, "id"))
		if contractID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad_contract_id"})
			return
		}

		// Confirm the contract is this tenant's before listing anything about
		// it, so a version list cannot be used to probe for contract ids.
		var exists int
		err := db.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM file_contracts WHERE tenant_id = ? AND id = ?`,
			scope.TenantID(), contractID).Scan(&exists)
		if err != nil || exists == 0 {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
			return
		}

		rows, err := db.QueryContext(r.Context(), `
			SELECT id, version, filename_pattern, format, expected_local, timezone,
			       grace_minutes, calendar_id, balanced_mode,
			       effective_from, effective_to, created_at
			FROM file_contract_versions
			WHERE tenant_id = ? AND contract_id = ?
			ORDER BY version DESC
			LIMIT ?`, scope.TenantID(), contractID, maxPageLimit)
		if err != nil {
			log.Printf("ops: contract versions read failed for %s: %v", contractID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		defer rows.Close()

		now := time.Now().UTC()
		out := []contractVersionView{}
		for rows.Next() {
			var (
				v        contractVersionView
				calendar sql.NullString
				from     time.Time
				to       sql.NullTime
				created  time.Time
			)
			if err := rows.Scan(&v.ID, &v.Version, &v.FilenamePattern, &v.Format,
				&v.ExpectedLocal, &v.Timezone, &v.GraceMinutes, &calendar,
				&v.BalancedMode, &from, &to, &created); err != nil {
				continue
			}
			v.CalendarID = calendar.String
			v.EffectiveFrom = from.UTC().Format(time.RFC3339)
			v.CreatedAt = created.UTC().Format(time.RFC3339)
			v.Current = !from.After(now)
			if to.Valid {
				v.EffectiveTo = to.Time.UTC().Format(time.RFC3339)
				if !to.Time.After(now) {
					v.Current = false
				}
			}
			out = append(out, v)
		}
		writeJSON(w, http.StatusOK, map[string]any{"contractId": contractID, "versions": out})
	}
}

// ---------------------------------------------------------------------------
// Evidence timeline
// ---------------------------------------------------------------------------

type evidenceEntry struct {
	ID            int64           `json:"id"`
	SequenceNo    int64           `json:"sequenceNo"`
	EventType     string          `json:"eventType"`
	Actor         string          `json:"actor"`
	ObjectType    string          `json:"objectType,omitempty"`
	ObjectID      *int64          `json:"objectId,omitempty"`
	CorrelationID string          `json:"correlationId,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	PreviousHash  string          `json:"previousHash"`
	CurrentHash   string          `json:"currentHash"`
	CreatedAt     string          `json:"createdAt"`
}

// listEvidence pages the append-only chain.
//
// The whole-chain read that /ledger performs also verifies the hash links, and
// that verification is the reason it cannot be paged: a page of a chain proves
// nothing about the chain. So these are two different endpoints answering two
// different questions -- "show me what happened" and "is the record intact" --
// and this one says so rather than implying the second.
func listEvidence(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadEvidence)
		if serr != nil {
			serr.write(w)
			return
		}
		req, err := parsePage(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest,
				map[string]any{"error": "bad_page", "detail": err.Error()})
			return
		}

		where := []string{"tenant_id = ?"}
		args := []any{scope.TenantID()}

		if t := strings.TrimSpace(r.URL.Query().Get("eventType")); t != "" {
			if len(t) > 64 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "filter_too_long"})
				return
			}
			where = append(where, "event_type = ?")
			args = append(args, t)
		}
		if c := strings.TrimSpace(r.URL.Query().Get("correlationId")); c != "" {
			if len(c) > 128 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "filter_too_long"})
				return
			}
			where = append(where, "correlation_id = ?")
			args = append(args, c)
		}
		if req.After > 0 {
			where = append(where, "id < ?")
			args = append(args, req.After)
		}
		args = append(args, req.Limit+1)

		rows, err := db.QueryContext(r.Context(), `
			SELECT id, sequence_no, event_type, actor, object_type, object_id,
			       correlation_id, payload, previous_hash, current_hash, created_at
			FROM audit_events
			WHERE `+strings.Join(where, " AND ")+`
			ORDER BY id DESC
			LIMIT ?`, args...)
		if err != nil {
			log.Printf("ops: evidence read failed for tenant %s: %v", scope.TenantID(), err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		defer rows.Close()

		var items []evidenceEntry
		for rows.Next() {
			var (
				e         evidenceEntry
				objType   sql.NullString
				objID     sql.NullInt64
				corrID    sql.NullString
				payload   string
				createdAt time.Time
			)
			if err := rows.Scan(&e.ID, &e.SequenceNo, &e.EventType, &e.Actor,
				&objType, &objID, &corrID, &payload,
				&e.PreviousHash, &e.CurrentHash, &createdAt); err != nil {
				continue
			}
			e.ObjectType, e.CorrelationID = objType.String, corrID.String
			if objID.Valid {
				e.ObjectID = &objID.Int64
			}
			// The payload is stored as the canonical JSON that was hashed. It
			// is passed through rather than re-encoded, because re-encoding it
			// would change the bytes the hash was taken over and make the
			// displayed record differ from the one that was signed.
			if json.Valid([]byte(payload)) {
				e.Payload = json.RawMessage(payload)
			} else {
				e.Payload = json.RawMessage(`null`)
			}
			e.CreatedAt = createdAt.UTC().Format(time.RFC3339)
			items = append(items, e)
		}
		writeJSON(w, http.StatusOK, finish(items, req, func(e evidenceEntry) int64 { return e.ID }))
	}
}

// ---------------------------------------------------------------------------
// Measured service health
// ---------------------------------------------------------------------------

type serviceHealthResponse struct {
	Profile   string `json:"profile"`
	Demo      bool   `json:"demo"`
	ServerNow string `json:"serverNow"`

	Database    componentHealth `json:"database"`
	Queue       *queueHealth    `json:"queue"`
	Outbox      *outboxHealth   `json:"outbox"`
	Scheduler   componentHealth `json:"scheduler"`
	ObjectStore componentHealth `json:"objectStore"`
	AITier      componentHealth `json:"aiTier"`
}

type componentHealth struct {
	// Status is one of OK, DEGRADED, UNAVAILABLE, NOT_CONFIGURED, UNKNOWN.
	//
	// NOT_CONFIGURED and UNKNOWN are separate from OK on purpose. A dependency
	// nobody configured is not healthy, and one nobody measured is not healthy
	// either -- reporting either as OK is how a dashboard tells an operator
	// everything is fine while nothing is running.
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Measured  bool   `json:"measured"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
}

type queueHealth struct {
	Queued    int `json:"queued"`
	Leased    int `json:"leased"`
	Running   int `json:"running"`
	Retryable int `json:"retryable"`
	Dead      int `json:"dead"`
	// OldestQueuedAgeSeconds is the backlog's age, which is what an operator
	// actually needs: a queue depth of 3 that has not moved in two hours is a
	// worse condition than a depth of 300 that is draining.
	OldestQueuedAgeSeconds *int64 `json:"oldestQueuedAgeSeconds"`
}

type outboxHealth struct {
	Undelivered int `json:"undelivered"`
	Dead        int `json:"dead"`
	// OldestUndeliveredAgeSeconds surfaces the last-mile gap the review and
	// breach work both left open: events are published and, with no subscriber
	// wired up, this number grows.
	OldestUndeliveredAgeSeconds *int64 `json:"oldestUndeliveredAgeSeconds"`
}

func serviceHealth(db *sql.DB, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope, serr := resolveScope(r, auth.PermReadTenant)
		if serr != nil {
			serr.write(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		out := serviceHealthResponse{
			Profile:   string(cfg.Profile),
			Demo:      cfg.IsDemo(),
			ServerNow: time.Now().UTC().Format(time.RFC3339),
		}

		start := time.Now()
		if err := db.PingContext(ctx); err != nil {
			out.Database = componentHealth{
				Status: "UNAVAILABLE", Detail: "database ping failed", Measured: true}
			// Nothing below can be measured without the database, and the
			// zero value of each of these is UNKNOWN rather than OK.
			out.Scheduler = componentHealth{Status: "UNKNOWN", Detail: "database unavailable"}
			writeJSON(w, http.StatusServiceUnavailable, out)
			return
		}
		out.Database = componentHealth{
			Status: "OK", Measured: true, LatencyMs: time.Since(start).Milliseconds()}

		out.Queue = queueDepth(ctx, db, scope.TenantID())
		out.Outbox = outboxDepth(ctx, db, scope.TenantID())
		out.Scheduler = schedulerHealth(ctx, db, scope.TenantID())

		if cfg.ObjectStoreURL == "" {
			out.ObjectStore = componentHealth{Status: "NOT_CONFIGURED"}
		} else {
			// Configured is not the same as reachable, and this endpoint does
			// not reach out to find out. Saying CONFIGURED rather than OK is
			// the difference between a fact and a guess.
			out.ObjectStore = componentHealth{Status: "UNKNOWN", Detail: "configured; not probed"}
		}
		if cfg.AITierURL == "" {
			out.AITier = componentHealth{Status: "NOT_CONFIGURED", Detail: "optional; ingestion does not depend on it"}
		} else {
			out.AITier = componentHealth{Status: "UNKNOWN", Detail: "configured; not probed"}
		}

		writeJSON(w, http.StatusOK, out)
	}
}

func queueDepth(ctx context.Context, db *sql.DB, tenantID string) *queueHealth {
	q := &queueHealth{}
	rows, err := db.QueryContext(ctx,
		`SELECT state, COUNT(*) FROM ingestion_jobs WHERE tenant_id = ? GROUP BY state`, tenantID)
	if err != nil {
		log.Printf("ops: queue depth failed for tenant %s: %v", tenantID, err)
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			continue
		}
		switch state {
		case "QUEUED":
			q.Queued = n
		case "LEASED":
			q.Leased = n
		case "RUNNING":
			q.Running = n
		case "RETRYABLE":
			q.Retryable = n
		case "DEAD":
			q.Dead = n
		}
	}

	var oldest sql.NullTime
	if err := db.QueryRowContext(ctx,
		`SELECT MIN(created_at) FROM ingestion_jobs
		 WHERE tenant_id = ? AND state IN ('QUEUED','RETRYABLE')`, tenantID).Scan(&oldest); err == nil && oldest.Valid {
		age := int64(time.Since(oldest.Time).Seconds())
		if age < 0 {
			age = 0
		}
		q.OldestQueuedAgeSeconds = &age
	}
	return q
}

func outboxDepth(ctx context.Context, db *sql.DB, tenantID string) *outboxHealth {
	o := &outboxHealth{}
	err := db.QueryRowContext(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE delivered_at IS NULL AND dead_at IS NULL),
		  COUNT(*) FILTER (WHERE dead_at IS NOT NULL)
		FROM outbox_events WHERE tenant_id = ?`, tenantID).Scan(&o.Undelivered, &o.Dead)
	if err != nil {
		log.Printf("ops: outbox depth failed for tenant %s: %v", tenantID, err)
		return nil
	}
	var oldest sql.NullTime
	if err := db.QueryRowContext(ctx,
		`SELECT MIN(created_at) FROM outbox_events
		 WHERE tenant_id = ? AND delivered_at IS NULL AND dead_at IS NULL`, tenantID).
		Scan(&oldest); err == nil && oldest.Valid {
		age := int64(time.Since(oldest.Time).Seconds())
		if age < 0 {
			age = 0
		}
		o.OldestUndeliveredAgeSeconds = &age
	}
	return o
}

// schedulerHealth reports whether the horizon is materialized, not whether the
// scheduler process is alive.
//
// Liveness of a goroutine tells an operator nothing they can act on. "The next
// expected file is 9 days out and the horizon is 14 days" tells them the
// scheduler ran; "there are no expectations at all" tells them it did not, or
// that no contract governs anything.
func schedulerHealth(ctx context.Context, db *sql.DB, tenantID string) componentHealth {
	var total int
	var furthest sql.NullTime
	// business_date is written by the scheduler as a UTC-midnight timestamp,
	// so the bound is one too. Comparing it against a "2006-01-02" string
	// works on a text-stored column and silently matches nothing on a
	// timestamp-stored one, which would report a healthy scheduler as
	// DEGRADED on exactly the deployments that matter.
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(business_date) FROM expectations
		WHERE tenant_id = ? AND business_date >= ?`,
		tenantID, today).Scan(&total, &furthest)
	if err != nil {
		return componentHealth{Status: "UNKNOWN", Detail: "expectation horizon could not be read"}
	}
	if total == 0 {
		return componentHealth{
			Status:   "DEGRADED",
			Detail:   "no expectations are materialized for today or later",
			Measured: true,
		}
	}
	h := componentHealth{Status: "OK", Measured: true}
	if furthest.Valid {
		days := int(furthest.Time.Sub(today).Hours() / 24)
		h.Detail = "horizon materialized to " + furthest.Time.UTC().Format("2006-01-02") +
			" (" + strconv.Itoa(days) + " day(s) ahead); " + strconv.Itoa(total) + " expectation(s) pending"
	}
	return h
}

// expectationStatuses is the allowlist for the SLA board's status filter.
//
// Same reasoning as artifactStatuses: an unrecognised value is refused, not
// dropped. A board that answered "show me the breached feeds" with the
// unfiltered list would read as "nothing is breached" to the person least able
// to check.
var expectationStatuses = map[string]bool{
	"PENDING": true, "DUE": true, "OVERDUE": true, "BREACHED": true,
	"ARRIVED": true, "WAIVED": true,
}

// incidentStatuses is the allowlist for the incident list's status filter.
var incidentStatuses = map[string]bool{
	"OPEN": true, "INVESTIGATING": true, "RESOLVED": true, "CLOSED": true,
}
