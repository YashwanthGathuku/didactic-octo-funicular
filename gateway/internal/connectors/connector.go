// Package connectors is the metadata-driven platform for customer source
// databases.
//
// It replaces the Integration Hub that Prompt 01 deleted. That surface returned
// `mTLSVerified: true` over plain HTTP with no client certificate and reported
// healthy connections to databases it had never contacted -- the connector
// equivalent of the zero-byte file that came back RELEASED.
//
// The rule that prevents its return is structural rather than cultural: a
// connector is selectable only when a real driver has passed the shared
// conformance suite against a real server. There is no code path that reports a
// successful connection without having made one, and `Registry.Register`
// refuses to accept a driver's claim of AVAILABLE without a conformance record
// to back it.
//
// This package is Sentinel Flow reading *customer* databases. Sentinel Flow's
// own system of record is unaffected by anything here.
package connectors

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

// Status is a catalog entry's lifecycle state.
//
// The distinction that matters is between "we intend to support this" and "a
// real driver has been tested against a real server". Collapsing them is how a
// catalog comes to advertise nine databases and connect to none.
type Status string

const (
	// StatusPlanned means no driver exists. The entry is in the catalog so the
	// field model and capability profile can be reviewed, and so nobody
	// implements it twice.
	StatusPlanned Status = "PLANNED"

	// StatusImplementing means a driver exists and has not passed conformance.
	// It is visible and not selectable.
	StatusImplementing Status = "IMPLEMENTING"

	// StatusAvailable means a real driver passed the shared conformance suite
	// against a real server, and the run is recorded. This is the only status
	// a tenant may create a connection under.
	StatusAvailable Status = "AVAILABLE"

	// StatusDegraded means it was available and its own health checks are
	// failing now. Existing connections keep working where they can; new ones
	// are refused.
	StatusDegraded Status = "DEGRADED"

	// StatusDisabled means an operator turned it off, or conformance regressed.
	StatusDisabled Status = "DISABLED"
)

// Selectable reports whether a tenant may create a connection on this status.
//
// Only AVAILABLE. DEGRADED deliberately does not qualify: a degraded connector
// is one whose real health checks are failing, and letting someone configure a
// new connection against it produces a configuration that has never worked and
// an operator who believes it has.
func (s Status) Selectable() bool { return s == StatusAvailable }

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

// ParameterStyle is how a driver spells a bound parameter.
//
// It is part of the capability model rather than hidden behind a rewriting
// layer, because rewriting placeholders means parsing SQL, and a parser that is
// wrong about a string literal turns a parameterised query into an injectable
// one.
type ParameterStyle string

const (
	// ParamDollar is $1, $2 -- PostgreSQL, Redshift.
	ParamDollar ParameterStyle = "DOLLAR"
	// ParamQuestion is ? -- MySQL, MariaDB, Databricks.
	ParamQuestion ParameterStyle = "QUESTION"
	// ParamAtNamed is @p1 -- SQL Server.
	ParamAtNamed ParameterStyle = "AT_NAMED"
	// ParamColonNamed is :name -- Oracle.
	ParamColonNamed ParameterStyle = "COLON_NAMED"
	// ParamNamed is @name -- BigQuery.
	ParamNamed ParameterStyle = "NAMED"
)

// Capabilities describes what a database can actually do.
//
// Databases do not share identical behaviour and pretending they do produces a
// lowest-common-denominator layer that is wrong about each of them. Oracle has
// no schema-qualified catalogs in the PostgreSQL sense; BigQuery has no
// read-only transaction; Snowflake quotes identifiers differently under
// different account settings. Each of those is a correctness question when the
// thing being generated is a query against a customer's financial data.
type Capabilities struct {
	// Structure.
	SupportsSchemas  bool
	SupportsCatalogs bool
	IdentifierQuote  string // `"` for PostgreSQL, backtick for MySQL, `[` for SQL Server
	ParameterStyle   ParameterStyle

	// Execution controls. Each of these is a control this package relies on,
	// so a false value is a statement that the corresponding protection must
	// come from somewhere else -- a session setting, a database role, or a
	// refusal to support the connector at all.
	ReadOnlyTransactions bool
	StatementTimeout     bool
	Cancellation         bool
	CursorPagination     bool

	// Transport.
	TLSModes  []string
	AuthModes []string

	// Bounds. Zero means the connector declares no limit of its own and the
	// platform's limit applies.
	MaxRows  int64
	MaxBytes int64

	// What may be asked of it.
	MetadataQueries  bool
	AggregateQueries bool
}

// ---------------------------------------------------------------------------
// The connector interface
// ---------------------------------------------------------------------------

// Resource is one discovered object a connection is allowed to see.
type Resource struct {
	Catalog string
	Schema  string
	Name    string
	Kind    string // TABLE | VIEW | MATERIALIZED_VIEW
}

// Qualified renders the resource for display. It is not a SQL identifier and
// must never be interpolated into a statement; see identifier allowlisting in
// query.go.
func (r Resource) Qualified() string {
	switch {
	case r.Catalog != "" && r.Schema != "":
		return r.Catalog + "." + r.Schema + "." + r.Name
	case r.Schema != "":
		return r.Schema + "." + r.Name
	default:
		return r.Name
	}
}

// HealthState is a real, timestamped observation.
type HealthState string

const (
	// HealthNeverChecked is the state of a connection nobody has tested. It is
	// a distinct state from healthy for the reason the whole package exists: a
	// UI that shows an untested connection as green is lying.
	HealthNeverChecked HealthState = "NEVER_CHECKED"
	HealthHealthy      HealthState = "HEALTHY"
	HealthDegraded     HealthState = "DEGRADED"
	HealthFailed       HealthState = "FAILED"
)

// Health is the result of a real check.
//
// CheckedAt is not optional. "Healthy" with no timestamp is a claim about the
// present derived from an observation of unknown age.
type Health struct {
	State     HealthState
	CheckedAt time.Time
	// ErrorCategory is a sanitized classification, never the driver's message.
	// Driver errors routinely contain the host, the username, and sometimes the
	// credential; see errors.go.
	ErrorCategory ErrorCategory
	// Detail is a redacted, human-readable summary safe for an operator screen.
	Detail  string
	Latency time.Duration
}

// QueryResult is the bounded outcome of an approved template.
type QueryResult struct {
	Columns []Column
	Rows    [][]any
	// Truncated reports that a limit stopped the read. It is reported rather
	// than hidden: a silently truncated result is a wrong answer that looks
	// like a right one.
	Truncated  bool
	LimitHit   string // "rows" | "bytes" | "time"
	Elapsed    time.Duration
	NextCursor string
}

// Column carries the classification a result must be masked by.
type Column struct {
	Name           string
	DatabaseType   string
	Classification Classification
}

// Connector is one driver.
//
// Every method is bounded and takes a context, and none accepts free-form SQL.
// ExecuteTemplate runs an administrator-approved parameterised template chosen
// by id -- there is deliberately no entry point through which a browser, an
// operator screen, or the AI tier can send a statement of its own.
type Connector interface {
	// Type is the catalog identifier, e.g. "postgresql".
	Type() string

	// Capabilities describes what this driver's database can do.
	Capabilities() Capabilities

	// ValidateConfig performs structural checks only. It makes no network call
	// and never logs, returns, or embeds a secret in its errors.
	ValidateConfig(cfg Config) error

	// TestConnection makes a real, bounded, read-only check. It must never
	// report success without having connected.
	TestConnection(ctx context.Context, cfg Config, sec Secrets) (Health, error)

	// DiscoverResources returns approved metadata only, restricted to the
	// connection's resource allowlist.
	DiscoverResources(ctx context.Context, cfg Config, sec Secrets) ([]Resource, error)

	// ExecuteTemplate runs one administrator-approved parameterised query.
	ExecuteTemplate(ctx context.Context, cfg Config, sec Secrets, t Template, args map[string]any, limits Limits) (*QueryResult, error)

	// Health re-checks an existing connection.
	Health(ctx context.Context, cfg Config, sec Secrets) Health

	// Close releases pooled resources. It must be safe to call twice and must
	// leave no goroutine behind; the conformance suite checks both.
	Close() error
}

// Limits bound a single execution.
//
// Every field has a platform default and a connection may only lower them. A
// connection that could raise its own limits is a connection that can exhaust
// the gateway on behalf of a customer database.
type Limits struct {
	MaxRows    int64
	MaxBytes   int64
	Timeout    time.Duration
	CursorSize int
}

// DefaultLimits are the platform's bounds.
//
// They are deliberately small. This platform reads approved metadata and
// aggregates, not financial rows; a limit sized for bulk extraction would
// invite bulk extraction.
func DefaultLimits() Limits {
	return Limits{
		MaxRows:    10_000,
		MaxBytes:   8 << 20,
		Timeout:    30 * time.Second,
		CursorSize: 1_000,
	}
}

// Clamp lowers l to fit within the platform maximum, never raising it.
func (l Limits) Clamp(max Limits) Limits {
	out := l
	if out.MaxRows <= 0 || out.MaxRows > max.MaxRows {
		out.MaxRows = max.MaxRows
	}
	if out.MaxBytes <= 0 || out.MaxBytes > max.MaxBytes {
		out.MaxBytes = max.MaxBytes
	}
	if out.Timeout <= 0 || out.Timeout > max.Timeout {
		out.Timeout = max.Timeout
	}
	if out.CursorSize <= 0 || out.CursorSize > max.CursorSize {
		out.CursorSize = max.CursorSize
	}
	return out
}

// ErrNotImplemented is returned by a catalog entry with no driver.
//
// It is a real error, not an empty success. A PLANNED connector that returned a
// healthy result would be the Integration Hub again.
var ErrNotImplemented = errors.New("this connector has no implementation yet")

// ErrNotSelectable is returned when a connection is attempted on a connector
// that has not passed conformance.
var ErrNotSelectable = errors.New("this connector is not available: no driver has passed the conformance suite")

// notImplemented is the driver every PLANNED catalog entry gets.
//
// Giving planned entries a driver that refuses is safer than giving them none:
// a nil driver invites a nil check that someone forgets, and the failure of a
// forgotten nil check is a panic in the request path rather than a clear
// refusal.
type notImplemented struct {
	connectorType string
	reason        string
}

func (n notImplemented) Type() string                { return n.connectorType }
func (n notImplemented) Capabilities() Capabilities  { return Capabilities{} }
func (n notImplemented) ValidateConfig(Config) error { return n.err() }
func (n notImplemented) Close() error                { return nil }

func (n notImplemented) err() error {
	return fmt.Errorf("%s: %w (%s)", n.connectorType, ErrNotImplemented, n.reason)
}

func (n notImplemented) TestConnection(context.Context, Config, Secrets) (Health, error) {
	return Health{State: HealthNeverChecked}, n.err()
}

func (n notImplemented) DiscoverResources(context.Context, Config, Secrets) ([]Resource, error) {
	return nil, n.err()
}

func (n notImplemented) ExecuteTemplate(context.Context, Config, Secrets, Template, map[string]any, Limits) (*QueryResult, error) {
	return nil, n.err()
}

func (n notImplemented) Health(context.Context, Config, Secrets) Health {
	return Health{
		State:         HealthNeverChecked,
		CheckedAt:     time.Now().UTC(),
		ErrorCategory: ErrorNotImplemented,
		Detail:        n.reason,
	}
}
