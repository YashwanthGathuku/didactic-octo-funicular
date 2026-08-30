package toolgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/candidate"
	"sentinel-gateway/internal/lens"
	"sentinel-gateway/internal/policy"
	"sentinel-gateway/internal/repository"
)

// Standard Read-Only and Governed Remediation Tool IDs
const (
	ToolIncidentGet                = "incident.get"
	ToolValidationFindingsList     = "validation.findings.list_redacted"
	ToolArtifactMetadataGet        = "artifact.metadata.get"
	ToolWorkflowGet                = "workflow.get"
	ToolRemediationCandidateCreate = "remediation.candidate.create"
	ToolLensQuery                  = "lens.query"
)

// Strongly typed output structures for P04 safe tools

// IncidentMetadataOutput represents structured incident metadata.
type IncidentMetadataOutput struct {
	IncidentID         string             `json:"incident_id"`
	TenantID           string             `json:"tenant_id"`
	Status             string             `json:"status"`
	DataClassification DataClassification `json:"data_classification"`
	CreatedAt          string             `json:"created_at"`
}

// RedactedFindingOutput represents a sanitized validation finding with zero unmasked raw financial data.
type RedactedFindingOutput struct {
	FindingCode         string             `json:"finding_code"`
	Severity            string             `json:"severity"`
	Message             string             `json:"message"`
	TenantID            string             `json:"tenant_id"`
	ArtifactID          string             `json:"artifact_id"`
	BatchNumber         int                `json:"batch_number,omitempty"`
	EntryDetailSequence int                `json:"entry_detail_sequence,omitempty"`
	RedactedAccountRef  string             `json:"redacted_account_ref,omitempty"` // e.g. "ACCT-****-1234"
	RuleCitation        string             `json:"rule_citation,omitempty"`
	DataClassification  DataClassification `json:"data_classification"`
}

// ArtifactMetadataOutput represents immutable artifact metadata.
type ArtifactMetadataOutput struct {
	ArtifactID         string             `json:"artifact_id"`
	TenantID           string             `json:"tenant_id"`
	State              string             `json:"state"`
	DataClassification DataClassification `json:"data_classification"`
	ArtifactSHA256     string             `json:"artifact_sha256"`
	ByteCount          int64              `json:"byte_count,omitempty"`
	CreatedAt          string             `json:"created_at"`
}

// WorkflowStatusOutput represents workflow lifecycle status.
type WorkflowStatusOutput struct {
	WorkflowID         string             `json:"workflow_id"`
	TenantID           string             `json:"tenant_id"`
	State              string             `json:"state"`
	AttemptCount       int                `json:"attempt_count"`
	DataClassification DataClassification `json:"data_classification"`
	UpdatedAt          string             `json:"updated_at"`
}

// DataProvider is an interface through which safe tools fetch domain data without raw SQL access from callers.
type DataProvider interface {
	GetIncident(ctx context.Context, tenantID, incidentID string) (map[string]interface{}, error)
	ListRedactedFindings(ctx context.Context, tenantID, artifactID string) ([]map[string]interface{}, error)
	GetArtifactMetadata(ctx context.Context, tenantID, artifactID string) (map[string]interface{}, error)
	GetWorkflow(ctx context.Context, tenantID, workflowID string) (map[string]interface{}, error)
}

// RegisterDefaultTools registers the safe P04 initial READ_ONLY tool proof set into a Registry.
func RegisterDefaultTools(reg *Registry, provider DataProvider) error {
	// 1. incident.get
	manifestIncidentGet := &ToolManifest{
		ToolID:                       ToolIncidentGet,
		Version:                      "1.0.0",
		Description:                  "Retrieves metadata and status for a quarantined file incident within tenant boundaries.",
		Owner:                        "sentinel-core",
		Status:                       ManifestStatusActive,
		PolicyDomain:                 policy.DomainArtifact,
		PolicyAction:                 policy.ActionGetIncident,
		RequiredCapabilities:         []ToolCapability{CapIncidentRead},
		SideEffectClass:              SideEffectReadOnly,
		MinAutonomy:                  1,
		MaxAutonomy:                  4,
		InputContract:                `{"type":"object","properties":{"incident_id":{"type":"string"}},"required":["incident_id"]}`,
		OutputContract:               `{"type":"object"}`,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               64 * 1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly, ClassificationPublic, ClassificationInternal},
		IdempotencyRequired:          false,
		VerificationRequired:         false,
		ShadowModeAllowed:            true,
		AllowedExecutionModes:        []string{"SHADOW", "ADVISORY", "LIVE"},
	}

	handlerIncidentGet := func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		var input struct {
			IncidentID string `json:"incident_id"`
		}
		if err := json.Unmarshal(args, &input); err != nil || input.IncidentID == "" {
			return nil, fmt.Errorf("%w: incident_id is required", ErrInputValidationFailed)
		}

		if provider == nil {
			res := &IncidentMetadataOutput{
				IncidentID:         input.IncidentID,
				TenantID:           execCtx.TenantID,
				Status:             "QUARANTINED",
				DataClassification: ClassificationMetadataOnly,
				CreatedAt:          execCtx.Timestamp.Format(time.RFC3339),
			}
			return json.Marshal(res)
		}

		data, err := provider.GetIncident(ctx, execCtx.TenantID, input.IncidentID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(data)
	}

	if err := reg.Register(manifestIncidentGet, handlerIncidentGet); err != nil {
		return err
	}

	// 2. validation.findings.list_redacted
	manifestFindingsList := &ToolManifest{
		ToolID:                       ToolValidationFindingsList,
		Version:                      "1.0.0",
		Description:                  "Lists deterministic validation findings with sensitive financial payload fields redacted.",
		Owner:                        "sentinel-core",
		Status:                       ManifestStatusActive,
		PolicyDomain:                 policy.DomainArtifact,
		PolicyAction:                 policy.ActionListFindings,
		RequiredCapabilities:         []ToolCapability{CapFindingsReadRedacted},
		SideEffectClass:              SideEffectReadOnly,
		MinAutonomy:                  1,
		MaxAutonomy:                  4,
		InputContract:                `{"type":"object","properties":{"artifact_id":{"type":"string"}},"required":["artifact_id"]}`,
		OutputContract:               `{"type":"array"}`,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               128 * 1024,
		DataClassifications:          []DataClassification{ClassificationRedactedFindings},
		AllowedOutputClassifications: []DataClassification{ClassificationRedactedFindings, ClassificationMetadataOnly, ClassificationPublic, ClassificationInternal},
		IdempotencyRequired:          false,
		VerificationRequired:         false,
		ShadowModeAllowed:            true,
		AllowedExecutionModes:        []string{"SHADOW", "ADVISORY", "LIVE"},
	}

	handlerFindingsList := func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		var input struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := json.Unmarshal(args, &input); err != nil || input.ArtifactID == "" {
			return nil, fmt.Errorf("%w: artifact_id is required", ErrInputValidationFailed)
		}

		if provider == nil {
			findings := []*RedactedFindingOutput{
				{
					FindingCode:        "ERR_NACHA_BATCH_HASH_MISMATCH",
					Severity:           "BLOCKING",
					Message:            "Entry hash accumulator mismatch on batch 01",
					TenantID:           execCtx.TenantID,
					ArtifactID:         input.ArtifactID,
					RedactedAccountRef: "ACCT-****-4321",
					RuleCitation:       "NACHA Operating Rules 2026, Section 2.2",
					DataClassification: ClassificationRedactedFindings,
				},
			}
			return json.Marshal(findings)
		}

		data, err := provider.ListRedactedFindings(ctx, execCtx.TenantID, input.ArtifactID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(data)
	}

	if err := reg.Register(manifestFindingsList, handlerFindingsList); err != nil {
		return err
	}

	// 3. artifact.metadata.get
	manifestArtifactGet := &ToolManifest{
		ToolID:                       ToolArtifactMetadataGet,
		Version:                      "1.0.0",
		Description:                  "Retrieves immutable artifact metadata (SHA-256, classification, quarantined state).",
		Owner:                        "sentinel-core",
		Status:                       ManifestStatusActive,
		PolicyDomain:                 policy.DomainArtifact,
		PolicyAction:                 policy.ActionGetArtifactMetadata,
		RequiredCapabilities:         []ToolCapability{CapArtifactMetadataRead},
		SideEffectClass:              SideEffectReadOnly,
		MinAutonomy:                  1,
		MaxAutonomy:                  4,
		InputContract:                `{"type":"object","properties":{"artifact_id":{"type":"string"}},"required":["artifact_id"]}`,
		OutputContract:               `{"type":"object"}`,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               64 * 1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly, ClassificationPublic, ClassificationInternal},
		IdempotencyRequired:          false,
		VerificationRequired:         false,
		ShadowModeAllowed:            true,
		AllowedExecutionModes:        []string{"SHADOW", "ADVISORY", "LIVE"},
	}

	handlerArtifactGet := func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		var input struct {
			ArtifactID string `json:"artifact_id"`
		}
		if err := json.Unmarshal(args, &input); err != nil || input.ArtifactID == "" {
			return nil, fmt.Errorf("%w: artifact_id is required", ErrInputValidationFailed)
		}

		if provider == nil {
			res := &ArtifactMetadataOutput{
				ArtifactID:         input.ArtifactID,
				TenantID:           execCtx.TenantID,
				State:              "QUARANTINED",
				DataClassification: ClassificationMetadataOnly,
				ArtifactSHA256:     execCtx.ArtifactSHA256,
				CreatedAt:          execCtx.Timestamp.Format(time.RFC3339),
			}
			return json.Marshal(res)
		}

		data, err := provider.GetArtifactMetadata(ctx, execCtx.TenantID, input.ArtifactID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(data)
	}

	if err := reg.Register(manifestArtifactGet, handlerArtifactGet); err != nil {
		return err
	}

	// 4. workflow.get
	manifestWorkflowGet := &ToolManifest{
		ToolID:                       ToolWorkflowGet,
		Version:                      "1.0.0",
		Description:                  "Retrieves agent workflow execution status and step timeline.",
		Owner:                        "sentinel-core",
		Status:                       ManifestStatusActive,
		PolicyDomain:                 policy.DomainAgent,
		PolicyAction:                 policy.ActionGetWorkflow,
		RequiredCapabilities:         []ToolCapability{CapWorkflowRead},
		SideEffectClass:              SideEffectReadOnly,
		MinAutonomy:                  1,
		MaxAutonomy:                  4,
		InputContract:                `{"type":"object","properties":{"workflow_id":{"type":"string"}},"required":["workflow_id"]}`,
		OutputContract:               `{"type":"object"}`,
		Timeout:                      5 * time.Second,
		MaxOutputBytes:               64 * 1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly, ClassificationPublic, ClassificationInternal},
		IdempotencyRequired:          false,
		VerificationRequired:         false,
		ShadowModeAllowed:            true,
		AllowedExecutionModes:        []string{"SHADOW", "ADVISORY", "LIVE"},
	}

	handlerWorkflowGet := func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		var input struct {
			WorkflowID string `json:"workflow_id"`
		}
		if err := json.Unmarshal(args, &input); err != nil || input.WorkflowID == "" {
			return nil, fmt.Errorf("%w: workflow_id is required", ErrInputValidationFailed)
		}

		if provider == nil {
			res := &WorkflowStatusOutput{
				WorkflowID:         input.WorkflowID,
				TenantID:           execCtx.TenantID,
				State:              "INVESTIGATING",
				AttemptCount:       1,
				DataClassification: ClassificationMetadataOnly,
				UpdatedAt:          execCtx.Timestamp.Format(time.RFC3339),
			}
			return json.Marshal(res)
		}

		data, err := provider.GetWorkflow(ctx, execCtx.TenantID, input.WorkflowID)
		if err != nil {
			return nil, err
		}
		return json.Marshal(data)
	}

	if err := reg.Register(manifestWorkflowGet, handlerWorkflowGet); err != nil {
		return err
	}

	if err := RegisterCandidateTool(reg, nil); err != nil {
		return err
	}

	return nil
}

// CandidateServiceRunner defines the interface needed by Tool Gateway to generate candidates.
type CandidateServiceRunner interface {
	GenerateCandidate(ctx context.Context, scope repository.Scope, req *candidate.CandidateCreationRequest) (*candidate.CandidateResult, error)
}

func makeSystemScope(tenantID, actorID string) repository.Scope {
	if actorID == "" {
		actorID = "system/tool-gateway"
	}
	p := &auth.Principal{
		Subject: actorID,
		Memberships: []auth.Membership{
			{
				TenantID: tenantID,
				Roles:    []auth.Role{auth.RoleOperator},
			},
		},
	}
	scope, _ := repository.NewScope(p, tenantID, auth.PermReadTenant)
	return scope
}

// RegisterLensTool exposes the Lens semantic compiler to governed agent/MCP callers.
// The caller can provide only a typed QueryIntent; raw SQL is never part of the contract.
func RegisterLensTool(reg *Registry, svc *lens.Service) error {
	if reg == nil {
		return errors.New("lens tool registry is required")
	}
	if svc == nil {
		return errors.New("lens service is required")
	}
	manifest := &ToolManifest{
		ToolID:                       ToolLensQuery,
		Version:                      "1.0.0",
		Description:                  "Executes a bounded, tenant-scoped SentinelFlow Lens semantic analytics intent. Raw SQL and financial mutations are not exposed.",
		Owner:                        "sentinel-lens",
		Status:                       ManifestStatusActive,
		PolicyDomain:                 policy.DomainTool,
		PolicyAction:                 policy.ActionQueryAnalytics,
		RequiredCapabilities:         []ToolCapability{CapAnalyticsQuery},
		SideEffectClass:              SideEffectReadOnly,
		MinAutonomy:                  1,
		MaxAutonomy:                  4,
		InputContract:                `{"type":"object","required":["schema_version","dataset_id","time_range","metrics","dimensions"],"additionalProperties":false}`,
		OutputContract:               `{"type":"object","required":["dataset_id","rows","chart","provenance"]}`,
		Timeout:                      8 * time.Second,
		MaxOutputBytes:               512 * 1024,
		DataClassifications:          []DataClassification{ClassificationInternal, ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationInternal, ClassificationMetadataOnly, ClassificationPublic},
		IdempotencyRequired:          false,
		VerificationRequired:         false,
		ShadowModeAllowed:            true,
		AllowedExecutionModes:        []string{"SHADOW", "ADVISORY", "LIVE"},
	}
	handler := func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		var intent lens.QueryIntent
		dec := json.NewDecoder(bytes.NewReader(args))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&intent); err != nil {
			return nil, fmt.Errorf("%w: invalid Lens query intent: %v", ErrInputValidationFailed, err)
		}
		result, err := svc.Execute(ctx, execCtx.TenantID, intent)
		if err != nil {
			return nil, fmt.Errorf("lens query: %w", err)
		}
		return json.Marshal(result)
	}
	return reg.Register(manifest, handler)
}

// RegisterCandidateTool registers or updates the remediation.candidate.create tool manifest and handler.
func RegisterCandidateTool(reg *Registry, candService CandidateServiceRunner) error {
	manifestRemediationCreate := &ToolManifest{
		ToolID:                       ToolRemediationCandidateCreate,
		Version:                      "1.0.0",
		Description:                  "Generates an immutable derived candidate financial artifact from a validated remediation plan.",
		Owner:                        "sentinel-core",
		Status:                       ManifestStatusActive,
		PolicyDomain:                 policy.DomainRemediation,
		PolicyAction:                 policy.ActionCreateCandidate,
		RequiredCapabilities:         []ToolCapability{CapCandidateCreate},
		SideEffectClass:              SideEffectCandidateSandboxWrite,
		MinAutonomy:                  2, // Autonomy A2 (Sandbox Candidate Creation)
		MaxAutonomy:                  2,
		InputContract:                `{"type":"object","properties":{"plan_hash":{"type":"string"}},"required":["plan_hash"]}`,
		OutputContract:               `{"type":"object"}`,
		Timeout:                      15 * time.Second,
		MaxOutputBytes:               128 * 1024,
		DataClassifications:          []DataClassification{ClassificationMetadataOnly},
		AllowedOutputClassifications: []DataClassification{ClassificationMetadataOnly, ClassificationPublic, ClassificationInternal},
		IdempotencyRequired:          true,
		VerificationRequired:         false,
		ShadowModeAllowed:            false, // Section 33: Sandbox candidate write blocked in SHADOW mode (plan-only in shadow)
		AllowedExecutionModes:        []string{"ADVISORY", "LIVE"},
	}

	handlerRemediationCreate := func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error) {
		var req candidate.CandidateCreationRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return nil, fmt.Errorf("%w: invalid candidate creation request: %v", ErrInputValidationFailed, err)
		}

		if req.PlanHash == "" {
			return nil, fmt.Errorf("%w: plan_hash is required", ErrInputValidationFailed)
		}

		// Inject trusted execution context properties
		req.TenantID = execCtx.TenantID
		req.WorkflowID = execCtx.WorkflowID

		if req.ParentArtifactID == 0 && execCtx.ArtifactID != "" {
			if id, err := strconv.ParseInt(execCtx.ArtifactID, 10, 64); err == nil && id > 0 {
				req.ParentArtifactID = id
			}
		}
		if req.IncidentID == 0 && execCtx.IncidentID != "" {
			if id, err := strconv.ParseInt(execCtx.IncidentID, 10, 64); err == nil && id > 0 {
				req.IncidentID = id
			}
		}
		if req.ExpectedParentSHA256 == "" {
			req.ExpectedParentSHA256 = execCtx.ArtifactSHA256
		} else if execCtx.ArtifactSHA256 != "" && req.ExpectedParentSHA256 != execCtx.ArtifactSHA256 {
			return nil, fmt.Errorf("%w: expected parent SHA-256 mismatch (%s != %s)", ErrPreconditionFailed, req.ExpectedParentSHA256, execCtx.ArtifactSHA256)
		}
		if req.AttemptNumber == 0 {
			if execCtx.ResourceVersion > 0 {
				req.AttemptNumber = execCtx.ResourceVersion
			} else {
				req.AttemptNumber = 1
			}
		}
		if req.AgentName == "" {
			req.AgentName = execCtx.CallerID
		}
		if req.AgentVersion == "" {
			req.AgentVersion = "1.0.0"
		}
		if req.PolicyDecisionHash == "" {
			req.PolicyDecisionHash = execCtx.PolicyBundleHash
		}
		if req.PolicyDecisionID == "" {
			req.PolicyDecisionID = fmt.Sprintf("POL-DEC-%s-%d", execCtx.WorkflowID, req.AttemptNumber)
		}
		if req.ToolManifestHash == "" {
			req.ToolManifestHash = "man-hash-remed-create"
		}
		if req.Confidence == "" {
			req.Confidence = "HIGH"
		}

		if candService == nil {
			res := map[string]interface{}{
				"status":              "SUCCESS",
				"plan_hash":           req.PlanHash,
				"candidate_id":        fmt.Sprintf("cand-%s-%d", execCtx.TenantID, time.Now().UnixNano()),
				"execution_source":    "GO_CANDIDATE_SERVICE_STUB",
				"data_classification": ClassificationMetadataOnly,
			}
			return json.Marshal(res)
		}

		scope := makeSystemScope(execCtx.TenantID, "toolgateway/remediation-candidate-create")
		res, err := candService.GenerateCandidate(ctx, scope, &req)
		if err != nil {
			return nil, err
		}

		return json.Marshal(res)
	}

	return reg.RegisterOrReplace(manifestRemediationCreate, handlerRemediationCreate)
}
