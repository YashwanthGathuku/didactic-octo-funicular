// Package auth verifies caller identity and answers authorization questions.
//
// It exists because actor identity was previously read from request bodies:
// an approval endpoint accepted `{"actor": "..."}` and defaulted it to a literal
// supervisor name when absent, so anyone who could reach the endpoint could
// record a decision under any name. Identity here comes only from a verified
// token.
package auth

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Role is a permission bundle within one tenant.
type Role string

const (
	RoleViewer      Role = "viewer"       // read tenant records
	RoleOperator    Role = "operator"     // upload and act on artifacts
	RoleReviewer    Role = "reviewer"     // approve or reject releases
	RoleTenantAdmin Role = "tenant_admin" // manage contracts, partners, members

	// RolePlatformAdmin is deliberately NOT a tenant role. It is held outside
	// any tenant and grants platform operations only; it does not confer read
	// access to tenant business records. A tenant admin cannot reach it, and
	// holding it does not silently make someone a reader of every tenant.
	RolePlatformAdmin Role = "platform_admin"
)

// tenantRoles is the set a tenant membership may legitimately carry.
var tenantRoles = map[Role]bool{
	RoleViewer:      true,
	RoleOperator:    true,
	RoleReviewer:    true,
	RoleTenantAdmin: true,
}

// Permission is a capability checked at a service boundary.
type Permission string

const (
	PermReadTenant     Permission = "tenant:read"
	PermUploadArtifact Permission = "artifact:upload"
	PermApproveRelease Permission = "release:approve"
	PermManageContract Permission = "contract:manage"
	PermReadEvidence   Permission = "evidence:read"
	PermPlatformAdmin  Permission = "platform:admin"
)

// rolePermissions is the authorization matrix.
//
// tenant_admin deliberately does NOT carry release:approve. Administering a
// tenant and approving the movement of money are different authorities, and
// collapsing them lets one account both configure the control and satisfy it.
var rolePermissions = map[Role][]Permission{
	RoleViewer:   {PermReadTenant, PermReadEvidence},
	RoleOperator: {PermReadTenant, PermReadEvidence, PermUploadArtifact},
	RoleReviewer: {PermReadTenant, PermReadEvidence, PermApproveRelease},
	RoleTenantAdmin: {
		PermReadTenant, PermReadEvidence, PermManageContract,
	},
	RolePlatformAdmin: {PermPlatformAdmin},
}

// Membership is one subject's roles within one tenant.
type Membership struct {
	TenantID string
	Roles    []Role
}

// Principal is a verified caller. It is constructed only from validated token
// claims; there is no exported way to build one from request input.
type Principal struct {
	// Subject is the immutable identifier from the identity provider. It is the
	// only acceptable source of an actor ID.
	Subject string
	// Issuer and audience are retained so audit records show which identity
	// provider vouched for this caller.
	Issuer   string
	Audience string
	// Email is informational only and is never used for authorization: an
	// identity provider may permit users to change it.
	Email string

	Memberships   []Membership
	PlatformAdmin bool
}

var (
	// ErrNoPrincipal means the request carried no verified identity.
	ErrNoPrincipal = errors.New("no authenticated principal")
	// ErrNotAMember means the principal holds no membership in the tenant.
	ErrNotAMember = errors.New("principal is not a member of this tenant")
	// ErrForbidden means the principal is a member but lacks the permission.
	ErrForbidden = errors.New("principal lacks the required permission")
)

// ActorID returns the value to record as the actor of an action.
//
// This is the subject from the verified token, never anything the caller sent
// in a body or header.
func (p *Principal) ActorID() string {
	if p == nil {
		return ""
	}
	return p.Subject
}

// Tenants lists the tenants this principal belongs to, sorted for stable
// output. Platform admin does not expand this: holding it grants no tenant
// membership.
func (p *Principal) Tenants() []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Memberships))
	for _, m := range p.Memberships {
		out = append(out, m.TenantID)
	}
	sort.Strings(out)
	return out
}

// RolesIn returns the roles held in a tenant, or nil if not a member.
func (p *Principal) RolesIn(tenantID string) []Role {
	if p == nil || tenantID == "" {
		return nil
	}
	for _, m := range p.Memberships {
		if m.TenantID == tenantID {
			return m.Roles
		}
	}
	return nil
}

// IsMemberOf reports tenant membership.
func (p *Principal) IsMemberOf(tenantID string) bool {
	return len(p.RolesIn(tenantID)) > 0
}

// Authorize answers whether this principal may exercise a permission in a
// tenant. It is the single authorization decision point.
//
// Fails closed on every uncertain input: a nil principal, an empty tenant, a
// tenant the principal does not belong to, or a permission no held role grants.
func (p *Principal) Authorize(tenantID string, perm Permission) error {
	if p == nil || p.Subject == "" {
		return ErrNoPrincipal
	}

	// Platform scope is separate. It is neither reachable from a tenant role
	// nor does it grant one.
	if perm == PermPlatformAdmin {
		if !p.PlatformAdmin {
			return fmt.Errorf("%w: platform:admin", ErrForbidden)
		}
		return nil
	}
	if p.PlatformAdmin && len(p.Memberships) == 0 {
		// A platform admin with no tenant membership must not read tenant
		// business records. Escalating to data access requires an explicit
		// membership, which is auditable.
		return fmt.Errorf("%w: platform_admin holds no membership in %s", ErrNotAMember, tenantID)
	}

	if tenantID == "" {
		return fmt.Errorf("%w: no tenant scope supplied", ErrNotAMember)
	}

	roles := p.RolesIn(tenantID)
	if len(roles) == 0 {
		return fmt.Errorf("%w: %s", ErrNotAMember, tenantID)
	}

	for _, r := range roles {
		for _, granted := range rolePermissions[r] {
			if granted == perm {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: %s in %s", ErrForbidden, perm, tenantID)
}

// ParseRole validates a role string from a token claim. Unknown roles are
// rejected rather than ignored, so a typo in an identity provider's mapping
// fails loudly instead of silently granting nothing.
func ParseRole(s string) (Role, error) {
	r := Role(strings.ToLower(strings.TrimSpace(s)))
	if tenantRoles[r] {
		return r, nil
	}
	if r == RolePlatformAdmin {
		return r, nil
	}
	return "", fmt.Errorf("unknown role %q", s)
}

// PermissionsFor exposes the matrix for the route-permission documentation and
// its test.
func PermissionsFor(r Role) []Permission { return rolePermissions[r] }
