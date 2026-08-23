"""Deploy SentinelFlow's governed ADK application to Google Agent Runtime.

The command is dry-run by default.  A real deployment requires ``--execute``
and valid Application Default Credentials.  The script never prints tokens and
never creates more than one resource implicitly: callers should pass an
existing resource name to their own update workflow rather than rerunning this
creation command blindly.
"""

from __future__ import annotations

import argparse
import json
import os
from dataclasses import asdict, dataclass
from typing import Any, Optional

from runtime.managed_adk import MANAGED_MODEL, build_agent_runtime_app


@dataclass(frozen=True)
class DeploymentPlan:
    project: str
    location: str
    model: str
    identity_type: str
    display_name: str
    tracing_enabled: bool


def _parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Deploy SentinelFlow to Google Agent Runtime")
    parser.add_argument(
        "--project",
        default=os.getenv("GOOGLE_CLOUD_PROJECT", "telos-agent"),
        help="Google Cloud project ID",
    )
    parser.add_argument(
        "--location",
        default=os.getenv("GOOGLE_CLOUD_LOCATION", "us-central1"),
        help="Google Cloud region",
    )
    parser.add_argument(
        "--display-name",
        default="sentinelflow-p11-dev",
        help="Managed runtime display name",
    )
    parser.add_argument(
        "--execute",
        action="store_true",
        help="Actually create the Agent Runtime resource. Without this flag the command is dry-run only.",
    )
    return parser.parse_args()


def _plan(args: argparse.Namespace) -> DeploymentPlan:
    return DeploymentPlan(
        project=args.project,
        location=args.location,
        model=MANAGED_MODEL,
        identity_type="AGENT_IDENTITY",
        display_name=args.display_name,
        tracing_enabled=True,
    )


def _resource_name(remote_app: Any) -> Optional[str]:
    for attr in ("resource_name", "name"):
        value = getattr(remote_app, attr, None)
        if value:
            return str(value)
    api_resource = getattr(remote_app, "api_resource", None)
    value = getattr(api_resource, "name", None) if api_resource is not None else None
    return str(value) if value else None


def deploy(args: argparse.Namespace) -> dict[str, Any]:
    plan = _plan(args)
    if not args.execute:
        return {
            "status": "DRY_RUN",
            "plan": asdict(plan),
            "message": "No Google Cloud resources were created.",
        }

    if not args.project or not args.location:
        raise RuntimeError("project and location are required for live Agent Runtime deployment")

    try:
        import google.auth
        import vertexai
        from vertexai import types
    except Exception as exc:  # pragma: no cover - dependency gate
        raise RuntimeError("Google Agent Platform SDK dependencies are not installed") from exc

    credentials, adc_project = google.auth.default()
    if credentials is None:
        raise RuntimeError("Application Default Credentials are unavailable")

    # The configured project is authoritative; ADC's quota/default project may
    # legitimately differ but is reported for operator visibility.
    client = vertexai.Client(
        project=args.project,
        location=args.location,
        http_options={"api_version": "v1beta1"},
    )
    app = build_agent_runtime_app(enable_tracing=True)

    remote_app = client.agent_engines.create(
        agent=app,
        config={
            "display_name": args.display_name,
            "identity_type": types.IdentityType.AGENT_IDENTITY,
            "requirements": [
                "google-adk>=2.7.1,<3.0.0",
                "google-genai>=2.18.1,<3.0.0",
                "google-cloud-aiplatform[agent_engines,adk]>=1.111.0,<3.0.0",
                "pydantic>=2.10.4,<3.0.0",
                "httpx>=0.27.0,<1.0.0",
            ],
            "env_vars": {
                "GOOGLE_CLOUD_PROJECT": args.project,
                "GOOGLE_CLOUD_LOCATION": args.location,
                "SENTINEL_PLATFORM_MODE": "managed",
                "SENTINEL_AI_MODE": "live",
            },
        },
    )

    return {
        "status": "CREATED",
        "project": args.project,
        "location": args.location,
        "adc_project": adc_project,
        "resource_name": _resource_name(remote_app),
        "identity_type": "AGENT_IDENTITY",
        "model": MANAGED_MODEL,
    }


def main() -> None:
    args = _parse_args()
    result = deploy(args)
    # Output contains resource metadata only; never credentials/tokens.
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
