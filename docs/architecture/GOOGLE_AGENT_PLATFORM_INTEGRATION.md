# Google Enterprise Agent Platform Integration (Phase P11)

## 1. Architectural Authority Boundary

SentinelFlow integrates with Google Cloud's Enterprise Agent Platform to provide managed hosting, identity attestation, network governance, and observability for its autonomous AI specialist fleet.

### Fundamental Principle: Managed Infrastructure is Not Financial Authority
$$\text{ManagedInfrastructure} \neq \text{FinancialAuthority}$$
$$\text{AgentRuntime} \neq \text{WorkflowAuthority}$$
$$\text{AgentIdentity} \neq \text{ToolAuthorization}$$
$$\text{AgentGatewayAllow} \neq \text{PolicyAllow}$$
$$\text{AgentRegistryPresence} \neq \text{CapabilityGrant}$$
$$\text{MemoryBankRecall} \neq \text{Evidence}$$
$$\text{ModelArmorPass} \neq \text{Authorization}$$

The authoritative execution conjunction remains:
$$\text{Execute} = \text{IdentityValid} \land \text{ToolCapabilityValid} \land \text{ToolManifestValid} \land \text{PolicyAllowed} \land \text{ObligationsSatisfied} \land \text{ResourceFresh} \land \text{DeterministicVerification} \land \text{HumanAuthorizationWhereRequired}$$

Google managed infrastructure satisfies infrastructure terms (process hosting, network routing, SPIFFE identity attestation, tracing). It cannot bypass or substitute for SentinelFlow's deterministic Go control plane.

---

## 2. Platform Architecture & Layered Security Topology

```
+-----------------------------------------------------------------------------------+
|                        Google Agent Runtime (Managed Hosting)                     |
|  - Packages Fixed 6-Agent Fleet: Commander, Diagnosis, PolicySLA, Memory,          |
|    Remediation, Verifier                                                          |
|  - Gemini 3.5 Flash Model Reasoning                                               |
|  - Identity Type: AGENT_IDENTITY (SPIFFE: spiffe://<project>/agent/<slug>)        |
+-----------------------------------------------------------------------------------+
                                       │
                                       ▼ (Outbound Egress)
+-----------------------------------------------------------------------------------+
|                        Google Agent Gateway (Default-Deny Egress)                 |
|  - Default Deny: All unregistered destinations and external IPs are blocked (403)  |
|  - Permitted Target: /internal/agent-tools (SentinelFlow Go Ingress)              |
|  - IAP Authorization: roles/iap.egressor bound to Agent Identity                  |
|  - Model Armor on Gateway (Defense-in-Depth network perimeter filter)             |
+-----------------------------------------------------------------------------------+
                                       │
                                       ▼ (Authenticated HTTPS Ingress)
+-----------------------------------------------------------------------------------+
|                        SentinelFlow Go Ingress: /internal/agent-tools             |
|  - AgentIdentityValidator: Decodes & verifies SPIFFE URI against project boundary |
|  - Maps Principal -> FixedCanonicalRoster (AgentManifest & Autonomy Ceiling)      |
|  - Strips untrusted client-supplied headers (X-Agent-Name, model tenant_id)       |
+-----------------------------------------------------------------------------------+
                                       │
                                       ▼
+-----------------------------------------------------------------------------------+
|                        SentinelFlow Tool Gateway (Business Control Authority)     |
|  - Evaluates 12-Step Lifecycle: Manifest allowlist, Autonomy ceiling,             |
|    Policy Engine verdict, Resource freshness, Dual-control human requirements    |
|  - Deterministic Persistence & Audit Ledger in Go / PostgreSQL                    |
+-----------------------------------------------------------------------------------+
```

---

## 3. Component Launch Status & Role Division

| Google Service | Launch Stage | SentinelFlow Role | Authority Status |
|---|---|---|---|
| **Agent Runtime** | GA / Managed | Container hosting for Python ADK fleet | Ephemeral process; no workflow authority |
| **Agent Identity** | Supported with Runtime | Cryptographic SPIFFE attestation | Workload identity; no tool permissions |
| **Agent Registry v1** | GA | Inventory of agents and approved endpoints | Inventory metadata; no roster expansion |
| **Agent Gateway** | GA | Egress proxy with default-deny routing | Network reachability; no policy authority |
| **Model Armor on Gateway**| GA | Defense-in-depth perimeter inspection | Perimeter filter; secondary to P09 app boundary |
| **Agent Observability** | GA | OpenTelemetry / Cloud Trace spans | Privacy metadata only; zero raw PII/payload |
| **Managed Agents API** | Preview | **Not Used** (Preserved custom ADK fleet) | N/A |

---

## 4. Layered Denial Matrix

| Scenario | Agent Gateway | SentinelFlow Tool Gateway | Final Execution | Rationale |
|---|---|---|---|---|
| **Unregistered Destination** (`evil.com`) | **DENY (403)** | N/A (Never reached) | **BLOCKED** | Default-deny network boundary |
| **Registered Endpoint + Allowed Tool** (`incident.get`) | **ALLOW (200)** | **ALLOW (200)** | **EXECUTED** | Both network and capability terms satisfied |
| **Registered Endpoint + Unauthorized Tool** (`remediation.candidate.create` by Verifier) | **ALLOW (200)** | **DENY (403)** | **BLOCKED** | $\text{NetworkReachable} \neq \text{ToolExecutable}$ |
| **Model Armor PASS + Policy Engine DENY** | **ALLOW (200)** | **DENY (403)** | **BLOCKED** | $\text{ModelArmorPass} \neq \text{PolicyAllow}$ |
| **Unknown SPIFFE Principal** | **ALLOW (200)** | **DENY (403)** | **BLOCKED** | Identity rejected at Go ingress validator |

---

## 5. Observability & Privacy Guarantee

$$\text{TraceMetadataAllowed} \neq \text{PromptPayloadLoggingAllowed}$$

- **Trace Spans**: Exported with sanitized metadata: workflow hash, agent name, stage, tool ID, policy decision ref, and latency.
- **Data Minimization Filter**: `FinancialPrivacySpanProcessor` actively scrubs 94-char NACHA lines, 10-17 digit bank account numbers, 9-digit ABA routing numbers, and credentials before export.
- **Environment Flags**:
  - `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=NO_CONTENT`
  - `ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS=false`
