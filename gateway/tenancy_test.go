package main

import (
	"database/sql"
	"strings"
	"testing"
)

// These tests pin what migration 002 buys. A tenant column is only worth
// something if the database refuses records without one and refuses states
// outside the modelled set.
//
// They do NOT claim tenant isolation is implemented: the request path has no
// identity to derive a tenant from until Prompt 04, so every write currently
// lands in DefaultTenantID. What is asserted here is that the storage layer can
// no longer hold an untenanted or impossible row.

func TestEveryBusinessTableRequiresATenant(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	cases := []struct {
		table  string
		insert string
	}{
		{"partners", `INSERT INTO partners (name, routing_number) VALUES ('X','021000021')`},
		{"file_contracts", `INSERT INTO file_contracts (partner_id, name, direction, filename_pattern, expected_time, grace_period_minutes, timezone)
			VALUES (1,'c','INBOUND','^x$','12:00:00',5,'UTC')`},
		{"expectations", `INSERT INTO expectations (contract_id, expected_delivery_start, expected_delivery_end, status)
			VALUES (1,'2026-01-01','2026-01-01','PENDING')`},
		{"file_instances", `INSERT INTO file_instances (filename, storage_path, size_bytes, sha256_hash, status, received_at)
			VALUES ('f','/p',1,'h','RECEIVED','2026-01-01')`},
		{"validation_findings", `INSERT INTO validation_findings (file_instance_id, code, description, severity)
			VALUES (1,'C','d','ERROR')`},
		{"incidents", `INSERT INTO incidents (type, severity, status) VALUES ('T','HIGH','OPEN')`},
		{"status_history", `INSERT INTO status_history (object_type, object_id, from_state, to_state, actor_id)
			VALUES ('artifact',1,'RECEIVED','VALIDATING','a')`},
		{"ingestion_jobs", `INSERT INTO ingestion_jobs (idempotency_key, state) VALUES ('k','QUEUED')`},
		{"policy_decisions", `INSERT INTO policy_decisions (file_instance_id, validation_run_id, policy_version, state, outcome, artifact_sha256)
			VALUES (1,1,'v1','PROPOSED','VALIDATED','h')`},
	}

	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			_, err := db.Exec(tc.insert)
			if err == nil {
				t.Errorf("%s accepted a row with no tenant_id", tc.table)
				return
			}
			if !strings.Contains(err.Error(), "tenant_id") && !strings.Contains(err.Error(), "NOT NULL") {
				t.Errorf("%s rejected the row, but not for the tenant column: %v", tc.table, err)
			}
		})
	}
}

// A status column that accepts arbitrary text is how an artifact came to hold
// RELEASED without ever being validated.
func TestArtifactStatusIsConstrainedToModelledStates(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	for _, bad := range []string{"SETTLED", "settled", "RELEASED_OK", "", "COMPLIANT", "HEALTHY"} {
		_, err := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
			VALUES ('TENANT-DEFAULT','f','/p',1,'h',?,'2026-01-01')`, bad)
		if err == nil {
			t.Errorf("file_instances accepted the unmodelled status %q", bad)
		}
	}

	// Every modelled state must be accepted.
	//
	// Each row carries a distinct hash: migration 004 added a uniqueness
	// constraint on (tenant_id, sha256_hash, size_bytes), because duplicate
	// delivery of identical content must not create a second artifact. Two
	// artifacts in genuinely different states have genuinely different content.
	for _, good := range []string{"RECEIVED", "VALIDATING", "VALIDATED", "QUARANTINED", "APPROVED", "RELEASED", "REJECTED"} {
		_, err := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
			VALUES ('TENANT-DEFAULT','f','/p',1,?,?,'2026-01-01')`, "hash-"+good, good)
		if err != nil {
			t.Errorf("file_instances rejected the modelled status %q: %v", good, err)
		}
	}

	// And the uniqueness constraint itself holds: the same content cannot be
	// recorded twice for one tenant.
	if _, err := db.Exec(`INSERT INTO file_instances (tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES ('TENANT-DEFAULT','again','/p',1,'hash-RECEIVED','RECEIVED','2026-01-01')`); err == nil {
		t.Error("file_instances accepted a second artifact with identical content for one tenant")
	}
}

// SETTLED is not a state in this product, in any table.
func TestSettlementIsNotAStateAnywhere(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO expectations (tenant_id, contract_id, expected_delivery_start, expected_delivery_end, status)
		VALUES ('TENANT-DEFAULT',1,'2026-01-01','2026-01-01','SETTLED')`); err == nil {
		t.Errorf("expectations accepted SETTLED")
	}
	if _, err := db.Exec(`INSERT INTO ingestion_jobs (tenant_id, idempotency_key, state)
		VALUES ('TENANT-DEFAULT','k','SETTLED')`); err == nil {
		t.Errorf("ingestion_jobs accepted SETTLED")
	}
}

// Duplicate delivery is a normal condition. A second arrival must collide on
// the idempotency key rather than create a second unit of work.
func TestDuplicateIdempotencyKeyIsRejectedPerTenant(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO ingestion_jobs (tenant_id, idempotency_key, state)
		VALUES ('TENANT-DEFAULT','key-1','QUEUED')`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ingestion_jobs (tenant_id, idempotency_key, state)
		VALUES ('TENANT-DEFAULT','key-1','QUEUED')`); err == nil {
		t.Errorf("a duplicate idempotency key created a second job")
	}

	// The same key in a different tenant is a different job and must be allowed.
	if _, err := db.Exec(`INSERT INTO tenants (id, name) VALUES ('TENANT-OTHER','Other')`); err != nil {
		t.Fatalf("create second tenant: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ingestion_jobs (tenant_id, idempotency_key, state)
		VALUES ('TENANT-OTHER','key-1','QUEUED')`); err != nil {
		t.Errorf("the same key in another tenant must be permitted: %v", err)
	}
}

// Dual control must not be satisfiable by one person approving twice. This is
// enforced in storage as well as in the domain, so a handler bypassing the
// domain still cannot record it.
func TestSamePersonCannotApproveTwice(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	mustExec(t, db, `INSERT INTO file_instances (id, tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (1,'TENANT-DEFAULT','f','/p',10,'h','VALIDATED','2026-01-01')`)
	mustExec(t, db, `INSERT INTO validation_runs (id, tenant_id, file_instance_id, parser_name, parser_version, rule_pack_version, parser_ok, records_parsed, started_at, completed_at)
		VALUES (1,'TENANT-DEFAULT',1,'moov','1','1',1,10,'2026-01-01','2026-01-01')`)
	mustExec(t, db, `INSERT INTO policy_decisions (id, tenant_id, file_instance_id, validation_run_id, policy_version, state, outcome, artifact_sha256)
		VALUES (1,'TENANT-DEFAULT',1,1,'v1','APPROVED','VALIDATED','h')`)

	mustExec(t, db, `INSERT INTO approvals (tenant_id, decision_id, actor_id, role, reason)
		VALUES ('TENANT-DEFAULT',1,'alice','reviewer','looks correct')`)

	if _, err := db.Exec(`INSERT INTO approvals (tenant_id, decision_id, actor_id, role, reason)
		VALUES ('TENANT-DEFAULT',1,'alice','reviewer','looks correct again')`); err == nil {
		t.Errorf("the same actor recorded two approvals against one decision")
	}

	// A second, distinct person must be able to approve.
	if _, err := db.Exec(`INSERT INTO approvals (tenant_id, decision_id, actor_id, role, reason)
		VALUES ('TENANT-DEFAULT',1,'bob','reviewer','independently checked')`); err != nil {
		t.Errorf("a distinct second approver must be permitted: %v", err)
	}
}

// An approval must carry an actor and a reason; empty strings are not identity
// or justification.
func TestApprovalRequiresActorAndReason(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	mustExec(t, db, `INSERT INTO file_instances (id, tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (1,'TENANT-DEFAULT','f','/p',10,'h','VALIDATED','2026-01-01')`)
	mustExec(t, db, `INSERT INTO validation_runs (id, tenant_id, file_instance_id, parser_name, parser_version, rule_pack_version, parser_ok, records_parsed, started_at, completed_at)
		VALUES (1,'TENANT-DEFAULT',1,'moov','1','1',1,10,'2026-01-01','2026-01-01')`)
	mustExec(t, db, `INSERT INTO policy_decisions (id, tenant_id, file_instance_id, validation_run_id, policy_version, state, outcome, artifact_sha256)
		VALUES (1,'TENANT-DEFAULT',1,1,'v1','APPROVED','VALIDATED','h')`)

	if _, err := db.Exec(`INSERT INTO approvals (tenant_id, decision_id, actor_id, role, reason)
		VALUES ('TENANT-DEFAULT',1,'','reviewer','a reason')`); err == nil {
		t.Errorf("an approval with an empty actor was accepted")
	}
	if _, err := db.Exec(`INSERT INTO approvals (tenant_id, decision_id, actor_id, role, reason)
		VALUES ('TENANT-DEFAULT',1,'alice','reviewer','')`); err == nil {
		t.Errorf("an approval with an empty reason was accepted")
	}
}

// Only one policy decision may exist per validation run, so two concurrent
// finalizations cannot both produce an answer.
func TestConcurrentDecisionsCannotBothFinalize(t *testing.T) {
	db := setupTestDb(t)
	defer db.Close()

	mustExec(t, db, `INSERT INTO file_instances (id, tenant_id, filename, storage_path, size_bytes, sha256_hash, status, received_at)
		VALUES (1,'TENANT-DEFAULT','f','/p',10,'h','VALIDATING','2026-01-01')`)
	mustExec(t, db, `INSERT INTO validation_runs (id, tenant_id, file_instance_id, parser_name, parser_version, rule_pack_version, parser_ok, records_parsed, started_at, completed_at)
		VALUES (1,'TENANT-DEFAULT',1,'moov','1','1',1,10,'2026-01-01','2026-01-01')`)

	mustExec(t, db, `INSERT INTO policy_decisions (tenant_id, file_instance_id, validation_run_id, policy_version, state, outcome, artifact_sha256)
		VALUES ('TENANT-DEFAULT',1,1,'v1','APPROVED','VALIDATED','h')`)

	// A racing worker reaching the opposite conclusion must lose, not coexist.
	if _, err := db.Exec(`INSERT INTO policy_decisions (tenant_id, file_instance_id, validation_run_id, policy_version, state, outcome, artifact_sha256)
		VALUES ('TENANT-DEFAULT',1,1,'v1','APPROVED','QUARANTINED','h')`); err == nil {
		t.Errorf("two policy decisions were recorded for one validation run")
	}
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("setup statement failed: %v\n  %s", err, q)
	}
}
