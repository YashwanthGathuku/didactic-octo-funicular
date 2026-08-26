#!/usr/bin/env python3
"""Prepare a bounded, privacy-safe ACH analytics sample from IBM AML HI-Small.

The source dataset is synthetic, but SentinelFlow still treats identifiers as
non-authoritative data. This tool:
- streams the CSV (suitable for the multi-million-row HI-Small file),
- selects only Payment Format == ACH,
- takes a deterministic reservoir sample,
- pseudonymizes account identifiers,
- NEVER exposes ``Is Laundering`` in the Lens input,
- writes the label only to a separate holdout file,
- records SHA-256 provenance for source and generated artifacts.

No network requests are made.
"""

from __future__ import annotations

import argparse
import csv
from dataclasses import asdict, dataclass
from decimal import Decimal, InvalidOperation
import hashlib
import json
from pathlib import Path
import random
import sys
from typing import Dict, Iterable

REQUIRED_COLUMNS = (
    "Timestamp",
    "From Bank",
    "Account",
    "To Bank",
    "Account.1",
    "Amount Received",
    "Receiving Currency",
    "Amount Paid",
    "Payment Currency",
    "Payment Format",
    "Is Laundering",
)

# Some mirrors rename the duplicate Account column. We resolve a small,
# explicit alias set rather than guessing arbitrary schemas.
ALIASES = {
    "Timestamp": ("Timestamp",),
    "From Bank": ("From Bank",),
    "From Account": ("Account", "From Account"),
    "To Bank": ("To Bank",),
    "To Account": ("Account.1", "To Account", "Account_1"),
    "Amount Received": ("Amount Received",),
    "Receiving Currency": ("Receiving Currency",),
    "Amount Paid": ("Amount Paid",),
    "Payment Currency": ("Payment Currency",),
    "Payment Format": ("Payment Format",),
    "Is Laundering": ("Is Laundering",),
}

LENS_FIELDS = (
    "event_id",
    "timestamp",
    "from_bank",
    "from_account_token",
    "to_bank",
    "to_account_token",
    "amount_received",
    "receiving_currency",
    "amount_paid",
    "payment_currency",
    "payment_format",
    "source_type",
    "verified",
)
GROUND_TRUTH_FIELDS = ("event_id", "is_laundering")


@dataclass(frozen=True)
class Manifest:
    source_name: str
    source_url: str
    source_sha256: str
    source_rows_scanned: int
    ach_rows_seen: int
    sample_rows: int
    sample_max_rows: int
    reservoir_seed: int
    lens_file: str
    lens_sha256: str
    ground_truth_file: str
    ground_truth_sha256: str
    ground_truth_excluded_from_lens: bool
    identifiers_pseudonymized: bool
    source_type: str
    verified: int


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Prepare IBM AML ACH sample for SentinelFlow Lens")
    p.add_argument("input_csv", type=Path)
    p.add_argument("--output-dir", type=Path, default=Path("demo/public/ibm-aml"))
    p.add_argument("--max-rows", type=int, default=100_000)
    p.add_argument("--seed", type=int, default=20260826)
    p.add_argument(
        "--source-url",
        default="https://www.kaggle.com/datasets/ealtman2019/ibm-transactions-for-anti-money-laundering-aml",
    )
    return p.parse_args()


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def canonical_header_map(fieldnames: Iterable[str] | None) -> Dict[str, str]:
    if not fieldnames:
        raise ValueError("input CSV has no header")
    existing = {name.strip(): name for name in fieldnames if name is not None}
    out: Dict[str, str] = {}
    for canonical, aliases in ALIASES.items():
        for alias in aliases:
            if alias in existing:
                out[canonical] = existing[alias]
                break
        if canonical not in out:
            raise ValueError(
                f"input CSV missing required field for {canonical!r}; accepted aliases={aliases!r}"
            )
    return out


def amount(value: str, field: str, row_number: int) -> str:
    try:
        dec = Decimal(value)
    except (InvalidOperation, ValueError) as exc:
        raise ValueError(f"invalid {field} at source row {row_number}: {value!r}") from exc
    if not dec.is_finite() or dec < 0:
        raise ValueError(f"invalid {field} at source row {row_number}: {value!r}")
    return format(dec, "f")


def token(kind: str, value: str, salt: str) -> str:
    digest = hashlib.sha256(f"{salt}|{kind}|{value}".encode("utf-8")).hexdigest()[:20]
    return f"{kind}_{digest}"


def event_id(row_number: int, row: dict[str, str], fields: Dict[str, str], source_sha: str) -> str:
    material = "|".join(
        [
            source_sha,
            str(row_number),
            row.get(fields["Timestamp"], ""),
            row.get(fields["From Bank"], ""),
            row.get(fields["From Account"], ""),
            row.get(fields["To Bank"], ""),
            row.get(fields["To Account"], ""),
            row.get(fields["Amount Paid"], ""),
        ]
    )
    return "ibm_" + hashlib.sha256(material.encode("utf-8")).hexdigest()[:24]


def normalized_row(
    row_number: int, row: dict[str, str], fields: Dict[str, str], source_sha: str
) -> tuple[dict[str, str], dict[str, str]]:
    eid = event_id(row_number, row, fields, source_sha)
    laundering = (row.get(fields["Is Laundering"], "") or "").strip()
    if laundering not in {"0", "1"}:
        raise ValueError(f"invalid Is Laundering label at source row {row_number}: {laundering!r}")

    lens = {
        "event_id": eid,
        "timestamp": (row.get(fields["Timestamp"], "") or "").strip(),
        "from_bank": (row.get(fields["From Bank"], "") or "").strip(),
        "from_account_token": token(
            "acct", (row.get(fields["From Account"], "") or "").strip(), source_sha
        ),
        "to_bank": (row.get(fields["To Bank"], "") or "").strip(),
        "to_account_token": token(
            "acct", (row.get(fields["To Account"], "") or "").strip(), source_sha
        ),
        "amount_received": amount(
            row.get(fields["Amount Received"], ""), "Amount Received", row_number
        ),
        "receiving_currency": (row.get(fields["Receiving Currency"], "") or "").strip(),
        "amount_paid": amount(row.get(fields["Amount Paid"], ""), "Amount Paid", row_number),
        "payment_currency": (row.get(fields["Payment Currency"], "") or "").strip(),
        "payment_format": "ACH",
        "source_type": "PUBLIC_SYNTHETIC_IBM_AML",
        "verified": "0",
    }
    return lens, {"event_id": eid, "is_laundering": laundering}


def prepare(args: argparse.Namespace) -> Manifest:
    if args.max_rows < 1 or args.max_rows > 1_000_000:
        raise ValueError("--max-rows must be in [1, 1000000]")
    if not args.input_csv.is_file():
        raise FileNotFoundError(args.input_csv)

    source_sha = sha256_file(args.input_csv)
    rng = random.Random(args.seed)
    sample: list[tuple[int, dict[str, str], dict[str, str]]] = []
    scanned = 0
    ach_seen = 0

    with args.input_csv.open("r", encoding="utf-8-sig", newline="") as f:
        reader = csv.DictReader(f)
        fields = canonical_header_map(reader.fieldnames)
        for row_number, row in enumerate(reader, start=2):
            scanned += 1
            payment_format = (row.get(fields["Payment Format"], "") or "").strip().upper()
            if payment_format != "ACH":
                continue
            ach_seen += 1
            lens, truth = normalized_row(row_number, row, fields, source_sha)
            item = (row_number, lens, truth)
            if len(sample) < args.max_rows:
                sample.append(item)
            else:
                j = rng.randrange(ach_seen)
                if j < args.max_rows:
                    sample[j] = item

    if not sample:
        raise ValueError("no ACH rows found in input CSV")
    sample.sort(key=lambda item: item[0])

    args.output_dir.mkdir(parents=True, exist_ok=True)
    lens_path = args.output_dir / "ibm_aml_ach_lens.csv"
    truth_path = args.output_dir / "ibm_aml_ach_ground_truth.csv"
    manifest_path = args.output_dir / "manifest.json"

    with lens_path.open("w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=LENS_FIELDS)
        writer.writeheader()
        writer.writerows(item[1] for item in sample)

    with truth_path.open("w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=GROUND_TRUTH_FIELDS)
        writer.writeheader()
        writer.writerows(item[2] for item in sample)

    manifest = Manifest(
        source_name=args.input_csv.name,
        source_url=args.source_url,
        source_sha256=source_sha,
        source_rows_scanned=scanned,
        ach_rows_seen=ach_seen,
        sample_rows=len(sample),
        sample_max_rows=args.max_rows,
        reservoir_seed=args.seed,
        lens_file=lens_path.name,
        lens_sha256=sha256_file(lens_path),
        ground_truth_file=truth_path.name,
        ground_truth_sha256=sha256_file(truth_path),
        ground_truth_excluded_from_lens=True,
        identifiers_pseudonymized=True,
        source_type="PUBLIC_SYNTHETIC_IBM_AML",
        verified=0,
    )
    manifest_path.write_text(json.dumps(asdict(manifest), indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return manifest


def main() -> None:
    try:
        manifest = prepare(parse_args())
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
    print(json.dumps(asdict(manifest), indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
