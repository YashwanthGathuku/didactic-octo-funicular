package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIAPJWTVerifier_ValidSignedAssertion(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-kid"
	jwkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "EC",
				"crv": "P-256",
				"alg": "ES256",
				"use": "sig",
				"kid": kid,
				"x": base64.RawURLEncoding.EncodeToString(key.PublicKey.X.Bytes()),
				"y": base64.RawURLEncoding.EncodeToString(key.PublicKey.Y.Bytes()),
			}},
		})
	}))
	defer jwkServer.Close()

	aud := "/projects/123456789/global/backendServices/987654321"
	claims := iapClaims{
		Email: "runtime@example.invalid",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IAPIssuer,
			Subject:   "agent-runtime-subject-123",
			Audience:  jwt.ClaimStrings{aud},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	verifier := NewIAPJWTVerifier(aud)
	verifier.JWKURL = jwkServer.URL
	verifier.HTTPClient = jwkServer.Client()

	got, err := verifier.Verify(context.Background(), raw)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got.Subject != "agent-runtime-subject-123" {
		t.Fatalf("subject = %q", got.Subject)
	}
	if got.Email != "runtime@example.invalid" {
		t.Fatalf("email = %q", got.Email)
	}
}

func TestIAPJWTVerifier_RejectsWrongAudience(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-kid"
	jwkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "EC", "crv": "P-256", "alg": "ES256", "kid": kid,
				"x": base64.RawURLEncoding.EncodeToString(key.PublicKey.X.Bytes()),
				"y": base64.RawURLEncoding.EncodeToString(key.PublicKey.Y.Bytes()),
			}},
		})
	}))
	defer jwkServer.Close()

	claims := iapClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    IAPIssuer,
		Subject:   "agent-runtime-subject-123",
		Audience:  jwt.ClaimStrings{"wrong-audience"},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
	}}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	verifier := NewIAPJWTVerifier("expected-audience")
	verifier.JWKURL = jwkServer.URL
	verifier.HTTPClient = jwkServer.Client()
	if _, err := verifier.Verify(context.Background(), raw); err == nil {
		t.Fatal("expected wrong audience to be rejected")
	}
}

type fakeIAPVerifier struct {
	identity *VerifiedIAPIdentity
	err      error
}

func (f fakeIAPVerifier) Verify(ctx context.Context, assertion string) (*VerifiedIAPIdentity, error) {
	return f.identity, f.err
}

func TestManagedAgentIdentityMiddleware_BindsVerifiedWorkloadToFixedRoster(t *testing.T) {
	validator := NewAgentIdentityValidator("telos-agent")
	verifier := fakeIAPVerifier{identity: &VerifiedIAPIdentity{
		Subject: "principal://agents.example/resources/aiplatform/projects/123/locations/us-central1/reasoningEngines/456",
		Issuer:  IAPIssuer,
	}}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := AgentIdentityFromContext(r.Context())
		if !ok {
			t.Fatal("missing identity in request context")
		}
		if identity.AgentName != "DiagnosisAgent" {
			t.Fatalf("agent = %q", identity.AgentName)
		}
		if identity.Principal != verifier.identity.Subject {
			t.Fatalf("principal was not derived from verified IAP subject")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	h := validator.ManagedAgentIdentityMiddleware(verifier, verifier.identity.Subject, next)
	req := httptest.NewRequest(http.MethodPost, "/internal/agent-tools", nil)
	req.Header.Set("X-Goog-IAP-JWT-Assertion", "signed-fixture")
	req.Header.Set("X-Sentinel-Agent-Name", "DiagnosisAgent")
	req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
	req.Header.Set("X-Agent-Identity-Principal", "spiffe://attacker.example/agent/admin")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestManagedAgentIdentityMiddleware_RejectsSubjectMismatch(t *testing.T) {
	validator := NewAgentIdentityValidator("telos-agent")
	verifier := fakeIAPVerifier{identity: &VerifiedIAPIdentity{Subject: "subject-A", Issuer: IAPIssuer}}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not execute")
	})
	h := validator.ManagedAgentIdentityMiddleware(verifier, "subject-B", next)
	req := httptest.NewRequest(http.MethodPost, "/internal/agent-tools", nil)
	req.Header.Set("X-Goog-IAP-JWT-Assertion", "signed-fixture")
	req.Header.Set("X-Sentinel-Agent-Name", "DiagnosisAgent")
	req.Header.Set("X-Sentinel-Agent-Version", "1.0.0")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
}
