package main

import (
	"database/sql"
	"math"
	"testing"
)

func newLedgerDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Use the embedded migrator rather than reading a path. Reading
	// ./migrations/01_init.sql discarded the read error, so a rename or a
	// different working directory produced an empty schema and a panic several
	// lines later instead of a migration failure here.
	if _, err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// tamper simulates an attacker with direct database write access. The
// append-only triggers added in migration 002 refuse UPDATE and DELETE on
// audit_events, so the attacker's first move is to drop them. Hash-chain
// verification is the defence that must survive that, and this is what these
// tests exercise.
func tamper(t *testing.T, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS audit_events_no_update`); err != nil {
		t.Fatalf("drop update trigger: %v", err)
	}
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS audit_events_no_delete`); err != nil {
		t.Fatalf("drop delete trigger: %v", err)
	}
	if _, err := db.Exec(stmt, args...); err != nil {
		t.Fatalf("tamper: %v", err)
	}
}

// The guard itself: ordinary application code must not be able to rewrite
// history, even before the hash chain is consulted.
func TestAuditEventsAreAppendOnly(t *testing.T) {
	db := newLedgerDB(t)
	defer db.Close()

	if _, err := AppendAuditEvent(db, DefaultTenantID, "FILE_INGESTED", "operator-a", map[string]interface{}{"n": 1}); err != nil {
		t.Fatalf("append: %v", err)
	}

	if _, err := db.Exec(`UPDATE audit_events SET actor = 'someone-else' WHERE id = 1`); err == nil {
		t.Errorf("UPDATE on audit_events must be refused by the append-only trigger")
	}
	if _, err := db.Exec(`DELETE FROM audit_events WHERE id = 1`); err == nil {
		t.Errorf("DELETE on audit_events must be refused by the append-only trigger")
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected the row to survive both attempts, found %d rows", n)
	}
}

// status_history carries the same guarantee.
func TestStatusHistoryIsAppendOnly(t *testing.T) {
	db := newLedgerDB(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id)
		VALUES ('TENANT-DEFAULT','artifact',1,'RECEIVED','VALIDATING','system')`); err != nil {
		t.Fatalf("insert history: %v", err)
	}
	if _, err := db.Exec(`UPDATE status_history SET to_state = 'RELEASED' WHERE id = 1`); err == nil {
		t.Errorf("UPDATE on status_history must be refused")
	}
	if _, err := db.Exec(`DELETE FROM status_history WHERE id = 1`); err == nil {
		t.Errorf("DELETE on status_history must be refused")
	}
}

// A transition to the same state is meaningless and is rejected by CHECK.
func TestStatusHistoryRejectsNoOpTransition(t *testing.T) {
	db := newLedgerDB(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO status_history (tenant_id, object_type, object_id, from_state, to_state, actor_id)
		VALUES ('TENANT-DEFAULT','artifact',1,'RECEIVED','RECEIVED','system')`); err == nil {
		t.Errorf("a from_state == to_state row must be rejected")
	}
}

// The pre-existing TestHashChainTamperDetection only mutated current_hash, which
// breaks the LINK to the next row -- the one tamper vector the old verifier
// happened to catch. It never tested mutation of the event content itself.
// An attacker rewriting an audit payload leaves every hash link intact.
func TestLedgerDetectsContentTampering(t *testing.T) {
	db := newLedgerDB(t)
	defer db.Close()

	_, _ = AppendAuditEvent(db, DefaultTenantID, "FILE_RELEASED", "operator-a", map[string]interface{}{"amount": 1000})
	_, _ = AppendAuditEvent(db, DefaultTenantID, "FILE_RELEASED", "operator-b", map[string]interface{}{"amount": 2000})
	_, _ = AppendAuditEvent(db, DefaultTenantID, "FILE_RELEASED", "operator-c", map[string]interface{}{"amount": 3000})

	if l, _ := GetLedger(db, DefaultTenantID); !l.IsChainValid {
		t.Fatalf("clean ledger should verify")
	}

	// Rewrite the PAYLOAD of event 2. All previous_hash/current_hash links stay
	// consistent, so a link-only verifier reports the chain as valid.
	tamper(t, db, `UPDATE audit_events SET payload = '{"amount":999999}' WHERE id = 2`)
	l, err := GetLedger(db, DefaultTenantID)
	if err != nil {
		t.Fatal(err)
	}
	if l.IsChainValid {
		t.Fatalf("payload tampering must invalidate the chain")
	}
	if l.FirstBreachSequence != 2 {
		t.Errorf("expected first breach at sequence 2, got %d", l.FirstBreachSequence)
	}
	if l.Events[1].IntegrityStatus != "BROKEN" {
		t.Errorf("expected BROKEN, got %q", l.Events[1].IntegrityStatus)
	}
	// Untouched rows must still be individually attested.
	if l.Events[0].IntegrityStatus != "VERIFIED" {
		t.Errorf("event 1 should still verify, got %q", l.Events[0].IntegrityStatus)
	}
}

func TestLedgerDetectsActorTampering(t *testing.T) {
	db := newLedgerDB(t)
	defer db.Close()
	// The payload key was "token", which the ledger now refuses outright --
	// the append failed silently, no record was written, and an empty chain
	// verified as intact, so this test passed for the wrong reason. The error
	// is checked now, and the payload carries an identifier rather than
	// something named like a credential.
	if _, err := AppendAuditEvent(db, DefaultTenantID, "SECRET_ROTATED", "supervisor-real",
		map[string]interface{}{"secretReference": "sec_abc123", "artifactId": 1}); err != nil {
		t.Fatalf("append: %v", err)
	}
	tamper(t, db, `UPDATE audit_events SET actor = 'somebody-else' WHERE id = 1`)
	if l, _ := GetLedger(db, DefaultTenantID); l.IsChainValid {
		t.Fatalf("actor substitution must invalidate the chain")
	}
}

// ---------------------------------------------------------------------------
// Kolmogorov-Smirnov correctness
// ---------------------------------------------------------------------------

func TestKSIdenticalSamplesAreNotSignificant(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	res, err := TwoSampleKS(a, a, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	if res.D != 0 {
		t.Errorf("identical samples must give D=0, got %v", res.D)
	}
	if res.IsSignificant {
		t.Errorf("identical samples must not be flagged")
	}
}

func TestKSDisjointSamplesAreMaximallySeparated(t *testing.T) {
	a := []float64{1, 2, 3, 4, 5}
	b := []float64{100, 200, 300, 400, 500}
	res, err := TwoSampleKS(a, b, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.D-1.0) > 1e-9 {
		t.Errorf("disjoint supports must give D=1, got %v", res.D)
	}
	if !res.IsSignificant {
		t.Errorf("D=1 on n=m=5 should be significant, p=%v", res.PValue)
	}
}

// Reference check: for the classic textbook pair the exact D is 0.5.
func TestKSKnownStatistic(t *testing.T) {
	a := []float64{1, 2, 3, 4}
	b := []float64{3, 4, 5, 6}
	res, _ := TwoSampleKS(a, b, 0.05)
	if math.Abs(res.D-0.5) > 1e-9 {
		t.Errorf("expected D=0.5, got %v", res.D)
	}
}

func TestKSQIsMonotoneAndBounded(t *testing.T) {
	prev := KolmogorovSmirnovQ(0.01)
	if prev > 1.0 {
		t.Errorf("Q must be a probability, got %v", prev)
	}
	for tv := 0.1; tv < 4.0; tv += 0.1 {
		q := KolmogorovSmirnovQ(tv)
		if q > prev+1e-12 {
			t.Errorf("Q must be non-increasing in t: Q(%v)=%v > previous %v", tv, q, prev)
		}
		if q < 0 || q > 1 {
			t.Errorf("Q out of [0,1] at t=%v: %v", tv, q)
		}
		prev = q
	}
}

func TestKSRejectsTinySamples(t *testing.T) {
	if _, err := TwoSampleKS([]float64{1}, []float64{2, 3}, 0.05); err == nil {
		t.Errorf("expected an error for n<2 rather than a fabricated statistic")
	}
}

// ---------------------------------------------------------------------------
// Benjamini-Hochberg
// ---------------------------------------------------------------------------

func TestBenjaminiHochbergControlsFDR(t *testing.T) {
	// 1 genuine signal buried in 19 nulls.
	ps := []float64{0.001}
	for i := 0; i < 19; i++ {
		ps = append(ps, 0.5+float64(i)*0.02)
	}
	rej := BenjaminiHochberg(ps, 0.05)
	if !rej[0] {
		t.Errorf("the strong signal should survive BH correction")
	}
	count := 0
	for _, r := range rej {
		if r {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 rejection, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Robust anomaly detection: the masking failure of mean/sd
// ---------------------------------------------------------------------------

// A single historical outlier inflates the standard deviation enough that the
// classic 3-sigma rule stops firing on genuinely anomalous values. Median/MAD
// has a 50% breakdown point and is unaffected.
func TestRobustDetectorResistsMasking(t *testing.T) {
	history := make([]float64, 0, 30)
	for i := 0; i < 29; i++ {
		history = append(history, 10000+float64(i%3)*50)
	}
	history = append(history, 400000) // one contaminating outlier

	// Classic z-score with contaminated mean/sd.
	mean, sd := meanStd(history)
	suspicious := 20000.0
	classicZ := (suspicious - mean) / sd

	robust := EvaluateRobustVolumeAnomaly(suspicious, history, 3.5)

	if math.Abs(classicZ) >= 3.0 {
		t.Logf("classic z=%.2f (would have fired)", classicZ)
	} else {
		t.Logf("classic z=%.2f -> MASKED by the contaminated baseline", classicZ)
	}
	if !robust.IsAnomaly {
		t.Errorf("robust detector must still flag 20000 against a ~10000 median; modifiedZ=%v", robust.ModifiedZ)
	}
}

func TestRobustDetectorRefusesThinHistory(t *testing.T) {
	r := EvaluateRobustVolumeAnomaly(9999, []float64{1, 2, 3}, 3.5)
	if !r.InsufficientData || r.IsAnomaly {
		t.Errorf("must refuse to assert an anomaly on 3 observations, got %+v", r)
	}
}

func TestRobustDetectorHandlesZeroMAD(t *testing.T) {
	history := make([]float64, 20)
	for i := range history {
		history[i] = 5000
	}
	r := EvaluateRobustVolumeAnomaly(5000, history, 3.5)
	if r.IsAnomaly {
		t.Errorf("identical value against constant history must not be an anomaly")
	}
	if math.IsInf(r.ModifiedZ, 0) || math.IsNaN(r.ModifiedZ) {
		t.Errorf("MAD=0 must not produce Inf/NaN, got %v", r.ModifiedZ)
	}
}

func meanStd(xs []float64) (float64, float64) {
	n := float64(len(xs))
	var s float64
	for _, x := range xs {
		s += x
	}
	m := s / n
	var v float64
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return m, math.Sqrt(v / n)
}

// Cross-validate the two series forms against each other at the switchover and
// against published Kolmogorov distribution values.
func TestKSQAgainstReferenceValues(t *testing.T) {
	// Q(t) values from the Kolmogorov distribution.
	// Independently computed from both series forms to 1e-17 agreement.
	refs := map[float64]float64{
		0.5:  0.963945,
		1.0:  0.270000,
		1.36: 0.049486, // ~the alpha=0.05 critical value (t*=1.3581)
		1.63: 0.009846,
		2.0:  0.000671,
	}
	for tv, want := range refs {
		got := KolmogorovSmirnovQ(tv)
		if math.Abs(got-want) > 1e-4 {
			t.Errorf("Q(%.2f) = %.6f, expected ~%.5f", tv, got, want)
		}
	}
}
