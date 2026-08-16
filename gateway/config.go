package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"sentinel-gateway/internal/objectstore"
	"sentinel-gateway/internal/secrets"
)

// Profile selects the startup contract the process must satisfy.
//
// The distinction is deliberate and named: "local-demo" is the only profile in
// which this software may run without authentication, and it says so on every
// affected surface. Anything else must be fully configured or refuse to start.
type Profile string

const (
	// ProfileLocalDemo binds to loopback, permits an unauthenticated API, and is
	// visibly labelled. It is for a developer machine and nothing else.
	ProfileLocalDemo Profile = "local-demo"

	// ProfileProduction refuses to start unless authentication, database and
	// object storage are configured. It has no defaults.
	ProfileProduction Profile = "production"
)

// Config is the validated runtime configuration. Every field is populated by
// Load and checked before the process serves traffic; nothing reads os.Getenv
// after startup.
type Config struct {
	Profile Profile

	// Server
	BindAddress string
	Port        int

	// Dependencies
	DatabaseURL    string
	ObjectStoreURL string
	AITierURL      string

	// Security
	//
	// The two credentials are secrets.Value rather than string so they redact
	// themselves everywhere. A Config is exactly the sort of struct that gets
	// dumped with %+v during an incident, which is how a credential reaches a
	// log aggregator with a one-year retention.
	APIToken       secrets.Value
	AllowedOrigin  string
	PGPKeyringPath string

	// OIDC. Required in the production profile: without an issuer and audience
	// there is no identity to authorize, and the process refuses to start.
	OIDCIssuer   string
	OIDCAudience string
	OIDCJWKSURL  string

	// MetricsToken guards /metrics. Prometheus scrapers send it as a bearer
	// token. Required in production: an open metrics endpoint is a free
	// inventory of a system's internals.
	MetricsToken secrets.Value

	// Sealer encrypts retrievable credentials before they reach storage. Its
	// key comes from SENTINEL_SECRET_SEAL_KEY and never from the database, so a
	// database compromise alone yields ciphertext.
	//
	// In the local-demo profile this is a process-scoped key, which is correct
	// for a store whose contents also die with the process. The production
	// profile refuses a process-scoped key, because a restart would render
	// every stored credential permanently unreadable.
	Sealer secrets.Sealer

	// Scrubber holds this process's credentials so they can be removed from log
	// output whatever shape they appear in.
	Scrubber *secrets.Scrubber

	// ObjectStore holds artifacts. It is built at startup rather than per
	// request so a misconfigured store fails before traffic arrives.
	//
	// A nil store means uploads are refused with 503. There is deliberately no
	// fallback to holding bytes in the database: accepting a financial file
	// this system cannot durably store is worse than refusing it.
	ObjectStore objectstore.ObjectStore

	// ArtifactStoreRoot is the filesystem root when the local adapter is in
	// use, retained for the startup log line.
	ArtifactStoreRoot string

	// Storage
	InboxPath string

	// WatcherTenant is the tenant the inbox watcher ingests into.
	//
	// The watcher is a background daemon with no request and therefore no
	// principal, so its tenant cannot come from a token. It must be an explicit
	// operator decision: when this is empty the watcher does not start at all,
	// rather than silently writing every arriving file into one tenant.
	WatcherTenant string
}

// ConfigError collects every configuration problem rather than reporting the
// first. An operator fixing a deployment should see the whole list once.
type ConfigError struct {
	Problems []string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("invalid configuration:\n  - %s", strings.Join(e.Problems, "\n  - "))
}

// IsDemo reports whether synthetic or unauthenticated behaviour is permitted.
func (c *Config) IsDemo() bool { return c.Profile == ProfileLocalDemo }

// Addr is the listen address. In the demo profile this is loopback-only, so a
// developer machine does not expose an unauthenticated financial API to its
// network.
func (c *Config) Addr() string { return fmt.Sprintf("%s:%d", c.BindAddress, c.Port) }

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// Load reads configuration from the environment and validates it against the
// selected profile. It returns a *ConfigError listing every problem found.
//
// Before this existed, configuration was read ad hoc at point of use: the AI
// tier address was hardcoded to 127.0.0.1 while AI_TIER_URL was set and ignored,
// the database path defaulted to a location outside the container's mounted
// volume, and a missing API token produced a log line rather than a refusal.
func Load() (*Config, error) {
	profile := Profile(strings.ToLower(env("SENTINEL_PROFILE", string(ProfileLocalDemo))))

	// The credentials are read as strings only long enough to be validated and
	// wrapped. Nothing downstream sees the raw form.
	rawAPIToken := env("SENTINEL_API_TOKEN", "")
	rawMetricsToken := env("SENTINEL_METRICS_TOKEN", "")
	rawSealKey := env("SENTINEL_SECRET_SEAL_KEY", "")

	cfg := &Config{
		Profile:        profile,
		DatabaseURL:    env("DATABASE_URL", ""),
		ObjectStoreURL: env("OBJECT_STORE_URL", ""),
		AITierURL:      env("AI_TIER_URL", ""),
		AllowedOrigin:  env("SENTINEL_ALLOWED_ORIGIN", ""),
		PGPKeyringPath: env("SENTINEL_PGP_KEYRING", ""),
		OIDCIssuer:     env("SENTINEL_OIDC_ISSUER", ""),
		OIDCAudience:   env("SENTINEL_OIDC_AUDIENCE", ""),
		OIDCJWKSURL:    env("SENTINEL_OIDC_JWKS_URL", ""),
		InboxPath:      env("SENTINEL_INBOX_PATH", "./inbox"),
		WatcherTenant:  env("SENTINEL_WATCHER_TENANT", ""),
		Scrubber:       secrets.NewScrubber(),
	}

	var problems []string

	// Wrapping happens after the length check below, but the wrap itself can
	// only fail for a value shorter than the minimum, which the profile checks
	// already report with a clearer message.
	if v, err := secrets.New(rawAPIToken); err == nil {
		cfg.APIToken = v
		cfg.Scrubber.Register(v)
	}
	if v, err := secrets.New(rawMetricsToken); err == nil {
		cfg.MetricsToken = v
		cfg.Scrubber.Register(v)
	}

	switch profile {
	case ProfileLocalDemo:
		// Defaults are permitted here, and only here.
		cfg.BindAddress = env("SENTINEL_BIND_ADDRESS", "127.0.0.1")
		if cfg.DatabaseURL == "" {
			cfg.DatabaseURL = "./data/sentinel.db"
		}
		if cfg.AllowedOrigin == "" {
			cfg.AllowedOrigin = "http://localhost:3000"
		}
		if cfg.BindAddress != "127.0.0.1" && cfg.BindAddress != "localhost" && cfg.BindAddress != "::1" {
			problems = append(problems, fmt.Sprintf(
				"SENTINEL_BIND_ADDRESS=%q: the local-demo profile may permit an unauthenticated API and must bind loopback only. Use SENTINEL_PROFILE=production to bind elsewhere",
				cfg.BindAddress))
		}
		// A configured key is honoured here too, so a developer can exercise the
		// production storage path. Absent one, the key is process-scoped, which
		// matches a store whose contents also die with the process.
		if rawSealKey != "" {
			sealer, err := secrets.SealerFromBase64(rawSealKey)
			if err != nil {
				problems = append(problems, fmt.Sprintf("SENTINEL_SECRET_SEAL_KEY is not usable: %v", err))
			} else {
				cfg.Sealer = sealer
			}
		} else if sealer, err := secrets.NewEphemeralSealer(); err != nil {
			problems = append(problems, fmt.Sprintf("cannot generate a development seal key: %v", err))
		} else {
			cfg.Sealer = sealer
		}

	case ProfileProduction:
		cfg.BindAddress = env("SENTINEL_BIND_ADDRESS", "0.0.0.0")

		// No defaults. A production process that guesses its dependencies is a
		// process that can silently point at the wrong one.
		if rawAPIToken == "" {
			problems = append(problems, "SENTINEL_API_TOKEN is required in the production profile: the API must not serve unauthenticated requests")
		} else if len(rawAPIToken) < secrets.MinSecretLength {
			problems = append(problems, fmt.Sprintf("SENTINEL_API_TOKEN is %d characters; require at least %d", len(rawAPIToken), secrets.MinSecretLength))
		}
		if cfg.DatabaseURL == "" {
			problems = append(problems, "DATABASE_URL is required in the production profile")
		}
		if cfg.ObjectStoreURL == "" {
			problems = append(problems, "OBJECT_STORE_URL is required in the production profile: artifacts must not live in relational rows")
		}
		if cfg.AllowedOrigin == "" {
			problems = append(problems, "SENTINEL_ALLOWED_ORIGIN is required in the production profile")
		}
		if cfg.PGPKeyringPath == "" {
			problems = append(problems, "SENTINEL_PGP_KEYRING is required in the production profile: signature verification fails closed without it")
		}
		// Identity is mandatory. A shared bearer token is not an identity: it
		// cannot name an actor, so no approval recorded under it is attributable.
		if cfg.OIDCIssuer == "" {
			problems = append(problems, "SENTINEL_OIDC_ISSUER is required in the production profile: actor identity must come from verified claims")
		}
		if cfg.OIDCAudience == "" {
			problems = append(problems, "SENTINEL_OIDC_AUDIENCE is required in the production profile: a token minted for another service must not be replayable here")
		}
		if cfg.OIDCJWKSURL == "" {
			problems = append(problems, "SENTINEL_OIDC_JWKS_URL is required in the production profile: signatures cannot be verified without the provider's keys")
		}
		if rawMetricsToken == "" {
			problems = append(problems, "SENTINEL_METRICS_TOKEN is required in the production profile: /metrics must not be open to anonymous callers")
		} else if len(rawMetricsToken) < secrets.MinSecretLength {
			problems = append(problems, fmt.Sprintf("SENTINEL_METRICS_TOKEN is %d characters; require at least %d", len(rawMetricsToken), secrets.MinSecretLength))
		}

		// A durable seal key is mandatory. Without it the process would start,
		// store retrievable credentials under a process-scoped key, and lose
		// every one of them at the next restart -- a failure that looks like
		// success until the worst possible moment.
		if rawSealKey == "" {
			problems = append(problems, "SENTINEL_SECRET_SEAL_KEY is required in the production profile: stored credentials would otherwise be sealed under a key that dies with the process")
		} else if sealer, err := secrets.SealerFromBase64(rawSealKey); err != nil {
			problems = append(problems, fmt.Sprintf("SENTINEL_SECRET_SEAL_KEY is not usable: %v", err))
		} else if err := secrets.RequireDurableSealer(sealer); err != nil {
			problems = append(problems, fmt.Sprintf("SENTINEL_SECRET_SEAL_KEY: %v", err))
		} else {
			cfg.Sealer = sealer
		}

	default:
		problems = append(problems, fmt.Sprintf(
			"SENTINEL_PROFILE=%q is not a known profile (want %q or %q)",
			profile, ProfileLocalDemo, ProfileProduction))
	}

	// Port
	portStr := env("PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		problems = append(problems, fmt.Sprintf("PORT=%q is not a valid TCP port", portStr))
	}
	cfg.Port = port

	// Reject known-bad credentials outright, in any profile. These are the
	// values shipped in the old compose files.
	//
	// secret-scan-allow: this is the rejection list itself, not a credential; naming the defaults is what lets the process refuse them
	for _, bad := range []string{"password", "minioadmin", "changeme", "secret", "admin"} {
		if strings.EqualFold(rawAPIToken, bad) {
			problems = append(problems, fmt.Sprintf("SENTINEL_API_TOKEN is the well-known default %q", bad))
		}
	}

	// Validate any URL that was supplied, in every profile: a malformed
	// dependency address should fail at startup, not at first use.
	for name, raw := range map[string]string{
		"AI_TIER_URL":      cfg.AITierURL,
		"OBJECT_STORE_URL": cfg.ObjectStoreURL,
	} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			problems = append(problems, fmt.Sprintf("%s=%q is not an absolute URL", name, raw))
		}
	}

	if len(problems) > 0 {
		return nil, &ConfigError{Problems: problems}
	}
	return cfg, nil
}

// WatcherEnabled reports whether the inbox watcher may run. It requires an
// explicit tenant, so an operator has to decide where watched files belong.
func (c *Config) WatcherEnabled() bool { return c.WatcherTenant != "" }

// ErrNoAITier is returned by AI-dependent handlers when no AI tier is
// configured. It is a normal condition, not a failure: deterministic ingestion
// does not depend on AI.
var ErrNoAITier = errors.New("no AI tier configured")

// DefaultTenantID scopes records written by code paths that do not yet have an
// authenticated tenant.
//
// This is a placeholder with a deliberate name, not a design. Every business
// table now requires a tenant, but the request path has no identity to derive
// one from until Prompt 04 supplies OIDC claims and tenant memberships. Until
// then all writes land in this single tenant, which is honest about the fact
// that there is exactly one isolation domain today rather than pretending the
// column implies isolation.
//
// Prompt 04 replaces every use of this constant with the tenant resolved from
// verified session claims.
const DefaultTenantID = "TENANT-DEFAULT"
