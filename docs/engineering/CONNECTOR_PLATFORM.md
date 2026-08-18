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

## The connection lifecycle is audited

Creating, testing, rotating and deleting a connection are written to the
append-only evidence chain. The secret store already recorded its own events for
the credential half; the connection half had none, and those are exactly the
records an operator reconstructs during an incident — "who pointed this at a
production replica, and when" had no answer.

The payload is built inside `internal/connectors`, so one place decides what a
connection event may say. Every field is an identifier, a state or a
classification. The **host is deliberately omitted**: a customer's internal
hostname is not a secret and is also not something that needs to be in an export
shared with a third party. A failed check is recorded as a failure rather than
skipped — a trail carrying only successes makes silence ambiguous between
"nobody tested it" and "everybody who tested it failed".

**An audit failure does not fail the operation it describes.** That is a real
trade-off, made deliberately: refusing to delete a connection because the ledger
is unavailable would leave a live credential in place for the duration of an
unrelated outage. The failure is logged loudly and counted, and
`Store.AuditFailures` exposes the count so a health endpoint can surface it — an
audit sink that has been silently failing is worse than one never configured,
because the trail looks complete.

## Executions are bounded per minute

`maxPerMinute` is now enforced, defaulting to 60. Per-execution limits bound one
query's rows, bytes and time; without a rate limit, an unbounded number of
bounded queries is still unbounded, and the load lands on a customer's
production database caused by their reporting integration.

It is a **fixed window**, not a token bucket. The boundary allows a burst of up
to twice the limit, which is a real imprecision and an acceptable one: the
purpose is to stop a runaway loop, not to shape traffic, and a fixed window is
something an operator reading the code can predict. Stale windows are swept, or
the map would grow by one entry per connection ever seen — a slow leak invisible
until a tenant with many connections arrives.

A refusal is audited. A connection hitting its limit is either a misconfigured
caller or a runaway loop, and an operator should find out from the trail rather
than from the customer.

## The operator surface

`SavedConnectionsPanel` lists saved connections in the cockpit rather than
behind a modal, because a connection that has never been checked, or that
started failing overnight, is something an operator should meet rather than go
looking for.

`NEVER_CHECKED` renders with its own icon and a **grey** treatment — never a
colour that reads as success. That is the same rule the schema enforces with a
CHECK constraint and the API enforces by returning the state explicitly: a
connection nobody has tested showing green is precisely the defect the
Integration Hub shipped.

Nothing in the panel masks a credential, because nothing in it receives one. A
bug in that file cannot disclose one. Replacing a credential is the only way to
change it, and the form says why the current value cannot be shown.

## Verification

```
gofmt PASS · vet PASS · go test PASS · go test -race PASS
21/21 conformance checks passed against real PostgreSQL 16.13, 0 skipped
frontend: tsc --noEmit PASS · npm run build PASS
Container builds NOT RUN: no Docker daemon in this environment
```

The full conformance output is in the CI job "Connector conformance against
PostgreSQL", which fails the build on any FAIL **or any SKIP**.

## Storing a connection

Migration 010 adds `source_connections`, `source_connection_secrets` and
`source_connection_health`. The shape is dictated by one rule: **a credential
never lands in these tables.** Non-secret configuration goes in a column; every
secret goes to the Prompt 05 secret store and only its *name* is recorded here.

`TestTheCredentialIsNotInTheConnectionTables` dumps every cell of every table in
the database — the same exhaustive dump the Prompt 01 quarantine tests use — and
searches for the fixture credential. Checking only the tables this package
writes would miss one that leaked sideways into an audit payload, which is
exactly how the Integration Hub's secret escaped.

### The write ordering is forced

Credentials are written to the secret store **before** this package's
transaction opens, and the secret name is derived from the tenant and the
connection's display name — which are unique together and known before the row
exists — rather than from the generated id.

It has to be that way round. The first implementation derived the name from the
id, so it called the secret store from inside its own transaction; on SQLite
that is a self-deadlock, and it presented as every save hanging for the full
ten-second busy timeout and then failing with "storing the credential failed".
It is also the correct order regardless: a connection row without its credential
can never work, while a sealed secret with no connection is inert — and is
retired if the transaction does not commit.

Reads use the **stored** name rather than re-deriving it. Re-deriving would
break the moment anything about the derivation changed, and the failure would be
a credential that cannot be read rather than an error someone sees at deploy
time.

### A weak customer credential is accepted and reported

A customer's six-character database password is stored, sealed, and flagged in
`weakSecrets` by field name. Refusing it would not make it longer; it would push
an operator to keep the password somewhere this application does not protect.
`TestAWeakCustomerCredentialIsAcceptedAndReported` also asserts the weak
credential is not disclosed — being weak is not a reason to protect it less.

### Health is never optimistic

A new connection is `NEVER_CHECKED` with no timestamp, and the schema's CHECK
constraint keeps that a distinct value from `HEALTHY`. `TestConnection` writes
whatever the driver returned; testing an unreachable host records `FAILED` with
a sanitized error class, and the stored detail is asserted to carry neither the
credential nor the account name.

Every check is kept in `source_connection_health` rather than overwriting the
last. A connection failing now is a different situation from one that has been
failing for a week, and the second is invisible if each check overwrites.

## Runtime evidence

The conformance run produces an artefact; the deployment carries it; the binary
validates it. `SENTINEL_CONNECTOR_EVIDENCE` names the file.

A record is rejected — leaving the connector unselectable — when it records a
failure, records a **skipped** check, is more than 90 days old, or names a
driver version that disagrees with the running build. That last one matters
most: evidence produced against a different driver build verified different
code.

The file is written by `WriteEvidence` and read by `LoadEvidence`, in the same
package, so producer and consumer cannot drift — and drift there would present
as a deployment that silently has no evidence, indistinguishable from one that
never ran the suite. `WriteEvidence` refuses to write for a run that did not
pass.

`TestConformanceRunPublishesEvidence` runs the real suite against a real server,
publishes the artefact, reads it back through the loader the binary uses, and
asserts it promotes PostgreSQL to `AVAILABLE`. CI uploads the same file.

With no evidence file, every connector is `IMPLEMENTING`: visible, unselectable,
and `Create` refuses. That is the default on a development machine and it is the
honest one.

## Verification

```
gofmt PASS · vet PASS · tidy PASS · go test PASS · go test -race PASS
21/21 conformance checks passed against real PostgreSQL 16.13, 0 skipped
evidence round trip verified: a real run promotes the connector to AVAILABLE
migrations 001-010 apply and are idempotent through the real command path
frontend: tsc --noEmit PASS · npm run build PASS
Container builds NOT RUN: no Docker daemon in this environment
```

## What is not done

1. **Only the password auth mode is verified.** The PostgreSQL descriptor also
   offers client certificates, and the conformance fixture runs one mode.
   Client-certificate authentication is NOT VERIFIED.
2. **TLS verification is demonstrated negatively against a non-TLS server.**
   The local fixture's server has no TLS, so the untrusted-certificate check
   passes by the connection being refused rather than by a certificate being
   rejected. A self-signed endpoint would be a stronger demonstration.
3. **No query templates ship.** `RegisterTemplate` and the whole approval model
   exist; the set a deployment would actually run is empty, and there is no
   route to define one. `ExecuteTemplate` is therefore reachable only from the
   conformance suite.
4. ~~**`maxPerMinute` is stored and not enforced.**~~ **Closed.** Enforced as a
   fixed window, with refusals audited. What remains: the limit is per process,
   so two gateway replicas each permit the configured rate.
5. ~~**No audit events for connection lifecycle.**~~ **Closed.** Create, test,
   rotate and delete are written to the evidence chain. What remains: an audit
   failure is counted and logged and does not fail the operation, so a
   sufficiently long ledger outage loses events that are only recoverable from
   the server log.
6. **Eight connectors have no driver.** That is the intended state of a staged
   rollout, stated so the catalog's size is not mistaken for coverage.
7. **`DiscoverResources` has no cursor.** Bounded by `MaxRows`; a tenant with
   more tables than the bound gets a truncated list with no way to page past it.
8. ~~**No UI for saved connections.**~~ **Closed.** `SavedConnectionsPanel`
   lists them with health, last-used, rate limit and a replace-credential
   action. **Closed further by Prompt 12:** the wizard now POSTs. It separates
   secret fields from the rest by the descriptor's own `sensitive` flag rather
   than by a list kept in the browser -- a second copy of "which fields are
   secret" would drift, and the direction it drifts in is a credential written
   to a column that read paths return -- and it clears every collected value
   from local state on success, so a password does not outlive its use in a
   React state variable. A refusal from the conformance gate renders as the
   gate's own sentence, which is how an operator learns the connector is
   IMPLEMENTING rather than that the product is broken.
9. **The 90-day evidence expiry is a judgement, not a measurement.** It is
   stated as one in the code.
