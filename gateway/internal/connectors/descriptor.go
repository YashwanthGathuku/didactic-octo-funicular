package connectors

import (
	"fmt"
	"strconv"
	"strings"
)

// The server-owned field model.
//
// The UI renders whatever the descriptor says and knows nothing about any
// specific database. That is what makes the connection wizard generic: adding
// Oracle adds a descriptor, not a React component, and there is no second place
// where "Oracle needs a service name" has to be remembered.
//
// It also puts the security rules on the server. A field marked Secret is
// write-only *here*, so a UI that forgets to mask it still cannot read it back
// -- the API does not return it.

// FieldKind is how a value is entered and, more importantly, how it is handled.
type FieldKind string

const (
	// KindText is a plain string.
	KindText FieldKind = "TEXT"
	// KindNumber is an integer, typically a port.
	KindNumber FieldKind = "NUMBER"
	// KindEnum is a fixed choice.
	KindEnum FieldKind = "ENUM"
	// KindBool is a checkbox.
	KindBool FieldKind = "BOOL"
	// KindSecret is write-only. It is never returned by any read path, and the
	// UI must render it as an empty field with a "replace" action rather than
	// as a populated one.
	KindSecret FieldKind = "SECRET"
	// KindSecretRef names an existing secret-store entry rather than carrying
	// a value. Used where the credential is managed elsewhere: a workload
	// identity, a wallet, a service-account file.
	KindSecretRef FieldKind = "SECRET_REF"
	// KindList is a repeated string, used for resource allowlists.
	KindList FieldKind = "LIST"
)

// WriteOnly reports whether a field's value may ever be read back.
func (k FieldKind) WriteOnly() bool { return k == KindSecret }

// Field is one input in the connection wizard.
type Field struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Kind     FieldKind `json:"kind"`
	Required bool      `json:"required"`

	// Help is shown under the input. It explains the field in the customer
	// DBA's vocabulary, because the person filling this in is reading their own
	// provider's console, not ours.
	Help string `json:"help,omitempty"`

	// Placeholder is an example, never a default that would be submitted.
	Placeholder string `json:"placeholder,omitempty"`

	// Default is applied when the field is left blank.
	Default string `json:"default,omitempty"`

	// Options are the permitted values for KindEnum.
	Options []Option `json:"options,omitempty"`

	// Pattern is a server-side validation description, human-readable. The
	// server validates; this is what the UI shows when it fails.
	Rule string `json:"rule,omitempty"`

	// AppliesToAuth restricts the field to particular authentication modes, so
	// selecting "key pair" hides the password input entirely rather than
	// showing an irrelevant one the user might fill in.
	AppliesToAuth []string `json:"appliesToAuth,omitempty"`

	// Sensitive marks a non-secret field whose value should still be masked in
	// exports and shared screenshots -- a hostname on an internal network, for
	// instance. It is not a secret and is returned by read paths.
	Sensitive bool `json:"sensitive,omitempty"`
}

// Option is one enum choice.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Help  string `json:"help,omitempty"`
	// Insecure marks a choice that weakens a control. The UI is expected to
	// warn; the server records the choice on the connection either way, so an
	// audit can find every connection that took it.
	Insecure bool `json:"insecure,omitempty"`
}

// AuthMode is one authentication method a connector supports.
type AuthMode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Help  string `json:"help,omitempty"`
	// Preferred marks the mode the platform recommends. Exactly one is
	// expected; the UI selects it by default so the secure choice is the
	// path of least resistance.
	Preferred bool `json:"preferred,omitempty"`
	// LocalTestingOnly marks a mode that must not be used against production
	// data. Snowflake password auth is the example the guide names.
	LocalTestingOnly bool `json:"localTestingOnly,omitempty"`
}

// Descriptor is everything the UI needs to render a connector, and everything
// the server needs to validate one.
type Descriptor struct {
	Type        string `json:"type"`
	DisplayName string `json:"displayName"`
	Status      Status `json:"status"`

	// StatusReason says why an entry is not AVAILABLE, in a sentence an
	// operator can act on. An unexplained "PLANNED" invites someone to ask
	// whether it is a bug.
	StatusReason string `json:"statusReason,omitempty"`

	Fields    []Field    `json:"fields"`
	AuthModes []AuthMode `json:"authModes"`

	// Template is a safe connection string with placeholders, shown as
	// documentation. It contains no value from any saved connection, so it can
	// never become a route by which a stored credential is redisplayed.
	//
	// Empty for providers where a URI is not the right model.
	Template string `json:"template,omitempty"`

	// SupportsURIPaste allows the convenience of pasting a connection string,
	// which is then parsed, split into fields and a secret, and discarded. It
	// is false for providers with no well-defined URI -- offering it there
	// would invite an operator to paste something that is not one.
	SupportsURIPaste bool `json:"supportsUriPaste"`

	Capabilities Capabilities `json:"capabilities"`

	// ConformanceRun records the evidence behind an AVAILABLE status. Absent
	// for every other status, by construction.
	Conformance *ConformanceRecord `json:"conformance,omitempty"`
}

// FieldByID finds a field.
func (d Descriptor) FieldByID(id string) (Field, bool) {
	for _, f := range d.Fields {
		if f.ID == id {
			return f, true
		}
	}
	return Field{}, false
}

// SecretFieldIDs returns the write-only fields, which is what the storage layer
// must route to the secret store rather than to a column.
func (d Descriptor) SecretFieldIDs() []string {
	var out []string
	for _, f := range d.Fields {
		if f.Kind == KindSecret || f.Kind == KindSecretRef {
			out = append(out, f.ID)
		}
	}
	return out
}

// Validate checks a submitted configuration against the descriptor.
//
// Structural only: no network call, and no secret value is read, logged or
// echoed. Errors name the field and say what is wrong with it without quoting
// what was submitted, because the submitted value may be the credential.
func (d Descriptor) Validate(cfg Config, sec Secrets, authMode string) error {
	if !d.hasAuthMode(authMode) {
		return fmt.Errorf("%s: %q is not a supported authentication mode", d.Type, authMode)
	}

	for _, f := range d.Fields {
		if !f.appliesTo(authMode) {
			continue
		}

		if f.Kind == KindSecret {
			// Presence only. The value is never inspected here: a validator
			// that checked a password's shape would be a validator that had
			// the password in a local variable, in a function that also
			// formats error messages.
			if f.Required && !sec.Has(f.ID) {
				return fmt.Errorf("%s: %s is required", d.Type, f.Label)
			}
			continue
		}

		value := cfg.Get(f.ID)
		if value == "" {
			value = f.Default
		}
		if value == "" {
			if f.Required {
				return fmt.Errorf("%s: %s is required", d.Type, f.Label)
			}
			continue
		}

		switch f.Kind {
		case KindNumber:
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("%s: %s must be a number", d.Type, f.Label)
			}
			if n < 1 || n > 65535 {
				return fmt.Errorf("%s: %s must be between 1 and 65535", d.Type, f.Label)
			}
		case KindEnum:
			if !f.allows(value) {
				return fmt.Errorf("%s: %s is not one of the permitted values", d.Type, f.Label)
			}
		case KindBool:
			if value != "true" && value != "false" {
				return fmt.Errorf("%s: %s must be true or false", d.Type, f.Label)
			}
		case KindText, KindSecretRef, KindList:
			// A control character in a connection field is either a mistake or
			// an injection into whatever consumes the value downstream -- a
			// DSN, a log line, a header.
			if strings.ContainsAny(value, "\x00\r\n") {
				return fmt.Errorf("%s: %s contains a control character", d.Type, f.Label)
			}
		}
	}

	// An empty allowlist is refused rather than treated as "everything". A
	// connection whose allowlist was never configured must not read the whole
	// database, and the failure has to happen at save time where someone is
	// looking, not at query time where the result is silently broad.
	if len(cfg.ResourceAllowlist) == 0 {
		return fmt.Errorf("%s: at least one approved schema, dataset or catalog is required", d.Type)
	}
	for _, r := range cfg.ResourceAllowlist {
		if err := ValidateIdentifier(r); err != nil {
			return fmt.Errorf("%s: approved resource %w", d.Type, err)
		}
	}
	return nil
}

func (d Descriptor) hasAuthMode(id string) bool {
	for _, m := range d.AuthModes {
		if m.ID == id {
			return true
		}
	}
	return false
}

func (f Field) appliesTo(authMode string) bool {
	if len(f.AppliesToAuth) == 0 {
		return true
	}
	for _, m := range f.AppliesToAuth {
		if m == authMode {
			return true
		}
	}
	return false
}

func (f Field) allows(value string) bool {
	for _, o := range f.Options {
		if o.Value == value {
			return true
		}
	}
	return false
}

// Summary is the masked, canonical view of a saved connection.
//
// This is what every read path returns. It carries no secret, no secret
// fingerprint that could be brute-forced against a weak credential, and no
// assembled connection string -- reconstructing the URI from the fields would
// hand back exactly the thing the split into Config and Secrets exists to
// prevent.
type Summary struct {
	ConnectorType string            `json:"connectorType"`
	DisplayName   string            `json:"displayName"`
	AuthMode      string            `json:"authMode"`
	Fields        map[string]string `json:"fields"`
	// SecretsConfigured lists which write-only fields have a value. The names
	// are field ids; no value or fingerprint appears.
	SecretsConfigured []string `json:"secretsConfigured"`
	ResourceAllowlist []string `json:"resourceAllowlist"`
	Health            Health   `json:"health"`
}

// Summarize builds the masked view. It reads only non-secret fields, so there
// is no path through which it could include one.
func (d Descriptor) Summarize(cfg Config, sec Secrets, authMode string, h Health) Summary {
	fields := map[string]string{}
	for _, f := range d.Fields {
		if f.Kind == KindSecret {
			continue // write-only, never returned
		}
		if v := cfg.Get(f.ID); v != "" {
			fields[f.ID] = v
		}
	}
	return Summary{
		ConnectorType:     d.Type,
		DisplayName:       d.DisplayName,
		AuthMode:          authMode,
		Fields:            fields,
		SecretsConfigured: sec.IDs(),
		ResourceAllowlist: append([]string(nil), cfg.ResourceAllowlist...),
		Health:            h,
	}
}
