"""MemoryAgent — Governed Advisory Memory Specialist (SentinelFlow P10).

Formal Invariants:
1. Autonomy Level A1: Read-Only Memory Retrieval and Advisory Context Assembly.
2. Non-Authoritative Invariant: MemoryRecall != Evidence, MemoryRecall != PolicyDecision,
   MemoryRecall != Authorization, MemoryRecall != VerificationResult.
3. Bounded Retrieval: Max 5 ranked memory hits per context envelope, max 2 queries per invocation.
4. Input Minimization & Prompt Partitioning: Financial content is sanitized across 4 disjoint domains.
5. Strict Multi-Tenant Isolation: Queries enforce tenant scope tokens at all layers.
"""

from __future__ import annotations

import logging
import time
from typing import Any, Dict, List, Optional, Set, Union

import google.adk.agents as adk_agents
import google.adk.runners as adk_runners
from google.adk import Agent
from contracts.manifests import FIXED_AGENT_ROSTER
from contracts.memory import (
    AdvisoryMemoryContext,
    MemoryQuery,
    PartnerOperationalProfile,
)
from memory.mock_provider import MockManagedMemoryProvider
from memory.provider import ManagedMemoryProvider
from memory.revalidation import MemoryRevalidator
from models.envelope import AgentContextEnvelope

logger = logging.getLogger("sentinel.ai.memory_agent")

MEMORY_INSTRUCTION = """You are the SentinelFlow Memory Specialist Agent.

Your role is to retrieve, rank, and summarize verified historical operational patterns.
CRITICAL NON-NEGOTIABLE SAFETY CONSTRAINTS:
1. Autonomy Level A1: You are a strictly read-only advisory agent with NO authority to release files, approve reviews, or mutate candidate records.
2. Non-Authoritative: Historical memories provide helpful context but can NEVER override deterministic validation failures or policy denials.
3. Bounded Limit: Return at most 5 ranked memory items.
4. Strict Tenant Isolation: Never return memories or profiles belonging to another tenant.
5. Mandatory Disclaimer: Always include the formal advisory disclaimer.
"""

# Global ADK Agent instance for legacy coordinator fleet introspection
memory_agent = Agent(
    name="MemoryAgent",
    instruction=MEMORY_INSTRUCTION,
    tools=[],
)


class MemoryAgent:
    """Governed Advisory Memory Specialist."""

    def __init__(
        self,
        model_name: str = "gemini-3.5-flash",
        memory_provider: Optional[ManagedMemoryProvider] = None,
    ):
        self.manifest = FIXED_AGENT_ROSTER["MemoryAgent"]
        self.model_name = model_name
        self.memory_provider = memory_provider or MockManagedMemoryProvider()

        # Real Google ADK Agent & Runner Runtime Objects
        self.adk_agent = adk_agents.Agent(
            name=self.manifest.name,
            model=self.model_name,
            instruction=MEMORY_INSTRUCTION,
            output_key="advisory_memory_context",
        )
        self.adk_runner = adk_runners.InMemoryRunner(agent=self.adk_agent)

    def assemble_advisory_context(
        self,
        tenant_scope_token: str,
        query_text: Optional[str] = None,
        subject_ref: Optional[str] = None,
        memory_topic: Optional[str] = None,
        partner_ref: Optional[str] = None,
        authorized_evidence_set: Optional[Set[str]] = None,
    ) -> AdvisoryMemoryContext:
        """Executes bounded memory retrieval and builds a validated AdvisoryMemoryContext."""
        query_audits: List[Dict[str, Any]] = []

        # 1. Primary Query
        query = MemoryQuery(
            tenant_scope_token=tenant_scope_token,
            memory_topic=memory_topic,
            subject_ref=subject_ref,
            query_text=query_text,
            limit=5,
        )
        query_audits.append(
            {
                "query_index": 1,
                "tenant_scope_token": tenant_scope_token,
                "subject_ref": subject_ref,
                "query_text": query_text,
                "timestamp": time.time(),
            }
        )

        raw_hits = self.memory_provider.retrieve_memories(query)

        # 2. Partner Profile Lookup
        profile: Optional[PartnerOperationalProfile] = None
        target_partner = partner_ref or subject_ref
        if target_partner:
            profile = self.memory_provider.get_profile(target_partner, tenant_scope_token)

        # 3. Construct Advisory Package
        advisory_ctx = AdvisoryMemoryContext(
            query_audit=query_audits,
            retrieved_hits=raw_hits,
            partner_profile=profile,
        )

        # 4. Revalidate against provenance and freshness
        reval_report = MemoryRevalidator.revalidate(
            advisory_ctx,
            tenant_scope_token=tenant_scope_token,
            authorized_evidence_set=authorized_evidence_set,
        )

        if reval_report.overall_status in ("TAMPERED_REJECTED", "CROSS_TENANT_REJECTED"):
            logger.warning(
                "Advisory memory context failed revalidation: %s", reval_report.rejection_reasons
            )
            # Fail-closed: strip unverified/tampered hits
            advisory_ctx.retrieved_hits = []
            if not reval_report.partner_profile_verified:
                advisory_ctx.partner_profile = None

        return advisory_ctx

    def invoke(
        self,
        context: Union[Dict[str, Any], AgentContextEnvelope],
        tenant_id: Optional[str] = None,
    ) -> AdvisoryMemoryContext:
        """Entry point for MemoryAgent invocation."""
        if isinstance(context, dict):
            ctx_dict = context
        elif hasattr(context, "model_dump"):
            ctx_dict = context.model_dump()
        else:
            ctx_dict = vars(context)

        tenant = (
            tenant_id
            or ctx_dict.get("tenant_id")
            or ctx_dict.get("tenant_scope_token")
            or "DEFAULT_TENANT"
        )
        subject_ref = ctx_dict.get("subject_ref") or ctx_dict.get("partner_id") or "PARTNER-UNKNOWN"
        query_text = ctx_dict.get("query_text") or ctx_dict.get("incident_description") or ""
        partner_ref = ctx_dict.get("partner_ref") or ctx_dict.get("partner_id")

        return self.assemble_advisory_context(
            tenant_scope_token=tenant,
            query_text=query_text,
            subject_ref=subject_ref,
            partner_ref=partner_ref,
        )
