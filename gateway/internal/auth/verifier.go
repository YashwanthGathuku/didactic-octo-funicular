package auth

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// VerifierConfig describes the identity provider this gateway trusts.
type VerifierConfig struct {
	// Issuer must match the token's iss claim exactly.
	Issuer string
	// Audience must appear in the token's aud claim.
	Audience string
	// Keys maps kid to the public key trusted for that key id. In deployment
	// this is populated from the provider's JWKS endpoint; the type is a plain
	// map so tests can supply a generated key without a network.
	Keys map[string]*rsa.PublicKey
	// Leeway tolerates small clock skew. Kept deliberately small.
	Leeway time.Duration
	// TenantClaim and RolesClaim name the custom claims carrying membership.
	TenantClaim string
	RolesClaim  string
}

// Verifier validates tokens and produces Principals.
type Verifier struct {
	cfg    VerifierConfig
	parser *jwt.Parser
}

var (
	// ErrVerifierNotConfigured means authentication cannot be performed. It is
	// never treated as permission to proceed.
	ErrVerifierNotConfigured = errors.New("token verifier is not configured")
	// ErrInvalidToken covers every rejection reason. The specific cause is
	// wrapped for logs but the caller sees one failure mode, so probing cannot
	// distinguish "wrong signature" from "expired".
	ErrInvalidToken = errors.New("invalid token")
)

// NewVerifier constructs a verifier, refusing an incomplete configuration.
//
// A misconfigured verifier is an error, not a permissive default. The previous
// authentication middleware treated an unset token as "authentication
// disabled", which is the failure mode this constructor exists to prevent.
func NewVerifier(cfg VerifierConfig) (*Verifier, error) {
	var missing []string
	if cfg.Issuer == "" {
		missing = append(missing, "issuer")
	}
	if cfg.Audience == "" {
		missing = append(missing, "audience")
	}
	if len(cfg.Keys) == 0 {
		missing = append(missing, "signing keys")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: missing %v", ErrVerifierNotConfigured, missing)
	}
	if cfg.TenantClaim == "" {
		cfg.TenantClaim = "https://sentinelflow.dev/tenants"
	}
	if cfg.RolesClaim == "" {
		cfg.RolesClaim = "roles"
	}
	if cfg.Leeway == 0 {
		cfg.Leeway = 30 * time.Second
	}

	return &Verifier{
		cfg: cfg,
		// Only RS256 is accepted. Restricting the algorithm list is what
		// prevents alg=none and the HMAC-with-the-public-key confusion attack:
		// the parser will not even attempt another family.
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{"RS256"}),
			jwt.WithIssuer(cfg.Issuer),
			jwt.WithAudience(cfg.Audience),
			jwt.WithLeeway(cfg.Leeway),
			jwt.WithExpirationRequired(),
		),
	}, nil
}

// Verify validates a bearer token and returns the Principal it describes.
//
// Every failure returns ErrInvalidToken. Callers must not branch on the wrapped
// detail to decide anything the caller can observe.
func (v *Verifier) Verify(raw string) (*Principal, error) {
	if v == nil {
		return nil, ErrVerifierNotConfigured
	}
	if raw == "" {
		return nil, fmt.Errorf("%w: empty token", ErrInvalidToken)
	}

	claims := jwt.MapClaims{}
	_, err := v.parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token has no kid header")
		}
		key, ok := v.cfg.Keys[kid]
		if !ok {
			return nil, fmt.Errorf("unknown key id %q", kid)
		}
		return key, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("%w: token has no subject", ErrInvalidToken)
	}

	p := &Principal{
		Subject:  sub,
		Issuer:   v.cfg.Issuer,
		Audience: v.cfg.Audience,
	}
	if e, ok := claims["email"].(string); ok {
		p.Email = e
	}

	// Memberships come from a namespaced claim shaped as:
	//   {"TENANT-A": ["operator","viewer"], "TENANT-B": ["reviewer"]}
	//
	// An unknown role is a hard failure. Silently dropping it would let a
	// mis-mapped provider configuration present as "user with no permissions",
	// which is indistinguishable from a deliberate revocation.
	if rawTenants, ok := claims[v.cfg.TenantClaim].(map[string]any); ok {
		for tenantID, rawRoles := range rawTenants {
			if tenantID == "" {
				return nil, fmt.Errorf("%w: membership with empty tenant id", ErrInvalidToken)
			}
			list, ok := rawRoles.([]any)
			if !ok {
				return nil, fmt.Errorf("%w: roles for %s are not a list", ErrInvalidToken, tenantID)
			}
			m := Membership{TenantID: tenantID}
			for _, r := range list {
				s, ok := r.(string)
				if !ok {
					return nil, fmt.Errorf("%w: non-string role in %s", ErrInvalidToken, tenantID)
				}
				role, err := ParseRole(s)
				if err != nil {
					return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
				}
				// platform_admin is not a tenant role and must not be smuggled
				// in through a membership list.
				if role == RolePlatformAdmin {
					return nil, fmt.Errorf("%w: platform_admin cannot be granted as a tenant role", ErrInvalidToken)
				}
				m.Roles = append(m.Roles, role)
			}
			p.Memberships = append(p.Memberships, m)
		}
	}

	// Platform admin comes from a separate top-level claim, never from a tenant
	// membership list.
	if roles, ok := claims[v.cfg.RolesClaim].([]any); ok {
		for _, r := range roles {
			if s, ok := r.(string); ok && Role(s) == RolePlatformAdmin {
				p.PlatformAdmin = true
			}
		}
	}

	return p, nil
}
