# The operations UI

**Established by:** Prompt 12
**Authority:** `src/api/`, `src/state/`, `src/components/ops/` and the gateway's
`ops.go`, `paging.go`, `stream.go` are the implementation; this document
describes them. Where they disagree, the code is correct and this file is a
defect.

## What this is

Six screens, each reading from the gateway, each able to say when it cannot.

What it replaces is the point. `App.tsx` held `SYNTHETIC_PARTNERS`,
`SYNTHETIC_CONTRACTS` and `generateInitialOccurrences()` as component state, ran
a hash chain in the browser, and — when AI triage failed — called a local
matcher and presented its output as an analysis. Every one of those screens
rendered perfectly with the gateway switched off.

**A component that can fall back to local state will, on the day it matters.**
So these are deletions, not refactors: 3,211 lines across twelve files, archived
verbatim in `REMOVED_CODE_ARCHIVE_UI.md`.

## One transport

`src/api/client.ts` is the only place this application calls the gateway.
Credentials, the tenant selector, the CSRF token, the timeout and the
classification of failure are attached in one function.

The old client had four failure conventions in one module: a result object for
two endpoints, `null` for two more, and a thrown `Error` for the rest. The
`null` ones are why an outage rendered as a healthy empty screen.

There is now one convention and **no branch on which a failed request yields
data**. A screen cannot render a failure as content, because there is nothing to
render — that is a type error, not a review comment.

| State | HTTP | Why it is its own state |
|---|---|---|
| `ok` | 2xx | |
| `unauthenticated` | 401 | The session is gone. Re-authenticating fixes it. |
| `forbidden` | 403 | A permission the caller does not hold. Re-authenticating changes nothing. |
| `notFound` | 404 | |
| `conflict` | 409 | Somebody changed it between the read and the write. Re-read; never retry. |
| `invalid` | 400/422 | This client sent something the server will never accept. |
| `unavailable` | 5xx, network, timeout | Retrying may work. |

Collapsing 401 and 403 into one "error" sends an operator to a login screen for
a permission they will never obtain.

Two failure modes that used to be indistinguishable from working: a server that
accepts the connection and never answers now times out rather than leaving the
screen in a loading state that reads as "still working"; and a non-JSON 200 —
which is what a proxy error page is — is `unavailable` rather than a
component-level exception.

## Paging is keyset, and the ceiling is enforced

Rows arrive at the head of every one of these lists while an operator reads
them. `OFFSET`'s page two repeats rows page one showed and skips whatever
crossed the boundary. On an evidence timeline that is a reader shown a
complete-looking list with a hole in it.

The cursor is therefore a **position in the ordering**, not a count of rows
skipped. Where the ordering key is not unique — the SLA board is ordered by when
a file is due, and two feeds can share a deadline — the cursor carries the row
id as a tiebreak and the comparison is lexicographic over the pair. Ordering by
a non-unique key alone is the most common way keyset paging is got wrong.

**"A large paginated dataset does not load all rows into browser memory" is not
a property a UI can have.** It holds whatever it is sent. It is a property of the
server: `limit=100000` returns 200 rows, and there is deliberately no `fetchAll`
helper in the client — one would exist to be called, and the first caller in a
hurry would use it on the evidence timeline.

An unknown filter value is a 400, not a silently unfiltered list. Answering
"show me the quarantined files" with everything reads as "nothing is
quarantined" to the person least able to check.

A malformed cursor is an error rather than a silent reset to the first page:
restarting shows an operator the head of the list again while they believe they
are advancing through it, which is the same defect as a skipped row.

## The event stream

`stream.go` reads the **transactional outbox**. What it replaces was an
in-process map of channels with no tenant scoping, no cursor, no persistence
and — decisively — no publisher: `Broadcast` had no caller, so the endpoint's
entire output was a hardcoded `CONNECTED` frame. Had anything ever published to
it, every subscriber would have received every tenant's events.

The outbox was already the right store: written in the same transaction as the
state it describes, immutable by trigger, and carrying a monotonic id that *is* a
last-event cursor. Replay is `WHERE tenant_id = ? AND id > ?`. This endpoint
marks nothing delivered and competes with no dispatcher; two readers of an
append-only log do not interfere.

**There is no de-duplication set on either side.** Duplicate suppression is the
log's ordering and a strictly-greater-than cursor. A seen-ids cache would be a
second source of truth about what arrived, and the two would drift.

A cursor older than the retained window produces a `gap` event. Silently
restarting at the head would hand the client a stream with a hole it had no way
to detect, and a UI built on that would show a state it never received the
transitions for.

The client does **not** use `EventSource`, which would have handled the
reconnect and the `Last-Event-ID` header by itself. `EventSource` cannot send
credentials or the tenant selector cross-origin and cannot read a non-2xx body,
so an authorization failure would present as an endless reconnect loop rather
than "you are not permitted to watch this". Fetch plus a manual reader gives up
the built-in reconnect and gains the ability to say what is wrong.

Frames are buffered across chunk boundaries. A boundary lands mid-frame often
enough that treating a partial frame as complete corrupts one event in every
long run.

## States every screen renders

`states.tsx` holds them, because the alternative is each screen inventing its
own and the one that gets skipped is always `unavailable`. `ResultState`
switches over the whole `ApiResult` union with no `default`, so adding a state
is a compile error rather than a silently unhandled case.

- **Unavailable** says *"It is not empty — it is unknown."*
- **Empty** is only ever rendered after a successful read.
- **Partial** renders the rows that loaded above a banner saying the page is
  incomplete. A list that quietly omitted a quarantined artifact would be worse
  than one that admits it.
- **Forbidden** says an administrator has to grant it, so nobody signs out and
  back in hoping.

## Timestamps

Rendered in the **source zone** by default, with UTC beside them and both in the
`title`. A partner disputing a breach checks the deadline against their own
agreement, and that agreement is written in their zone; a board showing only UTC
forces every such conversation through a mental conversion, which is where the
mistakes are. UTC never disappears, because the source zone is ambiguous exactly
twice a year and the evidence record is in UTC.

Nothing recomputes an instant from the browser's clock or zone. The clock is
settable by the person reading the screen; the zone is wherever they happen to
be. The header's ticking `new Date()` clock is gone for that reason.

## Consequential decisions

The confirmation restates the consequence in the system's own terms —
*"Releases artifact 41 with 0 of 2 approvals"* — because "Are you sure?" is
answered yes by reflex.

Accessibility here is not decoration. A release decided with a keyboard and a
screen reader has to be as safe as one decided with a mouse, and the ways that
fails are specific: focus must move into the dialog or the reader announces
nothing; it must be trapped or Tab walks into the queue underneath and a
reviewer confirms something they cannot see; it must return to the opener or a
keyboard user is dropped at the top of the document after every action; Escape
must cancel, because that is the reflex for "I did not mean this". Confirm
starts disabled when a reason is required, so the primary action cannot be
reached by pressing Enter on an empty form.

A control the caller may not use is **disabled with the reason** in both `title`
and `aria-describedby` — never hidden, never offered. Hiding it makes the
product look like it lacks the feature; offering it produces a refusal that
reads as a bug.

A 409 is shown as a conflict, the queue is re-read, and the previous action is
explicitly reported as **not applied**.

Permissions come from `GET /session`, which answers with the same `Authorize`
call the handlers use. No component reads `roles.includes('reviewer')` — that
would be a second copy of the role-to-permission mapping, and the two would
eventually disagree about something that decides whether money moves. It is a
presentation hint: every route re-authorizes.

## Demo mode

The server says whether this is a demo build; the banner is not dismissible. A
demo build that could be made to look like production by closing a banner is one
that will eventually be mistaken for production.

The banner is precise about what is and is not simulated: every value on the
screens is read from the gateway and nothing is fabricated, *and* it is the demo
tenant with a named demo principal on loopback.

## Defects found by running it

Four, none of which a build or a type check would have caught. Recorded because
each is a class of mistake, not an incident.

1. **Tailwind was never installed.** Every utility class in the console was
   inert. The build was green and the screens rendered unstyled.

2. **With Tailwind installed, padding and margin utilities still did nothing.**
   Not source order — `@import` is hoisted, so moving the import does nothing.
   Tailwind v4 emits utilities inside `@layer utilities`, and unlayered CSS beats
   layered CSS at any position, so the legacy `* { padding: 0 }` outranked `px-6`
   wherever either sat. `display: flex` and the background colours worked, which
   is exactly what made it look correctly configured. Found by reading the
   computed style in a real browser; reading the emitted CSS would not have shown
   it either, because all the rules were present.

3. **`Access-Control-Allow-Credentials` was never sent.** The client must use
   `credentials: 'include'` for the session cookie, so *every* request from the
   real UI failed at CORS — while the gateway logged them arriving and being
   answered normally. The least diagnosable shape this bug could take.

4. **`GET /contracts` and `/partners` returned the literal `null`** for an empty
   list. A Go nil slice marshals to `null`, the browser read `.length` on it, and
   React unmounted the whole tree: a tenant with no contracts got a blank page
   instead of "no contracts are configured" — on the first day of every
   deployment. Fixed at the server with a regression test; the client normalises
   a null list to empty anyway, because a wrong list beats a crashed console.

Three further defects were fixed on the way, all in code Prompt 12 had to touch:

5. **CSRF compared the header against the HttpOnly session cookie.** No browser
   could produce a matching value, so every cookie-authenticated mutation would
   have been refused. Invisible because nothing sets a session cookie yet — the
   PKCE login flow is implemented and wired to no route — so it would have
   surfaced on the day it was wired, as "the UI cannot approve anything".

6. **The API had two names for the same field.** Review and connection handlers
   sent `message`; everything else sent `detail`. The client read `detail`, so it
   showed the stable error code for every review conflict and dropped the
   sentence saying what changed. Unified on `detail`; `ingest`'s structured
   conflict facts moved to `conflict` so the two no longer share a key.

7. **`reviewStoreFor` cached one store process-wide behind a `sync.Once`.** It
   bound to whichever database called first and served that one to every later
   caller regardless of what they passed. The gateway opens one database, so it
   never misbehaved in production; it misbehaved immediately once a second test
   database existed, as `sql: database is closed` reported to the client as a
   400 `decision_rejected` — a refusal naming the decision as the problem when
   the decision was fine. Now keyed by the `*sql.DB`. A cache that ignores its
   key is not a cache.

## Verification

```
gofmt PASS · vet PASS · go test ./... PASS
tsc PASS · npm run build PASS · vitest 14 PASS

New backend tests: cursor walking without repeats or omissions, forged cursors
refused, unknown filter values refused, session permissions, unmeasured
dependencies never reporting healthy, cross-tenant artifact reads, empty lists
are [] and not null, SSE replay from a cursor without duplicates, SSE tenant
scoping, SSE refusing an unauthenticated caller, and a stale approval refused
with a 409 naming the findings.

New client tests: outage is unavailable and never empty-ok, a silent server
times out, expired session and permission denial are distinct, a stale decision
is a conflict carrying what changed, credentials and tenant on every request
with CSRF only on mutations, no actor ever sent, reconnect resumes from the last
delivered id with no duplicates, a split frame reassembles, a 403 stops the
retry loop, and a cursor gap is surfaced.

Rendered in Chromium against a running gateway: all six screens, the stream
connecting, health reporting DEGRADED and NOT_CONFIGURED honestly, and — with
the gateway stopped — "Gateway unreachable" and "Unavailable" rather than a
healthy empty screen.

Container builds NOT RUN: no Docker daemon in this environment.
```

## What is not done

1. **No login.** The PKCE flow exists in `internal/auth/pkce.go` and is wired to
   no route, so outside the demo profile the UI has no way to obtain a session.
   The CSRF fix above means it will work when it is wired; nothing proves that
   until it is.
2. **The review queue and the connector wizard are the only screens that
   write.** Contracts are read-only here even for a holder of
   `contract:manage`, and the release policy has a route and no form.
3. **`RoleReleaseSupervisor` is still issued by nothing.** The override control
   renders disabled with its reason for every account that can exist today.
4. **SSE reloads the whole active screen.** A screen reacts to any event by
   re-reading itself rather than patching rows from a payload. That is correct
   and coarse: patching would derive the screen from two sources that must
   agree.
5. **No virtualised list.** Paging bounds what is fetched; a page of 200 rows is
   still 200 DOM nodes. That is fine at these sizes and would not be at ten
   thousand.
6. **The upload modal still posts and then closes.** It no longer assembles a
   fabricated `FileInstance` and it now branches on the result instead of
   `alert(err.message)`, but it does not navigate to the artifact it created.
7. **The application still runs on SQLite.** Eighth subsystem verified against a
   real PostgreSQL server and not deployed on one.
