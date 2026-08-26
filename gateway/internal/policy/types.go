package policy

import (
	"time"
)

// Decision represents the exactly four top-level deterministic policy decisions.
type Decision string

const (
	DecisionAllow                Decision = "ALLOW"
	DecisionDeny                 Decision = "DENY"
	DecisionAllowWithObligations Decision = "ALLOW_WITH_OBLIGATIONS"
	DecisionRequireHuman         Decision = "REQUIRE_HUMAN"
)

// PolicyDomain defines the functional scope of a policy.
type PolicyDomain string

const (
	DomainAgent            PolicyDomain = "AGENT"
	DomainArtifact         PolicyDomain = "ARTIFACT"
	DomainRemediation      PolicyDomain = "REMEDIATION"
	DomainTool             PolicyDomain = "TOOL"
	DomainRelease          PolicyDomain = "RELEASE"
	DomainEnterpriseAction PolicyDomain = "ENTERPRISE_ACTION"
)

// PolicyLayer defines the trust tier and evaluation precedence.
type PolicyLayer string

const (
	LayerNetworkExternal PolicyLayer = "NETWORK_EXTERNAL" // Layer 1 (Outermost/Network bounds)
	LayerSentinelSafety  PolicyLayer = "SENTINEL_SAFETY"  // Layer 2 (Foundational system invariants)
	LayerEnterprise      PolicyLayer = "ENTERPRISE"       // Layer 3 (Organization-wide rules)
	LayerTenant          PolicyLayer = "TENANT"           // Layer 4 (Tenant-specific policies)
	LayerPartner         PolicyLayer = "PARTNER"          // Layer 5 (Counterparty / partner rules)
)

// LayerPrecedence maps layers to strict numerical hierarchy (lower number = higher precedence).
func LayerPrecedence(layer PolicyLayer) int {
	switch layer {
	case LayerNetworkExternal:
		return 10
	case LayerSentinelSafety:
		return 20
	case LayerEnterprise:
		return 30
	case LayerTenant:
		return 40
	case LayerPartner:
		return 50
	default:
		return 1000
	}
}

// PolicyStatus represents the lifecycle state of a policy definition.
type PolicyStatus string

const (
	StatusDraft   PolicyStatus = "DRAFT"
	StatusActive  PolicyStatus = "ACTIVE"
	StatusRetired PolicyStatus = "RETIRED"
)

// ObligationType defines standard machine-readable obligation types.
type ObligationType string

const (
	ObligationDeterministicRevalidation ObligationType = "DETERMINISTIC_REVALIDATION"
	ObligationDualControl               ObligationType = "DUAL_CONTROL"
	ObligationCandidateOnly             ObligationType = "CANDIDATE_ONLY"
	ObligationImmutableParentRequired   ObligationType = "IMMUTABLE_PARENT_REQUIRED"
	ObligationMaxAttempts               ObligationType = "MAX_ATTEMPTS"
	ObligationHumanReview               ObligationType = "HUMAN_REVIEW"
	ObligationExactArtifactHash         ObligationType = "EXACT_ARTIFACT_HASH"
	ObligationAuditRequired             ObligationType = "AUDIT_REQUIRED"
	ObligationSandboxOnly               ObligationType = "SANDBOX_ONLY"
)

// Obligation represents a typed obligation with optional structured parameters.
type Obligation struct {
	Type       ObligationType         `json:"type"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// ProhibitionType defines standard machine-readable prohibition types.
type ProhibitionType string

const (
	ProhibitionMutateOriginal ProhibitionType = "MUTATE_ORIGINAL"
	ProhibitionRelease        ProhibitionType = "RELEASE"
	ProhibitionApprove        ProhibitionType = "APPROVE"
	ProhibitionExecuteSQL     ProhibitionType = "EXECUTE_SQL"
	// secret-scan-allow: standard policy prohibition identifier
	ProhibitionAccessSecret                   ProhibitionType = "ACCESS_SECRET"
	ProhibitionCrossTenantAccess              ProhibitionType = "CROSS_TENANT_ACCESS"
	ProhibitionIrreversibleFinancialAuthority ProhibitionType = "IRREVERSIBLE_FINANCIAL_AUTHORITY"
)

// Prohibition represents a typed prohibition.
type Prohibition struct {
	Type        ProhibitionType `json:"type"`
	Description string          `json:"description,omitempty"`
}

// ToolCapability defines typed tool capabilities evaluated by Tool Gateway (P04).
type ToolCapability string

const (
	CapabilityLookupFinding      ToolCapability = "lookup_finding"
	CapabilityLookupRule         ToolCapability = "lookup_nacha_rule"
	CapabilityCheckSLA           ToolCapability = "check_sla_status"
	CapabilityRecallMemory       ToolCapability = "recall_partner_history"
	CapabilityStoreMemory        ToolCapability = "store_memory"
	CapabilityProposeCandidate   ToolCapability = "propose_derived_artifact"
	CapabilityVerifyCandidate    ToolCapability = "verify_candidate"
	CapabilityDirectArtifactEdit ToolCapability = "direct_artifact_edit"
	CapabilityReleaseFile        ToolCapability = "release_file"
	CapabilityApproveRelease     ToolCapability = "approve_release"
	CapabilityExecuteSQL         ToolCapability = "execute_sql"
	// secret-scan-allow: standard capability identifier
	CapabilityAccessSecret      ToolCapability = "access_secret"
	CapabilityCrossTenantAccess ToolCapability = "cross_tenant_access"
)

// Standard Actions
const (
	ActionModifyOriginalArtifact = "MODIFY_ORIGINAL_ARTIFACT"
	ActionReleaseArtifact        = "RELEASE_ARTIFACT"
	ActionApproveRelease         = "APPROVE_RELEASE"
	ActionCreateCandidate        = "CREATE_CANDIDATE"
	ActionExecuteTool            = "EXECUTE_TOOL"
	ActionQueryDatabase          = "QUERY_DATABASE"
	// secret-scan-allow: standard policy action identifier
	ActionAccessSecret        = "ACCESS_SECRET"
	ActionCrossTenantQuery    = "CROSS_TENANT_QUERY"
	ActionGetIncident         = "GET_INCIDENT"
	ActionListFindings        = "LIST_FINDINGS"
	ActionGetArtifactMetadata = "GET_ARTIFACT_METADATA"
	ActionGetWorkflow         = "GET_WORKFLOW"
	ActionQueryAnalytics      = "QUERY_ANALYTICS"
)

// CapabilityProhibitions maps tool capabilities to the prohibition types that block them.
var CapabilityProhibitions = map[ToolCapability][]ProhibitionType{
	CapabilityDirectArtifactEdit: {ProhibitionMutateOriginal},
	CapabilityReleaseFile:        {ProhibitionRelease, ProhibitionIrreversibleFinancialAuthority},
	CapabilityApproveRelease:     {ProhibitionApprove, ProhibitionIrreversibleFinancialAuthority},
	CapabilityExecuteSQL:         {ProhibitionExecuteSQL},
	CapabilityAccessSecret:       {ProhibitionAccessSecret},
	CapabilityCrossTenantAccess:  {ProhibitionCrossTenantAccess},
}

// IsCapabilityProhibited determines if a required tool capability is prohibited by active prohibitions.
func IsCapabilityProhibited(cap ToolCapability, prohs []Prohibition) (bool, ProhibitionType) {
	blockedTypes, exists := CapabilityProhibitions[cap]
	if !exists {
		return false, ""
	}
	for _, proh := range prohs {
		for _, blocked := range blockedTypes {
			if proh.Type == blocked {
				return true, blocked
			}
		}
	}
	return false, ""
}

// PolicyManifestEntry represents an exact immutable policy version entry within a bundle manifest.
type PolicyManifestEntry struct {
	PolicyID    string `json:"policy_id"`
	Version     int    `json:"version"`
	ContentHash string `json:"content_hash"`
}

// PolicyBundleManifest tracks an exact immutable policy bundle.
type PolicyBundleManifest struct {
	BundleID   string                `json:"bundle_id"`
	Version    string                `json:"version"`
	BundleHash string                `json:"bundle_hash"`
	Manifest   []PolicyManifestEntry `json:"manifest"`
}

// PolicyDefinition defines a versioned, immutable rule in the deterministic policy engine.
type PolicyDefinition struct {
	PolicyID            string             `json:"policy_id"`
	Version             int                `json:"version"`
	Domain              PolicyDomain       `json:"domain"`
	Layer               PolicyLayer        `json:"layer"`
	Priority            int                `json:"priority"` // Higher number = higher priority within same layer
	Status              PolicyStatus       `json:"status"`
	EffectiveFrom       time.Time          `json:"effective_from"`
	EffectiveTo         *time.Time         `json:"effective_to,omitempty"`
	TenantID            *string            `json:"tenant_id,omitempty"`  // Nil = global
	PartnerID           *string            `json:"partner_id,omitempty"` // Nil = all partners
	SubjectConstraints  SubjectConstraint  `json:"subject_constraints"`
	ResourceConstraints ResourceConstraint `json:"resource_constraints"`
	Action              string             `json:"action"`
	Conditions          map[string]string  `json:"conditions,omitempty"`
	Effect              Decision           `json:"effect"` // ALLOW, DENY, ALLOW_WITH_OBLIGATIONS, REQUIRE_HUMAN
	Obligations         []Obligation       `json:"obligations,omitempty"`
	Prohibitions        []Prohibition      `json:"prohibitions,omitempty"`
	ReasonCode          string             `json:"reason_code"`
	SourceReference     string             `json:"source_reference,omitempty"`
	CreatedAt           time.Time          `json:"created_at"`
	ContentHash         string             `json:"content_hash"`
}

// SubjectConstraint specifies matching criteria for the acting entity.
type SubjectConstraint struct {
	Type        string   `json:"type,omitempty"`         // "AGENT", "USER", "SYSTEM", "*"
	ID          string   `json:"id,omitempty"`           // Exact ID or "*"
	Roles       []string `json:"roles,omitempty"`        // Allowed roles
	MaxAutonomy int      `json:"max_autonomy,omitempty"` // Maximum autonomy level (0-5)
	MinAutonomy int      `json:"min_autonomy,omitempty"` // Minimum autonomy level (0-5)
}

// ResourceConstraint specifies matching criteria for the target entity.
type ResourceConstraint struct {
	Type           string   `json:"type,omitempty"`           // "ARTIFACT", "DATABASE", "SECRET", "*"
	ID             string   `json:"id,omitempty"`             // Exact ID or "*"
	States         []string `json:"states,omitempty"`         // E.g. "QUARANTINED", "VALIDATED"
	Classification string   `json:"classification,omitempty"` // E.g. "FINANCIAL_PAYLOAD", "METADATA"
}

// PolicyEvaluationRequest is the authoritative, server-side context evaluated by the engine.
type PolicyEvaluationRequest struct {
	RequestID               string                 `json:"request_id"`
	TenantID                string                 `json:"tenant_id"`
	Subject                 PolicySubject          `json:"subject"`
	Action                  string                 `json:"action"`
	Resource                PolicyResource         `json:"resource"`
	Workflow                PolicyWorkflowContext  `json:"workflow"`
	Environment             PolicyEnvironment      `json:"environment"`
	AuthoritativeAttributes map[string]interface{} `json:"authoritative_attributes,omitempty"`
}

// PolicySubject contains the authenticated subject attributes.
type PolicySubject struct {
	Type          string   `json:"type"` // "AGENT", "USER", "SYSTEM"
	ID            string   `json:"id"`
	Roles         []string `json:"roles"`
	AutonomyLevel int      `json:"autonomy_level"` // 0 = Manual, 1 = Advisory, 2 = Assisted, 3 = Conditional, 4 = High, 5 = Autonomous Irreversible
	TenantID      string   `json:"tenant_id"`
}

// PolicyResource contains the verified resource attributes.
type PolicyResource struct {
	Type           string `json:"type"` // "ARTIFACT", "DATABASE", "SECRET", "INCIDENT"
	ID             string `json:"id"`
	SHA256         string `json:"sha256,omitempty"`
	State          string `json:"state,omitempty"` // E.g. "QUARANTINED", "VALIDATING"
	Classification string `json:"classification,omitempty"`
	TenantID       string `json:"tenant_id"`
}

// PolicyWorkflowContext contains the current workflow lifecycle state.
type PolicyWorkflowContext struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	State      string `json:"state,omitempty"`
	Attempt    int    `json:"attempt,omitempty"`
}

// PolicyEnvironment contains the explicit evaluation timestamp and mode.
type PolicyEnvironment struct {
	EvaluationTime time.Time `json:"evaluation_time"`
	FleetMode      string    `json:"fleet_mode,omitempty"` // "SHADOW", "ADVISORY", "ACTIVE"
}

// PolicyDecision is the immutable result of a deterministic policy evaluation.
type PolicyDecision struct {
	DecisionID           string                `json:"decision_id"`
	RequestID            string                `json:"request_id"`
	Decision             Decision              `json:"decision"`
	Action               string                `json:"action"`
	ReasonCodes          []string              `json:"reason_codes"`
	Obligations          []Obligation          `json:"obligations"`
	Prohibitions         []Prohibition         `json:"prohibitions"`
	MatchedPolicyRefs    []string              `json:"matched_policy_refs"`
	PolicyBundleID       string                `json:"policy_bundle_id"`
	PolicyBundleVersion  string                `json:"policy_bundle_version"`
	PolicyBundleHash     string                `json:"policy_bundle_hash"`
	Manifest             []PolicyManifestEntry `json:"manifest,omitempty"`
	EvaluatedContextHash string                `json:"evaluated_context_hash"`
	EvaluatedAt          time.Time             `json:"evaluated_at"`
	EvaluatorVersion     string                `json:"evaluator_version"`
	DecisionHash         string                `json:"decision_hash"`
}

// IsExecutableDecision checks if a decision is immediately executable without outstanding obligations.
func IsExecutableDecision(d *PolicyDecision) bool {
	if d == nil {
		return false
	}
	return d.Decision == DecisionAllow
}

// CanExecuteWithSatisfiedObligations checks if an ALLOW_WITH_OBLIGATIONS decision is fully satisfied.
func CanExecuteWithSatisfiedObligations(d *PolicyDecision, satisfied []ObligationType) (bool, []ObligationType) {
	if d == nil {
		return false, nil
	}
	if d.Decision == DecisionAllow {
		return true, nil
	}
	if d.Decision != DecisionAllowWithObligations {
		return false, nil
	}

	satMap := make(map[ObligationType]struct{})
	for _, s := range satisfied {
		satMap[s] = struct{}{}
	}

	var missing []ObligationType
	for _, obl := range d.Obligations {
		if _, ok := satMap[obl.Type]; !ok {
			missing = append(missing, obl.Type)
		}
	}

	return len(missing) == 0, missing
}
