package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Authorization Code flow with PKCE.
//
// PKCE matters here even though this is a confidential client: it binds the
// authorization code to the browser that started the flow, so a code
// intercepted from a redirect, a log, or the Referer header cannot be redeemed
// by anyone else.
//
// state and nonce are separate and both required. state defeats login CSRF --
// an attacker cannot make a victim's browser complete a flow the attacker
// started. nonce binds the resulting ID token to this authorization request, so
// a token replayed from elsewhere is rejected.

// OIDCClientConfig describes this service as an OAuth client.
type OIDCClientConfig struct {
	Issuer        string
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	AuthorizeURL  string
	TokenURL      string
	Scopes        []string
	CookieDomain  string
	SecureCookies bool
}

// LoginFlow holds the per-request secrets a login needs. It is stored in a
// short-lived HttpOnly cookie, never in server memory keyed by state, so the
// gateway stays stateless across restarts and replicas.
type LoginFlow struct {
	State        string    `json:"state"`
	Nonce        string    `json:"nonce"`
	CodeVerifier string    `json:"code_verifier"`
	Redirect     string    `json:"redirect"`
	CreatedAt    time.Time `json:"created_at"`
}

var (
	// ErrFlowExpired means the login took too long to complete.
	ErrFlowExpired = errors.New("login flow expired")
	// ErrStateMismatch means the callback did not correspond to a flow this
	// browser started.
	ErrStateMismatch = errors.New("state does not match the login request")
	// ErrNonceMismatch means the ID token was not minted for this request.
	ErrNonceMismatch = errors.New("nonce does not match the login request")
)

// LoginFlowTTL bounds how long an in-progress login stays valid.
const LoginFlowTTL = 10 * time.Minute

// randomURLSafe returns n bytes of cryptographic randomness, URL-safe encoded.
func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewLoginFlow mints the state, nonce and PKCE verifier for one login.
func NewLoginFlow(redirectAfterLogin string) (*LoginFlow, error) {
	state, err := randomURLSafe(32)
	if err != nil {
		return nil, err
	}
	nonce, err := randomURLSafe(32)
	if err != nil {
		return nil, err
	}
	// RFC 7636 permits 43-128 characters; 64 random bytes encodes to 86.
	verifier, err := randomURLSafe(64)
	if err != nil {
		return nil, err
	}
	return &LoginFlow{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: verifier,
		Redirect:     sanitizeRedirect(redirectAfterLogin),
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// sanitizeRedirect prevents an open redirect. Only a same-site absolute path is
// accepted; anything with a scheme or host is discarded rather than followed.
func sanitizeRedirect(raw string) string {
	if raw == "" {
		return "/"
	}
	if strings.HasPrefix(raw, "//") {
		return "/" // protocol-relative URL pointing at another host
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return "/"
	}
	return u.Path
}

// CodeChallenge is the S256 challenge derived from the verifier.
//
// S256 only. The "plain" method sends the verifier itself in the authorization
// request, which defeats the purpose.
func (f *LoginFlow) CodeChallenge() string {
	sum := sha256.Sum256([]byte(f.CodeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Expired reports whether the flow has outlived its window.
func (f *LoginFlow) Expired(now time.Time) bool {
	return now.After(f.CreatedAt.Add(LoginFlowTTL))
}

// BuildAuthorizeURL builds the provider redirect for this flow.
func (c *OIDCClientConfig) BuildAuthorizeURL(f *LoginFlow) (string, error) {
	if c.AuthorizeURL == "" || c.ClientID == "" || c.RedirectURL == "" {
		return "", errors.New("oidc client is not configured")
	}
	u, err := url.Parse(c.AuthorizeURL)
	if err != nil {
		return "", err
	}
	scopes := c.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURL)
	q.Set("scope", strings.Join(scopes, " "))
	q.Set("state", f.State)
	q.Set("nonce", f.Nonce)
	q.Set("code_challenge", f.CodeChallenge())
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// TokenResponse is the provider's reply to a code exchange.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// ExchangeCode redeems an authorization code, presenting the PKCE verifier.
//
// The verifier is what proves this is the same browser that began the flow. A
// provider that ignores it is misconfigured, which is why the request always
// sends it rather than treating it as optional.
func (c *OIDCClientConfig) ExchangeCode(ctx context.Context, f *LoginFlow, code string) (*TokenResponse, error) {
	if code == "" {
		return nil, errors.New("no authorization code supplied")
	}
	if f.Expired(time.Now().UTC()) {
		return nil, ErrFlowExpired
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", c.RedirectURL)
	form.Set("client_id", c.ClientID)
	form.Set("code_verifier", f.CodeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if c.ClientSecret != "" {
		req.SetBasicAuth(url.QueryEscape(c.ClientID), url.QueryEscape(c.ClientSecret))
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// The provider's error body may name the client; do not propagate it.
		return nil, fmt.Errorf("token exchange failed: provider returned HTTP %d", resp.StatusCode)
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("token exchange: malformed response")
	}
	if tr.IDToken == "" {
		return nil, errors.New("token exchange: provider returned no id_token")
	}
	return &tr, nil
}

// VerifyCallbackState compares the returned state to the flow's, in constant
// time.
func (f *LoginFlow) VerifyCallbackState(returned string) error {
	if returned == "" {
		return ErrStateMismatch
	}
	if subtle.ConstantTimeCompare([]byte(returned), []byte(f.State)) != 1 {
		return ErrStateMismatch
	}
	return nil
}

// VerifyNonce checks the nonce carried by the verified ID token.
func (f *LoginFlow) VerifyNonce(tokenNonce string) error {
	if tokenNonce == "" {
		return ErrNonceMismatch
	}
	if subtle.ConstantTimeCompare([]byte(tokenNonce), []byte(f.Nonce)) != 1 {
		return ErrNonceMismatch
	}
	return nil
}

// SessionCookie builds the cookie that carries the session after login.
//
// HttpOnly so script cannot read it, SameSite=Lax so it is not sent on
// cross-site POSTs, Secure outside local development, and Path=/ scoped to this
// service.
func SessionCookie(name, value string, secure bool, domain string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   domain,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	}
}

// CSRFCookie carries the double-submit token.
//
// Deliberately NOT HttpOnly: the browser application must read it to echo it in
// the X-CSRF-Token header. It is not a credential on its own -- it is only
// meaningful alongside the HttpOnly session cookie.
func CSRFCookie(name, value string, secure bool, domain string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   domain,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	}
}

// ClearCookie expires a cookie by name.
func ClearCookie(name string, secure bool, domain string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Domain:   domain,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}
