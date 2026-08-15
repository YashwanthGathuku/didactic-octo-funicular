# Route and permission matrix

**Established by:** Prompt 04
**Authority:** `gateway/internal/auth/principal.go` is the implementation; this
document describes it. Where they disagree, the code is correct and this file is
a defect.

## Roles

| Role | Scope | Intent |
|---|---|---|
| `viewer` | tenant | Read tenant records and evidence |
| `operator` | tenant | Upload artifacts and act on them |
| `reviewer` | tenant | Approve or reject releases |
| `tenant_admin` | tenant | Manage partners, contracts and membership |
| `platform_admin` | **outside any tenant** | Platform operations only |

## Permission matrix

| Permission | viewer | operator | reviewer | tenant_admin | platform_admin |
|---|:--:|:--:|:--:|:--:|:--:|
| `tenant:read` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `evidence:read` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `artifact:upload` | ❌ | ✅ | ❌ | ❌ | ❌ |
| `release:approve` | ❌ | ❌ | ✅ | ❌ | ❌ |
| `contract:manage` | ❌ | ❌ | ❌ | ✅ | ❌ |
| `platform:admin` | ❌ | ❌ | ❌ | ❌ | ✅ |

Two deliberate gaps, both tested:

**`tenant_admin` does not hold `release:approve`.** Administering a tenant and
approving the movement of money are different authorities. An account holding
both can configure the control and then satisfy it, which defeats the point of
having one.

**`platform_admin` grants no tenant read access.** It is not a tenant role, it
cannot be granted through a membership claim, and a `tenant_admin` cannot reach
it. A platform administrator who needs to see a tenant's records must be given an
explicit membership, which is auditable, rather than inheriting universal access.

## Route enforcement

Enforcement happens at two layers, and the second is the one that actually
holds:

1. **Middleware** (`auth.Middleware.Authenticate`, `RequirePermission`) — rejects
   anonymous and unauthorized requests early with a uniform response.
2. **Repository** (`repository.Scope`) — every data method requires a `Scope`
   that can only be constructed from an authorized `Principal`. A zero `Scope`
   is refused by every method, and no method accepts a raw query.

Layer 2 is what makes this structural. A route added without the middleware
still cannot read another tenant's data, because there is no way to obtain a
usable `Scope` without passing the authorization check.

| Route | Method | Permission | Notes |
|---|---|---|---|
| `/api/v1/health` | GET | authenticated | Liveness. Checks no dependency by design. |
| `/api/v1/ready` | GET | authenticated | Probes the database; reports each dependency's real state. |
| `/api/v1/sla-board` | GET | `tenant:read` | |
| `/api/v1/partners` | GET | `tenant:read` | |
| `/api/v1/partners` | POST | `contract:manage` | |
| `/api/v1/contracts` | GET | `tenant:read` | |
| `/api/v1/incidents` | GET | `tenant:read` | |
| `/api/v1/ledger` | GET | `evidence:read` | |
| `/api/v1/compliance/export` | GET | `evidence:read` | Carries no regulatory claim. |
| `/api/v1/files/upload` | POST | `artifact:upload` | |
| `/api/v1/files/ingest-raw` | POST | `artifact:upload` | |
| `/api/v1/incidents/{id}/triage` | POST | `tenant:read` | Read-only AI analysis; 503 when unconfigured. |
| `/api/v1/incidents/{id}/approve` | POST | `release:approve` | Actor from token; justification required. |
| `/api/v1/security/verify-key` | POST | authenticated | Stateless utility. |
| `/api/v1/security/verify-signature` | POST | authenticated | Stateless utility. |
| `/api/v1/generator/sample` | GET | authenticated | Fixture generator. |
| `/api/v1/evals/run` | GET | authenticated | 503 when unconfigured; never a score. |
| `/api/v1/stream` | GET | authenticated | Emits only a connect heartbeat today. |
| `/metrics` | GET | **unauthenticated** | Known gap, below. |

## Profiles

**`production`** refuses to start without `SENTINEL_OIDC_ISSUER`,
`SENTINEL_OIDC_AUDIENCE` and `SENTINEL_OIDC_JWKS_URL`, alongside the database,
object store, allowed origin and PGP keyring. A router built without a verifier
returns `503 authentication_not_configured` on every route — it does not serve
them openly.

**`local-demo`** uses a named demo principal (`demo-operator@local`) holding all
four tenant roles in the default tenant. It binds loopback only and refuses any
other bind address. This is the only profile that runs without token
verification, and it says so at startup and on every screen.

## Token requirements

- `RS256` only. Restricting the algorithm list is what defeats `alg=none` and
  the HS256-with-the-RSA-public-key confusion attack.
- `iss`, `aud`, `exp` and `kid` are all required and all checked; `exp` is
  mandatory, so a token without one is rejected rather than treated as eternal.
- Memberships come from a namespaced claim shaped
  `{"TENANT-A": ["operator"], ...}`. An unknown role is a hard failure, not a
  silent drop: a mis-mapped provider configuration must not present as "user
  with no permissions", which is indistinguishable from deliberate revocation.
- `platform_admin` inside a membership list is rejected outright.

Every rejection returns the same body. A caller cannot distinguish an expired
token from a forged one, and a cross-tenant object ID returns the same error
text as an ID that does not exist.

## Tenant resolution

A request's tenant comes from verified claims, never from a request field:

1. an explicit `X-Sentinel-Tenant` header, **validated against memberships**
2. the sole membership, when the principal belongs to exactly one tenant
3. otherwise refused — guessing would silently pick a tenant for a caller who
   meant another

The header is a *selector*, not an assertion. Naming a tenant you do not belong
to is a 403, and a tenant that does not exist returns a byte-identical response
to one that exists but is not yours, so the header cannot be used to enumerate
tenants.

## Known gaps

These are real and tracked, not oversights:

1. **`/metrics` is unauthenticated.** It is registered outside `/api/v1`. It
   emits no tenant names, filenames or business values, but it should still sit
   behind network policy or its own credential. Prompt 13.
2. **The browser Authorization Code + PKCE flow is not implemented.** Token
   verification is complete and tested; the interactive login that obtains the
   token is not built, and the session cookie the CSRF middleware protects is
   not yet issued by this service.
3. **JWKS fetching is not exercised against a live provider.** Parsing is covered
   by tests; the network path is not, because no identity provider is available
   to this environment. First deployment against a real provider is where that
   is genuinely verified.
4. **Tenant resolution is complete for the HTTP path.** Every request handler
   resolves its tenant from verified claims via `resolveScope`, and cross-tenant
   isolation is proven end to end over HTTP in `tenant_isolation_test.go`: two
   tenants with real signed tokens cannot read, infer, update or enumerate each
   other's artifacts, incidents, ledger or evidence export.

   **The inbox watcher remains unscoped** (`watcher.go`). It is a background
   daemon with no request and therefore no principal, so it still writes to
   `DefaultTenantID`. Its tenant must come from the feed contract that matches
   the arriving file, which is Prompt 10 work. Until then, files dropped into
   the watched directory land in one tenant regardless of origin.
5. **PostgreSQL row-level security is not configured.** The application still
   uses SQLite, which has no equivalent. RLS becomes available with the
   PostgreSQL port.
