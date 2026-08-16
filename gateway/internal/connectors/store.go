package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"sentinel-gateway/internal/secrets"
)

// Storing a connection.
//
// The rule the whole file is built around: a credential never lands in this
// package's tables. Non-secret configuration goes in a column; every secret
// goes to the Prompt 05 secret store under a deterministic name, and only the
// name is recorded here.
//
// That is what makes "never redisplay a saved password" true structurally
// rather than by discipline. There is no read path in this file that could
// return one, because there is nothing to return.

// ErrConnectionNotFound is returned for an unknown id and for one belonging to
// another tenant, with identical text. Distinguishing them would confirm the
// existence of another tenant's connection.
var ErrConnectionNotFound = errors.New("connection not found in this tenant")

// dialect selects the database-specific statements for Sentinel Flow's own
// database -- not for any customer database, which is what the rest of this
// package is about.
type dialect int

const (
	dialectPostgres dialect = iota
	dialectSQLite
)

// Store persists connections.
type Store struct {
	db       *sql.DB
	dialect  dialect
	registry *Registry
	secrets  secrets.Store
	now      func() time.Time
}

// NewStore builds the persistence layer.
func NewStore(db *sql.DB, driverName string, r *Registry, sec secrets.Store) (*Store, error) {
	if db == nil {
		return nil, errors.New("the connection store requires a database handle")
	}
	if r == nil {
		return nil, errors.New("the connection store requires a connector registry")
	}
	if sec == nil {
		// Refused rather than defaulted to an in-memory store. A deployment
		// with no durable secret store would accept connections and lose their
		// credentials on restart, which presents as every connection failing
		// authentication the morning after a deploy.
		return nil, errors.New("the connection store requires a secret store")
	}

	var d dialect
	switch {
	case strings.Contains(driverName, "pgx"), strings.Contains(driverName, "postgres"):
		d = dialectPostgres
	case strings.Contains(driverName, "sqlite"):
		d = dialectSQLite
	default:
		return nil, fmt.Errorf("unsupported driver %q for the connection store", driverName)
	}
	return &Store{db: db, dialect: d, registry: r, secrets: sec, now: time.Now}, nil
}

// SetClock replaces the time source, for tests.
func (s *Store) SetClock(fn func() time.Time) { s.now = fn }

func (s *Store) rebind(q string) string {
	if s.dialect != dialectPostgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Connection is a stored connection, as every read path returns it.
//
// There is no secret field and no assembled connection string. SecretFields
// names which credentials exist; that is the most any caller gets.
type Connection struct {
	ID            int64             `json:"id"`
	TenantID      string            `json:"-"`
	ConnectorType string            `json:"connectorType"`
	DisplayName   string            `json:"displayName"`
	AuthMode      string            `json:"authMode"`
	Fields        map[string]string `json:"fields"`
	Allowlist     []string          `json:"resourceAllowlist"`
	Limits        Limits            `json:"-"`
	MaxPerMinute  int               `json:"maxPerMinute"`

	// SecretFields names the configured credentials, by field id. No value and
	// no fingerprint: a fingerprint is a stable identifier for a credential
	// and would let a weak one be confirmed by guessing.
	SecretFields []string `json:"secretsConfigured"`
	// secretNames maps a field id to its secret-store name. Unexported and
	// absent from JSON: the name is an internal lookup key, and publishing it
	// would tell a caller exactly what to ask the secret store for.
	secretNames map[string]string
	// WeakSecrets names credentials the customer chose below this
	// application's floor. Surfaced so an operator can act, never displayed.
	WeakSecrets []string `json:"weakSecrets,omitempty"`

	Health Health `json:"health"`

	ConformanceCommit string     `json:"conformanceCommit,omitempty"`
	ConformanceServer string     `json:"conformanceServer,omitempty"`
	LastUsedAt        *time.Time `json:"lastUsedAt,omitempty"`
	CreatedBy         string     `json:"createdBy"`
	CreatedAt         time.Time  `json:"createdAt"`
	RowVersion        int64      `json:"-"`
}

// CreateRequest is a new connection, as submitted.
type CreateRequest struct {
	DisplayName   string
	ConnectorType string
	AuthMode      string
	Fields        map[string]string
	Allowlist     []string
	Limits        Limits
	MaxPerMinute  int

	// Secrets are the write-only values, keyed by descriptor field id. They
	// are moved to the secret store and never written to this package's tables.
	Secrets map[string]string
}

// Create stores a connection.
//
// It refuses a connector that has not passed conformance. That is the same
// check `Registry.Driver` makes, applied one layer earlier so a tenant cannot
// even record a configuration against an unverified driver -- a stored
// connection is a promise that it will work, and a promise against an
// unverified driver is one nobody has checked.
func (s *Store) Create(ctx context.Context, sc secrets.Scope, req CreateRequest) (*Connection, error) {
	_, descriptor, err := s.registry.Driver(req.ConnectorType)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.DisplayName) == "" {
		return nil, errors.New("a connection needs a name")
	}

	cfg := Config{
		Type:              req.ConnectorType,
		Fields:            map[string]string{},
		ResourceAllowlist: req.Allowlist,
		Limits:            req.Limits.Clamp(DefaultLimits()),
	}
	// Only non-secret fields are copied. A submitted value for a write-only
	// field is routed to the secret store below, and a caller that put a
	// password in the plain field map does not get it persisted here.
	secretFields := map[string]bool{}
	for _, id := range descriptor.SecretFieldIDs() {
		secretFields[id] = true
	}
	for k, v := range req.Fields {
		if secretFields[k] {
			continue
		}
		cfg.Fields[k] = v
	}

	sealed := map[string]secrets.Value{}
	weak := map[string]bool{}
	for id, raw := range req.Secrets {
		if !secretFields[id] {
			return nil, fmt.Errorf("%q is not a credential field of %s", id, descriptor.DisplayName)
		}
		// NewExternal, not New: a customer's database password is not a
		// credential this application chose, so the MinSecretLength floor does
		// not apply. Its shortness is reported rather than refused.
		v, isWeak, err := secrets.NewExternal(raw)
		if err != nil {
			return nil, fmt.Errorf("the credential for %s could not be stored", id)
		}
		sealed[id] = v
		weak[id] = isWeak
	}

	if err := descriptor.Validate(cfg, NewSecrets(sealed), req.AuthMode); err != nil {
		return nil, err
	}

	maxPerMinute := req.MaxPerMinute
	if maxPerMinute <= 0 || maxPerMinute > defaultMaxPerMinute {
		maxPerMinute = defaultMaxPerMinute
	}

	fieldsJSON, err := json.Marshal(cfg.Fields)
	if err != nil {
		return nil, err
	}
	allowJSON, err := json.Marshal(cfg.ResourceAllowlist)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	var conformanceCommit, conformanceServer string
	if descriptor.Conformance != nil {
		conformanceCommit = descriptor.Conformance.TestCommit
		conformanceServer = descriptor.Conformance.ServerVersion
	}

	// Credentials go to the secret store before this package's transaction
	// opens. See secretName for why the order is forced.
	names := map[string]string{}
	written := make([]string, 0, len(sealed))
	retireWritten := func() {
		for _, name := range written {
			if ref, err := s.secrets.Get(ctx, sc, name); err == nil {
				_ = s.secrets.Retire(ctx, sc, ref.Name, ref.Version)
			}
		}
	}
	for field, value := range sealed {
		name := secretName(sc.TenantID(), req.DisplayName, field)
		if _, _, err := s.secrets.Create(ctx, sc, secrets.CreateRequest{
			Name:   name,
			Kind:   secrets.KindRetrieve,
			Import: value,
		}); err != nil {
			retireWritten()
			return nil, fmt.Errorf("storing the credential for %s failed", field)
		}
		names[field] = name
		written = append(written, name)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		retireWritten()
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	const insert = `
		INSERT INTO source_connections
			(tenant_id, connector_type, display_name, auth_mode, config_json,
			 resource_allowlist, max_rows, max_bytes, timeout_seconds, max_per_minute,
			 health_state, conformance_commit, conformance_server,
			 created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'NEVER_CHECKED', ?, ?, ?, ?, ?)`
	args := []any{
		sc.TenantID(), req.ConnectorType, req.DisplayName, req.AuthMode, string(fieldsJSON),
		string(allowJSON), cfg.Limits.MaxRows, cfg.Limits.MaxBytes,
		int(cfg.Limits.Timeout / time.Second), maxPerMinute,
		conformanceCommit, conformanceServer, sc.ActorID(), now, now,
	}

	var id int64
	if s.dialect == dialectPostgres {
		if err := tx.QueryRowContext(ctx, s.rebind(insert+" RETURNING id"), args...).Scan(&id); err != nil {
			retireWritten()
			return nil, err
		}
	} else {
		res, err := tx.ExecContext(ctx, insert, args...)
		if err != nil {
			retireWritten()
			return nil, err
		}
		if id, err = res.LastInsertId(); err != nil {
			retireWritten()
			return nil, err
		}
	}

	for field, name := range names {
		w := 0
		if weak[field] {
			w = 1
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`
			INSERT INTO source_connection_secrets
				(tenant_id, connection_id, field_id, secret_name, weak, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`),
			sc.TenantID(), id, field, name, w, now); err != nil {
			retireWritten()
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		// The connection row did not land, so its credentials are orphaned.
		// Retiring them keeps a sealed secret from outliving the thing it
		// authenticated for, with nothing left to revoke it by.
		retireWritten()
		return nil, err
	}
	return s.Get(ctx, sc.TenantID(), id)
}

// defaultMaxPerMinute bounds executions per connection per minute.
//
// Per-execution limits stop one query being unbounded; without this, an
// unbounded number of bounded queries is still unbounded, and the load lands on
// a customer's production database.
const defaultMaxPerMinute = 60

// secretName is the deterministic secret-store name for one credential field.
//
// Derived from the tenant and the connection's name -- which are unique
// together and known before the row is inserted -- rather than from the
// generated id. That ordering is what lets the credentials be written to the
// secret store *before* this package's transaction opens.
//
// It has to be that way round on SQLite: the secret store runs its own
// transactions, and calling it from inside this package's transaction is a
// self-deadlock that presents as every save hanging for the busy timeout and
// then failing. It is also the right order regardless -- a connection row that
// exists without its credential can never work, while a sealed secret with no
// connection is inert and is retired below.
func secretName(tenantID, displayName, field string) string {
	return fmt.Sprintf("connector/%s/%s/%s", tenantID, displayName, field)
}

// Get returns one connection, tenant-scoped.
func (s *Store) Get(ctx context.Context, tenantID string, id int64) (*Connection, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT id, tenant_id, connector_type, display_name, auth_mode, config_json,
		       resource_allowlist, max_rows, max_bytes, timeout_seconds, max_per_minute,
		       health_state, health_checked_at, health_error_class, health_detail, health_latency_ms,
		       conformance_commit, conformance_server, last_used_at, created_by, created_at, row_version
		FROM source_connections WHERE tenant_id = ? AND id = ?`), tenantID, id)

	c, err := scanConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.loadSecretFields(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// List returns every connection in a tenant.
func (s *Store) List(ctx context.Context, tenantID string) ([]*Connection, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, tenant_id, connector_type, display_name, auth_mode, config_json,
		       resource_allowlist, max_rows, max_bytes, timeout_seconds, max_per_minute,
		       health_state, health_checked_at, health_error_class, health_detail, health_latency_ms,
		       conformance_commit, conformance_server, last_used_at, created_by, created_at, row_version
		FROM source_connections WHERE tenant_id = ? ORDER BY display_name`), tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, c := range out {
		if err := s.loadSecretFields(ctx, c); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanConnection(sc interface{ Scan(...any) error }) (*Connection, error) {
	var (
		c          Connection
		fieldsJSON string
		allowJSON  string
		maxRows    int64
		maxBytes   int64
		timeoutSec int
		state      string
		checkedAt  sql.NullTime
		errClass   sql.NullString
		detail     sql.NullString
		latency    sql.NullInt64
		lastUsed   sql.NullTime
	)
	if err := sc.Scan(
		&c.ID, &c.TenantID, &c.ConnectorType, &c.DisplayName, &c.AuthMode, &fieldsJSON,
		&allowJSON, &maxRows, &maxBytes, &timeoutSec, &c.MaxPerMinute,
		&state, &checkedAt, &errClass, &detail, &latency,
		&c.ConformanceCommit, &c.ConformanceServer, &lastUsed, &c.CreatedBy, &c.CreatedAt, &c.RowVersion,
	); err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(fieldsJSON), &c.Fields); err != nil {
		return nil, fmt.Errorf("connection %d has unreadable configuration", c.ID)
	}
	if err := json.Unmarshal([]byte(allowJSON), &c.Allowlist); err != nil {
		return nil, fmt.Errorf("connection %d has an unreadable resource allowlist", c.ID)
	}

	c.Limits = Limits{
		MaxRows: maxRows, MaxBytes: maxBytes,
		Timeout: time.Duration(timeoutSec) * time.Second,
	}.Clamp(DefaultLimits())

	c.Health = Health{
		State:         HealthState(state),
		ErrorCategory: ErrorCategory(errClass.String),
		Detail:        detail.String,
	}
	if checkedAt.Valid {
		c.Health.CheckedAt = checkedAt.Time.UTC()
	}
	if latency.Valid {
		c.Health.Latency = time.Duration(latency.Int64) * time.Millisecond
	}
	if lastUsed.Valid {
		t := lastUsed.Time.UTC()
		c.LastUsedAt = &t
	}
	return &c, nil
}

// loadSecretFields records which credentials exist, by field id only.
func (s *Store) loadSecretFields(ctx context.Context, c *Connection) error {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT field_id, secret_name, weak FROM source_connection_secrets
		WHERE tenant_id = ? AND connection_id = ? ORDER BY field_id`), c.TenantID, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	c.secretNames = map[string]string{}
	for rows.Next() {
		var field, name string
		var weak int
		if err := rows.Scan(&field, &name, &weak); err != nil {
			return err
		}
		c.SecretFields = append(c.SecretFields, field)
		c.secretNames[field] = name
		if weak != 0 {
			c.WeakSecrets = append(c.WeakSecrets, field)
		}
	}
	return rows.Err()
}

// Config rebuilds the connector Config for a stored connection.
func (c *Connection) Config() Config {
	fields := make(map[string]string, len(c.Fields))
	for k, v := range c.Fields {
		fields[k] = v
	}
	return Config{
		Type:              c.ConnectorType,
		Fields:            fields,
		ResourceAllowlist: append([]string(nil), c.Allowlist...),
		Limits:            c.Limits,
	}
}

// withSecrets loads a connection's credentials for the duration of fn.
//
// The values come from the secret store's Use, which stamps the access and does
// not hand back a Value the caller can retain. They are assembled into a
// Secrets set that exists only inside fn.
func (s *Store) withSecrets(ctx context.Context, sc secrets.Scope, c *Connection, fn func(Secrets) error) error {
	values := map[string]secrets.Value{}

	var load func(i int) error
	load = func(i int) error {
		if i >= len(c.SecretFields) {
			return fn(NewSecrets(values))
		}
		field := c.SecretFields[i]
		// The stored name, not a re-derived one. Re-deriving would break the
		// moment anything about the derivation changed, and the failure would
		// be a credential that cannot be read rather than an error anyone sees
		// at deploy time.
		return s.secrets.Use(ctx, sc, c.secretNames[field], func(v secrets.Value) error {
			// Nested rather than collected in a loop, so every secret's
			// validity window encloses the callback. A loop that gathered
			// values first would keep each one live after its Use returned,
			// which is exactly what Use exists to prevent.
			values[field] = v
			return load(i + 1)
		})
	}
	return load(0)
}

// TestConnection runs a real check and records the result.
//
// The stored health is written from what the driver actually returned. There is
// no path through this function that records HEALTHY without a successful
// query, and no default that reads as healthy.
func (s *Store) TestConnection(ctx context.Context, sc secrets.Scope, id int64) (Health, error) {
	c, err := s.Get(ctx, sc.TenantID(), id)
	if err != nil {
		return Health{}, err
	}
	driver, _, err := s.registry.Driver(c.ConnectorType)
	if err != nil {
		// The connector has stopped being available since the connection was
		// created -- conformance regressed, or an operator disabled it. The
		// health is recorded as failed rather than left at its last value,
		// which would show a green connection nobody can use.
		h := Health{
			State: HealthFailed, CheckedAt: s.now().UTC(),
			ErrorCategory: ErrorNotImplemented, Detail: err.Error(),
		}
		_ = s.recordHealth(ctx, sc, c, h)
		return h, err
	}

	var health Health
	var checkErr error
	err = s.withSecrets(ctx, sc, c, func(sec Secrets) error {
		health, checkErr = driver.TestConnection(ctx, c.Config(), sec)
		return nil
	})
	if err != nil {
		health = Health{
			State: HealthFailed, CheckedAt: s.now().UTC(),
			ErrorCategory: ErrorInternal, Detail: ErrorInternal.Detail(),
		}
	}
	if health.CheckedAt.IsZero() {
		health.CheckedAt = s.now().UTC()
	}

	if err := s.recordHealth(ctx, sc, c, health); err != nil {
		return health, err
	}
	return health, checkErr
}

// recordHealth writes the observation to the connection and to its history.
func (s *Store) recordHealth(ctx context.Context, sc secrets.Scope, c *Connection, h Health) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE source_connections
		SET health_state = ?, health_checked_at = ?, health_error_class = ?,
		    health_detail = ?, health_latency_ms = ?, updated_at = ?,
		    row_version = row_version + 1
		WHERE tenant_id = ? AND id = ?`),
		string(h.State), h.CheckedAt, string(h.ErrorCategory), h.Detail,
		h.Latency.Milliseconds(), s.now().UTC(), c.TenantID, c.ID); err != nil {
		return err
	}

	// Every check is kept. A connection failing now is a different situation
	// from one failing for a week, and the second is invisible if each check
	// overwrites the last.
	_, err = tx.ExecContext(ctx, s.rebind(`
		INSERT INTO source_connection_health
			(tenant_id, connection_id, state, error_class, detail, latency_ms, actor_id, checked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		c.TenantID, c.ID, string(h.State), string(h.ErrorCategory), h.Detail,
		h.Latency.Milliseconds(), sc.ActorID(), h.CheckedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Delete removes a connection and retires its credentials.
//
// The secrets are retired rather than orphaned. A credential whose connection
// is gone is a credential nobody is watching that still authenticates against a
// customer's database.
func (s *Store) Delete(ctx context.Context, sc secrets.Scope, id int64) error {
	c, err := s.Get(ctx, sc.TenantID(), id)
	if err != nil {
		return err
	}

	for _, field := range c.SecretFields {
		ref, err := s.secrets.Get(ctx, sc, c.secretNames[field])
		if err != nil {
			continue // already gone
		}
		if err := s.secrets.Retire(ctx, sc, ref.Name, ref.Version); err != nil {
			return fmt.Errorf("the credential for %s could not be retired, so the connection "+
				"was not deleted", field)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.rebind(
		`DELETE FROM source_connection_secrets WHERE tenant_id = ? AND connection_id = ?`),
		sc.TenantID(), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.rebind(
		`DELETE FROM source_connection_health WHERE tenant_id = ? AND connection_id = ?`),
		sc.TenantID(), id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, s.rebind(
		`DELETE FROM source_connections WHERE tenant_id = ? AND id = ?`), sc.TenantID(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrConnectionNotFound
	}
	return tx.Commit()
}

// ReplaceSecret rotates one credential field.
//
// Replacing is the only way to change a stored credential; there is no update
// that takes the current value, because nothing can read the current value.
func (s *Store) ReplaceSecret(ctx context.Context, sc secrets.Scope, id int64, field, raw string) error {
	c, err := s.Get(ctx, sc.TenantID(), id)
	if err != nil {
		return err
	}
	_, descriptor, err := s.registry.Driver(c.ConnectorType)
	if err != nil {
		return err
	}
	valid := false
	for _, f := range descriptor.SecretFieldIDs() {
		if f == field {
			valid = true
		}
	}
	if !valid {
		return fmt.Errorf("%q is not a credential field of %s", field, descriptor.DisplayName)
	}

	value, weak, err := secrets.NewExternal(raw)
	if err != nil {
		return fmt.Errorf("the credential for %s could not be stored", field)
	}

	name := c.secretNames[field]
	if name == "" {
		name = secretName(c.TenantID, c.DisplayName, field)
	}
	now := s.now().UTC()

	if _, err := s.secrets.Get(ctx, sc, name); err != nil {
		// No prior version: this field was not configured before.
		if _, _, err := s.secrets.Create(ctx, sc, secrets.CreateRequest{
			Name: name, Kind: secrets.KindRetrieve, Import: value,
		}); err != nil {
			return fmt.Errorf("the credential for %s could not be stored", field)
		}
	} else {
		// Rotate with no overlap. An overlap window is right for a credential
		// this system issues and callers must pick up; a customer's database
		// password is changed at the database, and keeping the old one
		// verifiable would mean revocation did not take effect.
		if _, _, err := s.secrets.Rotate(ctx, sc, name, 0); err != nil {
			return fmt.Errorf("the credential for %s could not be rotated", field)
		}
		if _, _, err := s.secrets.Create(ctx, sc, secrets.CreateRequest{
			Name: name + "/v" + now.Format("20060102150405"),
			Kind: secrets.KindRetrieve, Import: value,
		}); err != nil {
			return fmt.Errorf("the credential for %s could not be stored", field)
		}
	}

	w := 0
	if weak {
		w = 1
	}
	_, err = s.db.ExecContext(ctx, s.rebind(`
		INSERT INTO source_connection_secrets
			(tenant_id, connection_id, field_id, secret_name, weak, created_at, rotated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, connection_id, field_id)
		DO UPDATE SET weak = EXCLUDED.weak, rotated_at = EXCLUDED.rotated_at`),
		sc.TenantID(), c.ID, field, name, w, now, now)
	return err
}

// MarkUsed stamps a connection's last use.
func (s *Store) MarkUsed(ctx context.Context, tenantID string, id int64) error {
	_, err := s.db.ExecContext(ctx, s.rebind(
		`UPDATE source_connections SET last_used_at = ? WHERE tenant_id = ? AND id = ?`),
		s.now().UTC(), tenantID, id)
	return err
}
