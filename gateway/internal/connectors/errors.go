package connectors

import (
	"context"
	"errors"
	"net"
	"strings"

	"sentinel-gateway/internal/secrets"
)

// ErrorCategory is what a connector failure is allowed to say.
//
// Driver error messages are the single worst disclosure surface in this
// package. They routinely carry the host, the port, the username and the
// database, and several drivers include the whole DSN -- credential included --
// when a connection string fails to parse. `pq: password authentication failed
// for user "svc_reporting"` is a real message from a real driver and it names
// the account an attacker should try next.
//
// So a failure is classified, and the category is what reaches the operator,
// the API, the metrics and the audit trail. The driver's own text is redacted
// and kept for the server log only.
type ErrorCategory string

const (
	// ErrorNone is a successful check.
	ErrorNone ErrorCategory = ""
	// ErrorConfiguration is a structurally invalid configuration. No network
	// call was made.
	ErrorConfiguration ErrorCategory = "CONFIGURATION"
	// ErrorUnreachable is a DNS, routing or refused-connection failure.
	ErrorUnreachable ErrorCategory = "UNREACHABLE"
	// ErrorTLS is a certificate or protocol negotiation failure. It is distinct
	// from ErrorUnreachable because the responses differ completely: one is a
	// network problem, the other is a possible interception.
	ErrorTLS ErrorCategory = "TLS"
	// ErrorAuthentication covers every credential failure, without saying which
	// part was wrong. Distinguishing "no such user" from "wrong password" is a
	// username oracle.
	ErrorAuthentication ErrorCategory = "AUTHENTICATION"
	// ErrorAuthorization is connected-but-forbidden: the identity exists and
	// lacks the grant.
	ErrorAuthorization ErrorCategory = "AUTHORIZATION"
	// ErrorTimeout is a deadline or cancellation.
	ErrorTimeout ErrorCategory = "TIMEOUT"
	// ErrorLimitExceeded is a row, byte or pool bound being hit.
	ErrorLimitExceeded ErrorCategory = "LIMIT_EXCEEDED"
	// ErrorNotAllowed is a request this platform refuses: a write, a DDL
	// statement, an unapproved resource.
	ErrorNotAllowed ErrorCategory = "NOT_ALLOWED"
	// ErrorNotImplemented is a connector with no driver.
	ErrorNotImplemented ErrorCategory = "NOT_IMPLEMENTED"
	// ErrorProvider is a fault on the customer's side that is none of the
	// above.
	ErrorProvider ErrorCategory = "PROVIDER"
	// ErrorInternal is a fault in this gateway.
	ErrorInternal ErrorCategory = "INTERNAL"
)

// safeDetail is the operator-facing sentence for a category.
//
// One fixed sentence per category, chosen so the two failures an attacker most
// wants to tell apart -- a username that exists and one that does not -- read
// identically.
var safeDetail = map[ErrorCategory]string{
	ErrorConfiguration:  "the connection settings are incomplete or invalid",
	ErrorUnreachable:    "the database could not be reached from this gateway",
	ErrorTLS:            "the TLS handshake failed or the server certificate was not trusted",
	ErrorAuthentication: "the database rejected the supplied credentials",
	ErrorAuthorization:  "the identity connected but is not permitted to perform this operation",
	ErrorTimeout:        "the operation exceeded its time limit and was cancelled",
	ErrorLimitExceeded:  "the operation exceeded a configured row, byte or connection limit",
	ErrorNotAllowed:     "this operation is not permitted by the connector platform",
	ErrorNotImplemented: "this connector has no driver yet",
	ErrorProvider:       "the database reported an error",
	ErrorInternal:       "the gateway could not complete the operation",
}

// Detail returns the fixed operator-facing sentence for a category.
func (c ErrorCategory) Detail() string {
	if d, ok := safeDetail[c]; ok {
		return d
	}
	return safeDetail[ErrorProvider]
}

// ConnectorError is a classified, safe-to-surface failure.
//
// It keeps the underlying error for errors.Is and errors.As -- a wrapper that
// broke error matching would be unwrapped by the first person it inconvenienced
// -- while its own Error() emits only the category's fixed sentence.
type ConnectorError struct {
	Category ErrorCategory
	// Operation names what was being attempted, e.g. "test_connection".
	Operation string
	// wrapped is the driver's error. It is redacted before it is ever
	// rendered, and it is never included in Error().
	wrapped error
}

func (e *ConnectorError) Error() string {
	if e.Operation != "" {
		return e.Operation + ": " + e.Category.Detail()
	}
	return e.Category.Detail()
}

func (e *ConnectorError) Unwrap() error { return e.wrapped }

// LogDetail returns the redacted driver message, for the server log only.
//
// Named so the call site says what it is. It runs through the Prompt 05
// scrubber, which removes URL userinfo, authorization headers, PEM blocks, JWTs
// and sensitive query parameters -- the shapes a DSN in an error message takes.
func (e *ConnectorError) LogDetail() string {
	if e.wrapped == nil {
		return e.Category.Detail()
	}
	return secrets.Redact(e.wrapped.Error())
}

// Classify turns a driver error into a safe one.
//
// The default is ErrorProvider rather than ErrorInternal: an unrecognised
// failure from a customer's database is far more likely to be theirs than ours,
// and calling it internal would send an operator to read the wrong logs.
func Classify(operation string, err error) *ConnectorError {
	if err == nil {
		return nil
	}
	return &ConnectorError{
		Category:  categorize(err),
		Operation: operation,
		wrapped:   err,
	}
}

// Categorized wraps an error with a category already decided.
func Categorized(operation string, category ErrorCategory, err error) *ConnectorError {
	return &ConnectorError{Category: category, Operation: operation, wrapped: err}
}

func categorize(err error) ErrorCategory {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return ErrorTimeout
	case errors.Is(err, ErrNotImplemented):
		return ErrorNotImplemented
	case errors.Is(err, ErrNotAllowed):
		return ErrorNotAllowed
	case errors.Is(err, ErrLimitExceeded):
		return ErrorLimitExceeded
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrorTimeout
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ErrorUnreachable
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return ErrorUnreachable
	}

	// Text matching is a last resort and is used only for classification, never
	// for anything the message itself is carried into. Drivers do not export
	// comparable error values for most of these conditions, so the choice is
	// between matching text and reporting every failure as PROVIDER -- which
	// would collapse the TLS and authentication cases an operator most needs to
	// tell apart.
	msg := strings.ToLower(err.Error())
	switch {
	case containsAny(msg, "certificate", "tls:", "x509", "ssl", "handshake"):
		return ErrorTLS
	case containsAny(msg, "password authentication", "authentication failed",
		"login failed", "invalid credentials", "access denied for user",
		"role \"", "no pg_hba.conf entry", "invalid_grant", "unauthorized"):
		return ErrorAuthentication
	case containsAny(msg, "permission denied", "must be owner", "not authorized",
		"insufficient privilege", "forbidden"):
		return ErrorAuthorization
	case containsAny(msg, "connection refused", "no such host", "network is unreachable",
		"connection reset", "server closed", "dial "):
		return ErrorUnreachable
	case containsAny(msg, "timeout", "timed out", "canceling statement", "query_canceled"):
		return ErrorTimeout
	case containsAny(msg, "too many connections", "connection limit", "resource exhausted"):
		return ErrorLimitExceeded
	}
	return ErrorProvider
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ErrNotAllowed is returned when the platform refuses a request outright: a
// write, a DDL statement, an unapproved resource, a second statement.
var ErrNotAllowed = errors.New("the connector platform does not permit this operation")

// ErrLimitExceeded is returned when a row, byte or time bound stops a read.
var ErrLimitExceeded = errors.New("a connector limit was exceeded")
