"""Bounded live Memory Bank write/retrieve/delete proof for SentinelFlow P17.

Uses the Memory Bank attached to an existing Agent Runtime reasoningEngine.
The fact is synthetic and advisory. By default the created memory is deleted
again after retrieval so the proof does not pollute long-term runtime memory.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from typing import Any

FACT = (
    "Synthetic demo memory: TEST-PAYROLL-17 historically exhibits a Friday batch pattern. "
    "Advisory only; this memory is not evidence and cannot authorize any financial action."
)
SCOPE = {"tenant_id": "SYNTHETIC-DEMO", "proof_id": "sentinelflow-p17"}


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser()
    p.add_argument("--project", default=os.getenv("GOOGLE_CLOUD_PROJECT", ""))
    p.add_argument("--location", default=os.getenv("GOOGLE_CLOUD_LOCATION", "us-central1"))
    p.add_argument(
        "--engine-name",
        default=os.getenv("SENTINEL_AGENT_RUNTIME_RESOURCE", ""),
        help="Full projects/.../locations/.../reasoningEngines/... resource name",
    )
    p.add_argument("--keep-memory", action="store_true")
    p.add_argument("--execute", action="store_true")
    return p.parse_args()


def _response_memory_name(operation: Any) -> str:
    response = getattr(operation, "response", None)
    name = getattr(response, "name", None) if response is not None else None
    if name:
        return str(name)
    if isinstance(response, dict):
        return str(response.get("name") or "")
    return ""


def _memory_fact(item: Any) -> str:
    memory = getattr(item, "memory", None)
    if memory is not None:
        fact = getattr(memory, "fact", None)
        if fact:
            return str(fact)
    fact = getattr(item, "fact", None)
    if fact:
        return str(fact)
    if isinstance(item, dict):
        candidate = item.get("memory", item)
        if isinstance(candidate, dict):
            return str(candidate.get("fact") or "")
    return ""


def run(args: argparse.Namespace) -> dict[str, object]:
    plan = {
        "project": args.project,
        "location": args.location,
        "engine_name": args.engine_name or "<required for live>",
        "scope": SCOPE,
        "synthetic_only": True,
        "cleanup_default": not args.keep_memory,
    }
    if not args.execute:
        return {"status": "NOT_RUN", "mode": "DRY_RUN", "plan": plan}
    if not args.project or not args.engine_name:
        raise RuntimeError("--project and --engine-name are required for live proof")
    expected_prefix = f"projects/{args.project}/locations/{args.location}/reasoningEngines/"
    if not args.engine_name.startswith(expected_prefix):
        raise RuntimeError(f"engine name must start with {expected_prefix!r}")

    import vertexai

    client = vertexai.Client(project=args.project, location=args.location)
    operation = client.agent_engines.memories.create(name=args.engine_name, fact=FACT, scope=SCOPE)
    memory_name = _response_memory_name(operation)

    retrieved = list(client.agent_engines.memories.retrieve(name=args.engine_name, scope=SCOPE))
    facts = [_memory_fact(item) for item in retrieved]
    matched = FACT in facts
    if not matched:
        raise RuntimeError("created synthetic fact was not retrieved from Memory Bank")

    cleaned_up = False
    cleanup_error = None
    if not args.keep_memory and memory_name:
        try:
            client.agent_engines.memories.delete(name=memory_name)
            cleaned_up = True
        except Exception as exc:  # cleanup status is reported, proof remains valid
            cleanup_error = type(exc).__name__

    return {
        "status": "PASS_LIVE",
        "proof_type": "LIVE_AGENT_PLATFORM_MEMORY_BANK",
        "project": args.project,
        "location": args.location,
        "engine_name": args.engine_name,
        "scope": SCOPE,
        "retrieved_count": len(retrieved),
        "fact_match": True,
        "fact_sha256": hashlib.sha256(FACT.encode("utf-8")).hexdigest(),
        "fact_content_logged": False,
        "memory_name_observed": bool(memory_name),
        "cleanup_requested": not args.keep_memory,
        "cleaned_up": cleaned_up,
        "cleanup_error_class": cleanup_error,
        "synthetic_only": True,
        "authority": "ADVISORY_ONLY",
    }


def main() -> None:
    print(json.dumps(run(parse_args()), indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
