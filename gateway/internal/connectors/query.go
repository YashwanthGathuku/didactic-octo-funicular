package connectors

import (
	"fmt"
	"regexp"
	"strings"
)

// What may be asked of a customer database.
//
// The answer is: an administrator-approved parameterised template, and nothing
// else. There is no entry point through which a browser, an operator screen, or
// the AI tier can send a statement of its own -- not a restricted one, not a
// filtered one. Filtering SQL means parsing SQL, and every filter of that kind
// is eventually defeated by a dialect feature its author did not know about.

// Template is an administrator-approved parameterised query.
//
// The SQL is fixed at definition time and never assembled from a request. The
// only things a caller supplies are parameter values, which are bound, and
// identifier choices, which are checked against an allowlist rather than
// interpolated freely.
type Template struct {
	ID          string
	Description string
	// ConnectorType restricts a template to the database it was written for.
	// A statement that is read-only in PostgreSQL is not necessarily read-only
	// elsewhere, and dialects differ enough that a shared template would have
	// to be written to the intersection.
	ConnectorType string

	// SQL is the fixed statement, with the driver's own parameter placeholders.
	SQL string

	// Params names the parameters in binding order.
	Params []string

	// Identifiers names the placeholders in SQL that take an identifier rather
	// than a value -- a schema or table name, which no database allows to be
	// bound as a parameter. Each supplied identifier is checked against the
	// connection's resource allowlist and quoted by the driver.
	Identifiers []string

	// Classifications declares each returned column's sensitivity. A column
	// absent from this map is treated as UNCLASSIFIED, which masks.
	Classifications map[string]Classification

	// resolved carries the identifier values chosen for one execution.
	//
	// Unexported so a caller cannot set it directly: it is populated by
	// WithIdentifiers, which is the only path, and the driver resolves it
	// against the connection's allowlist before the statement is built. A
	// template arriving with identifiers nobody checked is the shape of the
	// bug this design exists to make unrepresentable.
	resolved map[string]string
}

// WithIdentifiers attaches the identifier values for one execution.
//
// The values are not trusted here; they are validated and allowlist-checked by
// Resolve at the moment the statement is built.
func (t Template) WithIdentifiers(idents map[string]string) Template {
	out := t
	out.resolved = idents
	return out
}

// Identifiers chosen for this execution, for the driver to resolve.
func (t Template) ChosenIdentifiers() map[string]string { return t.resolved }

// Classification returns a column's declared class, defaulting to the most
// restrictive.
func (t Template) Classification(column string) Classification {
	if c, ok := t.Classifications[column]; ok {
		return c
	}
	return ClassUnclassified
}

// identifierPattern is what may name a schema, table or column.
//
// Deliberately narrow: ASCII letters, digits and underscore, starting with a
// letter or underscore, at most 63 characters -- the shortest limit among the
// supported databases. Anything a customer names outside this set cannot be
// read through this platform, which is a real limitation and a cheap price for
// removing quoting from the trust chain entirely.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

// ValidateIdentifier checks one identifier component.
//
// This is an allowlist, not an escaper. The difference matters: an escaper has
// to be right about every quoting rule of every dialect, including the ones
// that change with a session setting -- MySQL's ANSI_QUOTES, SQL Server's
// QUOTED_IDENTIFIER, Snowflake's case folding. An allowlist has to be right
// about one regular expression.
func ValidateIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("identifier is empty")
	}
	if !identifierPattern.MatchString(s) {
		return fmt.Errorf("identifier %q is not permitted: use letters, digits and underscore, "+
			"starting with a letter or underscore, at most 63 characters", redactIdentifier(s))
	}
	return nil
}

// ValidateQualifiedName checks a dotted name component by component.
func ValidateQualifiedName(s string) error {
	if s == "" {
		return fmt.Errorf("qualified name is empty")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return fmt.Errorf("qualified name has %d parts, at most three are permitted", len(parts))
	}
	for _, p := range parts {
		if err := ValidateIdentifier(p); err != nil {
			return err
		}
	}
	return nil
}

// redactIdentifier bounds what a rejected identifier can put into an error.
//
// A rejected identifier is attacker-controlled, and an error message is
// something that reaches a log, an API response and possibly a screen. It is
// truncated and stripped of control characters so it cannot forge a log line.
func redactIdentifier(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i >= 40 {
			b.WriteString("...")
			break
		}
		if r < 0x20 || r == 0x7f {
			b.WriteRune('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// forbiddenStatement matches anything this platform will not run.
//
// This is a second line of defence, not the first. The first is that SQL comes
// only from a registered template written by an administrator. This check runs
// at registration time, so a template containing a write is rejected when it is
// added rather than when it is executed -- the failure lands on the person who
// wrote it.
var forbiddenStatement = regexp.MustCompile(
	`(?is)\b(insert|update|delete|merge|upsert|truncate|drop|create|alter|grant|revoke|` +
		`comment|rename|call|do|copy|vacuum|analyze|reindex|cluster|lock|set|reset|` +
		`begin|commit|rollback|savepoint|prepare|execute|deallocate|listen|notify|` +
		`load|import|export|attach|detach|pragma|use)\b`)

// dangerousFunction matches provider functions that reach outside the database.
//
// Each of these is a documented way to turn read access into file or network
// access on the database host: PostgreSQL's large-object and COPY-adjacent
// functions, MySQL's LOAD_FILE, SQL Server's xp_cmdshell and OPENROWSET,
// Oracle's UTL_HTTP and UTL_FILE.
var dangerousFunction = regexp.MustCompile(
	`(?is)\b(pg_read_file|pg_read_binary_file|pg_ls_dir|pg_stat_file|lo_import|lo_export|` +
		`dblink|postgres_fdw|load_file|into\s+outfile|into\s+dumpfile|` +
		`xp_cmdshell|sp_oacreate|openrowset|opendatasource|bulk\s+insert|` +
		`utl_http|utl_file|utl_smtp|utl_tcp|dbms_scheduler|dbms_lob|` +
		`external\s+function|copy\s+into|system\$)\b`)

// RegisterTemplate validates a template and returns it.
//
// Every check happens here rather than at execution, so a bad template is
// rejected by the administrator who wrote it and never reaches a customer
// database.
func RegisterTemplate(t Template) (Template, error) {
	if strings.TrimSpace(t.ID) == "" {
		return Template{}, fmt.Errorf("a query template needs an id")
	}
	if strings.TrimSpace(t.ConnectorType) == "" {
		return Template{}, fmt.Errorf("template %s: a connector type is required; a statement that "+
			"is read-only in one dialect is not necessarily read-only in another", t.ID)
	}
	sql := strings.TrimSpace(t.SQL)
	if sql == "" {
		return Template{}, fmt.Errorf("template %s: the statement is empty", t.ID)
	}

	// A trailing semicolon is stripped; an interior one is refused. Multi-
	// statement execution is how a single injected parameter becomes a write,
	// and several drivers enable it by default.
	sql = strings.TrimSuffix(sql, ";")
	if strings.Contains(sql, ";") {
		return Template{}, fmt.Errorf("template %s: %w -- it contains more than one statement", t.ID, ErrNotAllowed)
	}

	// Comment markers are refused. A template is not a place for commentary,
	// and `--` before a newline is the classic way to make the tail of a
	// statement disappear.
	if strings.Contains(sql, "--") || strings.Contains(sql, "/*") {
		return Template{}, fmt.Errorf("template %s: %w -- it contains a SQL comment", t.ID, ErrNotAllowed)
	}

	if !startsWithRead(sql) {
		return Template{}, fmt.Errorf("template %s: %w -- a template must begin with SELECT or WITH",
			t.ID, ErrNotAllowed)
	}
	if forbiddenStatement.MatchString(sql) {
		return Template{}, fmt.Errorf("template %s: %w -- it contains a write, DDL or session-control keyword",
			t.ID, ErrNotAllowed)
	}
	if dangerousFunction.MatchString(sql) {
		return Template{}, fmt.Errorf("template %s: %w -- it calls a function that can reach the "+
			"filesystem or network of the database host", t.ID, ErrNotAllowed)
	}

	for _, id := range t.Identifiers {
		if !strings.Contains(sql, identifierPlaceholder(id)) {
			return Template{}, fmt.Errorf("template %s: identifier %q is declared but its placeholder "+
				"%s does not appear in the statement", t.ID, id, identifierPlaceholder(id))
		}
	}
	t.SQL = sql
	return t, nil
}

// startsWithRead reports whether the statement begins with SELECT or WITH.
func startsWithRead(sql string) bool {
	lower := strings.ToLower(strings.TrimSpace(sql))
	return strings.HasPrefix(lower, "select") || strings.HasPrefix(lower, "with")
}

// identifierPlaceholder is the token a template uses for an identifier slot.
//
// A distinct syntax from the driver's parameter placeholders, so the two can
// never be confused: parameters are bound by the driver, identifiers are
// substituted here after allowlist validation and quoting.
func identifierPlaceholder(name string) string { return "{{" + name + "}}" }

// Resolve substitutes validated, quoted identifiers into a template.
//
// Every identifier is checked against the connection's resource allowlist
// before it is quoted. The allowlist check is what makes this safe; the quoting
// is what makes it work.
func (t Template) Resolve(identifiers map[string]string, allowlist []string, quote string) (string, error) {
	sql := t.SQL
	for _, name := range t.Identifiers {
		value, ok := identifiers[name]
		if !ok {
			return "", fmt.Errorf("template %s: no value supplied for identifier %q", t.ID, name)
		}
		if err := ValidateQualifiedName(value); err != nil {
			return "", fmt.Errorf("template %s: %w", t.ID, err)
		}
		if !allowed(value, allowlist) {
			// The name is not echoed. A caller probing for which tables exist
			// learns nothing from a refusal that does not repeat its guess.
			return "", fmt.Errorf("template %s: %w -- the requested resource is not in this "+
				"connection's approved list", t.ID, ErrNotAllowed)
		}
		sql = strings.ReplaceAll(sql, identifierPlaceholder(name), quoteQualified(value, quote))
	}
	return sql, nil
}

// allowed reports whether a qualified name is covered by the allowlist.
//
// An allowlist entry may name a schema, which covers every object in it, or a
// fully qualified object. Matching is case-insensitive because the supported
// databases disagree about identifier folding and a case mismatch would present
// as a missing table.
func allowed(name string, allowlist []string) bool {
	lower := strings.ToLower(name)
	for _, entry := range allowlist {
		e := strings.ToLower(strings.TrimSpace(entry))
		if e == "" {
			continue
		}
		if lower == e || strings.HasPrefix(lower, e+".") {
			return true
		}
	}
	return false
}

// quoteQualified quotes each component of a dotted name.
//
// The components have already passed ValidateIdentifier, so they contain no
// quote character and nothing needs escaping. That ordering is the point: this
// function is safe because of what ran before it, not because of what it does.
func quoteQualified(name, quote string) string {
	if quote == "" {
		return name
	}
	closing := quote
	if quote == "[" {
		closing = "]"
	}
	parts := strings.Split(name, ".")
	for i, p := range parts {
		parts[i] = quote + p + closing
	}
	return strings.Join(parts, ".")
}

// BindArgs orders the supplied values to match the template's parameter list.
//
// A parameter with no supplied value is an error rather than a NULL. Silently
// binding NULL turns "the caller forgot a filter" into "the query matched
// nothing", which reads as a correct empty result.
func (t Template) BindArgs(args map[string]any) ([]any, error) {
	out := make([]any, 0, len(t.Params))
	for _, name := range t.Params {
		v, ok := args[name]
		if !ok {
			return nil, fmt.Errorf("template %s: no value supplied for parameter %q", t.ID, name)
		}
		out = append(out, v)
	}
	if len(args) != len(t.Params) {
		return nil, fmt.Errorf("template %s: %d parameters supplied, %d expected",
			t.ID, len(args), len(t.Params))
	}
	return out, nil
}

// MaskValue applies a column's classification to one value.
//
// Masking happens at the boundary of this package, so a caller cannot forget
// it: everything a driver returns has already been through here.
func MaskValue(v any, c Classification) any {
	if !c.Masked() || v == nil {
		return v
	}
	s := fmt.Sprint(v)
	if len(s) <= 4 {
		return "****"
	}
	// The last four characters are kept, which is the convention every
	// financial operator already reads, and is short enough not to identify.
	return "****" + s[len(s)-4:]
}
