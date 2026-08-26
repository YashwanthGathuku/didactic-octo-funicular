"""Minimal live Gemini 3.5 Flash proof using Vertex AI credentials.

Dry-run by default. Live mode sends one synthetic, non-financial prompt and
prints only response metadata/hash, never the response text itself.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import time

MODEL = "gemini-3.5-flash"
PROMPT = (
    "SentinelFlow P17 synthetic proof. Reply with a brief statement confirming that "
    "a deterministic financial control plane must remain authoritative over model output. "
    "Do not reference real customers, accounts, payments, or credentials."
)


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser()
    p.add_argument("--project", default=os.getenv("GOOGLE_CLOUD_PROJECT", ""))
    p.add_argument("--location", default=os.getenv("GOOGLE_CLOUD_LOCATION", "us-central1"))
    p.add_argument("--execute", action="store_true")
    return p.parse_args()


def run(args: argparse.Namespace) -> dict[str, object]:
    plan = {"project": args.project, "location": args.location, "model": MODEL, "synthetic_only": True}
    if not args.execute:
        return {"status": "NOT_RUN", "mode": "DRY_RUN", "plan": plan}
    if not args.project:
        raise RuntimeError("--project or GOOGLE_CLOUD_PROJECT is required")

    from google import genai
    from google.genai import types

    client = genai.Client(
        vertexai=True,
        project=args.project,
        location=args.location,
        http_options=types.HttpOptions(api_version="v1"),
    )
    started = time.monotonic()
    response = client.models.generate_content(
        model=MODEL,
        contents=PROMPT,
        config=types.GenerateContentConfig(temperature=0, max_output_tokens=160),
    )
    elapsed_ms = round((time.monotonic() - started) * 1000)
    text = (getattr(response, "text", None) or "").strip()
    if not text:
        raise RuntimeError("Gemini returned no text; live proof failed")

    usage = getattr(response, "usage_metadata", None)
    usage_safe = None
    if usage is not None:
        # Pydantic SDK objects commonly support model_dump; otherwise omit usage.
        dump = getattr(usage, "model_dump", None)
        if callable(dump):
            usage_safe = dump(exclude_none=True)

    return {
        "status": "PASS_LIVE",
        "proof_type": "LIVE_GEMINI_VERTEX_AI",
        "project": args.project,
        "location": args.location,
        "model": MODEL,
        "latency_ms": elapsed_ms,
        "response_chars": len(text),
        "response_sha256": hashlib.sha256(text.encode("utf-8")).hexdigest(),
        "response_content_logged": False,
        "synthetic_only": True,
        "usage_metadata": usage_safe,
    }


def main() -> None:
    print(json.dumps(run(parse_args()), indent=2, sort_keys=True, default=str))


if __name__ == "__main__":
    main()
