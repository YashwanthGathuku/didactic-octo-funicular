import csv
import importlib.util
import sqlite3
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GENERATOR = ROOT / "scripts" / "generate_lens_demo_data.py"
MIGRATION = ROOT / "gateway" / "migrations" / "023_lens_lite.sql"


def load_generator():
    spec = importlib.util.spec_from_file_location("lens_demo", GENERATOR)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader
    spec.loader.exec_module(module)
    return module


class LensDemoDataTests(unittest.TestCase):
    def test_fixture_is_deterministic_and_non_authoritative(self):
        mod = load_generator()
        a, b = mod.rows(), mod.rows()
        self.assertEqual(a, b)
        self.assertGreater(len(a), 50)
        self.assertTrue(all(row["source_type"] == "SYNTHETIC_DEMO" for row in a))
        self.assertTrue(all(row["verified"] == "0" for row in a))

    def test_final_window_has_clear_r11_payroll_concentration(self):
        mod = load_generator()
        rows = mod.rows()
        target = [r for r in rows if r["partner_id"] == "TEST-PAYROLL-17" and r["return_code"] == "R11"]
        self.assertGreaterEqual(len(target), 20)
        associated = sum(int(r["amount_cents"]) for r in target)
        self.assertGreater(associated, 300_000_000)  # > $3M associated synthetic observations

    def test_schema_constraint_forbids_synthetic_verified_row(self):
        conn = sqlite3.connect(":memory:")
        conn.executescript("""
        CREATE TABLE tenants (id TEXT PRIMARY KEY);
        INSERT INTO tenants VALUES ('TENANT-DEFAULT');
        CREATE TABLE incidents (id INTEGER PRIMARY KEY, tenant_id TEXT NOT NULL);
        CREATE UNIQUE INDEX idx_incidents_tenant_id_unique ON incidents(tenant_id,id);
        CREATE TABLE lens_return_events (
          id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id), occurred_at TIMESTAMP NOT NULL,
          partner_id TEXT NOT NULL, return_code TEXT NOT NULL, amount_cents INTEGER NOT NULL CHECK(amount_cents>=0),
          source_type TEXT NOT NULL CHECK(source_type IN ('SYNTHETIC_DEMO','CURATED_IMPORT')),
          verified INTEGER NOT NULL DEFAULT 0 CHECK(verified IN (0,1)), incident_id INTEGER,
          CHECK (NOT (source_type='SYNTHETIC_DEMO' AND verified=1)),
          FOREIGN KEY (tenant_id,incident_id) REFERENCES incidents(tenant_id,id));
        """)
        with self.assertRaises(sqlite3.IntegrityError):
            conn.execute("INSERT INTO lens_return_events VALUES ('x','TENANT-DEFAULT','2026-08-25','P','R11',100,'SYNTHETIC_DEMO',1,NULL)")

    def _migration_db(self):
        conn = sqlite3.connect(":memory:")
        conn.execute("PRAGMA foreign_keys=ON")
        conn.executescript("""
        CREATE TABLE tenants (id TEXT PRIMARY KEY);
        INSERT INTO tenants VALUES ('TENANT-A'), ('TENANT-B');
        CREATE TABLE incidents (
          id INTEGER PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(id),
          type TEXT NOT NULL DEFAULT 'RETURN', severity TEXT NOT NULL DEFAULT 'HIGH',
          status TEXT NOT NULL DEFAULT 'OPEN', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
        );
        INSERT INTO incidents(id, tenant_id) VALUES (7, 'TENANT-A');
        """)
        conn.executescript(MIGRATION.read_text())
        return conn

    def test_migration_tenant_binds_incident_references(self):
        conn = self._migration_db()
        with self.assertRaises(sqlite3.IntegrityError):
            conn.execute(
                "INSERT INTO lens_return_events (id,tenant_id,occurred_at,partner_id,return_code,amount_cents,source_type,verified,incident_id) VALUES (?,?,?,?,?,?,?,?,?)",
                ('cross', 'TENANT-B', '2026-08-25T00:00:00Z', 'P', 'R11', 100, 'CURATED_IMPORT', 1, 7),
            )

    def test_investigation_nodes_are_append_only(self):
        conn = self._migration_db()
        conn.execute("INSERT INTO lens_investigations (id,tenant_id,title,created_by) VALUES ('i','TENANT-A','test','operator')")
        conn.execute("INSERT INTO lens_investigation_nodes (id,investigation_id,tenant_id,question,query_intent_json,query_hash,result_hash,chart_spec_json,created_by) VALUES ('n','i','TENANT-A','q','{}','qh','rh','{}','operator')")
        with self.assertRaises(sqlite3.IntegrityError):
            conn.execute("UPDATE lens_investigation_nodes SET question='changed' WHERE id='n'")
        with self.assertRaises(sqlite3.IntegrityError):
            conn.execute("DELETE FROM lens_investigation_nodes WHERE id='n'")


if __name__ == "__main__":
    unittest.main()
