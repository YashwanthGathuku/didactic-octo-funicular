package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"
)

// formatTimeCanonical returns RFC3339 UTC string.
func formatTimeCanonical(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// sortedStrings returns a new sorted slice.
func sortedStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

// sortObligations returns a deterministic copy of obligations sorted by Type.
func sortObligations(obls []Obligation) []Obligation {
	if len(obls) == 0 {
		return []Obligation{}
	}
	out := make([]Obligation, len(obls))
	copy(out, obls)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			// If same type, sort canonically by serialized parameters
			bI, _ := CanonicalJSON(out[i].Parameters)
			bJ, _ := CanonicalJSON(out[j].Parameters)
			return string(bI) < string(bJ)
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// sortProhibitions returns a deterministic copy of prohibitions sorted by Type.
func sortProhibitions(prohs []Prohibition) []Prohibition {
	if len(prohs) == 0 {
		return []Prohibition{}
	}
	out := make([]Prohibition, len(prohs))
	copy(out, prohs)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Type < out[j].Type
	})
	return out
}

// CanonicalPolicyBytes builds the RFC 8785 canonical JSON byte representation of a policy definition.
func CanonicalPolicyBytes(p *PolicyDefinition) ([]byte, error) {
	var effToStr *string
	if p.EffectiveTo != nil {
		s := formatTimeCanonical(*p.EffectiveTo)
		effToStr = &s
	}

	payload := map[string]interface{}{
		"schema_version": "1.0",
		"policy_id":      p.PolicyID,
		"version":        p.Version,
		"domain":         string(p.Domain),
		"layer":          string(p.Layer),
		"priority":       p.Priority,
		"status":         string(p.Status),
		"effective_from": formatTimeCanonical(p.EffectiveFrom),
		"effective_to":   effToStr,
		"tenant_id":      p.TenantID,
		"partner_id":     p.PartnerID,
		"action":         p.Action,
		"effect":         string(p.Effect),
		"reason_code":    p.ReasonCode,
		"obligations":    sortObligations(p.Obligations),
		"prohibitions":   sortProhibitions(p.Prohibitions),
		"subject_constraints": map[string]interface{}{
			"type":         p.SubjectConstraints.Type,
			"id":           p.SubjectConstraints.ID,
			"roles":        sortedStrings(p.SubjectConstraints.Roles),
			"min_autonomy": p.SubjectConstraints.MinAutonomy,
			"max_autonomy": p.SubjectConstraints.MaxAutonomy,
		},
		"resource_constraints": map[string]interface{}{
			"type":           p.ResourceConstraints.Type,
			"id":             p.ResourceConstraints.ID,
			"states":         sortedStrings(p.ResourceConstraints.States),
			"classification": p.ResourceConstraints.Classification,
		},
		"conditions":       p.Conditions,
		"source_reference": p.SourceReference,
	}

	return CanonicalJSON(payload)
}

// ComputePolicyContentHash computes SHA-256 over CanonicalPolicyBytes.
func ComputePolicyContentHash(p *PolicyDefinition) string {
	b, err := CanonicalPolicyBytes(p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// BuildBundleManifest creates a sorted, deduplicated manifest from a slice of policies.
func BuildBundleManifest(bundleID, version string, policies []*PolicyDefinition) PolicyBundleManifest {
	entries := make([]PolicyManifestEntry, 0, len(policies))
	for _, p := range policies {
		h := p.ContentHash
		if h == "" {
			h = ComputePolicyContentHash(p)
		}
		entries = append(entries, PolicyManifestEntry{
			PolicyID:    p.PolicyID,
			Version:     p.Version,
			ContentHash: h,
		})
	}

	// Sort manifest by policy_id, then version
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PolicyID == entries[j].PolicyID {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].PolicyID < entries[j].PolicyID
	})

	m := PolicyBundleManifest{
		BundleID: bundleID,
		Version:  version,
		Manifest: entries,
	}
	m.BundleHash = ComputeBundleManifestHash(m)
	return m
}

// CanonicalBundleManifestBytes builds RFC 8785 canonical bytes for a PolicyBundleManifest.
func CanonicalBundleManifestBytes(m PolicyBundleManifest) ([]byte, error) {
	// Ensure entries are sorted
	entries := make([]PolicyManifestEntry, len(m.Manifest))
	copy(entries, m.Manifest)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PolicyID == entries[j].PolicyID {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].PolicyID < entries[j].PolicyID
	})

	payload := map[string]interface{}{
		"schema_version": "1.0",
		"bundle_id":      m.BundleID,
		"version":        m.Version,
		"manifest":       entries,
	}
	return CanonicalJSON(payload)
}

// ComputeBundleManifestHash computes SHA-256 over CanonicalBundleManifestBytes.
func ComputeBundleManifestHash(m PolicyBundleManifest) string {
	b, err := CanonicalBundleManifestBytes(m)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ComputePolicyBundleHash computes SHA-256 for a list of policies using the default bundle ID and version.
func ComputePolicyBundleHash(policies []*PolicyDefinition) string {
	m := BuildBundleManifest("bundle-sentinel-default", "1.0.0", policies)
	return m.BundleHash
}

// CanonicalRequestContextBytes builds RFC 8785 canonical bytes for a PolicyEvaluationRequest.
func CanonicalRequestContextBytes(req *PolicyEvaluationRequest) ([]byte, error) {
	payload := map[string]interface{}{
		"schema_version": "1.0",
		"request_id":     req.RequestID,
		"tenant_id":      req.TenantID,
		"action":         req.Action,
		"subject": map[string]interface{}{
			"type":           req.Subject.Type,
			"id":             req.Subject.ID,
			"roles":          sortedStrings(req.Subject.Roles),
			"autonomy_level": req.Subject.AutonomyLevel,
			"tenant_id":      req.Subject.TenantID,
		},
		"resource": map[string]interface{}{
			"type":           req.Resource.Type,
			"id":             req.Resource.ID,
			"sha256":         req.Resource.SHA256,
			"state":          req.Resource.State,
			"classification": req.Resource.Classification,
			"tenant_id":      req.Resource.TenantID,
		},
		"workflow": map[string]interface{}{
			"workflow_id": req.Workflow.WorkflowID,
			"state":       req.Workflow.State,
			"attempt":     req.Workflow.Attempt,
		},
		"environment": map[string]interface{}{
			"evaluation_time": formatTimeCanonical(req.Environment.EvaluationTime),
			"fleet_mode":      req.Environment.FleetMode,
		},
		"authoritative_attributes": req.AuthoritativeAttributes,
	}

	return CanonicalJSON(payload)
}

// ComputeEvaluatedContextHash computes SHA-256 over CanonicalRequestContextBytes.
func ComputeEvaluatedContextHash(req *PolicyEvaluationRequest) string {
	b, err := CanonicalRequestContextBytes(req)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// CanonicalDecisionBytes builds RFC 8785 canonical bytes for a PolicyDecision.
func CanonicalDecisionBytes(d *PolicyDecision) ([]byte, error) {
	payload := map[string]interface{}{
		"schema_version":         "1.0",
		"decision_id":            d.DecisionID,
		"request_id":             d.RequestID,
		"decision":               string(d.Decision),
		"action":                 d.Action,
		"reason_codes":           sortedStrings(d.ReasonCodes),
		"obligations":            sortObligations(d.Obligations),
		"prohibitions":           sortProhibitions(d.Prohibitions),
		"matched_policy_refs":    sortedStrings(d.MatchedPolicyRefs),
		"policy_bundle_id":       d.PolicyBundleID,
		"policy_bundle_version":  d.PolicyBundleVersion,
		"policy_bundle_hash":     d.PolicyBundleHash,
		"evaluated_context_hash": d.EvaluatedContextHash,
		"evaluator_version":      d.EvaluatorVersion,
		"evaluated_at":           formatTimeCanonical(d.EvaluatedAt),
	}

	return CanonicalJSON(payload)
}

// ComputeDecisionHash computes SHA-256 over CanonicalDecisionBytes.
func ComputeDecisionHash(d *PolicyDecision) string {
	b, err := CanonicalDecisionBytes(d)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
