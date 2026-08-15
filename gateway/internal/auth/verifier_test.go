package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer   = "https://idp.example.com/"
	testAudience = "sentinel-flow-api"
	testKID      = "key-1"
	tenantClaim  = "https://sentinelflow.dev/tenants"
)

type testIDP struct {
	key   *rsa.PrivateKey
	other *rsa.PrivateKey // an untrusted signer
}

func newIDP(t *testing.T) *testIDP {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	o, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &testIDP{key: k, other: o}
}

func (i *testIDP) verifier(t *testing.T) *Verifier {
	t.Helper()
	v, err := NewVerifier(VerifierConfig{
		Issuer:      testIssuer,
		Audience:    testAudience,
		Keys:        map[string]*rsa.PublicKey{testKID: &i.key.PublicKey},
		TenantClaim: tenantClaim,
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// sign builds a token. Every adversarial case below is a mutation of this.
func (i *testIDP) sign(t *testing.T, mutate func(jwt.MapClaims), opts ...func(*jwt.Token)) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": testIssuer,
		"aud": testAudience,
		"sub": "user-alice",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		tenantClaim: map[string]any{
			"TENANT-A": []any{"operator"},
		},
	}
	if mutate != nil {
		mutate(claims)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	for _, o := range opts {
		o(tok)
	}
	s, err := tok.SignedString(i.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestValidTokenProducesPrincipal(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)

	p, err := v.Verify(idp.sign(t, nil))
	if err != nil {
		t.Fatalf("a well-formed token must verify: %v", err)
	}
	if p.Subject != "user-alice" {
		t.Errorf("subject = %q", p.Subject)
	}
	if !p.IsMemberOf("TENANT-A") {
		t.Errorf("expected membership in TENANT-A, got %v", p.Tenants())
	}
	if p.ActorID() != "user-alice" {
		t.Errorf("ActorID must be the token subject, got %q", p.ActorID())
	}
}

// ---------------------------------------------------------------------------
// Adversarial: token validation
// ---------------------------------------------------------------------------

func TestRejectsMissingAndMalformedTokens(t *testing.T) {
	v := newIDP(t).verifier(t)
	for _, raw := range []string{"", "   ", "not-a-jwt", "a.b.c", "Bearer x"} {
		if _, err := v.Verify(raw); err == nil {
			t.Errorf("token %q was accepted", raw)
		}
	}
}

func TestRejectsExpiredToken(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	raw := idp.sign(t, func(c jwt.MapClaims) {
		c["exp"] = time.Now().Add(-2 * time.Hour).Unix()
		c["iat"] = time.Now().Add(-3 * time.Hour).Unix()
	})
	if _, err := v.Verify(raw); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("an expired token must be rejected, got %v", err)
	}
}

func TestRejectsTokenWithNoExpiry(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	raw := idp.sign(t, func(c jwt.MapClaims) { delete(c, "exp") })
	if _, err := v.Verify(raw); err == nil {
		t.Errorf("a token with no expiry must be rejected")
	}
}

func TestRejectsWrongIssuer(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	raw := idp.sign(t, func(c jwt.MapClaims) { c["iss"] = "https://attacker.example.com/" })
	if _, err := v.Verify(raw); err == nil {
		t.Errorf("a token from another issuer was accepted")
	}
}

func TestRejectsWrongAudience(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	// A token minted for a different service must not be replayable here.
	raw := idp.sign(t, func(c jwt.MapClaims) { c["aud"] = "some-other-service" })
	if _, err := v.Verify(raw); err == nil {
		t.Errorf("a token for another audience was accepted")
	}
}

// alg=none is the classic JWT bypass.
func TestRejectsAlgNone(t *testing.T) {
	v := newIDP(t).verifier(t)
	claims := jwt.MapClaims{
		"iss": testIssuer, "aud": testAudience, "sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tok.Header["kid"] = testKID
	raw, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(raw); err == nil {
		t.Fatalf("an alg=none token was accepted")
	}
}

// Algorithm confusion: sign with HMAC using the RSA public key as the secret.
func TestRejectsAlgorithmConfusion(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)

	// The attack: present an HS256 token to a verifier that expects RS256,
	// hoping it treats the RSA public key as an HMAC secret. Restricting the
	// parser to RS256 means the token is rejected before key lookup.
	claims := jwt.MapClaims{
		"iss": testIssuer, "aud": testAudience, "sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = testKID
	raw, err := tok.SignedString([]byte("any-hmac-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(raw); err == nil {
		t.Fatalf("an HS256 token was accepted by an RS256-only verifier")
	}
}

func TestRejectsUntrustedSigner(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)

	claims := jwt.MapClaims{
		"iss": testIssuer, "aud": testAudience, "sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = testKID
	raw, err := tok.SignedString(idp.other) // correct alg, wrong key
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(raw); err == nil {
		t.Fatalf("a token signed by an untrusted key was accepted")
	}
}

func TestRejectsUnknownKeyID(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	raw := idp.sign(t, nil, func(tok *jwt.Token) { tok.Header["kid"] = "some-other-key" })
	if _, err := v.Verify(raw); err == nil {
		t.Errorf("a token naming an unknown key id was accepted")
	}
}

func TestRejectsTokenWithoutSubject(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	raw := idp.sign(t, func(c jwt.MapClaims) { delete(c, "sub") })
	if _, err := v.Verify(raw); err == nil {
		t.Errorf("a token with no subject was accepted; there would be no actor to record")
	}
}

// A verifier that cannot be configured must not be usable.
func TestVerifierRefusesIncompleteConfiguration(t *testing.T) {
	idp := newIDP(t)
	full := VerifierConfig{
		Issuer: testIssuer, Audience: testAudience,
		Keys: map[string]*rsa.PublicKey{testKID: &idp.key.PublicKey},
	}
	if _, err := NewVerifier(full); err != nil {
		t.Fatalf("complete config must build: %v", err)
	}

	noIssuer := full
	noIssuer.Issuer = ""
	if _, err := NewVerifier(noIssuer); !errors.Is(err, ErrVerifierNotConfigured) {
		t.Errorf("missing issuer must fail closed, got %v", err)
	}

	noAud := full
	noAud.Audience = ""
	if _, err := NewVerifier(noAud); !errors.Is(err, ErrVerifierNotConfigured) {
		t.Errorf("missing audience must fail closed, got %v", err)
	}

	noKeys := full
	noKeys.Keys = nil
	if _, err := NewVerifier(noKeys); !errors.Is(err, ErrVerifierNotConfigured) {
		t.Errorf("missing keys must fail closed, got %v", err)
	}
}

// A nil verifier must refuse, not permit.
func TestNilVerifierFailsClosed(t *testing.T) {
	var v *Verifier
	if _, err := v.Verify("anything"); err == nil {
		t.Fatalf("an unconfigured verifier must never authenticate a request")
	}
}

// ---------------------------------------------------------------------------
// Adversarial: claim shaping
// ---------------------------------------------------------------------------

func TestRejectsUnknownRoleInClaim(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	raw := idp.sign(t, func(c jwt.MapClaims) {
		c[tenantClaim] = map[string]any{"TENANT-A": []any{"superuser"}}
	})
	if _, err := v.Verify(raw); err == nil {
		t.Errorf("an unknown role must fail loudly rather than being dropped")
	}
}

// platform_admin must not be grantable through a tenant membership list.
func TestPlatformAdminCannotBeSmuggledThroughMembership(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	raw := idp.sign(t, func(c jwt.MapClaims) {
		c[tenantClaim] = map[string]any{"TENANT-A": []any{"platform_admin"}}
	})
	if _, err := v.Verify(raw); err == nil {
		t.Fatalf("platform_admin was accepted as a tenant role")
	}
}

func TestRejectsEmptyTenantIDInClaim(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)
	raw := idp.sign(t, func(c jwt.MapClaims) {
		c[tenantClaim] = map[string]any{"": []any{"viewer"}}
	})
	if _, err := v.Verify(raw); err == nil {
		t.Errorf("a membership with an empty tenant id was accepted")
	}
}

// ---------------------------------------------------------------------------
// Authorization matrix
// ---------------------------------------------------------------------------

func principal(subject string, memberships ...Membership) *Principal {
	return &Principal{Subject: subject, Memberships: memberships}
}

func TestAuthorizeMatrix(t *testing.T) {
	cases := []struct {
		role  Role
		perm  Permission
		allow bool
	}{
		{RoleViewer, PermReadTenant, true},
		{RoleViewer, PermUploadArtifact, false},
		{RoleViewer, PermApproveRelease, false},
		{RoleViewer, PermManageContract, false},

		{RoleOperator, PermReadTenant, true},
		{RoleOperator, PermUploadArtifact, true},
		{RoleOperator, PermApproveRelease, false},
		{RoleOperator, PermManageContract, false},

		{RoleReviewer, PermApproveRelease, true},
		{RoleReviewer, PermUploadArtifact, false},
		{RoleReviewer, PermManageContract, false},

		{RoleTenantAdmin, PermManageContract, true},
		// Administering a tenant is not authority to approve a release.
		{RoleTenantAdmin, PermApproveRelease, false},
		{RoleTenantAdmin, PermUploadArtifact, false},
		{RoleTenantAdmin, PermPlatformAdmin, false},
	}

	for _, tc := range cases {
		p := principal("u", Membership{TenantID: "T1", Roles: []Role{tc.role}})
		err := p.Authorize("T1", tc.perm)
		if tc.allow && err != nil {
			t.Errorf("%s should be granted %s: %v", tc.role, tc.perm, err)
		}
		if !tc.allow && err == nil {
			t.Errorf("%s must NOT be granted %s", tc.role, tc.perm)
		}
	}
}

// Horizontal escalation: a member of one tenant reaching another's records.
func TestCrossTenantAccessIsDenied(t *testing.T) {
	p := principal("alice", Membership{TenantID: "TENANT-A", Roles: []Role{RoleTenantAdmin}})

	if err := p.Authorize("TENANT-A", PermReadTenant); err != nil {
		t.Fatalf("own tenant must be readable: %v", err)
	}
	if err := p.Authorize("TENANT-B", PermReadTenant); !errors.Is(err, ErrNotAMember) {
		t.Errorf("cross-tenant read must be denied, got %v", err)
	}
	// Even the highest tenant role does not reach another tenant.
	if err := p.Authorize("TENANT-B", PermManageContract); err == nil {
		t.Errorf("tenant_admin of A must not manage contracts in B")
	}
}

func TestEmptyTenantScopeIsDenied(t *testing.T) {
	p := principal("alice", Membership{TenantID: "TENANT-A", Roles: []Role{RoleViewer}})
	if err := p.Authorize("", PermReadTenant); err == nil {
		t.Errorf("an empty tenant scope must be denied, not treated as a wildcard")
	}
}

func TestNilOrAnonymousPrincipalIsDenied(t *testing.T) {
	var p *Principal
	if err := p.Authorize("TENANT-A", PermReadTenant); !errors.Is(err, ErrNoPrincipal) {
		t.Errorf("nil principal must be denied, got %v", err)
	}
	anon := &Principal{}
	if err := anon.Authorize("TENANT-A", PermReadTenant); !errors.Is(err, ErrNoPrincipal) {
		t.Errorf("a principal with no subject must be denied, got %v", err)
	}
}

// Vertical escalation: a tenant admin must not reach platform scope.
func TestTenantAdminCannotReachPlatformScope(t *testing.T) {
	p := principal("alice", Membership{TenantID: "TENANT-A", Roles: []Role{RoleTenantAdmin}})
	if err := p.Authorize("TENANT-A", PermPlatformAdmin); !errors.Is(err, ErrForbidden) {
		t.Errorf("tenant_admin reached platform scope, got %v", err)
	}
}

// And the converse: platform admin is not a universal read grant.
func TestPlatformAdminIsNotAUniversalReader(t *testing.T) {
	p := &Principal{Subject: "ops", PlatformAdmin: true}

	if err := p.Authorize("", PermPlatformAdmin); err != nil {
		t.Errorf("platform admin must hold platform scope: %v", err)
	}
	if err := p.Authorize("TENANT-A", PermReadTenant); err == nil {
		t.Errorf("platform_admin with no membership must not read tenant business records")
	}
}

func TestActorIdentityComesOnlyFromTheToken(t *testing.T) {
	idp := newIDP(t)
	v := idp.verifier(t)

	// A token whose body claims to be someone else in a non-standard field.
	raw := idp.sign(t, func(c jwt.MapClaims) {
		c["actor"] = "TREASURY_SUPERVISOR_01"
		c["supervisorId"] = "someone-important"
	})
	p, err := v.Verify(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.ActorID() != "user-alice" {
		t.Errorf("ActorID must be the verified subject, got %q", p.ActorID())
	}
	if p.ActorID() == "TREASURY_SUPERVISOR_01" {
		t.Errorf("a self-asserted actor claim was used as identity")
	}
}
