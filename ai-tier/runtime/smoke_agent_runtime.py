"""Bounded P17 smoke test for a deployed SentinelFlow ADK Agent Runtime.

Dry-run is the default. ``--execute`` performs one synthetic streamQuery call
against an existing reasoningEngine. The script never prints OAuth credentials
or raw financial data. It records only safe event authors/counts and proof
metadata so managed execution cannot be confused with local tests.
"""

from __future__ import annotations

import argparse
import json
import os
import time
from typing import Any

import google.auth
from google.auth.transport.requests import Request
import httpx

SYNTHETIC_PROMPT = (
    "SentinelFlow managed-runtime proof using synthetic data only. "
    "A synthetic payroll artifact was deterministically quarantined for CONTROL_TOTAL_MISMATCH. "
    "Consult DiagnosisAgent and PolicySLAAgent if delegation is appropriate. "
    "Do not propose release, approval, SQL, policy changes, or real financial actions. "
    "Return a short bounded operational summary and state that Go remains authoritative."
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Smoke-test deployed SentinelFlow Agent Runtime")
    parser.add_argument("--engine-id", required=True)
    parser.add_argument("--project", default=os.getenv("GOOGLE_CLOUD_PROJECT", "project-3687901b-8355-4073-ac3"))
    parser.add_argument("--location", default=os.getenv("GOOGLE_CLOUD_LOCATION", "us-central1"))
    parser.add_argument("--user-id", default="sentinelflow-p17-smoke")
    parser.add_argument("--timeout-seconds", type=float, default=60.0)
    parser.add_argument("--execute", action="store_true")
    return parser.parse_args()


def _safe_event_metadata(event: Any) -> dict[str, Any]:
    if not isinstance(event, dict):
        return {"type": type(event).__name__}
    author = event.get("author")
    event_id = event.get("id")
    content = event.get("content")
    has_content = isinstance(content, dict) and bool(content.get("parts"))
    return {
        "author": author if isinstance(author, str) else None,
        "event_id": event_id if isinstance(event_id, str) else None,
        "has_content": has_content,
    }


def _decode_sse(lines: list[str]) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    # 1. Try line-by-line decoding (SSE data: or raw JSON lines)
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("data:"):
            stripped = stripped[5:].strip()
        if not stripped or stripped == "[DONE]":
            continue
        try:
            value = json.loads(stripped)
            if isinstance(value, dict):
                events.append(value)
            elif isinstance(value, list):
                for item in value:
                    if isinstance(item, dict):
                        events.append(item)
        except json.JSONDecodeError:
            continue

    # 2. If line-by-line yielded nothing, attempt whole-payload JSON decode
    if not events and lines:
        try:
            full_text = "\n".join(lines)
            parsed = json.loads(full_text)
            if isinstance(parsed, list):
                for item in parsed:
                    if isinstance(item, dict):
                        events.append(item)
            elif isinstance(parsed, dict):
                events.append(parsed)
        except json.JSONDecodeError:
            pass

    return events


def run(args: argparse.Namespace) -> dict[str, Any]:
    from runtime.managed_adk import MANAGED_MODEL

    plan = {
        "project": args.project,
        "location": args.location,
        "engine_id": args.engine_id,
        "method": "async_stream_query",
        "model_requirement": MANAGED_MODEL,
        "synthetic_only": True,
    }
    if not args.execute:
        return {"status": "NOT_RUN", "mode": "DRY_RUN", "plan": plan}

    credentials, adc_project = google.auth.default(
        scopes=["https://www.googleapis.com/auth/cloud-platform"]
    )
    credentials.refresh(Request())
    token = getattr(credentials, "token", None)
    if not token:
        raise RuntimeError("ADC credentials did not produce an access token")

    url = (
        f"https://{args.location}-aiplatform.googleapis.com/v1/projects/{args.project}/"
        f"locations/{args.location}/reasoningEngines/{args.engine_id}:streamQuery?alt=sse"
    )
    body = {
        "class_method": "async_stream_query",
        "input": {"user_id": args.user_id, "message": SYNTHETIC_PROMPT},
    }

    started = time.monotonic()
    lines: list[str] = []
    with httpx.Client(timeout=max(5.0, min(args.timeout_seconds, 120.0))) as client:
        with client.stream(
            "POST",
            url,
            headers={
                "Authorization": f"Bearer {token}",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
            },
            json=body,
        ) as response:
            response.raise_for_status()
            for line in response.iter_lines():
                if line:
                    lines.append(line)
                if len(lines) >= 500:
                    break

    elapsed_ms = round((time.monotonic() - started) * 1000)
    events = _decode_sse(lines)
    if not events:
        return {
            "status": "FAIL",
            "reason": "streamQuery returned no decodable SSE events",
            "latency_ms": elapsed_ms,
            "plan": plan,
        }

    metadata = [_safe_event_metadata(event) for event in events]
    authors = sorted({item["author"] for item in metadata if item.get("author")})
    specialists = sorted(
        author
        for author in authors
        if author
        in {
            "DiagnosisAgent",
            "PolicySLAAgent",
            "MemoryAgent",
            "RemediationAgent",
            "VerifierAgent",
            "ReturnRiskAgent",
        }
    )

    return {
        "status": "PASS_LIVE",
        "proof_type": "LIVE_AGENT_RUNTIME_STREAM_QUERY",
        "project": args.project,
        "location": args.location,
        "engine_id": args.engine_id,
        "adc_project": adc_project,
        "event_count": len(events),
        "authors": authors,
        "specialist_authors_observed": specialists,
        "managed_multi_agent_observed": len(specialists) > 0,
        "latency_ms": elapsed_ms,
        "synthetic_only": True,
        "content_logged": False,
    }


def main() -> None:
    args = parse_args()
    print(json.dumps(run(args), indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
