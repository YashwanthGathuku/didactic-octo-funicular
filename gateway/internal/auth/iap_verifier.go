package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	DefaultIAPJWKURL = "https://www.gstatic.com/iap/verify/public_key-jwk"
	IAPIssuer        = "https://cloud.google.com/iap"
)

var (
	ErrMissingIAPAssertion  = errors.New("missing X-Goog-IAP-JWT-Assertion")
	ErrInvalidIAPAssertion  = errors.New("invalid IAP JWT assertion")
	ErrIAPSubjectMismatch   = errors.New("IAP subject does not match configured Agent Runtime identity")
	ErrMissingAgentName     = errors.New("missing trusted SentinelFlow agent name")
	ErrAgentVersionMismatch = errors.New("agent version does not match fixed canonical roster")
)

// VerifiedIAPIdentity is derived only after cryptographic verification of the
// signed IAP assertion. Unsigned compatibility headers are never authoritative.
type VerifiedIAPIdentity struct {
	Subject  string
	Email    string
	Issuer   string
	Audience []string
}

// IAPAssertionVerifier makes managed ingress testable without weakening the
// production JWT verification path.
type IAPAssertionVerifier interface {
	Verify(ctx context.Context, assertion string) (*VerifiedIAPIdentity, error)
}

type iapJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type iapJWKSet struct {
	Keys []iapJWK `json:"keys"`
}

type iapClaims struct {
	Email string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

// IAPJWTVerifier validates Google's signed IAP header with ES256 public keys.
// It intentionally has no ADC dependency: verification uses Google's public
// JWK set and validates issuer, audience, signature and expiry.
type IAPJWTVerifier struct {
	ExpectedAudience string
	JWKURL           string
	HTTPClient       *http.Client
	CacheTTL         time.Duration

	mu         sync.RWMutex
	keys       map[string]*ecdsa.PublicKey
	cacheUntil time.Time
}

func NewIAPJWTVerifier(expectedAudience string) *IAPJWTVerifier {
	return &IAPJWTVerifier{
		ExpectedAudience: strings.TrimSpace(expectedAudience),
		JWKURL:           DefaultIAPJWKURL,
		HTTPClient:       &http.Client{Timeout: 5 * time.Second},
		CacheTTL:         time.Hour,
		keys:             make(map[string]*ecdsa.PublicKey),
	}
}

func (v *IAPJWTVerifier) Verify(ctx context.Context, assertion string) (*VerifiedIAPIdentity, error) {
	assertion = strings.TrimSpace(assertion)
	if assertion == "" {
		return nil, ErrMissingIAPAssertion
	}
	if strings.TrimSpace(v.ExpectedAudience) == "" {
		return nil, fmt.Errorf("%w: expected audience is empty", ErrInvalidIAPAssertion)
	}

	claims := &iapClaims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"ES256"}),
		jwt.WithIssuer(IAPIssuer),
		jwt.WithAudience(v.ExpectedAudience),
		jwt.WithExpirationRequired(),
	)

	token, err := parser.ParseWithClaims(assertion, claims, func(token *jwt.Token) (interface{}, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("%w: missing kid", ErrInvalidIAPAssertion)
		}
		return v.keyForKID(ctx, kid)
	})
	if err != nil || token == nil || !token.Valid {
		return nil, fmt.Errorf("%w: %v", ErrInvalidIAPAssertion, err)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: missing subject", ErrInvalidIAPAssertion)
	}

	return &VerifiedIAPIdentity{
		Subject:  claims.Subject,
		Email:    claims.Email,
		Issuer:   claims.Issuer,
		Audience: append([]string(nil), claims.Audience...),
	}, nil
}

func (v *IAPJWTVerifier) keyForKID(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	v.mu.RLock()
	key := v.keys[kid]
	fresh := time.Now().Before(v.cacheUntil)
	v.mu.RUnlock()
	if key != nil && fresh {
		return key, nil
	}

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	key = v.keys[kid]
	if key == nil {
		return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidIAPAssertion, kid)
	}
	return key, nil
}

func (v *IAPJWTVerifier) refreshKeys(ctx context.Context) error {
	client := v.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	url := strings.TrimSpace(v.JWKURL)
	if url == "" {
		url = DefaultIAPJWKURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("iap jwk request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("iap jwk fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("iap jwk fetch: unexpected status %d", resp.StatusCode)
	}

	var set iapJWKSet
	// Public JWK documents are tiny. Limit the response defensively without
	// depending on server-only ResponseWriter helpers.
	dec := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := dec.Decode(&set); err != nil {
		return fmt.Errorf("iap jwk decode: %w", err)
	}

	parsed := make(map[string]*ecdsa.PublicKey)
	for _, jwk := range set.Keys {
		if jwk.Kid == "" || jwk.Kty != "EC" || jwk.Crv != "P-256" || jwk.Alg != "ES256" {
			continue
		}
		xb, errX := base64.RawURLEncoding.DecodeString(jwk.X)
		yb, errY := base64.RawURLEncoding.DecodeString(jwk.Y)
		if errX != nil || errY != nil {
			continue
		}
		x := new(big.Int).SetBytes(xb)
		y := new(big.Int).SetBytes(yb)
		if !elliptic.P256().IsOnCurve(x, y) {
			continue
		}
		parsed[jwk.Kid] = &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	}
	if len(parsed) == 0 {
		return fmt.Errorf("%w: no usable ES256 keys", ErrInvalidIAPAssertion)
	}

	ttl := v.CacheTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	v.mu.Lock()
	v.keys = parsed
	v.cacheUntil = time.Now().Add(ttl)
	v.mu.Unlock()
	return nil
}

// ManagedAgentIdentityMiddleware verifies IAP cryptographically before trusting
// SentinelFlow's internal specialist metadata. Agent Gateway/IAP authenticates
// the runtime workload; the fixed roster still governs specialist capability.
func (v *AgentIdentityValidator) ManagedAgentIdentityMiddleware(
	verifier IAPAssertionVerifier,
	expectedRuntimeSubject string,
	next http.Handler,
) http.Handler {
	if verifier == nil {
		panic("ManagedAgentIdentityMiddleware requires an IAPAssertionVerifier")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertion := r.Header.Get("X-Goog-IAP-JWT-Assertion")
		verified, err := verifier.Verify(r.Context(), assertion)
		if err != nil {
			http.Error(w, `{"error":"managed ingress authentication failed"}`, http.StatusUnauthorized)
			return
		}
		if expectedRuntimeSubject != "" && verified.Subject != expectedRuntimeSubject {
			http.Error(w, `{"error":"managed runtime subject mismatch"}`, http.StatusForbidden)
			return
		}

		agentName := strings.TrimSpace(r.Header.Get("X-Sentinel-Agent-Name"))
		if agentName == "" {
			http.Error(w, `{"error":"missing SentinelFlow agent metadata"}`, http.StatusBadRequest)
			return
		}
		canonical := normalizeAgentName(agentName)
		identity, ok := FixedCanonicalRoster[canonical]
		if !ok {
			http.Error(w, `{"error":"agent is not in fixed canonical roster"}`, http.StatusForbidden)
			return
		}
		version := strings.TrimSpace(r.Header.Get("X-Sentinel-Agent-Version"))
		if version == "" || version != identity.Version {
			http.Error(w, `{"error":"agent version mismatch"}`, http.StatusForbidden)
			return
		}

		bound := identity
		bound.Principal = verified.Subject
		bound.ProjectID = v.expectedProjectID
		ctx := ContextWithAgentIdentity(r.Context(), &bound)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
