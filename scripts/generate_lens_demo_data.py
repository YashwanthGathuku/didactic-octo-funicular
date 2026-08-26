#!/usr/bin/env python3
"""Generate deterministic, explicitly synthetic ACH-return observations for Lens Lite."""
from __future__ import annotations

import argparse
import csv
import random
from datetime import datetime, timedelta, timezone
from pathlib import Path

SEED = 20260825
PARTNERS = ["TEST-PAYROLL-17", "TEST-TREASURY-04", "TEST-BENEFITS-09", "TEST-AP-12"]
BASE_CODES = ["R01", "R03", "R10", "R11"]


def rows() -> list[dict[str, str]]:
    rng = random.Random(SEED)
    end = datetime(2026, 8, 25, 16, 0, tzinfo=timezone.utc)
    start = end - timedelta(days=44)
    out: list[dict[str, str]] = []
    idx = 1
    for day in range(45):
        when = start + timedelta(days=day)
        count = 1 + (1 if day % 5 == 0 else 0)
        for j in range(count):
            partner = PARTNERS[(day + j) % len(PARTNERS)]
            code = BASE_CODES[(day + j * 2) % len(BASE_CODES)]
            amount = 35_000_00 + rng.randint(0, 145_000_00)
            out.append({
                "id": f"demo-{idx:04d}",
                "tenant_id": "TENANT-DEFAULT",
                "occurred_at": (when + timedelta(hours=9 + j)).isoformat().replace("+00:00", "Z"),
                "partner_id": partner,
                "return_code": code,
                "amount_cents": str(amount),
                "source_type": "SYNTHETIC_DEMO",
                "verified": "0",
                "incident_id": "",
            })
            idx += 1

    # Final four days: a clear R11 concentration for a fictional payroll partner.
    for d in range(4):
        when = end - timedelta(days=3 - d)
        for j in range(5):
            out.append({
                "id": f"demo-{idx:04d}",
                "tenant_id": "TENANT-DEFAULT",
                "occurred_at": (when + timedelta(minutes=j * 17)).isoformat().replace("+00:00", "Z"),
                "partner_id": "TEST-PAYROLL-17",
                "return_code": "R11",
                "amount_cents": str(18_000_000 + rng.randint(0, 12_000_000)),
                "source_type": "SYNTHETIC_DEMO",
                "verified": "0",
                "incident_id": "",
            })
            idx += 1
    return out


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", default="demo/lens_return_events.csv")
    args = parser.parse_args()
    path = Path(args.output)
    path.parent.mkdir(parents=True, exist_ok=True)
    data = rows()
    with path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=list(data[0]), lineterminator="\n")
        writer.writeheader(); writer.writerows(data)
    print(f"wrote {len(data)} synthetic Lens rows to {path}")


if __name__ == "__main__":
    main()
