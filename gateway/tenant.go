package main

import (
	"encoding/json"
	"net/http"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/repository"
)

// TenantHeader lets a caller who belongs to several tenants say which one this
// request is for.
//
// It is a *selector*, not an assertion. The value is always checked against the
// principal's verified memberships, so naming a tenant you do not belong to is
// a denial rather than an escalation.
const TenantHeader = "X-Sentinel-Tenant"

// resolveScope determines which tenant a request operates on and authorizes it.
//
// Resolution order:
//  1. an explicit X-Sentinel-Tenant header, validated against memberships
//  2. the sole membership, when the principal belongs to exactly one tenant
//  3. otherwise refuse, because guessing would silently pick a tenant for a
//     caller who meant another one
//
// This is the function that closes the gap left at the end of Prompt 04: the
// tenant now comes from verified claims rather than a package constant.
func resolveScope(r *http.Request, perm auth.Permission) (repository.Scope, *scopeError) {
	p := auth.FromContext(r.Context())
	if p == nil || p.Subject == "" {
		return repository.Scope{}, &scopeError{status: http.StatusUnauthorized, code: "unauthorized"}
	}

	requested := r.Header.Get(TenantHeader)
	tenants := p.Tenants()

	var tenantID string
	switch {
	case requested != "":
		tenantID = requested
	case len(tenants) == 1:
		tenantID = tenants[0]
	case len(tenants) == 0:
		return repository.Scope{}, &scopeError{
			status: http.StatusForbidden,
			code:   "no_tenant_membership",
			detail: "this identity holds no tenant membership",
		}
	default:
		return repository.Scope{}, &scopeError{
			status: http.StatusBadRequest,
			code:   "tenant_selection_required",
			detail: "this identity belongs to multiple tenants; name one in the " + TenantHeader + " header",
		}
	}

	scope, err := repository.NewScope(p, tenantID, perm)
	if err != nil {
		// Not-a-member and lacking-permission both return 403 with the same
		// body. Distinguishing them would let a caller enumerate which tenants
		// exist by watching which denial they receive.
		return repository.Scope{}, &scopeError{status: http.StatusForbidden, code: "forbidden"}
	}
	return scope, nil
}

type scopeError struct {
	status int
	code   string
	detail string
}

// write emits the refusal. The body carries a stable code and never the
// internal reason.
func (e *scopeError) write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(e.status)
	body := map[string]string{"error": e.code}
	if e.detail != "" {
		body["detail"] = e.detail
	}
	_ = json.NewEncoder(w).Encode(body)
}
