package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	APIToken       string
	AllowedOrigin  string
	PGPKeyringPath string

	// Storage
	InboxPath string
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

	cfg := &Config{
		Profile:        profile,
		DatabaseURL:    env("DATABASE_URL", ""),
		ObjectStoreURL: env("OBJECT_STORE_URL", ""),
		AITierURL:      env("AI_TIER_URL", ""),
		APIToken:       env("SENTINEL_API_TOKEN", ""),
		AllowedOrigin:  env("SENTINEL_ALLOWED_ORIGIN", ""),
		PGPKeyringPath: env("SENTINEL_PGP_KEYRING", ""),
		InboxPath:      env("SENTINEL_INBOX_PATH", "./inbox"),
	}

	var problems []string

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

	case ProfileProduction:
		cfg.BindAddress = env("SENTINEL_BIND_ADDRESS", "0.0.0.0")

		// No defaults. A production process that guesses its dependencies is a
		// process that can silently point at the wrong one.
		if cfg.APIToken == "" {
			problems = append(problems, "SENTINEL_API_TOKEN is required in the production profile: the API must not serve unauthenticated requests")
		} else if len(cfg.APIToken) < 32 {
			problems = append(problems, fmt.Sprintf("SENTINEL_API_TOKEN is %d characters; require at least 32", len(cfg.APIToken)))
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
	for _, bad := range []string{"password", "minioadmin", "changeme", "secret", "admin"} {
		if strings.EqualFold(cfg.APIToken, bad) {
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
