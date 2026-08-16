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
| `secret:manage` | ❌ | ❌ | ❌ | ✅ | ❌ |
| `platform:admin` | ❌ | ❌ | ❌ | ❌ | ✅ |

`secret:manage` (added by Prompt 05) authorizes creating, rotating and retiring
a tenant's credentials. It does not authorize reading one, and no role does:
the secret store has no method that returns a stored value. A reviewer can
approve the movement of money and still cannot rotate the key that
authenticates the system. See `SECRETS_AND_EGRESS.md`.

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
| `/api/v1/files/upload` | POST | `artifact:upload` | Streams to immutable storage; returns 202 with identifiers and no verdict. Prompt 06. |
| `/api/v1/artifacts/{id}/content` | GET | `evidence:read` | Audited streaming proxy for raw bytes. Every read logged. Prompt 06. |

Since Prompt 08, `/files/upload` returns 202 and a background worker performs
validation; the artifact is `RECEIVED` when the response is written. See
`JOBS_AND_OUTBOX.md`.
| `/api/v1/files/ingest-raw` | POST | `artifact:upload` | **Legacy.** Reads the whole body into memory, writes no object, returns a synchronous verdict. Superseded by `/files/upload`; converges in Prompt 07. |
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

## Gap status

All five gaps recorded at the end of Prompt 04 have been addressed. What each
now is, precisely:

### 1. `/metrics` authentication — CLOSED

Guarded by its own credential (`SENTINEL_METRICS_TOKEN`, minimum 32 characters),
required in the production profile and compared in constant time. A scraper is a
machine with no tenant and no roles, so it gets a scrape credential rather than
an OIDC identity it could not meaningfully hold. In `local-demo` the process
binds loopback only, which is the guard there. Tested anonymous, wrong
credential and correct credential.

### 2. Authorization Code + PKCE — CLOSED (flow), with the wiring noted

`internal/auth/pkce.go` implements the flow: S256 challenge only, separate
`state` and `nonce`, a 10-minute flow TTL, open-redirect sanitisation, HttpOnly
+ SameSite session cookie and a readable CSRF cookie for double-submit.

Tested against a stub authorization server that **actually verifies the PKCE
verifier**, including the case that matters: a stolen authorization code
redeemed with a different verifier is refused, while the legitimate flow
succeeds. Also covered: state and nonce mismatch, expired flow, provider error
bodies not leaking into ours, and a response with no `id_token`.

Still to do: mounting `/auth/login`, `/auth/callback` and `/auth/logout` as
routes and issuing the session the CSRF middleware already protects. The flow
logic and its failure modes are done and tested; the HTTP handlers that call
them are not yet registered.

### 3. JWKS over the network — CLOSED, with named residue

`FetchJWKS` is now exercised against a real HTTP server: fetch then verify a
token signed by the matching key, multi-key rotation, HTTP error statuses,
malformed and empty documents, undersized (1024-bit) keys refused, keys with no
`kid` skipped, oversized bodies bounded, context cancellation honoured, and
unreachable hosts erroring rather than yielding an empty key set.

Precisely what remains unverified: TLS to a real host (httptest serves plain
HTTP), provider-specific document quirks such as `x5c` chains, and live rotation
timing. These are named in a comment at the foot of `jwks_test.go`.

### 4. Inbox watcher tenancy — CLOSED

The watcher no longer defaults to a tenant. `SENTINEL_WATCHER_TENANT` must name
one, the process verifies the tenant exists before starting the watcher, and
with no tenant configured **the watcher does not run at all** and says so. A
daemon that silently attributes every arriving file to one tenant is worse than
a daemon that does not run.

Contract-based tenant resolution — matching a filename to the feed contract that
expects it — is Prompt 10 and remains the eventual design.

### 5. PostgreSQL row-level security — CLOSED

`migrations_postgres/001_schema_and_rls.sql` enables **and forces** RLS on
`partners`, `file_instances`, `incidents` and `audit_events`, with `USING` and
`WITH CHECK` on every policy. `WITH CHECK` is what stops a write into another
tenant; `USING` alone would filter reads while still permitting the insert.

Verified against a real PostgreSQL 16 server, as a `NOSUPERUSER` role that owns
none of the tables:

| Property | Result |
|---|---|
| Query with no tenant set | **0 rows** — a forgotten scope starves rather than discloses |
| Scoped read | only the caller's own tenant |
| Cross-tenant `INSERT` | refused: `new row violates row-level security policy` |
| Cross-tenant `UPDATE` | affects 0 rows; the target row is unchanged |
| Counts | scoped; the unscoped count is 0 |
| App role | not superuser, owns no protected table |
| Every protected table | `relrowsecurity` and `relforcerowsecurity` both true |

Prompt 05 extended this to `secret_versions` and `secret_events` with the same
properties, plus triggers making credential material immutable and the rotation
trail append-only. One caveat worth stating precisely: `FORCE` subjects the
table *owner* to its policies, but a **superuser still bypasses RLS entirely**,
which is why `TestApplicationRoleIsNotSuperuserOrTableOwner` is the control that
actually matters.

A CI job runs these against a real PostgreSQL service container **and fails if
the tests skip**, because a security test that quietly does nothing is worse
than no test.

**Scope boundary:** the running application still uses SQLite. The PostgreSQL
schema and its policies are proven, and the repository layer is already written
against a tenant-scoped interface, but the runtime port is not done. RLS is
therefore a verified defence that is not yet in the request path.

## Original gap list (superseded by the section above)

Retained so the history of what was outstanding is visible:

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
