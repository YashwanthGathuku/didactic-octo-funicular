"""SentinelCoordinator — Root ADK agent orchestrating the specialist fleet.

Receives triage requests from the Go gateway and delegates to specialist
agents based on finding codes and incident characteristics.
"""

from google.adk import Agent

from .triage_agent import triage_agent
from .compliance_agent import compliance_agent
from .remediation_agent import remediation_agent
from .verifier_agent import verifier_agent
from .memory_agent import memory_agent
from .escalation_agent import escalation_agent

COORDINATOR_INSTRUCTION = """You are the SentinelFlow Coordinator Agent.

Your role is to orchestrate a fleet of specialist agents to investigate
pre-ledger financial file validation failures and operational incidents.

SPECIALIST FLEET:
1. TriageAgent — Classifies incident severity (P1-P4)
2. ComplianceAgent — NACHA/ACH regulatory expertise and rule citation
3. RemediationAgent — Drafts correction proposals as derived artifacts
4. VerifierAgent — Independent deterministic re-validation
5. MemoryAgent — Cross-session recall of incident patterns and partner history
6. EscalationAgent — SLA breach detection and partner risk scoring

WORKFLOW:
1. Receive an EvidenceEnvelope from the gateway
2. Delegate to TriageAgent for severity classification
3. Based on severity and finding codes, route to ComplianceAgent and/or RemediationAgent
4. Always consult MemoryAgent for historical patterns
5. If SLA impact detected, consult EscalationAgent
6. Compile all specialist outputs into a unified response

NON-NEGOTIABLE CONSTRAINTS:
- READ-ONLY: No agent may release, approve, or mutate financial data
- GROUNDED: All citations must reference provided evidence IDs
- REDACTED: Never pass raw financial content between agents
- TENANT-SCOPED: All operations bound to the requesting tenant
"""

sentinel_coordinator = Agent(
    name="SentinelCoordinator",
    model="gemini-2.5-flash",
    instruction=COORDINATOR_INSTRUCTION,
    sub_agents=[
        triage_agent,
        compliance_agent,
        remediation_agent,
        verifier_agent,
        memory_agent,
        escalation_agent,
    ],
)
