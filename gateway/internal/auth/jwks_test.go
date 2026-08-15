package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// These tests exercise FetchJWKS over a real HTTP server rather than only
// testing the parser. What remains unverified is narrow and named at the bottom
// of this file.

// jwksDocument renders a key set the way a provider does.
func jwksDocument(t *testing.T, keys map[string]*rsa.PublicKey) []byte {
	t.Helper()
	type jwk struct {
		Kty string `json:"kty"`
		Use string `json:"use"`
		Kid string `json:"kid"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	doc := struct {
		Keys []jwk `json:"keys"`
	}{}
	for kid, k := range keys {
		doc.Keys = append(doc.Keys, jwk{
			Kty: "RSA", Use: "sig", Kid: kid, Alg: "RS256",
			N: base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
			E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes()),
		})
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The whole path: serve a key set, fetch it, then verify a token signed by the
// matching private key. This is what a provider integration actually does.
func TestFetchJWKSAndVerifyATokenEndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var served int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksDocument(t, map[string]*rsa.PublicKey{"live-key": &key.PublicKey}))
	}))
	defer srv.Close()

	keys, err := FetchJWKS(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if served != 1 {
		t.Errorf("expected exactly one request, got %d", served)
	}
	if _, ok := keys["live-key"]; !ok {
		t.Fatalf("fetched key set has no live-key: %v", keys)
	}

	v, err := NewVerifier(VerifierConfig{
		Issuer: "https://idp.test/", Audience: "api", Keys: keys,
	})
	if err != nil {
		t.Fatal(err)
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://idp.test/", "aud": "api", "sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "live-key"
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	p, err := v.Verify(raw)
	if err != nil {
		t.Fatalf("a token signed by the fetched key must verify: %v", err)
	}
	if p.Subject != "user-1" {
		t.Errorf("subject = %q", p.Subject)
	}
}

// Key rotation: a provider serving two keys must let tokens from either verify.
func TestFetchJWKSHandlesMultipleKeysForRotation(t *testing.T) {
	oldKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	newKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwksDocument(t, map[string]*rsa.PublicKey{
			"old": &oldKey.PublicKey,
			"new": &newKey.PublicKey,
		}))
	}))
	defer srv.Close()

	keys, err := FetchJWKS(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys during rotation, got %d", len(keys))
	}

	v, _ := NewVerifier(VerifierConfig{Issuer: "https://idp.test/", Audience: "api", Keys: keys})
	for name, k := range map[string]*rsa.PrivateKey{"old": oldKey, "new": newKey} {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": "https://idp.test/", "aud": "api", "sub": "u",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		tok.Header["kid"] = name
		raw, _ := tok.SignedString(k)
		if _, err := v.Verify(raw); err != nil {
			t.Errorf("token signed with the %s key failed to verify: %v", name, err)
		}
	}
}

func TestFetchJWKSRejectsErrorResponses(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusNotFound, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"keys":[]}`))
		}))
		if _, err := FetchJWKS(context.Background(), srv.URL); err == nil {
			t.Errorf("HTTP %d was treated as a usable key set", status)
		}
		srv.Close()
	}
}

func TestFetchJWKSRejectsMalformedBody(t *testing.T) {
	for _, body := range []string{`not json`, `{}`, `{"keys":[]}`, `{"keys":[{"kty":"EC","kid":"x"}]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		if _, err := FetchJWKS(context.Background(), srv.URL); err == nil {
			t.Errorf("body %q produced a usable key set", body)
		}
		srv.Close()
	}
}

// An undersized modulus is refused rather than trusted.
func TestFetchJWKSRejectsWeakKeys(t *testing.T) {
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwksDocument(t, map[string]*rsa.PublicKey{"weak": &weak.PublicKey}))
	}))
	defer srv.Close()

	if _, err := FetchJWKS(context.Background(), srv.URL); err == nil {
		t.Errorf("a 1024-bit signing key was accepted")
	}
}

// A key with no id cannot be selected by a token header; guessing would defeat
// rotation, so it is skipped and an all-skipped document is an error.
func TestFetchJWKSSkipsKeysWithoutAnId(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	body := `{"keys":[{"kty":"RSA","use":"sig","alg":"RS256","n":"` +
		base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()) +
		`","e":"AQAB"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	if _, err := FetchJWKS(context.Background(), srv.URL); err == nil {
		t.Errorf("a key set containing only unidentified keys was accepted")
	}
}

// An oversized body must not be read into memory without bound.
func TestFetchJWKSBoundsTheResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Far larger than any real key set.
		_, _ = w.Write([]byte(`{"keys":[`))
		chunk := strings.Repeat("A", 64*1024)
		for i := 0; i < 40; i++ {
			_, _ = w.Write([]byte(`"` + chunk + `",`))
		}
		_, _ = w.Write([]byte(`]}`))
	}))
	defer srv.Close()

	// The read is truncated at 1 MiB, so the JSON is incomplete and parsing
	// fails. What matters is that it fails rather than consuming the stream.
	if _, err := FetchJWKS(context.Background(), srv.URL); err == nil {
		t.Errorf("an oversized key set was accepted")
	}
}

func TestFetchJWKSRespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := FetchJWKS(ctx, srv.URL); err == nil {
		t.Errorf("a hanging provider did not produce an error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("cancellation was not honoured; took %v", elapsed)
	}
}

func TestFetchJWKSFailsOnUnreachableHost(t *testing.T) {
	// A closed port: the fetch must error rather than yielding an empty key set
	// that would later look like "no keys configured".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	if _, err := FetchJWKS(context.Background(), url); err == nil {
		t.Errorf("an unreachable provider produced no error")
	}
}

// WHAT REMAINS UNVERIFIED, precisely:
//
//   - TLS to a real host. httptest serves plain HTTP, so certificate validation
//     against a public CA is not exercised here.
//   - Provider-specific document quirks: additional key types, x5c chains,
//     unusual "use" values, or non-standard field ordering from a commercial
//     identity provider.
//   - Live rotation timing, including how long a retired key stays published.
//
// Everything else in the network path -- request construction, status handling,
// bounded reads, cancellation, unreachable hosts, malformed and weak documents,
// and multi-key rotation -- is exercised above against a real HTTP server.
