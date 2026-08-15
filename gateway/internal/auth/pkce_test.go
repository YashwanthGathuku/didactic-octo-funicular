package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// The login flow is tested against a stub provider that behaves like a real
// one: it checks the PKCE verifier, and refuses the exchange when it does not
// match the challenge from the authorization request.

func clientFor(tokenURL string) *OIDCClientConfig {
	return &OIDCClientConfig{
		Issuer:       "https://idp.test/",
		ClientID:     "sentinel-flow",
		ClientSecret: "client-secret",
		RedirectURL:  "https://ops.example.com/auth/callback",
		AuthorizeURL: "https://idp.test/authorize",
		TokenURL:     tokenURL,
	}
}

func TestAuthorizeUrlCarriesPkceStateAndNonce(t *testing.T) {
	flow, err := NewLoginFlow("/incidents")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := clientFor("https://idp.test/token").BuildAuthorizeURL(flow)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()

	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q", q.Get("response_type"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("challenge method must be S256, got %q", q.Get("code_challenge_method"))
	}
	if q.Get("state") != flow.State || q.Get("nonce") != flow.Nonce {
		t.Errorf("state or nonce not carried into the authorize URL")
	}

	// The challenge must be the S256 hash, never the verifier itself.
	want := sha256.Sum256([]byte(flow.CodeVerifier))
	if q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(want[:]) {
		t.Errorf("code_challenge is not the S256 hash of the verifier")
	}
	if strings.Contains(raw, flow.CodeVerifier) {
		t.Errorf("the authorization URL leaked the PKCE verifier")
	}
}

func TestFlowSecretsAreDistinctAndUnpredictable(t *testing.T) {
	seenState := map[string]bool{}
	seenVerifier := map[string]bool{}

	for i := 0; i < 200; i++ {
		f, err := NewLoginFlow("/")
		if err != nil {
			t.Fatal(err)
		}
		if f.State == f.Nonce || f.State == f.CodeVerifier || f.Nonce == f.CodeVerifier {
			t.Fatalf("flow secrets must be independent values")
		}
		if seenState[f.State] {
			t.Fatalf("state repeated across flows")
		}
		if seenVerifier[f.CodeVerifier] {
			t.Fatalf("code verifier repeated across flows")
		}
		seenState[f.State] = true
		seenVerifier[f.CodeVerifier] = true

		// RFC 7636 requires 43-128 characters.
		if len(f.CodeVerifier) < 43 || len(f.CodeVerifier) > 128 {
			t.Fatalf("code verifier length %d is outside RFC 7636 bounds", len(f.CodeVerifier))
		}
	}
}

// Open-redirect defence: only same-site absolute paths survive.
func TestRedirectAfterLoginCannotLeaveTheSite(t *testing.T) {
	cases := map[string]string{
		"/incidents":                  "/incidents",
		"":                            "/",
		"https://attacker.example/x":  "/",
		"//attacker.example/x":        "/",
		"http://attacker.example":     "/",
		"javascript:alert(1)":         "/",
		"/legit/path?with=query":      "/legit/path",
		"\\\\attacker.example\\share": "/",
	}
	for in, want := range cases {
		f, err := NewLoginFlow(in)
		if err != nil {
			t.Fatal(err)
		}
		if f.Redirect != want {
			t.Errorf("redirect %q sanitized to %q, want %q", in, f.Redirect, want)
		}
	}
}

func TestStateAndNonceMismatchAreRejected(t *testing.T) {
	f, _ := NewLoginFlow("/")

	if err := f.VerifyCallbackState(f.State); err != nil {
		t.Errorf("the matching state was rejected: %v", err)
	}
	for _, bad := range []string{"", "wrong", f.State + "x", f.Nonce} {
		if err := f.VerifyCallbackState(bad); !errors.Is(err, ErrStateMismatch) {
			t.Errorf("state %q was accepted", bad)
		}
	}

	if err := f.VerifyNonce(f.Nonce); err != nil {
		t.Errorf("the matching nonce was rejected: %v", err)
	}
	for _, bad := range []string{"", "wrong", f.State} {
		if err := f.VerifyNonce(bad); !errors.Is(err, ErrNonceMismatch) {
			t.Errorf("nonce %q was accepted", bad)
		}
	}
}

func TestExpiredFlowCannotBeRedeemed(t *testing.T) {
	f, _ := NewLoginFlow("/")
	f.CreatedAt = time.Now().Add(-LoginFlowTTL - time.Minute)

	if !f.Expired(time.Now()) {
		t.Fatalf("a flow older than its TTL must be expired")
	}
	_, err := clientFor("https://idp.test/token").ExchangeCode(context.Background(), f, "some-code")
	if !errors.Is(err, ErrFlowExpired) {
		t.Errorf("an expired flow was exchanged: %v", err)
	}
}

// stubProvider behaves like a conforming authorization server: it verifies the
// PKCE verifier against the challenge it was given.
func stubProvider(t *testing.T, key *rsa.PrivateKey, challenge, nonce string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		verifier := r.Form.Get("code_verifier")
		if verifier == "" {
			http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
			return
		}
		sum := sha256.Sum256([]byte(verifier))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			// This is the branch that matters: a code redeemed with the wrong
			// verifier must not produce a token.
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}

		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": "https://idp.test/", "aud": "sentinel-flow-api", "sub": "user-alice",
			"nonce": nonce,
			"exp":   time.Now().Add(time.Hour).Unix(),
		})
		tok.Header["kid"] = "k1"
		signed, err := tok.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "access-token", IDToken: signed, TokenType: "Bearer", ExpiresIn: 3600,
		})
	}))
}

// The full flow: authorize, exchange with the verifier, verify the ID token,
// check the nonce.
func TestFullPkceLoginAgainstStubProvider(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	flow, err := NewLoginFlow("/incidents")
	if err != nil {
		t.Fatal(err)
	}
	srv := stubProvider(t, key, flow.CodeChallenge(), flow.Nonce)
	defer srv.Close()

	tr, err := clientFor(srv.URL).ExchangeCode(context.Background(), flow, "auth-code-123")
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}
	if tr.IDToken == "" {
		t.Fatal("no id_token returned")
	}

	v, err := NewVerifier(VerifierConfig{
		Issuer: "https://idp.test/", Audience: "sentinel-flow-api",
		Keys: map[string]*rsa.PublicKey{"k1": &key.PublicKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.Verify(tr.IDToken)
	if err != nil {
		t.Fatalf("the ID token from a completed login must verify: %v", err)
	}
	if p.Subject != "user-alice" {
		t.Errorf("subject = %q", p.Subject)
	}

	// And the nonce must bind the token to this request.
	claims := jwt.MapClaims{}
	_, _, _ = jwt.NewParser().ParseUnverified(tr.IDToken, claims)
	if err := flow.VerifyNonce(claims["nonce"].(string)); err != nil {
		t.Errorf("nonce from a legitimate login did not match: %v", err)
	}
}

// The point of PKCE: a stolen code is useless without the verifier.
func TestStolenCodeCannotBeRedeemedWithAnotherVerifier(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)

	victim, _ := NewLoginFlow("/")
	srv := stubProvider(t, key, victim.CodeChallenge(), victim.Nonce)
	defer srv.Close()

	// The attacker has the code but started their own flow, so they hold a
	// different verifier.
	attacker, _ := NewLoginFlow("/")
	if _, err := clientFor(srv.URL).ExchangeCode(context.Background(), attacker, "stolen-code"); err == nil {
		t.Fatalf("a stolen authorization code was redeemed with a different PKCE verifier")
	}

	// The victim's own flow still works, so the test is not passing by accident.
	if _, err := clientFor(srv.URL).ExchangeCode(context.Background(), victim, "stolen-code"); err != nil {
		t.Errorf("the legitimate flow failed: %v", err)
	}
}

func TestExchangeRejectsProviderErrorsWithoutLeakingDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"client sentinel-flow secret rotated"}`))
	}))
	defer srv.Close()

	f, _ := NewLoginFlow("/")
	_, err := clientFor(srv.URL).ExchangeCode(context.Background(), f, "code")
	if err == nil {
		t.Fatal("a provider error was treated as success")
	}
	if strings.Contains(err.Error(), "secret rotated") || strings.Contains(err.Error(), "sentinel-flow") {
		t.Errorf("the provider's error body leaked into ours: %v", err)
	}
}

func TestExchangeRejectsResponseWithoutIdToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "token_type": "Bearer"})
	}))
	defer srv.Close()

	f, _ := NewLoginFlow("/")
	if _, err := clientFor(srv.URL).ExchangeCode(context.Background(), f, "code"); err == nil {
		t.Errorf("a response with no id_token was accepted; there would be no identity to authorize")
	}
}

// Cookie hardening.
func TestSessionCookieIsHttpOnlyAndSameSite(t *testing.T) {
	c := SessionCookie("sentinel_session", "value", true, "", time.Hour)
	if !c.HttpOnly {
		t.Errorf("the session cookie must be HttpOnly so script cannot read it")
	}
	if !c.Secure {
		t.Errorf("the session cookie must be Secure outside local development")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("the session cookie must set SameSite")
	}
	if c.Path != "/" {
		t.Errorf("unexpected cookie path %q", c.Path)
	}
}

// The CSRF cookie is intentionally readable; it is not a credential alone.
func TestCsrfCookieIsReadableButScoped(t *testing.T) {
	c := CSRFCookie("sentinel_csrf", "value", true, "", time.Hour)
	if c.HttpOnly {
		t.Errorf("the CSRF cookie must be readable so the app can echo it in a header")
	}
	if !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("the CSRF cookie must still be Secure and SameSite")
	}
}

func TestClearCookieExpiresImmediately(t *testing.T) {
	c := ClearCookie("sentinel_session", true, "")
	if c.MaxAge >= 0 || c.Value != "" {
		t.Errorf("logout must expire the cookie, got MaxAge=%d value=%q", c.MaxAge, c.Value)
	}
}

func TestAuthorizeUrlRefusesUnconfiguredClient(t *testing.T) {
	f, _ := NewLoginFlow("/")
	if _, err := (&OIDCClientConfig{}).BuildAuthorizeURL(f); err == nil {
		t.Errorf("an unconfigured client produced an authorize URL")
	}
}
