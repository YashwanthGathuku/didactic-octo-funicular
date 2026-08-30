#!/usr/bin/env python3
"""
Synthetic NACHA ACH File Generator for SentinelFlow

Generates compliant 94-character fixed-width NACHA (National Automated Clearing House Association)
PPD (Prearranged Payment and Deposit) files for local development, integration testing, and demos.
All financial data, routing numbers, and names are synthetic to prevent PII/PAN spillage.
"""

import argparse
import datetime
import random
import sys

def compute_routing_check_digit(routing_8: str) -> str:
    """Computes the 9th check digit for an 8-digit routing transit number using weights (3, 7, 1)."""
    weights = [3, 7, 1, 3, 7, 1, 3, 7]
    s = sum(int(digit) * weight for digit, weight in zip(routing_8, weights))
    return str((10 - (s % 10)) % 10)

def generate_nacha_file(num_entries: int, total_amount_cents: int, output_path: str, error_type: str = "none"):
    now = datetime.datetime.now()
    file_date = now.strftime("%y%m%d")
    file_time = now.strftime("%H%M")
    effective_date = (now + datetime.timedelta(days=1)).strftime("%y%m%d")

    # Immediate Destination & Origin (Synthetic Federal Reserve format)
    dest_routing_8 = "01100001"
    dest_check = compute_routing_check_digit(dest_routing_8)
    imm_dest = f" {dest_routing_8}{dest_check}"
    imm_origin = " 121042882"
    origin_name = "SENTINEL TREASURY"
    dest_name = "FEDERAL RESERVE BANK"

    lines = []

    # 1. File Header Record (Record Type 1)
    file_header = (
        "1"                             # Record Type Code (1)
        "01"                            # Priority Code (01)
        f"{imm_dest:<10}"               # Immediate Destination (10)
        f"{imm_origin:<10}"             # Immediate Origin (10)
        f"{file_date}"                  # File Creation Date (YYMMDD)
        f"{file_time}"                  # File Creation Time (HHMM)
        "A"                             # File ID Modifier (A)
        "094"                           # Record Size (094)
        "10"                            # Blocking Factor (10)
        "1"                             # Format Code (1)
        f"{dest_name:<23}"              # Immediate Destination Name (23)
        f"{origin_name:<23}"            # Immediate Origin Name (23)
        "DEMO0001"                      # Reference Code (8)
    )
    lines.append(file_header[:94])

    # 2. Company / Batch Header Record (Record Type 5)
    batch_header = (
        "5"                             # Record Type Code (5)
        "220"                           # Service Class Code (220 = Credits & Debits)
        f"{'SENTINELFLOW':<16}"         # Company Name (16)
        f"{'':<20}"                     # Company Discretionary Data (20)
        "1234567890"                    # Company Identification (10)
        "PPD"                           # Standard Entry Class Code (PPD)
        f"{'PAYROLL':<10}"              # Company Entry Description (10)
        f"{file_date}"                  # Company Descriptive Date (6)
        f"{effective_date}"             # Effective Entry Date (6)
        f"{'':<3}"                      # Settlement Date (3)
        "1"                             # Originator Status Code (1)
        f"{imm_origin.strip()[:8]}"     # Originating DFI ID (8)
        "0000001"                       # Batch Number (7)
    )
    lines.append(batch_header[:94])

    # 3. Entry Detail Records (Record Type 6)
    total_debit_cents = 0
    total_credit_cents = 0
    entry_hash_sum = 0
    amount_per_entry = total_amount_cents // num_entries
    remainder = total_amount_cents % num_entries

    for i in range(num_entries):
        entry_amt = amount_per_entry + (remainder if i == 0 else 0)
        total_credit_cents += entry_amt

        # Synthetic DFI routing: 011000015
        dfi_routing_8 = "01100001"
        dfi_check = compute_routing_check_digit(dfi_routing_8)
        if error_type == "invalid-routing" and i == 0:
            dfi_check = "9" if dfi_check != "9" else "8" # deliberately corrupt check digit

        entry_hash_sum += int(dfi_routing_8)

        account_num = f"9999{random.randint(100000, 999999)}"
        trace_num = f"{dfi_routing_8}{i+1:07d}"

        entry_detail = (
            "6"                         # Record Type Code (6)
            "22"                        # Transaction Code (22 = Checking Credit)
            f"{dfi_routing_8}{dfi_check}" # Receiving DFI Identification + Check Digit (9)
            f"{account_num:<17}"        # DFI Account Number (17)
            f"{entry_amt:010d}"         # Amount in Cents (10)
            f"EMP-{i+1:06d}     "       # Individual Identification Number (15)
            f"SYNTHETIC USER {i+1:<7}"  # Individual Name (22)
            "  "                        # Discretionary Data (2)
            "0"                         # Addenda Record Indicator (0)
            f"{trace_num}"              # Trace Number (15)
        )
        lines.append(entry_detail[:94])

    # 4. Batch Control Record (Record Type 8)
    if error_type == "hash-mismatch":
        # Deliberately corrupt entry hash by adding 999 (Rule 0802 violation)
        batch_hash_str = f"{(entry_hash_sum + 999) % 10000000000:010d}"
    else:
        batch_hash_str = f"{entry_hash_sum % 10000000000:010d}"

    reported_credits = total_credit_cents
    if error_type == "unbalanced":
        reported_credits += 50000 # $500 discrepancy

    batch_control = (
        "8"                             # Record Type Code (8)
        "220"                           # Service Class Code (220)
        f"{num_entries:06d}"            # Entry / Addenda Count (6)
        f"{batch_hash_str}"             # Entry Hash (10)
        f"{total_debit_cents:012d}"     # Total Debit Amount (12)
        f"{reported_credits:012d}"      # Total Credit Amount (12)
        "1234567890"                    # Company Identification (10)
        f"{'':<19}"                     # Message Authentication Code (19)
        f"{'':<6}"                      # Reserved (6)
        f"{imm_origin.strip()[:8]}"     # Originating DFI ID (8)
        "0000001"                       # Batch Number (7)
    )
    lines.append(batch_control[:94])

    # 5. File Control Record (Record Type 9)
    total_records = len(lines) + 1 # include file control itself
    block_count = (total_records + 9) // 10
    file_control = (
        "9"                             # Record Type Code (9)
        "000001"                        # Batch Count (6)
        f"{block_count:06d}"            # Block Count (6)
        f"{num_entries:08d}"            # Total Detail and Addenda Record Count (8)
        f"{batch_hash_str}"             # Entry Hash (10)
        f"{total_debit_cents:012d}"     # Total Debit Amount (12)
        f"{reported_credits:012d}"      # Total Credit Amount (12)
        f"{'':<39}"                     # Reserved (39)
    )
    lines.append(file_control[:94])

    # 6. Block padding: file must be padded to a multiple of 10 records with 94-char 9s
    while len(lines) % 10 != 0:
        lines.append("9" * 94)

    content = "\r\n".join(lines) + "\r\n"
    with open(output_path, "wb") as f:
        f.write(content.encode("ascii"))

    desc = f" [{error_type.upper()}]" if error_type != "none" else " [VALID]"
    print(f"[OK] Generated synthetic NACHA file{desc}: {output_path}")
    print(f"     Records: {len(lines)} | Entries: {num_entries} | Total: ${total_amount_cents/100:,.2f} | Hash: {batch_hash_str}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Synthetic NACHA ACH File Generator")
    parser.add_argument("--entries", type=int, default=10, help="Number of entry records (default: 10)")
    parser.add_argument("--amount-cents", type=int, default=1250000, help="Total amount in cents (default: $12,500.00)")
    parser.add_argument("--output", type=str, default="synthetic_payroll.ach", help="Output filepath")
    parser.add_argument(
        "--error-type",
        choices=["none", "hash-mismatch", "invalid-routing", "unbalanced"],
        default="none",
        help="Inject specific validation error for demo triage testing (default: none)",
    )
    args = parser.parse_args()

    generate_nacha_file(args.entries, args.amount_cents, args.output, error_type=args.error_type)
