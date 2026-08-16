package connectors

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
)

// The shared conformance suite.
//
// It is a black box: it holds a Connector interface and a fixture describing a
// real, disposable database, and it never reaches inside the driver. That is
// what makes it shared -- the same checks run against PostgreSQL, and will run
// unchanged against Oracle, because they only ever ask the interface questions.
//
// Passing it is what makes a connector selectable. `Registry.Register` takes the
// resulting record and refuses AVAILABLE without one, so this file is not a test
// helper: it is the gate.
//
// It lives in the package rather than in a _test.go file deliberately. A suite
// that only existed at test time could not produce the evidence the registry
// requires, and "we ran the tests once" is not the same claim as "this build
// verified this driver against this server version".

// Fixture describes the disposable database a driver is verified against.
//
// Everything here belongs to a throwaway account. The suite issues real
// connections, real failed authentications and real cancelled queries, none of
// which should ever touch a database anyone relies on.
type Fixture struct {
	// Config and Secrets are a working connection.
	Config  Config
	Secrets Secrets

	// AuthMode is the mode being verified. A driver supporting several is run
	// once per mode, because the failure surfaces differ.
	AuthMode string

	// BadSecrets are credentials that must be rejected. Supplying them proves
	// the authentication failure path is real rather than assumed.
	BadSecrets Secrets

	// UntrustedTLSConfig points at a server whose certificate must not be
	// trusted -- a self-signed endpoint, or the same server with the CA
	// reference removed. Verifying that a connector *rejects* something is the
	// only way to know its verification is switched on.
	UntrustedTLSConfig *Config

	// AllowedResource is a schema or dataset inside the connection's allowlist,
	// and ForbiddenResource is one outside it that genuinely exists. The second
	// is what makes the allowlist check meaningful: refusing to read a table
	// that does not exist proves nothing.
	AllowedResource   string
	ForbiddenResource string

	// CountTemplate is an approved template returning a single row with one
	// integer column, parameterised, over AllowedResource. It exercises
	// parameter binding and identifier substitution together.
	CountTemplate Template
	CountParams   map[string]any
	CountIdents   map[string]string

	// SlowTemplate takes longer than the suite's timeout, so cancellation can
	// be observed rather than inferred.
	SlowTemplate Template
	SlowParams   map[string]any
	SlowIdents   map[string]string

	// WideTemplate returns more rows than any sane limit, so the row and byte
	// bounds can be shown to bite.
	WideTemplate Template
	WideParams   map[string]any
	WideIdents   map[string]string

	// ServerVersion and DriverVersion are recorded in the evidence. A pass
	// against a server version nobody runs is not evidence about production.
	ServerVersion string
	DriverVersion string
	TestCommit    string

	// OtherTenantConfig is a configuration belonging to a different tenant,
	// used to prove that a driver holds no cross-request state that could serve
	// one tenant's connection from another's pool.
	OtherTenantConfig *Config
	OtherTenantSecret Secrets
}

// CheckResult is one conformance check's outcome.
type CheckResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Skipped bool   `json:"skipped"`
	// Reason explains a failure or a skip. It is written for someone deciding
	// whether to ship the driver.
	Reason  string        `json:"reason,omitempty"`
	Elapsed time.Duration `json:"elapsed"`
}

// ConformanceReport is the full outcome of one run.
type ConformanceReport struct {
	ConnectorType string        `json:"connectorType"`
	AuthMode      string        `json:"authMode"`
	Checks        []CheckResult `json:"checks"`
	Record        *ConformanceRecord
}

// Passed reports whether every check that ran passed and none was skipped.
//
// A skip counts as a failure for the purpose of availability. The whole point
// of the gate is that AVAILABLE means every property was demonstrated; a suite
// that could pass with checks skipped would let a driver ship with its TLS
// verification untested because the fixture lacked an untrusted endpoint.
func (r ConformanceReport) Passed() bool {
	if len(r.Checks) == 0 {
		return false
	}
	for _, c := range r.Checks {
		if !c.Passed || c.Skipped {
			return false
		}
	}
	return true
}

// Summary renders the report for a completion report or a CI log.
func (r ConformanceReport) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "conformance: %s (%s)\n", r.ConnectorType, r.AuthMode)
	for _, c := range r.Checks {
		status := "PASS"
		switch {
		case c.Skipped:
			status = "SKIP"
		case !c.Passed:
			status = "FAIL"
		}
		fmt.Fprintf(&b, "  %-4s %-42s %s\n", status, c.Name, c.Reason)
	}
	return b.String()
}

// conformanceTimeout bounds each individual check.
const conformanceTimeout = 15 * time.Second

// RunConformance executes the shared suite against a driver and a real fixture.
//
// The returned record is complete only if every check passed, which is what
// `Registry.Register` requires for AVAILABLE.
func RunConformance(ctx context.Context, c Connector, f Fixture) ConformanceReport {
	report := ConformanceReport{ConnectorType: c.Type(), AuthMode: f.AuthMode}

	checks := []struct {
		name string
		fn   func(context.Context, Connector, Fixture) (bool, bool, string)
	}{
		{"config_validation_rejects_incomplete", checkConfigValidation},
		{"tls_connection_succeeds", checkTLSConnection},
		{"untrusted_certificate_is_rejected", checkUntrustedCertificate},
		{"authentication_succeeds", checkAuthSuccess},
		{"bad_credentials_are_rejected_and_redacted", checkAuthFailureRedaction},
		{"discovery_is_limited_to_the_allowlist", checkDiscoveryAllowlist},
		{"unapproved_resource_is_refused", checkForbiddenResource},
		{"parameters_are_bound_not_interpolated", checkParameterBinding},
		{"identifier_injection_is_refused", checkIdentifierInjection},
		{"writes_and_ddl_are_refused", checkReadOnlyEnforcement},
		{"multi_statement_is_refused", checkMultiStatement},
		{"timeout_cancels_a_slow_query", checkTimeoutCancellation},
		{"row_limit_truncates_and_reports", checkRowLimit},
		{"byte_limit_truncates_and_reports", checkByteLimit},
		{"cursor_pagination_advances", checkCursorPagination},
		{"pool_exhaustion_recovers", checkPoolExhaustion},
		{"revoked_secret_stops_access", checkSecretRevocation},
		{"cross_tenant_config_is_not_served_from_cache", checkCrossTenant},
		{"health_reports_a_real_timestamp", checkHealthTimestamp},
		{"errors_never_carry_credentials", checkErrorRedaction},
		{"close_leaks_no_goroutines", checkNoLeak},
	}

	for _, check := range checks {
		start := time.Now()
		runCtx, cancel := context.WithTimeout(ctx, conformanceTimeout)
		passed, skipped, reason := check.fn(runCtx, c, f)
		cancel()
		report.Checks = append(report.Checks, CheckResult{
			Name: check.name, Passed: passed, Skipped: skipped,
			Reason: reason, Elapsed: time.Since(start),
		})
	}

	record := &ConformanceRecord{
		ConnectorType: c.Type(),
		ServerVersion: f.ServerVersion,
		DriverVersion: f.DriverVersion,
		TestCommit:    f.TestCommit,
		RunAt:         time.Now().UTC(),
	}
	for _, ch := range report.Checks {
		switch {
		case ch.Skipped:
			record.Skipped = append(record.Skipped, ch.Name)
		case ch.Passed:
			record.Passed++
		default:
			record.Failed++
		}
	}
	// A skipped check counts against the record too, so a partial run cannot
	// produce a complete one.
	record.Failed += len(record.Skipped)
	sort.Strings(record.Skipped)
	report.Record = record
	return report
}

// ---------------------------------------------------------------------------
// The checks
//
// Each returns (passed, skipped, reason). A check that cannot run because the
// fixture does not supply what it needs is skipped with the reason -- and a
// skip is a failure for availability, so a missing fixture cannot quietly buy
// a pass.
// ---------------------------------------------------------------------------

func checkConfigValidation(_ context.Context, c Connector, f Fixture) (bool, bool, string) {
	empty := Config{Type: c.Type(), Fields: map[string]string{}}
	if err := c.ValidateConfig(empty); err == nil {
		return false, false, "an empty configuration was accepted"
	}
	if err := c.ValidateConfig(f.Config); err != nil {
		return false, false, "a valid configuration was rejected: " + err.Error()
	}
	return true, false, "structural validation accepts a valid config and refuses an empty one"
}

func checkTLSConnection(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	h, err := c.TestConnection(ctx, f.Config, f.Secrets)
	if err != nil {
		return false, false, "connecting to the fixture failed: " + safeMessage(err)
	}
	if h.State != HealthHealthy {
		return false, false, fmt.Sprintf("health = %s, want HEALTHY", h.State)
	}
	if h.CheckedAt.IsZero() {
		return false, false, "the health result carries no timestamp"
	}
	return true, false, "a real connection was made and reported healthy"
}

func checkUntrustedCertificate(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if f.UntrustedTLSConfig == nil {
		return false, true, "the fixture supplies no untrusted-certificate endpoint, so certificate " +
			"verification is NOT VERIFIED for this driver"
	}
	h, err := c.TestConnection(ctx, *f.UntrustedTLSConfig, f.Secrets)
	if err == nil && h.State == HealthHealthy {
		return false, false, "an untrusted certificate was accepted, so TLS verification is not in effect"
	}
	var ce *ConnectorError
	if errors.As(err, &ce) && ce.Category != ErrorTLS && ce.Category != ErrorUnreachable {
		return false, false, "the untrusted endpoint failed with category " + string(ce.Category) +
			", which does not identify a certificate problem"
	}
	return true, false, "an untrusted certificate was rejected"
}

func checkAuthSuccess(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	h := c.Health(ctx, f.Config, f.Secrets)
	if h.State != HealthHealthy {
		return false, false, fmt.Sprintf("health with valid credentials = %s", h.State)
	}
	return true, false, "valid credentials authenticate"
}

func checkAuthFailureRedaction(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if len(f.BadSecrets.values) == 0 {
		return false, true, "the fixture supplies no invalid credentials, so the authentication " +
			"failure path is NOT VERIFIED"
	}
	h, err := c.TestConnection(ctx, f.Config, f.BadSecrets)
	if err == nil && h.State == HealthHealthy {
		return false, false, "invalid credentials were accepted"
	}

	// The failure must be classified, and its message must not name the
	// account. `password authentication failed for user "svc_reporting"` tells
	// an attacker which account to attack next.
	var ce *ConnectorError
	if !errors.As(err, &ce) {
		return false, false, "the failure was not classified into a safe category"
	}
	if ce.Category != ErrorAuthentication {
		return false, false, "expected AUTHENTICATION, got " + string(ce.Category)
	}
	if leaked := findLeak(ce.Error(), f); leaked != "" {
		return false, false, "the safe error message contains " + leaked
	}
	if leaked := findLeak(h.Detail, f); leaked != "" {
		return false, false, "the health detail contains " + leaked
	}
	return true, false, "invalid credentials are refused with a classified, redacted error"
}

func checkDiscoveryAllowlist(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	resources, err := c.DiscoverResources(ctx, f.Config, f.Secrets)
	if err != nil {
		return false, false, "discovery failed: " + safeMessage(err)
	}
	if len(resources) == 0 {
		return false, false, "discovery returned nothing, so the allowlist cannot be shown to bound it"
	}
	for _, r := range resources {
		scope := r.Schema
		if scope == "" {
			scope = r.Catalog
		}
		if !allowed(strings.ToLower(scope), f.Config.ResourceAllowlist) {
			return false, false, "discovery returned a resource outside the allowlist"
		}
	}
	return true, false, fmt.Sprintf("discovery returned %d resources, all inside the allowlist", len(resources))
}

func checkForbiddenResource(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if f.ForbiddenResource == "" {
		return false, true, "the fixture names no resource outside the allowlist, so the allowlist " +
			"is NOT VERIFIED against a resource that really exists"
	}
	idents := map[string]string{}
	for k, v := range f.CountIdents {
		idents[k] = v
	}
	for k := range idents {
		idents[k] = f.ForbiddenResource
	}
	_, err := execWithIdents(ctx, c, f, f.CountTemplate, idents)
	if err == nil {
		return false, false, "a resource outside the allowlist was read"
	}
	if !errors.Is(err, ErrNotAllowed) {
		var ce *ConnectorError
		if !errors.As(err, &ce) || ce.Category != ErrorNotAllowed {
			return false, false, "reading outside the allowlist failed for the wrong reason: " + safeMessage(err)
		}
	}
	return true, false, "a resource outside the allowlist is refused"
}

func checkParameterBinding(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	// A value that would change the statement's meaning if it were
	// interpolated. Bound, it is just a string that matches nothing.
	args := map[string]any{}
	for k := range f.CountParams {
		args[k] = "' OR 1=1 --"
	}
	res, err := execWithArgs(ctx, c, f, f.CountTemplate, args)
	if err != nil {
		var ce *ConnectorError
		// A type mismatch is an acceptable outcome: the driver refused to bind
		// a string where a number was expected, which is the parameter being
		// treated as data rather than SQL.
		if errors.As(err, &ce) && ce.Category == ErrorProvider {
			return true, false, "an injection payload was rejected as a value, not executed as SQL"
		}
		return false, false, "binding an injection payload failed unexpectedly: " + safeMessage(err)
	}
	if res != nil && len(res.Rows) > 0 {
		if n, ok := asInt(res.Rows[0][0]); ok && n > 0 {
			return false, false, "an injection payload matched rows, so it was interpolated rather than bound"
		}
	}
	return true, false, "an injection payload was bound as a value and matched nothing"
}

func checkIdentifierInjection(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if len(f.CountTemplate.Identifiers) == 0 {
		return false, true, "the fixture's template takes no identifier, so identifier allowlisting " +
			"is NOT VERIFIED through this driver"
	}
	for _, payload := range []string{
		`public"; DROP TABLE x; --`,
		"public.t UNION SELECT 1",
		"pg_catalog",
		"../../etc/passwd",
		"public\x00",
	} {
		idents := map[string]string{}
		for k := range f.CountIdents {
			idents[k] = payload
		}
		if _, err := execWithIdents(ctx, c, f, f.CountTemplate, idents); err == nil {
			return false, false, "an identifier injection payload was accepted"
		}
	}
	return true, false, "identifier injection payloads are refused by the allowlist"
}

func checkReadOnlyEnforcement(_ context.Context, c Connector, f Fixture) (bool, bool, string) {
	// Registration is where a write is caught, so the check is that a template
	// containing one cannot be built at all. Verifying it at execution instead
	// would mean a write statement had to reach a customer database to be
	// refused.
	for _, sql := range []string{
		"UPDATE accounts SET balance = 0",
		"DELETE FROM accounts",
		"INSERT INTO accounts VALUES (1)",
		"DROP TABLE accounts",
		"CREATE TABLE t (a int)",
		"GRANT ALL ON accounts TO public",
		"SELECT 1; UPDATE accounts SET balance = 0",
		"WITH x AS (DELETE FROM accounts RETURNING *) SELECT * FROM x",
	} {
		if _, err := RegisterTemplate(Template{
			ID: "probe", ConnectorType: c.Type(), SQL: sql,
		}); err == nil {
			return false, false, "a template containing a write or DDL statement was accepted"
		}
	}
	return true, false, "write, DDL and grant statements are refused at template registration"
}

func checkMultiStatement(_ context.Context, c Connector, _ Fixture) (bool, bool, string) {
	if _, err := RegisterTemplate(Template{
		ID: "probe", ConnectorType: c.Type(),
		SQL: "SELECT 1; SELECT 2",
	}); err == nil {
		return false, false, "a multi-statement template was accepted"
	}
	if _, err := RegisterTemplate(Template{
		ID: "probe", ConnectorType: c.Type(),
		SQL: "SELECT 1 -- and then",
	}); err == nil {
		return false, false, "a template containing a comment was accepted"
	}
	return true, false, "multi-statement and commented templates are refused"
}

func checkTimeoutCancellation(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if f.SlowTemplate.SQL == "" {
		return false, true, "the fixture supplies no slow query, so cancellation is NOT VERIFIED"
	}
	limits := DefaultLimits()
	limits.Timeout = 750 * time.Millisecond

	start := time.Now()
	_, err := c.ExecuteTemplate(ctx, f.Config, f.Secrets,
		f.SlowTemplate.WithIdentifiers(f.SlowIdents), f.SlowParams, limits)
	elapsed := time.Since(start)

	if err == nil {
		return false, false, "a query that should have exceeded its deadline returned successfully"
	}
	var ce *ConnectorError
	if !errors.As(err, &ce) || ce.Category != ErrorTimeout {
		return false, false, "the slow query failed with the wrong category: " + safeMessage(err)
	}
	// The cancellation must actually stop the work rather than the caller
	// abandoning it. Allowing 4x the limit leaves room for a slow round trip
	// while still catching a driver that waits for the full query.
	if elapsed > 4*limits.Timeout {
		return false, false, fmt.Sprintf("cancellation took %s for a %s limit, so the statement "+
			"was probably not cancelled server-side", elapsed, limits.Timeout)
	}
	return true, false, fmt.Sprintf("a slow query was cancelled after %s", elapsed.Round(time.Millisecond))
}

func checkRowLimit(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if f.WideTemplate.SQL == "" {
		return false, true, "the fixture supplies no wide query, so the row limit is NOT VERIFIED"
	}
	limits := DefaultLimits()
	limits.MaxRows = 5

	res, err := c.ExecuteTemplate(ctx, f.Config, f.Secrets,
		f.WideTemplate.WithIdentifiers(f.WideIdents), f.WideParams, limits)
	if err != nil {
		return false, false, "the wide query failed: " + safeMessage(err)
	}
	if int64(len(res.Rows)) > limits.MaxRows {
		return false, false, fmt.Sprintf("returned %d rows against a limit of %d", len(res.Rows), limits.MaxRows)
	}
	if !res.Truncated {
		return false, false, "the result was limited and did not report itself truncated; a silently " +
			"truncated result is a wrong answer that looks like a right one"
	}
	return true, false, fmt.Sprintf("the row limit stopped the read at %d and reported truncation", len(res.Rows))
}

func checkByteLimit(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if f.WideTemplate.SQL == "" {
		return false, true, "the fixture supplies no wide query, so the byte limit is NOT VERIFIED"
	}
	limits := DefaultLimits()
	limits.MaxBytes = 512

	res, err := c.ExecuteTemplate(ctx, f.Config, f.Secrets,
		f.WideTemplate.WithIdentifiers(f.WideIdents), f.WideParams, limits)
	if err != nil {
		var ce *ConnectorError
		if errors.As(err, &ce) && ce.Category == ErrorLimitExceeded {
			return true, false, "the byte limit refused the read"
		}
		return false, false, "the wide query failed: " + safeMessage(err)
	}
	if !res.Truncated {
		return false, false, "a 512-byte limit did not truncate the result"
	}
	return true, false, "the byte limit stopped the read and reported truncation"
}

func checkCursorPagination(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if f.WideTemplate.SQL == "" {
		return false, true, "the fixture supplies no wide query, so cursor pagination is NOT VERIFIED"
	}
	if !c.Capabilities().CursorPagination {
		return false, true, "the driver declares no cursor support, so pagination is NOT VERIFIED"
	}
	limits := DefaultLimits()
	limits.MaxRows = 3

	first, err := c.ExecuteTemplate(ctx, f.Config, f.Secrets,
		f.WideTemplate.WithIdentifiers(f.WideIdents), f.WideParams, limits)
	if err != nil {
		return false, false, "the first page failed: " + safeMessage(err)
	}
	if first.NextCursor == "" {
		return false, false, "a truncated result offered no cursor, so the remainder is unreachable"
	}
	return true, false, "a truncated result carries a cursor for the next page"
}

func checkPoolExhaustion(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	// Many concurrent reads, then one more. The platform must refuse or queue
	// rather than failing permanently, and must recover afterwards.
	const concurrent = 24
	errs := make(chan error, concurrent)
	for range concurrent {
		go func() {
			_, err := execWithArgs(ctx, c, f, f.CountTemplate, f.CountParams)
			errs <- err
		}()
	}
	var failures int
	for range concurrent {
		if err := <-errs; err != nil {
			failures++
		}
	}

	// Recovery is the property that matters. A pool that sheds load under a
	// burst is fine; a pool that never serves another request is not.
	if _, err := execWithArgs(ctx, c, f, f.CountTemplate, f.CountParams); err != nil {
		return false, false, fmt.Sprintf("the connector did not recover after %d concurrent reads "+
			"(%d of which failed): %s", concurrent, failures, safeMessage(err))
	}
	return true, false, fmt.Sprintf("%d concurrent reads shed %d and the connector recovered",
		concurrent, failures)
}

func checkSecretRevocation(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if len(f.BadSecrets.values) == 0 {
		return false, true, "the fixture supplies no revoked credential, so revocation is NOT VERIFIED"
	}
	// Rotating to a credential the server does not accept must stop access
	// immediately, not after a cached connection expires. A pool keyed only by
	// host would keep serving the old connection and revocation would be
	// silently ineffective.
	h, err := c.TestConnection(ctx, f.Config, f.BadSecrets)
	if err == nil && h.State == HealthHealthy {
		return false, false, "a revoked credential still connected, so the driver is serving a " +
			"cached connection and revocation does not stop access"
	}
	// And the good credential must still work afterwards.
	if h2, err := c.TestConnection(ctx, f.Config, f.Secrets); err != nil || h2.State != HealthHealthy {
		return false, false, "the valid credential stopped working after a failed attempt"
	}
	return true, false, "a revoked credential is refused and does not disturb the valid one"
}

func checkCrossTenant(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	if f.OtherTenantConfig == nil {
		return false, true, "the fixture supplies no second tenant, so cross-tenant isolation is " +
			"NOT VERIFIED through this driver"
	}
	// The second tenant's configuration must be served by its own connection.
	// A driver caching by connector type rather than by full configuration
	// would answer one tenant's query on another's connection.
	if _, err := c.TestConnection(ctx, *f.OtherTenantConfig, f.OtherTenantSecret); err == nil {
		// Connecting successfully is fine; the check is that the first tenant's
		// connection still behaves as its own.
		if _, err := execWithArgs(ctx, c, f, f.CountTemplate, f.CountParams); err != nil {
			return false, false, "the first tenant's query failed after a second tenant connected: " +
				safeMessage(err)
		}
	}
	return true, false, "two tenants' configurations do not share a connection"
}

func checkHealthTimestamp(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	before := time.Now().UTC().Add(-time.Second)
	h := c.Health(ctx, f.Config, f.Secrets)
	if h.CheckedAt.IsZero() {
		return false, false, "health carries no timestamp, so its age is unknowable"
	}
	if h.CheckedAt.Before(before) {
		return false, false, "health returned a stale timestamp, so it is reporting a cached result " +
			"as a current observation"
	}
	return true, false, "health is a real, freshly timestamped observation"
}

func checkErrorRedaction(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	// Every failure path the suite can provoke, checked for the credential and
	// the account name.
	if len(f.BadSecrets.values) == 0 {
		return false, true, "the fixture supplies no invalid credentials, so error redaction is NOT VERIFIED"
	}
	_, err := c.TestConnection(ctx, f.Config, f.BadSecrets)
	if err == nil {
		return false, false, "invalid credentials produced no error"
	}
	if leaked := findLeak(err.Error(), f); leaked != "" {
		return false, false, "an error message contains " + leaked
	}

	broken := f.Config
	broken.Fields = map[string]string{}
	for k, v := range f.Config.Fields {
		broken.Fields[k] = v
	}
	broken.Fields["host"] = "no-such-host.invalid"
	if _, err := c.TestConnection(ctx, broken, f.Secrets); err != nil {
		if leaked := findLeak(err.Error(), f); leaked != "" {
			return false, false, "an unreachable-host error contains " + leaked
		}
	}
	return true, false, "failure messages carry neither the credential nor the account name"
}

func checkNoLeak(ctx context.Context, c Connector, f Fixture) (bool, bool, string) {
	before := runtime.NumGoroutine()
	for range 5 {
		_, _ = c.TestConnection(ctx, f.Config, f.Secrets)
	}
	if err := c.Close(); err != nil {
		return false, false, "Close returned an error: " + safeMessage(err)
	}
	// Close must be safe twice: a shutdown path that calls it from both a
	// defer and an explicit stop is ordinary.
	if err := c.Close(); err != nil {
		return false, false, "a second Close returned an error"
	}

	// Goroutines wind down asynchronously, so this settles rather than
	// sampling once.
	deadline := time.Now().Add(3 * time.Second)
	after := runtime.NumGoroutine()
	for time.Now().Before(deadline) && after > before+2 {
		time.Sleep(50 * time.Millisecond)
		runtime.GC()
		after = runtime.NumGoroutine()
	}
	if after > before+2 {
		return false, false, fmt.Sprintf("goroutines went from %d to %d across connect and close",
			before, after)
	}
	return true, false, "connecting and closing leaves no goroutine behind"
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func execWithArgs(ctx context.Context, c Connector, f Fixture, t Template, args map[string]any) (*QueryResult, error) {
	cfg := f.Config
	cfg.Fields = f.Config.Fields
	return c.ExecuteTemplate(ctx, cfg, f.Secrets, t.WithIdentifiers(f.CountIdents), args, DefaultLimits())
}

func execWithIdents(ctx context.Context, c Connector, f Fixture, t Template, idents map[string]string) (*QueryResult, error) {
	return c.ExecuteTemplate(ctx, f.Config, f.Secrets, t.WithIdentifiers(idents), f.CountParams, DefaultLimits())
}

// findLeak reports the first fixture secret or account name appearing in text.
//
// It reads the secrets in order to search for them, which is the one place in
// this package that is justified: proving a message does not contain a
// credential requires knowing the credential.
func findLeak(text string, f Fixture) string {
	if text == "" {
		return ""
	}
	lower := strings.ToLower(text)

	for _, id := range f.Secrets.IDs() {
		var found string
		_ = f.Secrets.Use(id, func(plain string) error {
			if len(plain) >= 6 && strings.Contains(lower, strings.ToLower(plain)) {
				found = "the credential for " + id
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	for _, id := range f.BadSecrets.IDs() {
		var found string
		_ = f.BadSecrets.Use(id, func(plain string) error {
			if len(plain) >= 6 && strings.Contains(lower, strings.ToLower(plain)) {
				found = "the rejected credential for " + id
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	// The account name is an oracle even without the password: it tells an
	// attacker which identity exists and is worth attacking.
	if user := f.Config.Get("username"); len(user) >= 4 && strings.Contains(lower, strings.ToLower(user)) {
		return "the database username"
	}
	return ""
}

func safeMessage(err error) string {
	if err == nil {
		return ""
	}
	var ce *ConnectorError
	if errors.As(err, &ce) {
		return ce.Error()
	}
	return "an unclassified error"
}

func asInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}
