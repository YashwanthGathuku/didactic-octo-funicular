package policy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrPolicyNotFound        = errors.New("policy definition not found")
	ErrDecisionNotFound      = errors.New("policy decision not found")
	ErrActivePolicyImmutable = errors.New("cannot modify active policy in place: must create a new version")
)

// Store handles durable database operations for policy definitions and evaluation decisions.
type Store struct {
	db *sql.DB
}

// NewStore creates a new policy Store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// StorePolicyDefinition persists a new policy definition. If an ACTIVE version already exists with identical ID and version, modification is rejected.
func (s *Store) StorePolicyDefinition(ctx context.Context, p *PolicyDefinition) error {
	if p.PolicyID == "" || p.Version <= 0 {
		return errors.New("policy_id and version > 0 are required")
	}
	if p.Status == "" {
		p.Status = StatusActive
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.ContentHash == "" {
		p.ContentHash = ComputePolicyContentHash(p)
	}

	// Check if this exact (policy_id, version) already exists
	var existingStatus string
	err := s.db.QueryRowContext(ctx, "SELECT status FROM policy_definitions WHERE policy_id = ? AND version = ?", p.PolicyID, p.Version).Scan(&existingStatus)
	if err == nil {
		return fmt.Errorf("%w: policy %s v%d already exists with status %s", ErrActivePolicyImmutable, p.PolicyID, p.Version, existingStatus)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check existing policy: %w", err)
	}

	subjJSON, _ := CanonicalJSON(p.SubjectConstraints)
	resJSON, _ := CanonicalJSON(p.ResourceConstraints)
	condJSON, _ := CanonicalJSON(p.Conditions)
	oblsJSON, _ := CanonicalJSON(p.Obligations)
	prohsJSON, _ := CanonicalJSON(p.Prohibitions)

	query := `
		INSERT INTO policy_definitions (
			policy_id, version, domain, layer, priority, status,
			effective_from, effective_to, tenant_id, partner_id, action,
			subject_constraints, resource_constraints, conditions,
			effect, obligations, prohibitions, reason_code, source_reference,
			created_at, content_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		p.PolicyID, p.Version, string(p.Domain), string(p.Layer), p.Priority, string(p.Status),
		p.EffectiveFrom, p.EffectiveTo, p.TenantID, p.PartnerID, p.Action,
		string(subjJSON), string(resJSON), string(condJSON),
		string(p.Effect), string(oblsJSON), string(prohsJSON), p.ReasonCode, p.SourceReference,
		p.CreatedAt, p.ContentHash,
	)
	if err != nil {
		return fmt.Errorf("insert policy_definition: %w", err)
	}
	return nil
}

// GetPolicyDefinition retrieves a specific version of a policy.
func (s *Store) GetPolicyDefinition(ctx context.Context, policyID string, version int) (*PolicyDefinition, error) {
	query := `
		SELECT policy_id, version, domain, layer, priority, status,
		       effective_from, effective_to, tenant_id, partner_id, action,
		       subject_constraints, resource_constraints, conditions,
		       effect, obligations, prohibitions, reason_code, COALESCE(source_reference, ''),
		       created_at, content_hash
		FROM policy_definitions
		WHERE policy_id = ? AND version = ?`

	var (
		p         PolicyDefinition
		domainStr string
		layerStr  string
		statusStr string
		effectStr string
		effTo     sql.NullTime
		tenantID  sql.NullString
		partnerID sql.NullString
		subjJSON  string
		resJSON   string
		condJSON  string
		oblsJSON  string
		prohsJSON string
		srcRef    string
	)

	err := s.db.QueryRowContext(ctx, query, policyID, version).Scan(
		&p.PolicyID, &p.Version, &domainStr, &layerStr, &p.Priority, &statusStr,
		&p.EffectiveFrom, &effTo, &tenantID, &partnerID, &p.Action,
		&subjJSON, &resJSON, &condJSON,
		&effectStr, &oblsJSON, &prohsJSON, &p.ReasonCode, &srcRef,
		&p.CreatedAt, &p.ContentHash,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPolicyNotFound
		}
		return nil, fmt.Errorf("query policy_definition: %w", err)
	}

	p.Domain = PolicyDomain(domainStr)
	p.Layer = PolicyLayer(layerStr)
	p.Status = PolicyStatus(statusStr)
	p.Effect = Decision(effectStr)
	p.SourceReference = srcRef

	if effTo.Valid {
		p.EffectiveTo = &effTo.Time
	}
	if tenantID.Valid {
		p.TenantID = &tenantID.String
	}
	if partnerID.Valid {
		p.PartnerID = &partnerID.String
	}

	_ = json.Unmarshal([]byte(subjJSON), &p.SubjectConstraints)
	_ = json.Unmarshal([]byte(resJSON), &p.ResourceConstraints)
	_ = json.Unmarshal([]byte(condJSON), &p.Conditions)
	_ = json.Unmarshal([]byte(oblsJSON), &p.Obligations)
	_ = json.Unmarshal([]byte(prohsJSON), &p.Prohibitions)

	return &p, nil
}

// ListActivePolicies loads all ACTIVE policies applicable to a tenant (including global safety policies).
func (s *Store) ListActivePolicies(ctx context.Context, tenantID string) ([]*PolicyDefinition, error) {
	query := `
		SELECT policy_id, version, domain, layer, priority, status,
		       effective_from, effective_to, tenant_id, partner_id, action,
		       subject_constraints, resource_constraints, conditions,
		       effect, obligations, prohibitions, reason_code, COALESCE(source_reference, ''),
		       created_at, content_hash
		FROM policy_definitions
		WHERE status = 'ACTIVE' AND (tenant_id IS NULL OR tenant_id = ?)`

	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query active policies: %w", err)
	}
	defer rows.Close()

	var policies []*PolicyDefinition
	for rows.Next() {
		var (
			p         PolicyDefinition
			domainStr string
			layerStr  string
			statusStr string
			effectStr string
			effTo     sql.NullTime
			tID       sql.NullString
			pID       sql.NullString
			subjJSON  string
			resJSON   string
			condJSON  string
			oblsJSON  string
			prohsJSON string
			srcRef    string
		)

		if err := rows.Scan(
			&p.PolicyID, &p.Version, &domainStr, &layerStr, &p.Priority, &statusStr,
			&p.EffectiveFrom, &effTo, &tID, &pID, &p.Action,
			&subjJSON, &resJSON, &condJSON,
			&effectStr, &oblsJSON, &prohsJSON, &p.ReasonCode, &srcRef,
			&p.CreatedAt, &p.ContentHash,
		); err != nil {
			return nil, fmt.Errorf("scan policy row: %w", err)
		}

		p.Domain = PolicyDomain(domainStr)
		p.Layer = PolicyLayer(layerStr)
		p.Status = PolicyStatus(statusStr)
		p.Effect = Decision(effectStr)
		p.SourceReference = srcRef

		if effTo.Valid {
			p.EffectiveTo = &effTo.Time
		}
		if tID.Valid {
			p.TenantID = &tID.String
		}
		if pID.Valid {
			p.PartnerID = &pID.String
		}

		_ = json.Unmarshal([]byte(subjJSON), &p.SubjectConstraints)
		_ = json.Unmarshal([]byte(resJSON), &p.ResourceConstraints)
		_ = json.Unmarshal([]byte(condJSON), &p.Conditions)
		_ = json.Unmarshal([]byte(oblsJSON), &p.Obligations)
		_ = json.Unmarshal([]byte(prohsJSON), &p.Prohibitions)

		policies = append(policies, &p)
	}

	return policies, nil
}

// RecordDecisionTx atomically persists an evaluated PolicyDecision and emits a generic durable domain event in a transaction.
// It does NOT require an AgentWorkflow, serving as universal SGACA policy infrastructure for agents, humans, API callers, and tools.
func (s *Store) RecordDecisionTx(ctx context.Context, tx *sql.Tx, tenantID, workflowID string, d *PolicyDecision) error {
	if d.DecisionID == "" || d.RequestID == "" {
		return errors.New("decision_id and request_id are required")
	}
	if d.DecisionHash == "" {
		d.DecisionHash = ComputeDecisionHash(d)
	}

	reasonsJSON, _ := CanonicalJSON(d.ReasonCodes)
	oblsJSON, _ := CanonicalJSON(d.Obligations)
	prohsJSON, _ := CanonicalJSON(d.Prohibitions)
	refsJSON, _ := CanonicalJSON(d.MatchedPolicyRefs)
	manifestJSON, _ := CanonicalJSON(d.Manifest)

	bID := d.PolicyBundleID
	if bID == "" {
		bID = "bundle-sentinel-default"
	}
	bVer := d.PolicyBundleVersion
	if bVer == "" {
		bVer = "1.0.0"
	}

	query := `
		INSERT INTO agent_policy_decisions (
			id, request_id, tenant_id, workflow_id, decision, action,
			reason_codes, obligations, prohibitions, matched_policy_refs,
			policy_bundle_id, policy_bundle_version, policy_bundle_hash, manifest,
			evaluated_context_hash, evaluated_at, evaluator_version, decision_hash, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, query,
		d.DecisionID, d.RequestID, tenantID, workflowID, string(d.Decision), d.Action,
		string(reasonsJSON), string(oblsJSON), string(prohsJSON), string(refsJSON),
		bID, bVer, d.PolicyBundleHash, string(manifestJSON),
		d.EvaluatedContextHash, d.EvaluatedAt, d.EvaluatorVersion, d.DecisionHash, now,
	)
	if err != nil {
		return fmt.Errorf("insert agent_policy_decision: %w", err)
	}

	// 1. Universal Transactional Outbox Event: POLICY_DECISION_RECORDED
	// Emitted for ALL subjects (agents, users, systems, release gates)
	eventPayload := map[string]interface{}{
		"tenant_id":              tenantID,
		"decision_id":            d.DecisionID,
		"request_id":             d.RequestID,
		"policy_bundle_id":       bID,
		"policy_bundle_version":  bVer,
		"policy_bundle_hash":     d.PolicyBundleHash,
		"decision_hash":          d.DecisionHash,
		"decision":               string(d.Decision),
		"action":                 d.Action,
		"evaluated_context_hash": d.EvaluatedContextHash,
		"evaluated_at":           d.EvaluatedAt.Format(time.RFC3339),
	}
	if workflowID != "" {
		eventPayload["workflow_id"] = workflowID
	}

	payloadBytes, _ := CanonicalJSON(eventPayload)
	dedupeKey := fmt.Sprintf("policy-dec-%s", d.DecisionID)

	outboxQuery := `
		INSERT OR IGNORE INTO outbox_events (
			tenant_id, event_type, subject_type, subject_id, payload, dedupe_key, created_at
		) VALUES (?, 'POLICY_DECISION_RECORDED', 'POLICY_DECISION', 0, ?, ?, ?)`

	_, _ = tx.ExecContext(ctx, outboxQuery,
		tenantID, string(payloadBytes), dedupeKey, now,
	)

	// 2. Optional: If a workflow_id is provided and workflow events table exists, link to workflow timeline
	if workflowID != "" {
		wfEventID := fmt.Sprintf("evt-pdec-%s", d.DecisionID)
		wfEventQuery := `
			INSERT OR IGNORE INTO agent_workflow_events (
				id, workflow_id, tenant_id, idempotency_key, event_type,
				state_from, state_to, row_version, payload, created_at
			) VALUES (?, ?, ?, ?, 'POLICY_DECISION_EVALUATED', 'EVALUATING', ?, 1, ?, ?)`

		_, _ = tx.ExecContext(ctx, wfEventQuery,
			wfEventID, workflowID, tenantID, d.DecisionID, string(d.Decision), string(payloadBytes), now,
		)
	}

	return nil
}

// RecordDecision persists an evaluated PolicyDecision inside a fresh transaction.
func (s *Store) RecordDecision(ctx context.Context, tenantID, workflowID string, d *PolicyDecision) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := s.RecordDecisionTx(ctx, tx, tenantID, workflowID, d); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// GetDecision fetches an evaluated policy decision within tenant boundaries.
func (s *Store) GetDecision(ctx context.Context, tenantID, decisionID string) (*PolicyDecision, error) {
	query := `
		SELECT id, request_id, decision, action, reason_codes, obligations,
		       prohibitions, matched_policy_refs, policy_bundle_id, policy_bundle_version,
		       policy_bundle_hash, manifest, evaluated_context_hash, evaluated_at,
		       evaluator_version, decision_hash
		FROM agent_policy_decisions
		WHERE id = ? AND tenant_id = ?`

	var (
		d            PolicyDecision
		decisionStr  string
		reasonsJSON  string
		oblsJSON     string
		prohsJSON    string
		refsJSON     string
		manifestJSON string
	)

	err := s.db.QueryRowContext(ctx, query, decisionID, tenantID).Scan(
		&d.DecisionID, &d.RequestID, &decisionStr, &d.Action,
		&reasonsJSON, &oblsJSON, &prohsJSON, &refsJSON,
		&d.PolicyBundleID, &d.PolicyBundleVersion, &d.PolicyBundleHash, &manifestJSON,
		&d.EvaluatedContextHash, &d.EvaluatedAt, &d.EvaluatorVersion, &d.DecisionHash,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDecisionNotFound
		}
		return nil, fmt.Errorf("query agent_policy_decision: %w", err)
	}

	d.Decision = Decision(decisionStr)
	_ = json.Unmarshal([]byte(reasonsJSON), &d.ReasonCodes)
	_ = json.Unmarshal([]byte(oblsJSON), &d.Obligations)
	_ = json.Unmarshal([]byte(prohsJSON), &d.Prohibitions)
	_ = json.Unmarshal([]byte(refsJSON), &d.MatchedPolicyRefs)
	_ = json.Unmarshal([]byte(manifestJSON), &d.Manifest)

	return &d, nil
}
