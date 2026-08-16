# The customer source connector platform

**Established by:** Prompt 16, stages 16.1–16.4
**Authority:** `gateway/internal/connectors/` is the implementation; this
document describes it. Where they disagree, the code is correct and this file is
a defect.

## What this replaces

Prompt 01 deleted the Integration Hub. It returned `mTLSVerified: true` over
plain HTTP with no client certificate, reported healthy connections to databases
it had never contacted, and returned webhook secrets from its list endpoint, its
create response and its SQL console.

Nobody intended the secret disclosure. It happened because the credential lived
in the same struct as everything else and three code paths marshalled the
struct. Nobody intended the fake health either — it was a constant.

This platform is built so that neither is expressible.

## Sentinel Flow's own database is not affected

PostgreSQL remains Sentinel Flow's internal system of record. Everything here is
Sentinel Flow reading a **customer's** database, under a read-only identity,
through administrator-approved queries.

## The rule that gates everything

> A connector is selectable only when a real driver has passed the shared
> conformance suite against a real server.

It is structural, not cultural. `Registry.Register` derives the status from the
evidence; there is no argument by which a caller can assert availability.
`Registry.Driver` — the single choke point through which any customer database
can be contacted — refuses anything that is not `AVAILABLE`.

| Status | Meaning |
|---|---|
| `PLANNED` | No driver. The field model is published for review. |
| `IMPLEMENTING` | A driver exists and has not passed conformance. Visible, not selectable. |
| `AVAILABLE` | A real driver passed every check against a real server, and the run is recorded. |
| `DEGRADED` | Was available; its own health checks are failing. Existing connections continue; new ones are refused. |
| `DISABLED` | Turned off, or conformance regressed. |

`DEGRADED` deliberately does not qualify as selectable. Configuring a new
connection against a connector whose health checks are failing produces a
configuration that has never worked and an operator who believes it has.

**A skipped check counts as a failure.** A suite that could pass with checks
skipped would let a driver ship with its TLS verification untested because the
fixture lacked an untrusted endpoint.

## The catalog

All nine entries the guide names, each with a field model reviewed against the
provider's own connection documentation:

| Connector | Status | Why |
|---|---|---|
| PostgreSQL | driver written, passes conformance in CI | verified against real PostgreSQL 16 |
| MySQL | `PLANNED` | no driver |
| MariaDB | `PLANNED` | no driver; a **separate** entry from MySQL, not an alias |
| Microsoft SQL Server | `PLANNED` | no driver |
| Oracle | `PLANNED` | no driver |
| Snowflake | `PLANNED` | no driver |
| Amazon Redshift | `PLANNED` | no driver; a separate profile from PostgreSQL |
| Google BigQuery | `PLANNED` | no driver |
| Databricks SQL | `PLANNED` | no driver |

The eight without drivers are listed rather than hidden. A catalog that showed
only what works would make the platform look complete and invite two people to
implement Oracle twice. Each carries a `statusReason` so an operator can tell
"not built" from "broken", and each has a `notImplemented` driver that returns a
real error — a nil driver invites a nil check that someone forgets, and a
forgotten nil check panics in the request path instead of refusing clearly.

MariaDB and Redshift are separate entries on purpose. MariaDB shares MySQL's
wire protocol and diverges on sequences, JSON handling and `information_schema`
contents; Redshift is wire-compatible with PostgreSQL and is a different engine
with different system catalogs and no enforced foreign keys. Both differences
are exactly the kind a shared entry would hide.

## Stage 16.1 — contracts and capabilities

The interface is `ValidateConfig`, `TestConnection`, `DiscoverResources`,
`ExecuteTemplate`, `Health`, `Close`. Every method is bounded, takes a context,
and **none accepts free-form SQL**.

The capability model exists because databases do not share identical behaviour
and pretending they do produces a lowest-common-denominator layer that is wrong
about each of them. Three examples that are declared rather than assumed:

- **Snowflake has no read-only transaction mode.** `ReadOnlyTransactions: false`
  says so, which makes "read-only comes from the role's grants" a reviewable
  fact rather than an oversight.
- **BigQuery has no transactions in the interactive query API**, so read-only
  comes from IAM. Also declared false.
- **MySQL has no schema layer separate from the database.** Modelling it as a
  schema would make the allowlist mean something different from what the
  customer selected.

Parameter style is part of the model rather than hidden behind a rewriting
layer. Rewriting placeholders means parsing SQL, and a parser that is wrong
about a string literal turns a parameterised query into an injectable one.

## Stage 16.2 — the metadata-driven surface

`GET /connectors`, `GET /connectors/{type}`, `POST /connectors/{type}/parse-uri`.

The browser holds no knowledge of any specific database. `ConnectorWizardModal`
renders whatever fields the descriptor lists, so adding a connector is a server
change and there is no client-side field list that can drift out of step with
what the server accepts. `selectable` is computed on the server and sent
explicitly — a client that reimplemented the rule and got it wrong would offer a
connector the server then refuses, which reads to an operator as a broken
product rather than a deliberate gate.

### Secrets are write-only by construction

`Config` and `Secrets` are separate types. Everything in `Config` may be
displayed, logged and stored in plain columns; nothing in `Secrets` may. The
type system, not a convention, keeps them apart.

`Secrets` holds `secrets.Value` from Prompt 05 — AES-GCM ciphertext under a
process key, so `%v`, `%#v`, `%p`, a JSON marshal and a reflective dump all
yield nothing. `MarshalJSON` **refuses** rather than emitting `{}`: an empty
object would let a secret set be silently dropped into a response a reader would
believe was complete.

`Summarize` reads only non-secret fields, so there is no path by which it could
include one — and it does not assemble a connection string, because
reconstructing the URI from the fields would hand back exactly what the split
exists to prevent.

`disclosure_test.go` searches every rendering path for a known credential: every
fmt verb, JSON marshalling, a reflective walk that reads raw bytes rather than
calling `Interface()`, the saved summary, every descriptor, and the URI parser's
output and errors.

### A field a UI must not redisplay is one the API never sends

The write-only rule is enforced on the server. A UI bug therefore cannot expose
a saved password, key, token, wallet, service-account file or complete
connection string, because none is in the response. The wizard renders a secret
field as an empty password input with a "set or replace" affordance, always.

### Connection-string paste

Offered only where the provider defines one — so not for BigQuery, Snowflake or
Databricks, where a paste box would invite an operator to paste a
service-account JSON into a field that is not a secret field.

Where it is offered: parsed on the server, the credential separated into the
secret store, the raw string discarded. It is never stored, logged, echoed, put
in an audit payload or returned. Parsing happens server-side rather than in the
browser because a browser-side parse leaves the whole string in a React state
variable, in the form's DOM, and in whatever the next error boundary logs.

Two details:

- The MySQL DSN is parsed by hand. Feeding `user:pass@tcp(host:3306)/db` to
  `url.Parse` produces a plausible-looking wrong answer — the `@tcp(...)`
  section parses as a host — and a wrong answer here puts a credential in the
  host field.
- A parse error never contains the input. A malformed connection string is very
  often a working one with a typo.

A pasted string that selects a TLS mode which does not verify the server is
**warned about, not silently accepted**. The person pasting it has usually never
looked at that parameter.

## Stage 16.3 — the shared conformance suite

Twenty-one black-box checks against a real, disposable fixture. It holds only
the `Connector` interface, which is what makes it shared: the same checks will
run unchanged against Oracle.

It lives in the package rather than in a `_test.go` file, because a suite that
existed only at test time could not produce the evidence the registry requires.
"We ran the tests once" is not the claim "this build verified this driver
against this server version".

The checks that can only be demonstrated by a refusal are the point of the
fixture design:

| Check | What the fixture must supply |
|---|---|
| untrusted certificate is rejected | an endpoint whose certificate is not trusted |
| bad credentials rejected and redacted | credentials that really fail |
| unapproved resource is refused | a schema outside the allowlist **that exists** |
| timeout cancels a slow query | a query that really is slow |
| row and byte limits truncate | a result larger than the limit |

Refusing to read a table that does not exist proves nothing, which is why
`ForbiddenResource` must be real.

### What the suite found

Two things, both in the suite rather than the driver:

- The row-limit, byte-limit and cursor checks executed the wide template without
  attaching its identifiers, so all three failed with "not permitted" — the
  allowlist refusing a template whose `{{scope}}` was never supplied. Correct
  behaviour, wrong test.
- `secrets.New` enforces a 32-character floor, which is right for a credential
  *this application chooses* and wrong for a customer's existing database
  password. `NewExternal` was added: the same sealing and non-disclosure, with
  the length reported as `weak` rather than refused. Refusing would not make the
  customer's password longer; it would push an operator to store it somewhere
  this application does not protect.

## Stage 16.4 — the PostgreSQL driver

Chosen first because it is the protocol and auth surface the others are measured
against, and because a real server is available to verify it. `jackc/pgx` is
already a dependency for Sentinel Flow's own PostgreSQL support, so no new
driver and no new licence review.

Read-only is enforced three times, at descending levels of trust:

1. **Template registration** rejects writes, DDL, grants, session control,
   multi-statement bodies, comments, and provider functions that reach the
   filesystem or network of the database host (`pg_read_file`, `lo_import`,
   `dblink`, and the MySQL/SQL Server/Oracle equivalents for the drivers to
   come). This runs when the template is written, so the failure lands on its
   author and no write statement ever reaches a customer database.
2. **A `READ ONLY` transaction**, which PostgreSQL enforces even for a statement
   reaching a volatile function the template checks did not recognise.
3. **A server-side `statement_timeout`** slightly under the context deadline.
   The context cancels this gateway's wait; `statement_timeout` cancels the
   customer's query. Without it, abandoning a request leaves their database
   still working.

Identifiers are **allowlisted, not escaped**. An escaper has to be right about
every quoting rule of every dialect, including the ones that change with a
session setting — MySQL's `ANSI_QUOTES`, SQL Server's `QUOTED_IDENTIFIER`,
Snowflake's case folding. An allowlist has to be right about one regular
expression. The cost is real: a customer table named outside
`[A-Za-z_][A-Za-z0-9_]{0,62}` cannot be read through this platform.

The connection pool is keyed by a digest of the **whole DSN including the
credential**. Keying by host and user would let a revoked password keep working
through a cached connection, which is why `revoked_secret_stops_access` is one
of the checks.

Results are masked at this boundary by the column's declared classification, and
an undeclared column is treated as `UNCLASSIFIED`, which masks. Defaulting to
public would disclose every column anyone forgot to classify.

## Verification

```
gofmt PASS · vet PASS · go test PASS · go test -race PASS
21/21 conformance checks passed against real PostgreSQL 16.13, 0 skipped
frontend: tsc --noEmit PASS · npm run build PASS
Container builds NOT RUN: no Docker daemon in this environment
```

The full conformance output is in the CI job "Connector conformance against
PostgreSQL", which fails the build on any FAIL **or any SKIP**.

## What is not done

1. **No connection is storable.** There is no `connections` table, no migration,
   and no create/update/delete route. The catalog, descriptors, validation, URI
   parsing, the driver and the conformance suite all exist; the persistence
   between them does not. A tenant cannot yet save a connection, so nothing in
   this platform is reachable in production use.
2. **A running process reports every connector as `IMPLEMENTING`.** The
   conformance run happens in CI, not at startup, so `loadConformanceRecord`
   returns nil and nothing is selectable at runtime. That is deliberate and it
   is the honest state: promoting the entry from a constant in the binary would
   be a claim of verification made by the component that did not do the
   verifying — the same defect as `mTLSVerified: true`. Closing it means
   shipping the CI record as a build artefact the binary reads and validates
   against its own driver version.
3. **Only the password auth mode is verified.** The PostgreSQL descriptor also
   offers client certificates, and the conformance fixture runs one auth mode.
   Client-certificate authentication is therefore NOT VERIFIED.
4. **TLS verification is verified negatively only against a non-TLS server.**
   The local fixture connects to a server without TLS, so the
   untrusted-certificate check passes by the connection being refused rather
   than by a certificate being rejected. A fixture with a self-signed endpoint
   would be a stronger demonstration.
5. **No query templates ship.** `RegisterTemplate` and the whole approval model
   exist; the set of administrator-approved templates a deployment would
   actually run is empty, and there is no route to define one.
6. **No per-connector rate or concurrency limits.** `Limits` bounds rows, bytes,
   time and cursor size per execution. There is no limit on executions per
   minute, so a caller can issue bounded queries without bound.
7. **No audit events or last-used metadata.** The guide asks for both on the
   connector model. Neither exists, because there is no connector model
   persisted to attach them to.
8. **Eight connectors have no driver**, so eight ninths of the catalog is a
   field model and a capability profile. That is the intended state of a staged
   rollout and it is stated here so the catalog's size is not mistaken for
   coverage.
9. **`DiscoverResources` has no cursor.** It is bounded by `MaxRows` and a
   tenant with more tables than the bound gets a truncated list with no way to
   page past it.
