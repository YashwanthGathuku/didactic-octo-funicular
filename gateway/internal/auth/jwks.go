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

// FetchJWKS retrieves an identity provider's public signing keys.
//
// Only RSA keys usable for signature verification are returned; anything else
// is skipped rather than coerced, so an unexpected key type cannot end up
// trusted.
//
// NOT EXERCISED AGAINST A LIVE PROVIDER in this repository's test suite: there
// is no identity provider available to the test environment. The parsing is
// covered by a fixture test; the network path is not. Treat first deployment
// against a real provider as the point where this is genuinely verified.
func FetchJWKS(ctx context.Context, url string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
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
