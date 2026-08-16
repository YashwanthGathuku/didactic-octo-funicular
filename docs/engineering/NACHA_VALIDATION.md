# NACHA validation and the release policy

**Established by:** Prompt 07
**Authority:** `gateway/internal/nacha/` is the implementation; this document
describes it. Where they disagree, the code is correct and this file is a defect.

## The two things that matter most

**It fails closed.** The validator starts from quarantine and only reaches
VALIDATED by falling through every check. The defect this replaces did the
reverse: ingestion initialised every result to `RELEASED` and downgraded on a
positive finding, so a zero-byte file — which produces no findings, because
there is nothing to find — was released as balanced.

**It is honest about what it does not know.** The ACH file *format* is public.
The Nacha *Operating Rules* are licensed and this repository does not have them.
Rules that depend on them are declared, marked `UNVERIFIED_REQUIRES_LICENSED_RULES`,
and **cannot be blocking** — `TestUnverifiedRulesAreNeverBlocking` enforces it.

## Rule provenance

| Provenance | Meaning | May block |
|---|---|---|
| `FILE_FORMAT` | Checkable from the file itself: record layout, declared totals, and the arithmetic relating them | Yes |
| `UNVERIFIED_REQUIRES_LICENSED_RULES` | Requires the Nacha Operating Rules | **No** |

Every blocking rule is `FILE_FORMAT`. That is a deliberately narrow foundation
and a real one: a file whose batch control disagrees with the entries it
contains is wrong under any edition of the rules.

Four rules are declared and not enforced, so the gap appears in the registry
rather than being implied by absence: SEC code validity, addenda limits per SEC
code, effective-date windows, and per-code amount limits. Each carries a
citation naming what would be needed to verify it. A `Result` reports them in
`NotChecked` and a `Decision` carries them forward, so a VALIDATED outcome reads
as "valid as far as this system can tell" with the qualification attached.

The previous implementation set `RuleReference: "Nacha Operating Rules 2025"` on
every finding regardless of what produced it — citing a source the repository
does not have, uniformly, for rules that were mostly arithmetic.

## What is checked

**Structure** — record length exactly 94, record type in {1,5,6,7,8,9},
printable ASCII, file header first, file control present, no batch left open, no
orphan entry or addenda.

**Arithmetic** — per batch and per file: entry/addenda count, entry hash
(sum of the leading eight digits of each RDFI routing number, truncated to ten
digits), total debits, total credits. Batch number continuity between header and
control is a warning.

**Routing** — the ABA check digit, `3(d1+d4+d7) + 7(d2+d5+d8) + 1(d3+d6+d9) mod 10 = 0`.
This is arithmetic on the number itself, which is why it can be blocking here.

**Overflow** — accumulation refuses to wrap. A wrapped total can equal a declared
total and silently balance a file that does not.

## Money

`Amount` is a signed count of minor units. Never a float.

The previous implementation computed totals as `float64(cents) / 100.0` and
compared debit and credit sums for equality. That is a correctness defect, not a
style preference: `0.1 + 0.2 != 0.3` in binary floating point, and a file with
enough entries can report itself unbalanced when it balances. An ACH amount
field is already in minor units, so the conversion was introducing a
representation the format never used.

`Count` is a separate type from `Amount` so a record count and a dollar total
cannot be added or compared by accident — both are integers here, and confusing
them produces a validator that passes.

## Balance is a contract term

There is no `isBalanced` field anywhere any more.

Whether a file must balance is a property of the arrangement, not of the file. A
credit-only payroll file never balances and is entirely correct; an offsetting
file that must balance and does not is a defect. The old code returned
`isBalanced: debits == credits` from every validation and the UI rendered it as
a green "Balanced (Zero-Net)" success badge under the heading "Settlement
State" — asserting both a correctness claim the file cannot support alone and a
settlement concept this product does not have.

`FeedContract.RequireBalanced` now carries it, and
`TestBalanceRequirementComesFromTheContract` proves the same file is validated
under one contract and quarantined under another.

## Findings carry location, not content

A finding holds rule ID, rule version, provenance, severity, record number, byte
offset, field character range, a redacted excerpt, and the two sides of any
arithmetic disagreement.

Redaction happens where the finding is produced, not where it is displayed — a
redaction applied on the way out is one someone can forget to apply. Digits are
masked, control characters are replaced, and the excerpt is bounded at 40
characters, well below the 94-character record width, so no combination of
findings reconstructs a line.

Totals and counts appear in full. A mismatched batch total *is* the finding, and
a total is not a payment instruction.

## Baseline P0-11 is closed

`validation_findings.raw_data` held the complete 94-character record — account
number, routing number, amount, trace number — and `GET /api/v1/incidents`
selected it and returned it. Every response, log line, support export and AI
triage request that touched an incident carried it.

Migration 005 rebuilds the table without the column, carrying existing findings
across without their content and collapsing legacy severities upward
(ERROR/CRITICAL/FATAL → BLOCKING, the fail-closed direction). `TriageRequest` no
longer has a `RawData` field, so nothing raw reaches a model provider.

`TestNoRawRecordContentIsStoredOrReturned` asserts it end to end: it processes a
fixture, then checks that no record from it appears in the JSON response or in
any column of any row of the database.

## The release policy

`Decide(result, contract)` returns a versioned `Decision`. A decision without a
policy version is an opinion, so the version is on every one.

Quarantine when: the parser failed, zero records, zero entries, any blocking
finding, or a contract term is violated. Otherwise VALIDATED — which is not a
release. Release requires approval and dual control, which live in
`internal/domain`.

## The fixture generator

`generator.go` was rewritten because it did not do what it claimed. It built
records as hand-written literals, padded any line shorter than 94 characters,
and **never truncated a longer one**. Five of its six presets emitted records of
95, 96, 102 or 103 characters while the doc comment said "94-character
fixed-width". The preset named `CORRUPTED_ENTRY_HASH` was quarantined for
malformed record widths rather than for the hash mismatch it advertised.

Nothing caught this because the validator it fed did not check record length.

Records are now assembled field by field at exact widths and every control value
is computed from the records, so a valid fixture is valid by construction and
each invalid preset is one stated change to it. Each preset declares
`ExpectedRuleIDs`, and `TestEachPresetProducesExactlyTheFindingsItClaims`
asserts the validator raises those and no unrelated blocking rule.

## Fixtures and acceptance

Fixtures are generated rather than checked in. A hand-written fixture with a
hand-computed entry hash drifts, and the usual fix is to adjust the expected
value until the test passes — which turns the arithmetic check into a check that
the test agrees with itself.

`TestNoInvalidFixtureCanReachRelease` runs 21 invalid fixtures — empty,
whitespace, wrong record length, invalid record type, bad check digit,
non-numeric amount, five kinds of count/hash/total mismatch at batch and file
level, orphan entry, orphan addenda, truncation, malformed characters — and
requires QUARANTINED with a stated reason for every one.

`TestValidationIsDeterministic` runs each fixture six times and requires
byte-identical findings. Evidence that changes between runs is not evidence.

### One fixture that was lying

A fixture named "amount overflow" claimed to drive the minor-unit accumulator
past its range. It did not: 32 entries at the ten-character field maximum sum to
about 3.2e11 and int64 holds 9.2e18. It passed because of an unrelated corrupted
control total — the test asserted one thing and demonstrated another.

It is renamed to what it is (`unreachableDeclaredTotal`), and the overflow path
is now exercised directly in `TestAccumulatorOverflowIsBlocking`, which drives
the accumulator to its boundary. A fixture that claims to overflow and does not
is worse than no fixture.

## What is not done

1. **`POST /files/ingest-raw` still reads the whole body into memory.** It now
   validates through this package and writes redacted findings, but it does not
   stream and writes no object. The safe path is `/files/upload` from Prompt 06.
2. **Feed contracts are not resolved per counterparty.** `DefaultContract`
   applies to every artifact, so `RequireBalanced` and the amount limit are
   available and unused in the request path. Contract resolution is Prompt 10;
   the decision records an empty `ContractID` so "no contract was applied" is
   visible rather than inferred.
3. **No duplicate file/reference detection inside the validator.** Duplicate
   *artifacts* are caught at ingest by content hash (Prompt 06). Detecting a
   re-sent file by its header's origin, creation date and ID modifier — the
   format's own duplicate signal — is not implemented; `Result` retains those
   fields for it.
4. **PGP signature policy is not wired to validation.** `security.go` verifies
   detached signatures and nothing calls it from this path. The separation the
   guide asks for exists structurally — signature verification is not in the
   parser — but the policy that would consume it is not built.
5. **The experimental ISO 20022, BAI2 and SWIFT parsers still use `float64`**
   and their own finding shapes. They remain experimental per `SCOPE.md` and
   were not migrated; their findings now carry redacted evidence but no rule
   version or provenance.
6. **Nothing consumes the validation job queue.** Ingest enqueues, and the
   worker that would run this validator against a stored artifact is Prompt 08.
   Today validation runs synchronously inside `ProcessFileBytes`.
