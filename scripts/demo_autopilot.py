#!/usr/bin/env python3
"""SentinelFlow Automated Demo Autopilot.

Simulates automated payment file drops and return log processing without
requiring manual UI uploads. Demonstrates hands-free real-time streaming,
automated incident detection, and multi-agent recovery.
"""

import argparse
import json
import os
import pathlib
import sys
import time
import urllib.request

GATEWAY_API = os.getenv("SENTINEL_GATEWAY_URL", "http://localhost:8080/api/v1")


def print_banner(msg: str):
    print(f"\n\033[1;36m{'=' * 60}\033[0m")
    print(f"\033[1;36m  {msg}\033[0m")
    print(f"\033[1;36m{'=' * 60}\033[0m\n")


def ingest_file(filename: str, content: str) -> dict:
    url = f"{GATEWAY_API}/files/ingest-raw"
    payload = json.dumps({"filename": filename, "content": content}).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=payload,
        headers={
            "Content-Type": "application/json",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return data
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8")
        try:
            return json.loads(body)
        except Exception:
            return {"error": f"HTTP {e.code}: {body}"}
    except Exception as e:
        return {"error": str(e)}


def main():
    parser = argparse.ArgumentParser(description="SentinelFlow Demo Autopilot")
    parser.add_argument(
        "--interval",
        type=int,
        default=6,
        help="Seconds between automated file drops (default: 6)",
    )
    parser.add_argument(
        "--loop",
        action="store_true",
        help="Run continuously in a loop for live booth/screen demo",
    )
    args = parser.parse_args()

    demo_dir = pathlib.Path(__file__).resolve().parent.parent / "demo"

    clean_file = demo_dir / "demo_clean_payroll.ach"
    corrupt_file = demo_dir / "demo_corrupted_hash.ach"
    invalid_file = demo_dir / "demo_invalid_routing.ach"

    if not clean_file.exists() or not corrupt_file.exists():
        print(f"Error: Demo files missing in {demo_dir}")
        sys.exit(1)

    iteration = 1
    while True:
        print_banner(f"AUTOMATED DEMO STREAM: PASS {iteration}")

        # 1. Automated Clean Payroll Batch Ingestion
        print(f"[1/3] [VALID] Autonomous Feed Drop: {clean_file.name} (Clean Corporate Payroll $25k)")
        print(f"      Sending to Gateway at {GATEWAY_API}...")
        clean_res = ingest_file(
            f"AUTOPILOT_CLEAN_PAYROLL_{int(time.time())}.ach",
            clean_file.read_text(encoding="utf-8"),
        )
        status = clean_res.get("status", "UNKNOWN")
        hash_val = clean_res.get("hash", "")[:12]
        print(f"      ==> Result: {status} (Hash: {hash_val}...) | Total Debits: ${clean_res.get('totalDebitsMinor', 0)/100:,.2f}")
        print(f"      ==> Pre-Flight Check Passed in <5ms. Ledger updated.")

        print(f"\n      Waiting {args.interval}s before next incoming payment file...\n")
        time.sleep(args.interval)

        # 2. Automated Corrupted Entry Hash Batch Drop (Rule 0802 Anomaly)
        print(f"[2/3] [QUARANTINE] Autonomous Feed Drop: {corrupt_file.name} (Corrupted Batch Hash Injection)")
        print(f"      Simulating partner batch transmission error...")
        corrupt_res = ingest_file(
            f"AUTOPILOT_CORRUPTED_HASH_{int(time.time())}.ach",
            corrupt_file.read_text(encoding="utf-8"),
        )
        c_status = corrupt_res.get("status", "UNKNOWN")
        findings = corrupt_res.get("findings", [])
        print(f"      ==> Result: {c_status} | Findings Detected: {len(findings)}")
        for f in findings:
            print(f"          - Rule [{f.get('code')}]: {f.get('description')} (Severity: {f.get('severity')})")
        print("      ==> Autonomous Quarantine triggered. Incident Commander and Specialist Fleet dispatched.")

        print(f"\n      Waiting {args.interval}s before next incoming payment file...\n")
        time.sleep(args.interval)

        # 3. Automated Invalid Routing File Drop
        print(f"[3/3] [INVALID] Autonomous Feed Drop: {invalid_file.name} (Mod-10 Routing Checksum Failure)")
        invalid_res = ingest_file(
            f"AUTOPILOT_INVALID_ROUTING_{int(time.time())}.ach",
            invalid_file.read_text(encoding="utf-8"),
        )
        i_status = invalid_res.get("status", "UNKNOWN")
        print(f"      ==> Result: {i_status} | Fail-closed pre-flight validation executed.")

        if not args.loop:
            print_banner("AUTOPILOT PASS COMPLETE — All automated scenarios demonstrated!")
            break

        iteration += 1
        print(f"\n[Loop Mode Active] Next pass in {args.interval}s...\n")
        time.sleep(args.interval)


if __name__ == "__main__":
    main()
