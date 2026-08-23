# SentinelFlow Phase P11 — Cloud Identity & Ingress Security Graph

This specification defines the cryptographic identity, authentication, and authorization transitions governing agent tool execution in SentinelFlow.

---

## 1. End-to-End Security Graph

```mermaid
graph TD
    User([Human Operator / Trigger]) -->|Trigger Incident| Runtime[Google Agent Runtime]
    
    subgraph Google Agent Platform
        Runtime -->|SPIFFE Principal Header| Identity[Agent Identity]
        Identity -->|Egress Traffic| Gateway[Agent Gateway]
        Gateway -->|Default Deny Filter| RegistryCheck{Registered in Agent Registry?}
        RegistryCheck -->|No| Drop[403 Deny Drop]
        RegistryCheck -->|Yes| IAPEval[IAP Egress Token roles/iap.egressor]
    end

    subgraph SentinelFlow Go Control Plane
        IAPEval -->|HTTPS Ingress| Ingress[/internal/agent-tools]
        Ingress --> Validator[AgentIdentityValidator]
        Validator -->|Verify Project & Token| RosterLookup{In Fixed Canonical Roster?}
        RosterLookup -->|No| RejectAuth[403 ErrAgentNotRegistered]
        RosterLookup -->|Yes| MapIdentity[RegisteredAgentIdentity & Manifest]
        
        MapIdentity --> ToolGW[SentinelFlow Tool Gateway]
        ToolGW --> AutonomyCheck{Autonomy Ceiling & Allowlist?}
        AutonomyCheck -->|Denied| RejectTool[403 ErrUnauthorizedToolCall]
        AutonomyCheck -->|Allowed| PolicyEngine[Deterministic Policy Engine]
        
        PolicyEngine --> PolicyCheck{Policy Verdict == ALLOW?}
        PolicyCheck -->|DENY| RejectPolicy[403 Policy Blocked]
        PolicyCheck -->|ALLOW| ExecService[Service Execution & Immutable Ledger]
    end
```

---

## 2. Transition Authority Matrix

| Transition Edge | Authentication Mechanism | Authorization Check | Data Classification | Audit Source |
|---|---|---|---|---|
| **Runtime -> Gateway** | TLS Client Certificate / Build Root CA | Registered Destination Egress Rule | Sanitized Metadata | Gateway Egress Log |
| **Gateway -> Go Ingress** | IAP OIDC JWT / SPIFFE Header | `roles/iap.egressor` on Destination | Masked Payload | Cloud Audit Log / IAP |
| **Go Ingress -> Tool Gateway**| In-memory Context Principal | `FixedCanonicalRoster` + Project Scope | Redacted Envelope | Go Application Log |
| **Tool Gateway -> Policy Engine** | Internal Go Function Call | Autonomy Tier (A1/A2) + Allowlist | High-Assurance Context | Go Audit Ledger Record |
| **Policy Engine -> Persistence** | DB Transaction Scope | Deterministic Rulepack Evaluation | Authoritative Business State | Linear Hash-Chained Ledger |

---

## 3. Decoupling Axioms

1. **Identity is Not Authority**:
   $$\text{AgentIdentityValid} \not\implies \text{ToolAuthorization}$$
2. **Network Reachability is Not Tool Execution**:
   $$\text{NetworkReachable} \not\implies \text{ToolExecutable}$$
3. **Cloud Registry is Not Application Roster**:
   $$\text{RegistryContains}(A) \not\implies \text{SentinelFlowRosterAllows}(A)$$
4. **Perimeter Pass is Not Deterministic Decision**:
   $$\text{ModelArmorPass} \not\implies \text{PolicyAllow}$$
