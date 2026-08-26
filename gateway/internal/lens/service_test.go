package lens

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func lensTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:lens-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE lens_return_events (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, occurred_at TIMESTAMP NOT NULL, partner_id TEXT NOT NULL, return_code TEXT NOT NULL, amount_cents INTEGER NOT NULL, source_type TEXT NOT NULL, verified INTEGER NOT NULL, incident_id INTEGER)`,
		`CREATE TABLE incidents (id INTEGER PRIMARY KEY, tenant_id TEXT NOT NULL, type TEXT NOT NULL, severity TEXT NOT NULL, status TEXT NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE validation_findings (id INTEGER PRIMARY KEY, tenant_id TEXT NOT NULL, code TEXT NOT NULL, severity TEXT NOT NULL, provenance TEXT NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE file_instances (id INTEGER PRIMARY KEY, tenant_id TEXT NOT NULL, status TEXT NOT NULL, size_bytes INTEGER NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE agent_runs (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, agent_name TEXT NOT NULL, status TEXT NOT NULL, model_name TEXT, latency_ms INTEGER NOT NULL, input_tokens INTEGER NOT NULL, output_tokens INTEGER NOT NULL, started_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE lens_investigations (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, title TEXT NOT NULL, created_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE lens_investigation_nodes (id TEXT PRIMARY KEY, investigation_id TEXT NOT NULL, tenant_id TEXT NOT NULL, parent_node_id TEXT, question TEXT NOT NULL, query_intent_json TEXT NOT NULL, query_hash TEXT NOT NULL, result_hash TEXT NOT NULL, chart_spec_json TEXT NOT NULL, evidence_refs_json TEXT NOT NULL, created_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestExecuteRejectsUnknownFieldsAndCrossTenantData(t *testing.T) {
	db := lensTestDB(t)
	now := time.Now().UTC()
	_, _ = db.Exec(`INSERT INTO lens_return_events VALUES ('a','TENANT-A',?,?,?,?,?,?,NULL)`, now.Add(-time.Hour), "PARTNER-A", "R11", 250000, "SYNTHETIC_DEMO", 0)
	_, _ = db.Exec(`INSERT INTO lens_return_events VALUES ('b','TENANT-B',?,?,?,?,?,?,NULL)`, now.Add(-time.Hour), "PARTNER-B", "R10", 999999, "CURATED_IMPORT", 1)
	svc := NewService(db)
	intent := QueryIntent{SchemaVersion: "1.0", DatasetID: "ach_return_intelligence", TimeRange: TimeRange{Start: now.Add(-24 * time.Hour), End: now}, Metrics: []string{"return_count"}, Dimensions: []string{"partner_id"}}
	res, err := svc.Execute(context.Background(), "TENANT-A", intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0]["partner_id"] != "PARTNER-A" {
		t.Fatalf("cross-tenant row leaked: %#v", res.Rows)
	}
	intent.Dimensions = []string{"raw_sql"}
	if _, err := svc.Execute(context.Background(), "TENANT-A", intent); err == nil {
		t.Fatal("unknown/raw field must be rejected")
	}
}

func TestSyntheticRowsCannotMintEvidence(t *testing.T) {
	db := lensTestDB(t)
	now := time.Now().UTC()
	_, _ = db.Exec(`INSERT INTO incidents (id,tenant_id,type,severity,status,created_at) VALUES (7,'TENANT-A','RETURN','HIGH','OPEN',?)`, now)
	_, _ = db.Exec(`INSERT INTO lens_return_events VALUES ('a','TENANT-A',?,?,?,?,?,?,?)`, now.Add(-time.Hour), "PARTNER-A", "R11", 250000, "SYNTHETIC_DEMO", 0, 7)
	svc := NewService(db)
	res, err := svc.Execute(context.Background(), "TENANT-A", QueryIntent{SchemaVersion: "1.0", DatasetID: "ach_return_intelligence", TimeRange: TimeRange{Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour)}, Metrics: []string{"return_count"}, Dimensions: []string{"day"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Provenance.EvidenceRefs) != 0 {
		t.Fatalf("synthetic row minted evidence: %#v", res.Provenance.EvidenceRefs)
	}
	if res.Provenance.SourceClass != "SYNTHETIC_DEMO" {
		t.Fatalf("unexpected source class %s", res.Provenance.SourceClass)
	}
}

func TestCuratedVerifiedIncidentCanBeReferenced(t *testing.T) {
	db := lensTestDB(t)
	now := time.Now().UTC()
	_, _ = db.Exec(`INSERT INTO incidents (id,tenant_id,type,severity,status,created_at) VALUES (9,'TENANT-A','RETURN','HIGH','OPEN',?)`, now)
	_, _ = db.Exec(`INSERT INTO lens_return_events VALUES ('a','TENANT-A',?,?,?,?,?,?,?)`, now.Add(-time.Hour), "PARTNER-A", "R11", 250000, "CURATED_IMPORT", 1, 9)
	svc := NewService(db)
	res, err := svc.Execute(context.Background(), "TENANT-A", QueryIntent{SchemaVersion: "1.0", DatasetID: "ach_return_intelligence", TimeRange: TimeRange{Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour)}, Metrics: []string{"return_count"}, Dimensions: []string{"day"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Provenance.EvidenceRefs) != 1 || res.Provenance.EvidenceRefs[0] != "INCIDENT/9" {
		t.Fatalf("expected authoritative ref, got %#v", res.Provenance.EvidenceRefs)
	}
}

func TestInvestigationNodeReexecutesAndPersistsHashes(t *testing.T) {
	db := lensTestDB(t)
	now := time.Now().UTC()
	_, _ = db.Exec(`INSERT INTO lens_return_events VALUES ('a','TENANT-A',?,?,?,?,?,?,NULL)`, now.Add(-time.Hour), "PARTNER-A", "R11", 250000, "SYNTHETIC_DEMO", 0)
	svc := NewService(db)
	inv, err := svc.CreateInvestigation(context.Background(), "TENANT-A", "Return spike", "operator@test")
	if err != nil {
		t.Fatal(err)
	}
	intent := QueryIntent{SchemaVersion: "1.0", DatasetID: "ach_return_intelligence", TimeRange: TimeRange{Start: now.Add(-24 * time.Hour), End: now}, Metrics: []string{"return_count"}, Dimensions: []string{"partner_id"}}
	node, err := svc.AddNode(context.Background(), "TENANT-A", inv.ID, "", "Which partner?", "operator@test", intent)
	if err != nil {
		t.Fatal(err)
	}
	if node.QueryHash == "" || node.ResultHash == "" {
		t.Fatal("hashes must be persisted")
	}
	loaded, err := svc.GetInvestigation(context.Background(), "TENANT-A", inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Nodes) != 1 || loaded.Nodes[0].ResultHash != node.ResultHash {
		t.Fatalf("unexpected loaded investigation %#v", loaded)
	}
}
