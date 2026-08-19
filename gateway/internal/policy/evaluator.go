package policy

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const EvaluatorVersion = "1.0.0"

var (
	ErrNilRequest          = errors.New("policy evaluation request is nil")
	ErrMissingRequestID    = errors.New("request_id is required")
	ErrMissingTenantID     = errors.New("tenant_id is required")
	ErrMissingAction       = errors.New("action is required")
	ErrZeroEvaluationTime  = errors.New("evaluation_time is required and must not be zero")
	ErrEmptyPolicyBundle   = errors.New("policy bundle contains zero active policies")
	ErrBundleNotFound      = errors.New("policy bundle not found")
)

// CompiledBundle is an immutable snapshot of a policy bundle ready for fast evaluation.
type CompiledBundle struct {
	BundleID   string
	Version    string
	BundleHash string
	Manifest   PolicyBundleManifest
	Policies   []*PolicyDefinition
	ByID       map[string][]*PolicyDefinition
}

// CompileBundle compiles and validates an immutable policy bundle.
func CompileBundle(bundleID, version string, policies []*PolicyDefinition) (*CompiledBundle, error) {
	if len(policies) == 0 {
		return nil, ErrEmptyPolicyBundle
	}

	// 1. Mandatory safety policy bootstrap validation
	if err := ValidateSafetyBootstrap(policies); err != nil {
		return nil, err
	}

	compiledPolicies := make([]*PolicyDefinition, 0, len(policies))
	byID := make(map[string][]*PolicyDefinition)

	for _, p := range policies {
		if p.PolicyID == "" || p.Version <= 0 {
			return nil, fmt.Errorf("invalid policy ID %q or version %d", p.PolicyID, p.Version)
		}
		if p.ContentHash == "" {
			p.ContentHash = ComputePolicyContentHash(p)
		}
		compiledPolicies = append(compiledPolicies, p)
		byID[p.PolicyID] = append(byID[p.PolicyID], p)
	}

	manifest := BuildBundleManifest(bundleID, version, compiledPolicies)

	return &CompiledBundle{
		BundleID:   bundleID,
		Version:    version,
		BundleHash: manifest.BundleHash,
		Manifest:   manifest,
		Policies:   compiledPolicies,
		ByID:       byID,
	}, nil
}

// PolicyEngine provides thread-safe, fast, deterministic policy evaluation with atomic bundle swapping.
type PolicyEngine struct {
	activeBundle atomic.Pointer[CompiledBundle]
}

// NewEngine constructs a PolicyEngine initialized with a compiled policy bundle.
func NewEngine(policies []*PolicyDefinition) (*PolicyEngine, error) {
	return NewEngineWithBundle("bundle-sentinel-default", "1.0.0", policies)
}

// NewEngineWithBundle constructs a PolicyEngine with a specific bundle ID and version.
func NewEngineWithBundle(bundleID, version string, policies []*PolicyDefinition) (*PolicyEngine, error) {
	compiled, err := CompileBundle(bundleID, version, policies)
	if err != nil {
		return nil, err
	}

	engine := &PolicyEngine{}
	engine.activeBundle.Store(compiled)
	return engine, nil
}

// NewEngineWithDefaults constructs a PolicyEngine pre-seeded with foundational Sentinel safety policies.
func NewEngineWithDefaults() *PolicyEngine {
	engine, _ := NewEngine(SeedSafetyPolicies())
	return engine
}

// SetBundle compiles and atomically replaces the active policy bundle.
func (e *PolicyEngine) SetBundle(policies []*PolicyDefinition) error {
	return e.SetBundleWithID("bundle-sentinel-default", "1.0.0", policies)
}

// SetBundleWithID compiles and atomically swaps the active policy bundle.
func (e *PolicyEngine) SetBundleWithID(bundleID, version string, policies []*PolicyDefinition) error {
	compiled, err := CompileBundle(bundleID, version, policies)
	if err != nil {
		return err
	}
	e.activeBundle.Store(compiled)
	return nil
}

// GetActiveBundle returns the current active compiled bundle.
func (e *PolicyEngine) GetActiveBundle() *CompiledBundle {
	return e.activeBundle.Load()
}

// GetBundleHash returns the current active policy bundle digest.
func (e *PolicyEngine) GetBundleHash() string {
	b := e.activeBundle.Load()
	if b == nil {
		return ""
	}
	return b.BundleHash
}

// ValidateRequest checks all required authoritative request fields.
func ValidateRequest(req *PolicyEvaluationRequest) error {
	if req == nil {
		return ErrNilRequest
	}
	if req.RequestID == "" {
		return ErrMissingRequestID
	}
	if req.TenantID == "" {
		return ErrMissingTenantID
	}
	if req.Action == "" {
		return ErrMissingAction
	}
	if req.Environment.EvaluationTime.IsZero() {
		return ErrZeroEvaluationTime
	}
	return nil
}

// matchesPolicy checks if a given PolicyDefinition matches the request context and evaluation time.
func matchesPolicy(p *PolicyDefinition, req *PolicyEvaluationRequest, evalTime time.Time) bool {
	// 1. Status check: Only ACTIVE policies can evaluate
	if p.Status != StatusActive {
		return false
	}

	// 2. Temporal validity check
	if evalTime.Before(p.EffectiveFrom) {
		return false
	}
	if p.EffectiveTo != nil && evalTime.After(*p.EffectiveTo) {
		return false
	}

	// 3. Tenant scoping check
	if p.TenantID != nil && *p.TenantID != req.TenantID {
		return false
	}

	// 4. Partner scoping check (if rule specifies partner)
	if p.PartnerID != nil {
		reqPartner, _ := req.AuthoritativeAttributes["partner_id"].(string)
		if reqPartner != *p.PartnerID {
			return false
		}
	}

	// 5. Action check
	if p.Action != "*" && !strings.EqualFold(p.Action, req.Action) {
		return false
	}

	// 6. Subject constraints
	if p.SubjectConstraints.Type != "" && p.SubjectConstraints.Type != "*" {
		if !strings.EqualFold(p.SubjectConstraints.Type, req.Subject.Type) {
			return false
		}
	}
	if p.SubjectConstraints.ID != "" && p.SubjectConstraints.ID != "*" {
		if p.SubjectConstraints.ID != req.Subject.ID {
			return false
		}
	}
	if len(p.SubjectConstraints.Roles) > 0 {
		matchedRole := false
		for _, requiredRole := range p.SubjectConstraints.Roles {
			for _, subjRole := range req.Subject.Roles {
				if strings.EqualFold(requiredRole, subjRole) || requiredRole == "*" {
					matchedRole = true
					break
				}
			}
			if matchedRole {
				break
			}
		}
		if !matchedRole {
			return false
		}
	}
	if p.SubjectConstraints.MinAutonomy > 0 && req.Subject.AutonomyLevel < p.SubjectConstraints.MinAutonomy {
		return false
	}
	if p.SubjectConstraints.MaxAutonomy > 0 && req.Subject.AutonomyLevel > p.SubjectConstraints.MaxAutonomy {
		return false
	}

	// 7. Resource constraints
	if p.ResourceConstraints.Type != "" && p.ResourceConstraints.Type != "*" {
		if !strings.EqualFold(p.ResourceConstraints.Type, req.Resource.Type) {
			return false
		}
	}
	if p.ResourceConstraints.ID != "" && p.ResourceConstraints.ID != "*" {
		if p.ResourceConstraints.ID != req.Resource.ID {
			return false
		}
	}
	if len(p.ResourceConstraints.States) > 0 {
		matchedState := false
		for _, st := range p.ResourceConstraints.States {
			if strings.EqualFold(st, req.Resource.State) || st == "*" {
				matchedState = true
				break
			}
		}
		if !matchedState {
			return false
		}
	}
	if p.ResourceConstraints.Classification != "" && p.ResourceConstraints.Classification != "*" {
		if !strings.EqualFold(p.ResourceConstraints.Classification, req.Resource.Classification) {
			return false
		}
	}

	return true
}

// Evaluate executes deterministic policy evaluation against the active policy bundle.
func (e *PolicyEngine) Evaluate(req *PolicyEvaluationRequest) (*PolicyDecision, error) {
	bundle := e.activeBundle.Load()
	if bundle == nil {
		return nil, ErrEmptyPolicyBundle
	}
	return EvaluateWithBundle(bundle, req)
}

// EvaluateWithBundle evaluates a request against an exact immutable CompiledBundle.
func EvaluateWithBundle(bundle *CompiledBundle, req *PolicyEvaluationRequest) (*PolicyDecision, error) {
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}

	evalTime := req.Environment.EvaluationTime.UTC()
	contextHash := ComputeEvaluatedContextHash(req)

	// 1. Cross-Tenant Hardening Invariant: Subject vs Resource tenant isolation
	if req.Subject.TenantID != "" && req.Resource.TenantID != "" && req.Subject.TenantID != req.Resource.TenantID {
		crossProhs := []Prohibition{{Type: ProhibitionCrossTenantAccess, Description: "Cross-tenant access forbidden"}}
		return buildDecision(req, DecisionDeny, []string{"CROSS_TENANT_FORBIDDEN"}, nil, crossProhs, []string{"SF-SAFE-006:v1"}, bundle, contextHash, evalTime), nil
	}
	if req.TenantID != "" && req.Subject.TenantID != "" && req.TenantID != req.Subject.TenantID {
		crossProhs := []Prohibition{{Type: ProhibitionCrossTenantAccess, Description: "Tenant context mismatch forbidden"}}
		return buildDecision(req, DecisionDeny, []string{"TENANT_CONTEXT_MISMATCH"}, nil, crossProhs, []string{"SF-SAFE-006:v1"}, bundle, contextHash, evalTime), nil
	}

	// 2. Find all matching active policies
	var matched []*PolicyDefinition
	for _, p := range bundle.Policies {
		if matchesPolicy(p, req, evalTime) {
			matched = append(matched, p)
		}
	}

	// 3. Fail-Closed if no active matching policy exists
	if len(matched) == 0 {
		return buildDecision(req, DecisionDeny, []string{"NO_APPLICABLE_POLICY"}, nil, nil, nil, bundle, contextHash, evalTime), nil
	}

	// 4. Sort matched policies by Layer Precedence (ascending: 10, 20, 30...), then Priority (descending)
	// Priority acts strictly as deterministic ordering metadata within the same layer.
	// Deny dominance and accumulation are preserved across all priority levels.
	sort.Slice(matched, func(i, j int) bool {
		precI := LayerPrecedence(matched[i].Layer)
		precJ := LayerPrecedence(matched[j].Layer)
		if precI != precJ {
			return precI < precJ // Higher layer rank first (lower integer)
		}
		if matched[i].Priority != matched[j].Priority {
			return matched[i].Priority > matched[j].Priority // Higher priority first
		}
		if matched[i].PolicyID != matched[j].PolicyID {
			return matched[i].PolicyID < matched[j].PolicyID
		}
		return matched[i].Version < matched[j].Version
	})

	// 5. Accumulate decisions, obligations, prohibitions, and reason codes
	var (
		matchedRefs  []string
		reasonCodes  []string
		obligations  []Obligation
		prohibitions []Prohibition
		hasDeny      bool
		hasHuman     bool
		hasAllowObl  bool
		hasAllow     bool
	)

	oblSet := make(map[string]struct{})
	prohSet := make(map[ProhibitionType]struct{})
	reasonSet := make(map[string]struct{})

	for _, p := range matched {
		ref := fmt.Sprintf("%s:v%d", p.PolicyID, p.Version)
		matchedRefs = append(matchedRefs, ref)

		if p.ReasonCode != "" {
			if _, exists := reasonSet[p.ReasonCode]; !exists {
				reasonSet[p.ReasonCode] = struct{}{}
				reasonCodes = append(reasonCodes, p.ReasonCode)
			}
		}

		// Prohibitions always accumulate across all matched rules
		for _, proh := range p.Prohibitions {
			if _, exists := prohSet[proh.Type]; !exists {
				prohSet[proh.Type] = struct{}{}
				prohibitions = append(prohibitions, proh)
			}
		}

		// Obligations accumulate from ALLOW and ALLOW_WITH_OBLIGATIONS rules
		for _, obl := range p.Obligations {
			paramBytes, _ := CanonicalJSON(obl.Parameters)
			key := fmt.Sprintf("%s:%s", obl.Type, string(paramBytes))
			if _, exists := oblSet[key]; !exists {
				oblSet[key] = struct{}{}
				obligations = append(obligations, obl)
			}
		}

		switch p.Effect {
		case DecisionDeny:
			hasDeny = true
		case DecisionRequireHuman:
			hasHuman = true
		case DecisionAllowWithObligations:
			hasAllowObl = true
		case DecisionAllow:
			hasAllow = true
		}
	}

	// 6. Deny Dominance & Hierarchy Resolution
	var finalDecision Decision

	if hasDeny {
		finalDecision = DecisionDeny
	} else if hasHuman {
		finalDecision = DecisionRequireHuman
	} else if len(obligations) > 0 || hasAllowObl {
		finalDecision = DecisionAllowWithObligations
	} else if hasAllow {
		finalDecision = DecisionAllow
	} else {
		// Fail closed fallback
		finalDecision = DecisionDeny
		reasonCodes = append(reasonCodes, "UNRESOLVED_POLICY_EFFECT")
	}

	// 7. Check for Prohibition vs Action conflicts
	if len(prohibitions) > 0 {
		for _, proh := range prohibitions {
			if isActionProhibited(req.Action, proh.Type) {
				finalDecision = DecisionDeny
				reasonCodes = append(reasonCodes, fmt.Sprintf("PROHIBITION_VIOLATION_%s", proh.Type))
				break
			}
		}
	}

	return buildDecision(req, finalDecision, reasonCodes, obligations, prohibitions, matchedRefs, bundle, contextHash, evalTime), nil
}

// isActionProhibited checks if the requested action directly violates an accumulated prohibition.
func isActionProhibited(action string, prohibition ProhibitionType) bool {
	switch prohibition {
	case ProhibitionMutateOriginal:
		return strings.EqualFold(action, ActionModifyOriginalArtifact)
	case ProhibitionRelease:
		return strings.EqualFold(action, ActionReleaseArtifact)
	case ProhibitionApprove:
		return strings.EqualFold(action, ActionApproveRelease)
	case ProhibitionExecuteSQL:
		return strings.EqualFold(action, ActionQueryDatabase)
	case ProhibitionAccessSecret:
		return strings.EqualFold(action, ActionAccessSecret)
	case ProhibitionCrossTenantAccess:
		return strings.EqualFold(action, ActionCrossTenantQuery)
	}
	return false
}

// buildDecision constructs an immutable PolicyDecision and computes its canonical hash.
func buildDecision(
	req *PolicyEvaluationRequest,
	decision Decision,
	reasonCodes []string,
	obligations []Obligation,
	prohibitions []Prohibition,
	matchedRefs []string,
	bundle *CompiledBundle,
	contextHash string,
	evalTime time.Time,
) *PolicyDecision {
	cleanReasons := sortedStrings(reasonCodes)
	cleanObls := sortObligations(obligations)
	cleanProhs := sortProhibitions(prohibitions)
	cleanRefs := sortedStrings(matchedRefs)

	dID := fmt.Sprintf("pdec-%s-%s", req.RequestID, contextHash[:min(8, len(contextHash))])

	var (
		bID      string
		bVersion string
		bHash    string
		manifest []PolicyManifestEntry
	)

	if bundle != nil {
		bID = bundle.BundleID
		bVersion = bundle.Version
		bHash = bundle.BundleHash
		manifest = bundle.Manifest.Manifest
	}

	d := &PolicyDecision{
		DecisionID:           dID,
		RequestID:            req.RequestID,
		Decision:             decision,
		Action:               req.Action,
		ReasonCodes:          cleanReasons,
		Obligations:          cleanObls,
		Prohibitions:         cleanProhs,
		MatchedPolicyRefs:    cleanRefs,
		PolicyBundleID:       bID,
		PolicyBundleVersion:  bVersion,
		PolicyBundleHash:     bHash,
		Manifest:             manifest,
		EvaluatedContextHash: contextHash,
		EvaluatedAt:          evalTime,
		EvaluatorVersion:     EvaluatorVersion,
	}

	d.DecisionHash = ComputeDecisionHash(d)
	return d
}

// ReplayEvaluation executes historical replay by compiling the exact historical bundle and evaluating request.
func ReplayEvaluation(manifest PolicyBundleManifest, policies []*PolicyDefinition, req *PolicyEvaluationRequest) (*PolicyDecision, error) {
	// Filter policies to only those in the manifest
	manifestMap := make(map[string]PolicyManifestEntry)
	for _, entry := range manifest.Manifest {
		key := fmt.Sprintf("%s:v%d", entry.PolicyID, entry.Version)
		manifestMap[key] = entry
	}

	exactPolicies := make([]*PolicyDefinition, 0, len(manifest.Manifest))
	for _, p := range policies {
		key := fmt.Sprintf("%s:v%d", p.PolicyID, p.Version)
		if entry, ok := manifestMap[key]; ok {
			// Verify content hash match
			pHash := p.ContentHash
			if pHash == "" {
				pHash = ComputePolicyContentHash(p)
			}
			if pHash != entry.ContentHash {
				return nil, fmt.Errorf("historical policy %s content hash mismatch", key)
			}
			exactPolicies = append(exactPolicies, p)
		}
	}

	compiled, err := CompileBundle(manifest.BundleID, manifest.Version, exactPolicies)
	if err != nil {
		return nil, fmt.Errorf("recompile historical bundle: %w", err)
	}

	return EvaluateWithBundle(compiled, req)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
