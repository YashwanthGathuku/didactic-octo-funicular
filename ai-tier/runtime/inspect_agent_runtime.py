"""Inspect a real SentinelFlow Agent Runtime resource without printing tokens.

The script reads Agent Runtime's server-returned ``spec.effectiveIdentity`` and
``spec.deploymentSpec.agentGatewayConfig``.  It is intended for P17 evidence
capture after a live deployment and performs no mutations.
"""

from __future__ import annotations

import argparse
import json
import os
from typing import Any

import google.auth
from google.auth.transport.requests import Request
import httpx


def _args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Inspect SentinelFlow Agent Runtime live metadata")
    parser.add_argument("--engine-id", required=True, help="Reasoning Engine resource ID")
    parser.add_argument(
        "--project",
        default=os.getenv("GOOGLE_CLOUD_PROJECT", "project-3687901b-8355-4073-ac3"),
        help="Google Cloud project ID",
    )
    parser.add_argument(
        "--location",
        default=os.getenv("GOOGLE_CLOUD_LOCATION", "us-central1"),
        help="Agent Runtime region",
    )
    return parser.parse_args()


def _get(data: dict[str, Any], *path: str) -> Any:
    current: Any = data
    for key in path:
        if not isinstance(current, dict):
            return None
        current = current.get(key)
    return current


def inspect(project: str, location: str, engine_id: str) -> dict[str, Any]:
    credentials, adc_project = google.auth.default(
        scopes=["https://www.googleapis.com/auth/cloud-platform"]
    )
    credentials.refresh(Request())
    token = getattr(credentials, "token", None)
    if not token:
        raise RuntimeError("ADC credentials did not produce an access token")

    url = (
        f"https://{location}-aiplatform.googleapis.com/v1/projects/{project}/"
        f"locations/{location}/reasoningEngines/{engine_id}"
    )
    with httpx.Client(timeout=15.0, follow_redirects=False) as client:
        response = client.get(url, headers={"Authorization": f"Bearer {token}"})
    response.raise_for_status()
    data = response.json()

    effective_identity = _get(data, "spec", "effectiveIdentity") or ""
    iam_principal = ""
    if isinstance(effective_identity, str) and effective_identity.startswith("agents."):
        iam_principal = f"principal://{effective_identity}"
    elif isinstance(effective_identity, str) and effective_identity.startswith(
        "principal://agents."
    ):
        iam_principal = effective_identity

    gateway_config = _get(data, "spec", "deploymentSpec", "agentGatewayConfig")
    return {
        "project": project,
        "location": location,
        "engine_id": engine_id,
        "resource_name": data.get("name"),
        "display_name": data.get("displayName"),
        "effective_identity": effective_identity,
        "iam_principal": iam_principal,
        "agent_gateway_config": gateway_config,
        "create_time": data.get("createTime"),
        "update_time": data.get("updateTime"),
        "adc_project": adc_project,
        "proof": {
            "identity_present": bool(iam_principal),
            "gateway_bound": bool(gateway_config),
        },
    }


def main() -> None:
    args = _args()
    print(
        json.dumps(inspect(args.project, args.location, args.engine_id), indent=2, sort_keys=True)
    )


if __name__ == "__main__":
    main()
