package connectors

import (
	"fmt"
	"net/url"
	"strings"

	"sentinel-gateway/internal/secrets"
)

// Pasting a connection URI.
//
// A convenience, offered only where a URI is a real thing the provider defines.
// It is not offered for BigQuery, Snowflake or Databricks: those have no
// password URI, so a paste box there would invite an operator to paste
// something else -- most likely a service-account JSON -- into a field that is
// not a secret field.
//
// The rule for the ones that do support it: parse immediately, split the
// credential into the secret store, and discard the raw string. It is never
// stored, logged, echoed, put in an audit payload, or returned. The parsed
// fields and a masked summary are all that survives.

// ParsedURI is the result of a paste.
type ParsedURI struct {
	// Fields are the non-secret values extracted from the URI.
	Fields map[string]string
	// Secret is the credential, already sealed. The raw URI is gone by the
	// time this returns.
	Secret secrets.Value
	// HasSecret reports whether the URI carried one.
	HasSecret bool
	// WeakSecret reports a credential shorter than this application's floor.
	// Surfaced to the operator rather than refused: it is the customer's
	// password and refusing it would not make it longer.
	WeakSecret bool
	// Warnings names settings in the URI that weaken a control, so the wizard
	// can show them before the connection is saved rather than after it is
	// attacked.
	Warnings []string
}

// ParseConnectionURI splits a pasted connection string.
//
// The returned error never contains the input. A malformed URI is very often a
// well-formed one with a typo, which means the string being reported is a
// working credential.
func ParseConnectionURI(d Descriptor, raw string) (*ParsedURI, error) {
	if !d.SupportsURIPaste {
		return nil, fmt.Errorf("%s does not have a connection URI; use the structured fields", d.DisplayName)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("the connection string is empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		// The parse error from net/url quotes the input. It is discarded.
		return nil, fmt.Errorf("the connection string could not be parsed")
	}

	out := &ParsedURI{Fields: map[string]string{}}

	switch d.Type {
	case "postgresql", "redshift":
		if u.Scheme != "postgres" && u.Scheme != "postgresql" {
			return nil, fmt.Errorf("expected a postgres:// connection string")
		}
	case "sqlserver":
		if u.Scheme != "sqlserver" {
			return nil, fmt.Errorf("expected a sqlserver:// connection string")
		}
	case "oracle":
		if u.Scheme != "oracle" {
			return nil, fmt.Errorf("expected an oracle:// connection string")
		}
	case "mysql", "mariadb":
		// The MySQL DSN is not a URL, so it is parsed separately below.
		return parseMySQLDSN(d, raw)
	default:
		return nil, fmt.Errorf("%s does not have a connection URI", d.DisplayName)
	}

	out.Fields["host"] = u.Hostname()
	if p := u.Port(); p != "" {
		out.Fields["port"] = p
	}
	if path := strings.TrimPrefix(u.Path, "/"); path != "" {
		if d.Type == "oracle" {
			out.Fields["service_name"] = path
		} else {
			out.Fields["database"] = path
		}
	}
	if u.User != nil {
		out.Fields["username"] = u.User.Username()
		if pw, ok := u.User.Password(); ok && pw != "" {
			sealed, weak, err := secrets.NewExternal(pw)
			if err != nil {
				return nil, fmt.Errorf("the credential in the connection string could not be stored")
			}
			out.Secret, out.HasSecret, out.WeakSecret = sealed, true, weak
		}
	}

	q := u.Query()
	if d.Type == "sqlserver" {
		if db := q.Get("database"); db != "" {
			out.Fields["database"] = db
		}
	}
	for _, key := range []string{"sslmode", "tls", "encrypt", "ssl-mode"} {
		if v := q.Get(key); v != "" {
			out.Fields["tls_mode"] = v
			break
		}
	}

	// A pasted string frequently carries a weakened transport setting the
	// person pasting it has never looked at. It is surfaced rather than
	// silently accepted.
	if mode := out.Fields["tls_mode"]; mode != "" {
		if f, ok := d.FieldByID("tls_mode"); ok {
			for _, o := range f.Options {
				if o.Value == mode && o.Insecure {
					out.Warnings = append(out.Warnings, fmt.Sprintf(
						"the pasted string selects %q, which does not verify the server's identity", o.Label))
				}
			}
			if !f.allows(mode) {
				out.Warnings = append(out.Warnings, fmt.Sprintf(
					"the pasted string selects a TLS mode this platform does not offer (%q); "+
						"choose one from the list", mode))
				delete(out.Fields, "tls_mode")
			}
		}
	} else {
		out.Warnings = append(out.Warnings,
			"the pasted string names no TLS mode; choose one before saving")
	}

	if out.HasSecret && out.WeakSecret {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"the credential is shorter than %d characters", secrets.MinSecretLength))
	}
	return out, nil
}

// parseMySQLDSN handles the Go MySQL DSN, which is not a URL.
//
// `user:pass@tcp(host:3306)/db?tls=...`. Written out by hand because feeding it
// to url.Parse produces a plausible-looking wrong answer -- the `@tcp(...)`
// section parses as a host -- and a wrong answer here means a credential ends
// up in the host field.
func parseMySQLDSN(d Descriptor, raw string) (*ParsedURI, error) {
	out := &ParsedURI{Fields: map[string]string{}}

	at := strings.LastIndex(raw, "@")
	if at < 0 {
		return nil, fmt.Errorf("expected a MySQL DSN of the form user:password@tcp(host:port)/database")
	}
	credentials, remainder := raw[:at], raw[at+1:]

	if colon := strings.Index(credentials, ":"); colon >= 0 {
		out.Fields["username"] = credentials[:colon]
		if pw := credentials[colon+1:]; pw != "" {
			sealed, weak, err := secrets.NewExternal(pw)
			if err != nil {
				return nil, fmt.Errorf("the credential in the connection string could not be stored")
			}
			out.Secret, out.HasSecret, out.WeakSecret = sealed, true, weak
		}
	} else {
		out.Fields["username"] = credentials
	}

	open, close := strings.Index(remainder, "("), strings.Index(remainder, ")")
	if open < 0 || close < open {
		return nil, fmt.Errorf("expected a MySQL DSN of the form user:password@tcp(host:port)/database")
	}
	address := remainder[open+1 : close]
	if colon := strings.LastIndex(address, ":"); colon >= 0 {
		out.Fields["host"] = address[:colon]
		out.Fields["port"] = address[colon+1:]
	} else {
		out.Fields["host"] = address
	}

	rest := remainder[close+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		out.Fields["database"] = strings.TrimPrefix(rest[:q], "/")
		if values, err := url.ParseQuery(rest[q+1:]); err == nil {
			if tls := values.Get("tls"); tls != "" {
				out.Fields["tls_mode"] = tls
			}
		}
	} else {
		out.Fields["database"] = strings.TrimPrefix(rest, "/")
	}

	if out.Fields["tls_mode"] == "" {
		out.Warnings = append(out.Warnings,
			"the pasted string names no TLS mode; choose one before saving")
	}
	if out.HasSecret && out.WeakSecret {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"the credential is shorter than %d characters", secrets.MinSecretLength))
	}
	_ = d
	return out, nil
}
