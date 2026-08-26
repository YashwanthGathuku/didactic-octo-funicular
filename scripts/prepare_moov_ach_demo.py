#!/usr/bin/env python3
"""Validate and stage genuine 94-character ACH test fixtures for SentinelFlow demos.

The script does not download anything. Point it at a checked-out Moov ACH test
file or directory. Every non-empty record must be 94 printable ASCII characters
and begin with a valid NACHA record type 1-9.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import shutil
import sys


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def ach_files(path: Path) -> list[Path]:
    if path.is_file():
        return [path]
    if not path.is_dir():
        raise FileNotFoundError(path)
    return sorted(p for p in path.rglob("*.ach") if p.is_file())


def validate(path: Path) -> int:
    raw = path.read_bytes()
    try:
        text = raw.decode("ascii")
    except UnicodeDecodeError as exc:
        raise ValueError(f"{path}: file is not ASCII") from exc
    records = [line for line in text.splitlines() if line != ""]
    if not records:
        raise ValueError(f"{path}: no ACH records")
    for idx, record in enumerate(records, start=1):
        if len(record) != 94:
            raise ValueError(f"{path}: record {idx} has {len(record)} chars; expected 94")
        if record[0] not in "123456789":
            raise ValueError(f"{path}: record {idx} has invalid record type {record[0]!r}")
        if any(ord(ch) < 32 or ord(ch) > 126 for ch in record):
            raise ValueError(f"{path}: record {idx} contains non-printable ASCII")
    return len(records)


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("input", type=Path)
    p.add_argument("--output-dir", type=Path, default=Path("demo/public/moov-ach"))
    p.add_argument("--source-url", default="https://github.com/moov-io/ach")
    p.add_argument("--source-commit", default="UNSPECIFIED")
    args = p.parse_args()

    try:
        files = ach_files(args.input)
        if not files:
            raise ValueError("no .ach files found")
        args.output_dir.mkdir(parents=True, exist_ok=True)
        entries = []
        for source in files:
            count = validate(source)
            target = args.output_dir / source.name
            shutil.copyfile(source, target)
            entries.append(
                {
                    "name": source.name,
                    "records": count,
                    "source_sha256": sha256_file(source),
                    "staged_sha256": sha256_file(target),
                }
            )
        manifest = {
            "source_url": args.source_url,
            "source_commit": args.source_commit,
            "format": "NACHA_94_CHAR_ASCII",
            "source_type": "PUBLIC_OPEN_SOURCE_TEST_FIXTURE",
            "verified": 0,
            "files": entries,
        }
        (args.output_dir / "manifest.json").write_text(
            json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )
        print(json.dumps({"status": "PASS", **manifest}, indent=2, sort_keys=True))
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc


if __name__ == "__main__":
    main()
