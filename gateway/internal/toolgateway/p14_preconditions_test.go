package toolgateway

import (
	"errors"
	"testing"
)

func TestVerifyResourcePreconditions_AllDeclaredFieldsMatch(t *testing.T) {
	ctx := &TrustedExecutionContext{
		ArtifactSHA256:   "abc123",
		ResourceVersion:  7,
		WorkflowState:    "INVESTIGATING",
		PolicyBundleHash: "policy-hash-1",
	}
	pre := &ResourcePreconditions{
		ExpectedArtifactSHA256: "abc123",
		ExpectedRowVersion:     7,
		ExpectedWorkflowState:  "INVESTIGATING",
		ExpectedPolicyBundle:   "policy-hash-1",
	}
	if err := VerifyResourcePreconditions(pre, ctx); err != nil {
		t.Fatalf("matching preconditions rejected: %v", err)
	}
}

func TestVerifyResourcePreconditions_WorkflowStateMismatchFailsClosed(t *testing.T) {
	ctx := &TrustedExecutionContext{WorkflowState: "HUMAN_REVIEW"}
	pre := &ResourcePreconditions{ExpectedWorkflowState: "VERIFIED"}
	if err := VerifyResourcePreconditions(pre, ctx); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
}

func TestVerifyResourcePreconditions_PolicyBundleMismatchFailsClosed(t *testing.T) {
	ctx := &TrustedExecutionContext{PolicyBundleHash: "new-policy"}
	pre := &ResourcePreconditions{ExpectedPolicyBundle: "old-policy"}
	if err := VerifyResourcePreconditions(pre, ctx); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
}

func TestVerifyResourcePreconditions_DeclaredContextCannotBeMissing(t *testing.T) {
	cases := []struct {
		name string
		pre  *ResourcePreconditions
		ctx  *TrustedExecutionContext
	}{
		{"artifact", &ResourcePreconditions{ExpectedArtifactSHA256: "sha"}, &TrustedExecutionContext{}},
		{"row-version", &ResourcePreconditions{ExpectedRowVersion: 1}, &TrustedExecutionContext{}},
		{"workflow-state", &ResourcePreconditions{ExpectedWorkflowState: "VERIFIED"}, &TrustedExecutionContext{}},
		{"policy-bundle", &ResourcePreconditions{ExpectedPolicyBundle: "bundle"}, &TrustedExecutionContext{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifyResourcePreconditions(tc.pre, tc.ctx); !errors.Is(err, ErrPreconditionFailed) {
				t.Fatalf("expected fail-closed precondition error, got %v", err)
			}
		})
	}
}
