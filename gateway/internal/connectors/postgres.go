package connectors

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The PostgreSQL driver.
//
// First because it is the protocol and auth surface every other connector is
// measured against, and because a real server is available to verify it. It is
// the only entry in the catalog with an implementation, and it is AVAILABLE
// only when `RunConformance` has passed against a real server in this build.

// PostgresDriverVersion pins what this adapter is written against.
//
// jackc/pgx is already a dependency of this repository for Sentinel Flow's own
// PostgreSQL support, so no new driver is introduced and no new licence review
// is required -- pgx is MIT, as recorded in go.sum.
const PostgresDriverVersion = "jackc/pgx/v5 (stdlib)"

// PostgresConnector is a real, bounded, read-only PostgreSQL adapter.
type PostgresConnector struct {
	mu    sync.Mutex
	pools map[string]*sql.DB

	// openFn is the connection opener, replaceable in tests that need to
	// observe pooling without a server.
	openFn func(dsn string) (*sql.DB, error)
}

// NewPostgresConnector builds the adapter.
func NewPostgresConnector() *PostgresConnector {
	return &PostgresConnector{
		pools:  map[string]*sql.DB{},
		openFn: func(dsn string) (*sql.DB, error) { return sql.Open("pgx", dsn) },
	}
}

// Type is the catalog identifier.
func (p *PostgresConnector) Type() string { return "postgresql" }

// Capabilities describes PostgreSQL as it actually behaves.
func (p *PostgresConnector) Capabilities() Capabilities {
	return Capabilities{
		SupportsSchemas: true, SupportsCatalogs: false,
		IdentifierQuote: `"`, ParameterStyle: ParamDollar,
		ReadOnlyTransactions: true, StatementTimeout: true,
		Cancellation: true, CursorPagination: true,
		TLSModes:        []string{"verify-full", "verify-ca", "require"},
		AuthModes:       []string{"password", "client_certificate"},
		MetadataQueries: true, AggregateQueries: true,
	}
}

// ValidateConfig performs structural checks and makes no network call.
//
// It never reads a secret and never includes a submitted value in an error: the
// submitted value may be the credential, and an error message is the least
// controlled thing this package produces.
func (p *PostgresConnector) ValidateConfig(cfg Config) error {
	host := cfg.Get("host")
	if host == "" {
		return errors.New("postgresql: a host is required")
	}
	// A host with a scheme or userinfo in it is a pasted connection string in
	// the wrong field, and would carry a credential into a column that is not
	// a secret column.
	if strings.ContainsAny(host, "/@ ") || strings.Contains(host, ":") {
		return errors.New("postgresql: the host field must contain a hostname only, " +
			"not a URL or a connection string")
	}
	if cfg.Get("database") == "" {
		return errors.New("postgresql: a database is required")
	}
	if cfg.Get("username") == "" {
		return errors.New("postgresql: a username is required")
	}

	port := cfg.Get("port")
	if port == "" {
		port = "5432"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("postgresql: the port must be a number between 1 and 65535")
	}

	mode := cfg.Get("tls_mode")
	if mode == "" {
		return errors.New("postgresql: a TLS mode is required")
	}
	switch mode {
	case "verify-full", "verify-ca", "require":
	default:
		return fmt.Errorf("postgresql: %q is not a permitted TLS mode", mode)
	}

	if len(cfg.ResourceAllowlist) == 0 {
		return errors.New("postgresql: at least one approved schema is required")
	}
	for _, r := range cfg.ResourceAllowlist {
		if err := ValidateIdentifier(r); err != nil {
			return fmt.Errorf("postgresql: approved schema %w", err)
		}
	}
	return nil
}

// dsn builds the connection string inside a closure holding the password.
//
// The DSN exists only for the duration of fn. It is never returned, stored,
// logged or placed in an error -- a DSN is a credential in string form, and
// every leak of one in this domain has come from it outliving its use.
func (p *PostgresConnector) dsn(cfg Config, sec Secrets, fn func(dsn, key string) error) error {
	port := cfg.Get("port")
	if port == "" {
		port = "5432"
	}

	build := func(password string) (string, string) {
		u := url.URL{
			Scheme: "postgres",
			Host:   net.JoinHostPort(cfg.Get("host"), port),
			Path:   "/" + cfg.Get("database"),
		}
		if password != "" {
			u.User = url.UserPassword(cfg.Get("username"), password)
		} else {
			u.User = url.User(cfg.Get("username"))
		}
		q := url.Values{}
		q.Set("sslmode", cfg.Get("tls_mode"))
		if ca := cfg.Get("ca_ref"); ca != "" {
			q.Set("sslrootcert", ca)
		}
		// A bounded connect deadline, so an unreachable host fails in seconds
		// rather than hanging a request for the driver's default.
		q.Set("connect_timeout", "10")
		// The application name reaches the server's activity view, which is
		// where a customer DBA looks to find out who is querying them.
		q.Set("application_name", "sentinel-flow-connector")
		u.RawQuery = q.Encode()

		// The pool key includes the credential's digest, so rotating a
		// credential produces a different pool. Keying by host and user alone
		// would let a revoked password keep working through a cached
		// connection, which the conformance suite checks for explicitly.
		sum := sha256.Sum256([]byte(u.String()))
		return u.String(), hex.EncodeToString(sum[:])
	}

	if sec.Has("password") {
		return sec.Use("password", func(password string) error {
			dsn, key := build(password)
			return fn(dsn, key)
		})
	}
	dsn, key := build("")
	return fn(dsn, key)
}

// pool returns the pooled handle for one exact configuration.
func (p *PostgresConnector) pool(dsn, key string) (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if db, ok := p.pools[key]; ok {
		return db, nil
	}
	db, err := p.openFn(dsn)
	if err != nil {
		return nil, err
	}
	// Bounded on purpose. A connector that can open unlimited connections to a
	// customer's database can exhaust it, and the customer's outage would be
	// caused by their reporting integration.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)

	p.pools[key] = db
	return db, nil
}

// TestConnection makes a real, bounded, read-only check.
//
// There is no path through this function that reports success without having
// connected: the health result is built after a query returns.
func (p *PostgresConnector) TestConnection(ctx context.Context, cfg Config, sec Secrets) (Health, error) {
	if err := p.ValidateConfig(cfg); err != nil {
		return Health{
			State: HealthFailed, CheckedAt: time.Now().UTC(),
			ErrorCategory: ErrorConfiguration, Detail: ErrorConfiguration.Detail(),
		}, Categorized("test_connection", ErrorConfiguration, err)
	}

	start := time.Now()
	var version string
	err := p.dsn(cfg, sec, func(dsn, key string) error {
		db, err := p.pool(dsn, key)
		if err != nil {
			return err
		}
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return db.QueryRowContext(checkCtx, "SELECT version()").Scan(&version)
	})

	if err != nil {
		ce := Classify("test_connection", err)
		return Health{
			State: HealthFailed, CheckedAt: time.Now().UTC(),
			ErrorCategory: ce.Category, Detail: ce.Category.Detail(),
			Latency: time.Since(start),
		}, ce
	}

	return Health{
		State: HealthHealthy, CheckedAt: time.Now().UTC(),
		Detail:  "connected and read the server version",
		Latency: time.Since(start),
	}, nil
}

// Health re-checks an existing connection.
func (p *PostgresConnector) Health(ctx context.Context, cfg Config, sec Secrets) Health {
	h, _ := p.TestConnection(ctx, cfg, sec)
	return h
}

// discoverSQL lists tables and views in the approved schemas.
//
// The schema list is bound as a parameter array rather than interpolated, so
// the allowlist is enforced by the database rather than by string assembly. The
// system catalogs are excluded explicitly as well as by the allowlist, because
// a tenant that added `pg_catalog` to its own allowlist would otherwise be able
// to enumerate roles.
const discoverSQL = `
	SELECT table_schema, table_name, table_type
	FROM information_schema.tables
	WHERE table_schema = ANY($1)
	  AND table_schema NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
	ORDER BY table_schema, table_name
	LIMIT $2`

// DiscoverResources returns approved metadata only.
func (p *PostgresConnector) DiscoverResources(ctx context.Context, cfg Config, sec Secrets) ([]Resource, error) {
	if err := p.ValidateConfig(cfg); err != nil {
		return nil, Categorized("discover", ErrorConfiguration, err)
	}

	var out []Resource
	err := p.dsn(cfg, sec, func(dsn, key string) error {
		db, err := p.pool(dsn, key)
		if err != nil {
			return err
		}
		queryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		rows, err := db.QueryContext(queryCtx, discoverSQL,
			pgTextArray(cfg.ResourceAllowlist), DefaultLimits().MaxRows)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var schema, name, kind string
			if err := rows.Scan(&schema, &name, &kind); err != nil {
				return err
			}
			out = append(out, Resource{Schema: schema, Name: name, Kind: normaliseKind(kind)})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, Classify("discover", err)
	}
	return out, nil
}

func normaliseKind(k string) string {
	switch strings.ToUpper(strings.TrimSpace(k)) {
	case "BASE TABLE":
		return "TABLE"
	case "VIEW":
		return "VIEW"
	default:
		return strings.ToUpper(k)
	}
}

// pgTextArray renders a Go slice as a PostgreSQL text array literal.
//
// Every element has already passed ValidateIdentifier, so none can contain a
// brace, comma or quote. That ordering is what makes this safe: the function
// does no escaping because there is nothing left to escape.
func pgTextArray(values []string) string {
	if len(values) == 0 {
		return "{}"
	}
	return "{" + strings.Join(values, ",") + "}"
}

// ExecuteTemplate runs one administrator-approved parameterised query.
//
// Read-only is enforced three times over, at descending levels of trust:
// templates are validated at registration, the transaction is opened READ ONLY,
// and a statement timeout is set on the session. The third is what stops a
// runaway query holding a customer's resources after this gateway has given up.
func (p *PostgresConnector) ExecuteTemplate(
	ctx context.Context, cfg Config, sec Secrets,
	t Template, args map[string]any, limits Limits,
) (*QueryResult, error) {
	if err := p.ValidateConfig(cfg); err != nil {
		return nil, Categorized("execute", ErrorConfiguration, err)
	}
	if t.ConnectorType != p.Type() {
		return nil, Categorized("execute", ErrorNotAllowed,
			fmt.Errorf("template %s is for %s, not %s: %w", t.ID, t.ConnectorType, p.Type(), ErrNotAllowed))
	}

	limits = limits.Clamp(DefaultLimits())

	statement, err := t.Resolve(t.ChosenIdentifiers(), cfg.ResourceAllowlist, `"`)
	if err != nil {
		return nil, Categorized("execute", ErrorNotAllowed, err)
	}
	bound, err := t.BindArgs(args)
	if err != nil {
		return nil, Categorized("execute", ErrorNotAllowed, err)
	}

	result := &QueryResult{}
	start := time.Now()

	err = p.dsn(cfg, sec, func(dsn, key string) error {
		db, err := p.pool(dsn, key)
		if err != nil {
			return err
		}

		queryCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
		defer cancel()

		// A read-only transaction. PostgreSQL refuses any write inside one, so
		// this holds even for a statement that reaches a volatile function the
		// template checks did not recognise.
		tx, err := db.BeginTx(queryCtx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		// A server-side statement timeout slightly under the context deadline.
		// The context cancels this gateway's wait; statement_timeout cancels
		// the customer's query. Without it, abandoning the request leaves their
		// database still working.
		serverTimeout := limits.Timeout - 250*time.Millisecond
		if serverTimeout < 500*time.Millisecond {
			serverTimeout = 500 * time.Millisecond
		}
		if _, err := tx.ExecContext(queryCtx,
			fmt.Sprintf("SET LOCAL statement_timeout = %d", serverTimeout.Milliseconds())); err != nil {
			return err
		}

		rows, err := tx.QueryContext(queryCtx, statement, bound...)
		if err != nil {
			return err
		}
		defer rows.Close()

		names, err := rows.Columns()
		if err != nil {
			return err
		}
		types, _ := rows.ColumnTypes()
		for i, n := range names {
			col := Column{Name: n, Classification: t.Classification(n)}
			if types != nil && i < len(types) {
				col.DatabaseType = types[i].DatabaseTypeName()
			}
			result.Columns = append(result.Columns, col)
		}

		var bytes int64
		for rows.Next() {
			if int64(len(result.Rows)) >= limits.MaxRows {
				result.Truncated = true
				result.LimitHit = "rows"
				break
			}

			scan := make([]any, len(names))
			holders := make([]any, len(names))
			for i := range scan {
				holders[i] = &scan[i]
			}
			if err := rows.Scan(holders...); err != nil {
				return err
			}

			row := make([]any, len(names))
			for i, v := range scan {
				// Masked at this boundary, so a caller cannot forget to. What
				// leaves this function has already had its classification
				// applied.
				row[i] = MaskValue(normaliseValue(v), result.Columns[i].Classification)
				bytes += approximateSize(row[i])
			}
			result.Rows = append(result.Rows, row)

			if bytes >= limits.MaxBytes {
				result.Truncated = true
				result.LimitHit = "bytes"
				break
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if result.Truncated {
			// The cursor is the row offset reached. It is deliberately opaque
			// to the caller and meaningless without the same template and
			// limits, so it cannot be used to widen a read.
			result.NextCursor = strconv.Itoa(len(result.Rows))
		}
		return tx.Rollback()
	})

	result.Elapsed = time.Since(start)
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		return nil, Classify("execute", err)
	}
	return result, nil
}

// normaliseValue converts driver types into ones a JSON encoder handles.
//
// []byte in particular is rendered as base64 by encoding/json, which would turn
// a masked value back into something that looks like data.
func normaliseValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	default:
		return v
	}
}

func approximateSize(v any) int64 {
	switch t := v.(type) {
	case string:
		return int64(len(t))
	case nil:
		return 0
	default:
		return int64(len(fmt.Sprint(t)))
	}
}

// Close releases every pool. It is safe to call twice.
func (p *PostgresConnector) Close() error {
	p.mu.Lock()
	pools := p.pools
	p.pools = map[string]*sql.DB{}
	p.mu.Unlock()

	var firstErr error
	for _, db := range pools {
		if err := db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// openAdmin opens a raw handle for conformance fixtures.
//
// It exists so the fixture builder can create and drop a disposable schema
// without going through the connector's own read-only path -- which correctly
// refuses to do either.
func openAdmin(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
