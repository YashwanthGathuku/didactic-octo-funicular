"""Deploy SentinelFlow's fixed Google ADK fleet to Agent Runtime.

Dry-run is the default. A live create requires ``--execute``, ADC, an explicit
GCS staging bucket, and (when supplied) an Agent-to-Anywhere Gateway in the same
project/region used by this hackathon deployment.

Managed infrastructure never becomes SentinelFlow financial authority.
"""

from __future__ import annotations

import argparse
from dataclasses import asdict, dataclass
import json
import os
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
    staging_bucket: Optional[str]
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
        default="sentinelflow-p17-dev",
        help="Managed runtime display name",
    )
    parser.add_argument(
        "--staging-bucket",
        default=os.getenv("SENTINEL_AGENT_RUNTIME_STAGING_BUCKET", ""),
        help="Existing gs:// bucket used to stage Agent Runtime deployment artifacts",
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
        help="Create the Agent Runtime resource. Without this flag no cloud resource is created.",
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
        agent_gateway=args.agent_gateway.strip() or None,
        staging_bucket=args.staging_bucket.strip() or None,
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


def _validate_live_args(args: argparse.Namespace) -> None:
    if not args.project or not args.location:
        raise RuntimeError("project and location are required for live Agent Runtime deployment")
    staging = args.staging_bucket.strip()
    if not staging.startswith("gs://"):
        raise RuntimeError(
            "--execute requires --staging-bucket=gs://... (or SENTINEL_AGENT_RUNTIME_STAGING_BUCKET)"
        )


def _build_config(args: argparse.Namespace, identity_type: Any) -> dict[str, Any]:
    config: dict[str, Any] = {
        "display_name": args.display_name,
        "identity_type": identity_type,
        "requirements": [
            "google-cloud-aiplatform[agent_engines,adk]>=1.111.0,<3.0.0",
            "google-adk[agent-identity]>=2.7.1,<3.0.0",
            "google-genai>=2.18.1,<3.0.0",
            "pydantic>=2.10.4,<3.0.0",
            "httpx>=0.27.0,<1.0.0",
        ],
        "staging_bucket": args.staging_bucket.strip(),
        "env_vars": {
            "GOOGLE_CLOUD_PROJECT": args.project,
            "GOOGLE_CLOUD_LOCATION": args.location,
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
        config["agent_gateway_config"] = {"agent_to_anywhere_config": {"agent_gateway": gateway}}
        # Current Agent Runtime guidance requires this opt-out when routing
        # Google service calls through Agent Gateway with Agent Identity. It is
        # deliberately scoped to managed Runtime, not local development.
        config["env_vars"]["GOOGLE_API_PREVENT_AGENT_TOKEN_SHARING_FOR_GCP_SERVICES"] = False

    return config


def deploy(args: argparse.Namespace) -> dict[str, Any]:
    plan = _plan(args)
    if not args.execute:
        return {
            "status": "DRY_RUN",
            "plan": asdict(plan),
            "message": "No Google Cloud resources were created.",
        }

    _validate_live_args(args)

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
            "Agent Runtime create returned without a resource name; deployment is not proven"
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
        "staging_bucket": plan.staging_bucket,
        "vertex_ai_backend": True,
    }


def main() -> None:
    args = _parse_args()
    print(json.dumps(deploy(args), indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
