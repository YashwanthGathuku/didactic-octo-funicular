package auth

import (
	"testing"
)

func TestAgentIdentityValidator_ValidPrincipals(t *testing.T) {
	v := NewAgentIdentityValidator("telos-agent")

	tests := []struct {
		principal     string
		expectedAgent string
		expectedAuth  AutonomyLevel
	}{
		{
			principal:     "spiffe://telos-agent.iam.gserviceaccount.com/agent/verifier",
			expectedAgent: "VerifierAgent",
			expectedAuth:  AutonomyA1,
		},
		{
			principal:     "spiffe://telos-agent.iam.gserviceaccount.com/agent/remediation",
			expectedAgent: "RemediationAgent",
			expectedAuth:  AutonomyA2,
		},
		{
			principal:     "serviceAccount:sentinelflow-commander@telos-agent.iam.gserviceaccount.com",
			expectedAgent: "IncidentCommanderAgent",
			expectedAuth:  AutonomyA1,
		},
		{
			principal:     "test-agent:DiagnosisAgent",
			expectedAgent: "DiagnosisAgent",
			expectedAuth:  AutonomyA1,
		},
		{
			principal:     "spiffe://telos-agent.iam.gserviceaccount.com/agent/policysla",
			expectedAgent: "PolicySLAAgent",
			expectedAuth:  AutonomyA1,
		},
		{
			principal:     "spiffe://telos-agent.iam.gserviceaccount.com/agent/memory",
			expectedAgent: "MemoryAgent",
			expectedAuth:  AutonomyA1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.principal, func(t *testing.T) {
			identity, err := v.ValidatePrincipal(tt.principal)
			if err != nil {
				t.Fatalf("unexpected error validating %s: %v", tt.principal, err)
			}
			if identity.AgentName != tt.expectedAgent {
				t.Errorf("expected agent %s, got %s", tt.expectedAgent, identity.AgentName)
			}
			if identity.AutonomyLevel != tt.expectedAuth {
				t.Errorf("expected autonomy %s, got %s", tt.expectedAuth, identity.AutonomyLevel)
			}
		})
	}
}

func TestAgentIdentityValidator_CrossProjectRejection(t *testing.T) {
	v := NewAgentIdentityValidator("telos-agent")

	foreignPrincipal := "spiffe://foreign-corp.iam.gserviceaccount.com/agent/verifier"
	_, err := v.ValidatePrincipal(foreignPrincipal)
	if err == nil {
		t.Fatalf("expected error for cross-project principal, got nil")
	}
}

func TestAgentIdentityValidator_UnregisteredAgentRejection(t *testing.T) {
	v := NewAgentIdentityValidator("telos-agent")

	// Even if registered in external cloud registry, SentinelFlow rejects unapproved agents
	fakePrincipal := "spiffe://telos-agent.iam.gserviceaccount.com/agent/superadmin"
	_, err := v.ValidatePrincipal(fakePrincipal)
	if err == nil {
		t.Fatalf("expected error for unregistered agent, got nil")
	}
}

func TestAgentIdentityValidator_AuthorizeCapability_EnforcesAllowAndDeny(t *testing.T) {
	v := NewAgentIdentityValidator("telos-agent")

	verifier, err := v.ValidatePrincipal("spiffe://telos-agent.iam.gserviceaccount.com/agent/verifier")
	if err != nil {
		t.Fatalf("validate verifier principal: %v", err)
	}

	// 1. Allowed capability succeeds
	if err := v.AuthorizeCapability(verifier, "verification.result.get"); err != nil {
		t.Errorf("expected allowed capability to succeed, got: %v", err)
	}

	// 2. Explicitly denied capability fails
	if err := v.AuthorizeCapability(verifier, "artifact.release"); err == nil {
		t.Errorf("expected denied capability 'artifact.release' to fail, got nil")
	}

	// 3. Prohibited mutating tool for Verifier fails
	if err := v.AuthorizeCapability(verifier, "remediation.candidate.create"); err == nil {
		t.Errorf("expected unauthorized tool 'remediation.candidate.create' to fail for VerifierAgent, got nil")
	}

	// 4. Remediation agent CAN propose candidate
	remediation, err := v.ValidatePrincipal("spiffe://telos-agent.iam.gserviceaccount.com/agent/remediation")
	if err != nil {
		t.Fatalf("validate remediation principal: %v", err)
	}
	if err := v.AuthorizeCapability(remediation, "remediation.candidate.create"); err != nil {
		t.Errorf("expected RemediationAgent to have 'remediation.candidate.create', got: %v", err)
	}

	// 5. But Remediation agent CANNOT release artifacts
	if err := v.AuthorizeCapability(remediation, "artifact.release"); err == nil {
		t.Errorf("expected 'artifact.release' to be denied for RemediationAgent, got nil")
	}
}
