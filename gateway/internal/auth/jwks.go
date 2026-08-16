package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

// FetchJWKS retrieves an identity provider's public signing keys using an
// unrestricted HTTP client.
//
// Only RSA keys usable for signature verification are returned; anything else
// is skipped rather than coerced, so an unexpected key type cannot end up
// trusted.
//
// Prefer FetchJWKSWithClient with an egress-guarded client. The JWKS URL is
// operator-configured, which makes it the natural place for a
// misconfiguration -- or an attacker who can influence configuration -- to
// point this process at an internal address. This variant applies no such
// restriction and remains for tests and for callers that supply their own.
//
// What the test suite does and does not cover is recorded at the foot of
// jwks_test.go: the fetch is exercised against a real HTTP server, while TLS to
// a real host, provider-specific document quirks and live rotation timing are
// not.
func FetchJWKS(ctx context.Context, url string) (map[string]*rsa.PublicKey, error) {
	return FetchJWKSWithClient(ctx, url, &http.Client{Timeout: 10 * time.Second})
}

// FetchJWKSWithClient retrieves signing keys through a caller-supplied client.
//
// Injecting the client is what lets the egress policy apply here: the guarded
// client refuses to connect to loopback, private, link-local and cloud metadata
// addresses, and re-validates every redirect hop.
func FetchJWKSWithClient(ctx context.Context, url string, client *http.Client) (map[string]*rsa.PublicKey, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch jwks: provider returned HTTP %d", resp.StatusCode)
	}

	// Bounded read: a JWKS document is small, and an unbounded read from a
	// remote host is a memory-exhaustion vector.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return ParseJWKS(body)
}

// ParseJWKS decodes a JWKS document into usable RSA public keys.
func ParseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Use string `json:"use"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}

	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		// "use" is optional, but if present it must be for signatures.
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if k.Alg != "" && k.Alg != "RS256" {
			continue
		}
		if k.Kid == "" {
			// A key with no id cannot be selected by a token header, and
			// guessing would defeat key rotation.
			continue
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, fmt.Errorf("parse jwks: key %s modulus: %w", k.Kid, err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, fmt.Errorf("parse jwks: key %s exponent: %w", k.Kid, err)
		}

		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)
		if !e.IsInt64() || e.Int64() > 1<<31-1 || e.Int64() < 3 {
			return nil, fmt.Errorf("parse jwks: key %s has an implausible exponent", k.Kid)
		}
		// Reject undersized moduli outright rather than trusting a weak key.
		if n.BitLen() < 2048 {
			return nil, fmt.Errorf("parse jwks: key %s modulus is %d bits; minimum 2048", k.Kid, n.BitLen())
		}

		out[k.Kid] = &rsa.PublicKey{N: n, E: int(e.Int64())}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("parse jwks: no usable RS256 signing keys found")
	}
	return out, nil
}
