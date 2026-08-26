#!/usr/bin/env python3
"""Verify SentinelFlow's prepared public/synthetic demo-data boundary."""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
from pathlib import Path
import sys

FORBIDDEN_LENS_COLUMNS = {"Is Laundering", "is_laundering", "ground_truth", "label"}


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def verify(directory: Path) -> dict[str, object]:
    manifest_path = directory / "manifest.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    lens_path = directory / manifest["lens_file"]
    truth_path = directory / manifest["ground_truth_file"]

    assert sha256_file(lens_path) == manifest["lens_sha256"], "Lens file hash drift"
    assert sha256_file(truth_path) == manifest["ground_truth_sha256"], "ground-truth hash drift"
    assert manifest["ground_truth_excluded_from_lens"] is True
    assert manifest["identifiers_pseudonymized"] is True
    assert manifest["verified"] == 0

    with lens_path.open("r", encoding="utf-8", newline="") as f:
        reader = csv.DictReader(f)
        headers = set(reader.fieldnames or [])
        assert not (headers & FORBIDDEN_LENS_COLUMNS), "ground-truth column leaked into Lens input"
        rows = list(reader)
    assert rows, "Lens dataset is empty"
    for row in rows:
        assert row["payment_format"] == "ACH"
        assert row["source_type"] == "PUBLIC_SYNTHETIC_IBM_AML"
        assert row["verified"] == "0"
        assert row["from_account_token"].startswith("acct_")
        assert row["to_account_token"].startswith("acct_")

    with truth_path.open("r", encoding="utf-8", newline="") as f:
        truth_rows = list(csv.DictReader(f))
    assert len(truth_rows) == len(rows), "holdout/Lens sample row-count mismatch"
    assert {r["event_id"] for r in truth_rows} == {r["event_id"] for r in rows}
    assert all(r["is_laundering"] in {"0", "1"} for r in truth_rows)

    return {
        "status": "PASS",
        "rows": len(rows),
        "lens_sha256": manifest["lens_sha256"],
        "ground_truth_sha256": manifest["ground_truth_sha256"],
        "ground_truth_hidden": True,
        "synthetic_non_authoritative": True,
    }


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("directory", type=Path)
    args = p.parse_args()
    try:
        print(json.dumps(verify(args.directory), indent=2, sort_keys=True))
    except Exception as exc:
        print(f"FAIL: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc


if __name__ == "__main__":
    main()
