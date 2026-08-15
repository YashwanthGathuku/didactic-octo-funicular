package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type ctxKey int

const principalKey ctxKey = iota

// FromContext returns the verified principal, or nil.
//
// Handlers must treat nil as "deny". There is no anonymous principal, because
// an anonymous principal is an object other code will eventually authorize.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

// AuditSink records security-relevant events. Denials are recorded, not only
// successes: a login that fails is the signal worth keeping.
type AuditSink func(event string, actor string, detail map[string]any)

// Middleware authenticates requests.
//
// It fails closed in every direction:
//   - a nil verifier rejects every request rather than passing them through
//   - a missing or malformed token is 401
//   - every rejection returns the same body, so probing cannot distinguish
//     "expired" from "wrong signature" from "unknown key"
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

// Authenticate verifies the bearer token and attaches the principal.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.DemoPrincipal != nil {
			ctx := context.WithValue(r.Context(), principalKey, m.DemoPrincipal)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if m.Verifier == nil {
			// Unconfigured authentication is a refusal, never a bypass. The
			// previous middleware treated an unset token as "auth disabled".
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
			// The reason is logged but never returned.
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
func RequireCSRFToken(cookieName, headerName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				// No session cookie: not a cookie-authenticated mutation.
				next.ServeHTTP(w, r)
				return
			}

			sent := r.Header.Get(headerName)
			if sent == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(c.Value)) != 1 {
				deny(w, http.StatusForbidden, "csrf_token_invalid")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission enforces a permission for a tenant resolved from the
// request. It is defence in depth: the repository refuses an unauthorized scope
// regardless, so a route registered without this middleware still cannot read
// another tenant's data.
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
				// Not-a-member and lacking-permission return the same status,
				// so probing cannot map another tenant's existence.
				deny(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
