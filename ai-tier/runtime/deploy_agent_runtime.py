"""Deploy SentinelFlow's fixed Google ADK fleet to Agent Runtime.

Safety properties:
- dry-run by default;
- a real deployment requires ``--execute`` and ADC;
- Agent Identity is requested at *creation* time;
- an optional existing Agent-to-Anywhere Gateway can be bound at creation;
- no token, credential or financial payload is printed.

This command creates a new reasoning engine when ``--execute`` is supplied. It
therefore intentionally does not auto-retry creation or silently create a second
resource. Reuse/update of an existing engine is handled by the P17 live runbook.
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
    agent_gateway: Optional[str]
    vertex_ai_backend: bool


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
        "--agent-gateway",
        default=os.getenv("SENTINEL_AGENT_GATEWAY_RESOURCE", ""),
        help=(
            "Optional full Agent-to-Anywhere Gateway resource, for example "
            "projects/PROJECT/locations/us-central1/agentGateways/NAME"
        ),
    )
    parser.add_argument(
        "--execute",
        action="store_true",
        help="Actually create the Agent Runtime resource. Without this flag the command is dry-run only.",
    )
    return parser.parse_args()


def _plan(args: argparse.Namespace) -> DeploymentPlan:
    gateway = args.agent_gateway.strip() or None
    return DeploymentPlan(
        project=args.project,
        location=args.location,
        model=MANAGED_MODEL,
        identity_type="AGENT_IDENTITY",
        display_name=args.display_name,
        tracing_enabled=True,
        agent_gateway=gateway,
        vertex_ai_backend=True,
    )


def _resource_name(remote_app: Any) -> Optional[str]:
    for attr in ("resource_name", "name"):
        value = getattr(remote_app, attr, None)
        if value:
            return str(value)
    api_resource = getattr(remote_app, "api_resource", None)
    value = getattr(api_resource, "name", None) if api_resource is not None else None
    return str(value) if value else None


def _build_config(args: argparse.Namespace, identity_type: Any) -> dict[str, Any]:
    config: dict[str, Any] = {
        "display_name": args.display_name,
        "identity_type": identity_type,
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
            # Managed ADK model calls use Vertex AI + ADC/Agent Identity rather
            # than requiring a long-lived Gemini API key in the Runtime.
            "GOOGLE_GENAI_USE_VERTEXAI": "TRUE",
            "SENTINEL_PLATFORM_MODE": "managed",
            "SENTINEL_AI_MODE": "live",
        },
    }

    gateway = args.agent_gateway.strip()
    if gateway:
        expected_prefix = f"projects/{args.project}/locations/{args.location}/agentGateways/"
        if not gateway.startswith(expected_prefix):
            raise RuntimeError(
                "for the hackathon single-project deployment, --agent-gateway must be in "
                f"the same project/region and start with {expected_prefix!r}"
            )
        config["agent_gateway_config"] = {
            "agent_to_anywhere_config": {"agent_gateway": gateway}
        }

    return config


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

    client = vertexai.Client(
        project=args.project,
        location=args.location,
        http_options={"api_version": "v1beta1"},
    )
    app = build_agent_runtime_app(enable_tracing=True)
    config = _build_config(args, types.IdentityType.AGENT_IDENTITY)

    remote_app = client.agent_engines.create(agent=app, config=config)
    resource_name = _resource_name(remote_app)
    if not resource_name:
        raise RuntimeError(
            "Agent Runtime create returned without a resource name; do not treat the deployment as proven"
        )

    return {
        "status": "CREATED",
        "project": args.project,
        "location": args.location,
        "adc_project": adc_project,
        "resource_name": resource_name,
        "identity_type": "AGENT_IDENTITY",
        "model": MANAGED_MODEL,
        "agent_gateway": plan.agent_gateway,
        "vertex_ai_backend": True,
    }


def main() -> None:
    args = _parse_args()
    result = deploy(args)
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
