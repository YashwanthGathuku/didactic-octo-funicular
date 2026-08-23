package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

type ctxKey int

const principalKey ctxKey = iota

// FromContext returns the verified browser/API principal, or nil.
// Managed agent ingress uses a separate signed-IAP context in agent_identity.go.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

// AuditSink records security-relevant events. Denials are recorded, not only successes.
type AuditSink func(event string, actor string, detail map[string]any)

// Middleware authenticates browser/API requests.
type Middleware struct {
	Verifier *Verifier
	Audit    AuditSink
	// DemoPrincipal, when non-nil, is used INSTEAD of token verification.
	// It exists solely for the named local-demo profile and the gateway refuses
	// to set it in any other profile.
	DemoPrincipal *Principal
}

func (m *Middleware) audit(event, actor string, detail map[string]any) {
	if m.Audit != nil {
		m.Audit(event, actor, detail)
		return
	}
	log.Printf("audit event=%s actor=%s detail=%v", event, actor, detail)
}

func deny(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// isManagedIAPIngressCandidate is intentionally narrow. It bypasses *only* the
// browser/API bearer verifier so the exact managed-agent endpoint can perform
// its stronger, workload-specific signed IAP verification. A header is not
// accepted as identity here; cryptographic verification still occurs in
// ManagedAgentIdentityMiddleware before the tool handler can execute.
func isManagedIAPIngressCandidate(r *http.Request) bool {
	enabled := strings.TrimSpace(os.Getenv("SENTINEL_MANAGED_AGENT_INGRESS"))
	if enabled != "1" && !strings.EqualFold(enabled, "true") {
		return false
	}
	if r.URL.Path != "/api/v1/internal/agent-tools" {
		return false
	}
	return strings.TrimSpace(r.Header.Get("X-Goog-IAP-JWT-Assertion")) != ""
}

// Authenticate verifies the bearer token and attaches the principal.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Managed agent ingress has no end-user bearer token. The exact route is
		// allowed through this layer only when a signed-IAP assertion is present;
		// the assertion is verified downstream before any request body is trusted.
		if isManagedIAPIngressCandidate(r) {
			next.ServeHTTP(w, r)
			return
		}

		if m.DemoPrincipal != nil {
			ctx := context.WithValue(r.Context(), principalKey, m.DemoPrincipal)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if m.Verifier == nil {
			m.audit("AUTH_MISCONFIGURED", "", map[string]any{"path": r.URL.Path})
			deny(w, http.StatusServiceUnavailable, "authentication_not_configured")
			return
		}

		raw := bearerToken(r)
		if raw == "" {
			m.audit("AUTH_DENIED", "", map[string]any{"path": r.URL.Path, "reason": "no_token"})
			deny(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		p, err := m.Verifier.Verify(raw)
		if err != nil {
			m.audit("AUTH_DENIED", "", map[string]any{"path": r.URL.Path, "reason": err.Error()})
			deny(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), principalKey, p)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// RequireCSRFToken protects cookie-authenticated mutations.
//
// Only applied when the request carries a session cookie: a request
// authenticated purely by an Authorization header is not CSRF-able, because a
// browser will not attach that header cross-origin on its own.
func RequireCSRFToken(sessionCookie, csrfCookie, headerName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			c, err := r.Cookie(sessionCookie)
			if err != nil || c.Value == "" {
				// No session cookie: not a cookie-authenticated mutation. Managed IAP
				// requests also arrive without the browser session cookie and are
				// authenticated by their signed assertion downstream.
				next.ServeHTTP(w, r)
				return
			}

			token, err := r.Cookie(csrfCookie)
			if err != nil || token.Value == "" {
				deny(w, http.StatusForbidden, "csrf_token_missing")
				return
			}

			sent := r.Header.Get(headerName)
			if sent == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(token.Value)) != 1 {
				deny(w, http.StatusForbidden, "csrf_token_invalid")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission enforces a permission for a tenant resolved from the request.
func (m *Middleware) RequirePermission(tenantOf func(*http.Request) string, perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := FromContext(r.Context())
			if p == nil {
				deny(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			tenant := tenantOf(r)
			if err := p.Authorize(tenant, perm); err != nil {
				m.audit("AUTHZ_DENIED", p.ActorID(), map[string]any{
					"path": r.URL.Path, "tenant": tenant, "permission": string(perm),
				})
				deny(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
