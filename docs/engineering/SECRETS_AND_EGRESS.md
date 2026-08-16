# Secret management and network egress

**Established by:** Prompt 05
**Authority:** `gateway/internal/secrets/` and `gateway/internal/egress/` are the
implementation; this document describes them. Where they disagree, the code is
correct and this file is a defect.

## What this replaces

The Prompt 00 baseline reproduced four separate credential defects in one
subsystem, and one network defect:

| Defect | Where |
|---|---|
| Signing secrets generated from `time.Now().UnixNano()` | removed `webhook.go` |
| The create response returned the secret | removed `POST /webhooks` |
| The list endpoint returned every stored secret | removed `GET /webhooks` |
| A SQL console endpoint could select the secret column | removed `POST /sql/query` |
| `POST /webhooks/test` issued a request to `169.254.169.254` | removed `webhook.go` |

The subsystem is gone — its source is archived verbatim in
`REMOVED_CODE_ARCHIVE.md` — but deleting the code does not remove the class of
defect. Each of those five is what happens when a credential is an ordinary
`string` and a destination is an ordinary URL. Prompt 05 makes both into types
that carry their own restrictions.

## The credential type

`secrets.Value` holds a credential and redacts itself on every output path:
`fmt` under any verb, `encoding/json`, `log`, `log/slog`, and error wrapping.
Reading it requires calling `Expose()`, so `grep -rn '\.Expose()'` is a complete
inventory of every disclosure in the codebase.

Two details are load-bearing and easy to get wrong:

**`UnmarshalJSON` refuses.** A `Value` cannot be populated from a request body.
Without this, an endpoint could accept a caller-supplied credential and echo it
back through a struct that is otherwise safe to marshal.

**The field holds ciphertext, not the credential.** `fmt` handles `%T` and `%p`
*before* consulting `fmt.Formatter`, and its bad-verb path then dumps the
argument's fields by reflection — unexported ones included. An earlier draft
held the credential in a plain `string` field, and `fmt.Sprintf("%p", v)`
printed it in full; `TestValueSurvivesReflectiveInspection` is the regression.
The field is now AES-256-GCM ciphertext under a key generated once per process
and never stored in any `Value`, so a reflective walk finds ciphertext.

What that does **not** do, stated plainly: it is no defence against a core dump,
a debugger, `/proc/pid/mem`, or a Go heap profile. The key is in the same
address space. It defends against accidental disclosure through the language's
own output machinery, nothing more.

## The store

`secrets.Store` has two adapters, both checked by one shared conformance suite
(`store_conformance_test.go`). A contract verified against a single
implementation is a description of that implementation, not a contract.

| Adapter | Use | Durability |
|---|---|---|
| `MemoryStore` | local development | none; dies with the process |
| `SQLStore` | production | `secret_versions` / `secret_events`, migration 003 |

### Write-once display

`Create` and `Rotate` return a `Value`. **No other method can.** `Get` and
`List` return a `Reference` — identifier, tenant, name, kind, version,
fingerprint, timestamps — and nothing from which a credential could be derived.

### Two kinds, and why the distinction matters

| Kind | Stored as | Recoverable |
|---|---|---|
| `KindVerify` | salted SHA-256 digest | never, by anyone, including an operator |
| `KindRetrieve` | AES-256-GCM ciphertext under the `Sealer` key | only inside `Use` |

Choose `KindVerify` wherever possible; `KindRetrieve` is the exception that has
to be justified per secret. An inbound credential this system only ever *checks*
does not need to be recoverable, so it is not stored in a recoverable form.

Salted SHA-256 rather than Argon2id is deliberate and depends on a stated
precondition: these are 256-bit values from `crypto/rand`, not human-chosen
passwords, so there is no dictionary to slow down and a KDF would add latency to
every inbound request for nothing. `MinSecretLength` (32) is what makes that
reasoning hold. **If that minimum were ever relaxed to admit operator-chosen
values, the digest would need to become Argon2id.**

### The sealer

`Sealer` encrypts retrievable credentials before they reach storage. The
requirement it enforces is that the key does not live where the ciphertext
lives: a backup, a replica, or a compromised read-only reporting user yields
ciphertext alone.

`SENTINEL_SECRET_SEAL_KEY` carries the key, base64 for 32 bytes. The production
profile refuses to start without it, and `RequireDurableSealer` refuses a
process-scoped key — a deployment that forgot to set one would otherwise work
perfectly until its first restart, at which point every stored credential would
be unrecoverable ciphertext.

### Rotation with overlap

`Rotate` appends a new version and leaves the previous one verifiable for the
overlap window. Without an overlap, the instant a new credential is minted every
holder of the old one is rejected — which in practice means rotations get
postponed indefinitely. `Retire` cuts a window short, which is the response to a
suspected compromise, and is idempotent so it is safe to run again.

`secret_events` records create, rotate, retire, use, verify and reject, each
with an actor and a fingerprint and never a value. The table is append-only by
trigger, and credential material is immutable by trigger: rotation appends a
version, it never rewrites one.

### Verification failures are indistinguishable

Unknown name, unknown tenant, wrong value, retired version and expired overlap
all return `ErrVerificationFailed` with byte-identical text. A caller that could
tell them apart would have an oracle for enumerating which credentials exist.

### Authorization

`secret:manage` is held by `tenant_admin` only. `viewer`, `operator` and
`reviewer` cannot administer credentials — a reviewer can approve the movement
of money and still must not be able to rotate the key that authenticates the
system. **No role can read a stored credential**, because no method returns one.

## Redaction

The type is the first line; redaction is the second, for material that is not a
`Value` by the time anything sees it — an `Authorization` header copied into an
error, a connection string in a startup message, a provider's error body echoed
into ours.

`secrets.Redact` scrubs `Authorization` headers, URL userinfo, sensitive query
and form parameters, compact JWTs, and PEM private key blocks. `Scrubber`
additionally removes the specific credentials this process holds, in any shape,
replacing each with its fingerprint so a redacted log line still says *which*
credential was involved. `NewLogWriter` wraps the standard logger's output at
startup, which is what makes redaction automatic for log calls written later by
someone who has not read the package.

Treating redaction as the primary control would be a mistake, and this is worth
stating because it is a common one: a scrubber only removes what it recognises,
and the credential it does not recognise is the one that leaks.

## Egress

Webhook delivery is not being reintroduced. The guard exists because this
process still fetches two operator-configured URLs — the identity provider's
JWKS document and the AI tier — and because a control built after the first
caller-supplied URL arrives is a control built too late.

`egress.Policy` denies by default; the zero policy allows nothing. It enforces
an exact-hostname allowlist (no wildcards — `*.example.com` invites a subdomain
takeover to become an SSRF), an HTTPS requirement, a timeout, a response size
limit, a redirect limit, and a per-host rate limit.

**The mechanism that matters:** the guard resolves a hostname, validates every
address it resolves to, and then dials a *validated address*. It does not
validate a hostname and hand the hostname to the dialer. That distinction is the
whole of DNS rebinding — a name that resolves to a public address during a check
and to `127.0.0.1` a moment later when the connection is made.
`TestTheDialerConnectsToTheValidatedAddress` asserts the resolver is called
exactly once per request; every additional resolution would be a rebinding
window.

Blocked ranges, each with a test: loopback (all of `127.0.0.0/8`), unspecified,
link-local — which contains `169.254.169.254`, the address the baseline actually
reached — private, carrier-grade NAT, multicast, broadcast, reserved,
documentation and benchmarking ranges, IPv6 loopback, link-local and
unique-local, and NAT64. IPv4-mapped IPv6 is unmapped before checking, because
`::ffff:127.0.0.1` is loopback written as an IPv6 address and a check that skips
the unmapping treats it as an ordinary global address.

A redirect is re-validated as a new destination, which stops the classic bypass:
an allowlisted host answering `302` to the metadata service.

`AllowPrivateAddresses` disables the range checks for the local-demo profile and
for tests using a loopback server. It is a named field rather than an inferred
condition, so enabling it is visible in configuration and in a diff.

## Configuration

| Variable | Profile | Notes |
|---|---|---|
| `SENTINEL_API_TOKEN` | production, required | ≥32 characters; a `secrets.Value` |
| `SENTINEL_METRICS_TOKEN` | production, required | ≥32 characters; a `secrets.Value` |
| `SENTINEL_SECRET_SEAL_KEY` | production, required | base64 for exactly 32 bytes; must be durable |

In `local-demo` the seal key is generated per process, which matches a store
whose contents also die with the process. A configured key is honoured there
too, so a developer can exercise the production storage path.

## The hygiene scan

`gateway/secret_hygiene_test.go` walks the repository and fails on literal
credential material in source or configuration. It is a test rather than a
CI-only tool because it catches what `gitleaks` is weakest at: a credential that
looks like ordinary text because a developer chose it. `password`, `minioadmin`
and `changeme` were all present in this repository's compose files and none has
the entropy a scanner looks for.

Two tests keep the scan honest: one asserts it detects each shape it claims to,
and one asserts it does not fire on the patterns this codebase uses correctly.
A hygiene test that passes because its patterns match nothing reports a clean
result it did not earn.

Suppression is an inline `secret-scan-allow: <reason>` annotation with a
mandatory written reason, not a path exclusion. A directory added to a skip list
stays skipped forever and nobody revisits it. The four current suppressions are
all the same class — security controls that necessarily *name* credential
shapes: a redaction pattern, a rejection list, an eval detector — plus the
ephemeral CI service container, whose password cannot be generated because a
`services:` block is evaluated before any step runs.

`docs/` is excluded, and the reason is stated in the code: it holds the verbatim
archives of code removed in Prompt 01, including the webhook subsystem that
generated secrets from the clock. Those archives are the record of the defect
and necessarily contain its source. They are not built, imported or deployed.

## What is not done

1. **No HTTP surface for secret administration.** The store is complete and
   tested; no route creates, rotates or lists a credential. There is no
   `SecretStore` instance in the running request path — `cfg.Sealer` is built at
   startup and the store is constructible from it, but nothing constructs one
   yet.
2. **`SENTINEL_API_TOKEN` authenticates nothing.** It is required in production
   and is now a redacting `Value`, but the shared-secret middleware it belonged
   to was removed in Prompt 04 when OIDC replaced it. It is a required setting
   with no consumer. Either it acquires one or it should be dropped.
3. **The AI tier client does not use the egress guard.** Only the JWKS fetch is
   wired. The AI tier URL is equally operator-configured.
4. **The egress guard does not use a proxy.** The guide lists an egress
   proxy/policy; this is host-level filtering inside the process, which a
   compromised process could bypass by not using the guard. A network-level
   egress policy is deployment work, not application work, and is not done.
5. **Rotation is not scheduled.** `Rotate` exists and is tested; nothing calls
   it on a schedule, and there is no age-based warning that a credential is
   overdue.
6. **PostgreSQL is still not in the request path.** The secret tables and their
   RLS policies are verified against a real PostgreSQL 16 server, as the other
   tables are, but the running application uses SQLite. This is the same
   boundary recorded in `ROUTE_PERMISSION_MATRIX.md`, now extended to credential
   storage.
