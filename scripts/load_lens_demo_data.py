#!/usr/bin/env python3
"""Load only SentinelFlow's own explicitly synthetic Lens fixture into local SQLite."""
from __future__ import annotations

import argparse
import csv
import sqlite3
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", default="gateway/data/sentinel.db")
    parser.add_argument("--csv", default="demo/lens_return_events.csv")
    args = parser.parse_args()
    db_path = Path(args.db)
    csv_path = Path(args.csv)
    if not db_path.exists():
        raise SystemExit(f"database not found: {db_path}; start/migrate the gateway first")
    if not csv_path.exists():
        raise SystemExit(f"fixture not found: {csv_path}; run scripts/generate_lens_demo_data.py")

    conn = sqlite3.connect(db_path)
    try:
        with conn:
            # This loader owns only the SYNTHETIC_DEMO rows it generated.
            conn.execute("DELETE FROM lens_return_events WHERE source_type='SYNTHETIC_DEMO' AND id LIKE 'demo-%'")
            with csv_path.open(newline="", encoding="utf-8") as fh:
                reader = csv.DictReader(fh)
                rows = list(reader)
            conn.executemany(
                """INSERT INTO lens_return_events
                   (id,tenant_id,occurred_at,partner_id,return_code,amount_cents,source_type,verified,incident_id)
                   VALUES (?,?,?,?,?,?,?,?,NULL)""",
                [
                    (
                        row["id"], row["tenant_id"], row["occurred_at"], row["partner_id"],
                        row["return_code"], int(row["amount_cents"]), row["source_type"], int(row["verified"]),
                    )
                    for row in rows
                ],
            )
        print(f"loaded {len(rows)} explicitly synthetic Lens rows into {db_path}")
    finally:
        conn.close()


if __name__ == "__main__":
    main()
