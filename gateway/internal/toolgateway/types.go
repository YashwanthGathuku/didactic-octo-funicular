package toolgateway

import (
	"errors"
	"time"
)

// Common SentinelFlow Tool Gateway errors
var (
	ErrUnregisteredTool           = errors.New("tool gateway: unregistered tool")
	ErrUnknownToolVersion         = errors.New("tool gateway: unknown tool version")
	ErrDuplicateToolRegistration  = errors.New("tool gateway: duplicate tool registration")
	ErrInvalidManifest            = errors.New("tool gateway: invalid tool manifest")
	ErrTenantMismatch             = errors.New("tool gateway: caller tenant does not match execution context")
	ErrMissingTenantID            = errors.New("tool gateway: tenant ID is required")
	ErrUnauthorizedCapability     = errors.New("tool gateway: caller lacks required capability")
	ErrToolNotInAllowlist         = errors.New("tool gateway: tool is not permitted in execution context allowlist")
	ErrAutonomyExceeded           = errors.New("tool gateway: caller autonomy level exceeds manifest maximum")
	ErrAutonomyInsufficient       = errors.New("tool gateway: caller autonomy level below manifest minimum")
	ErrPolicyDenial               = errors.New("tool gateway: action denied by deterministic policy")
	ErrRequireHumanReview         = errors.New("tool gateway: action requires human review and cannot be executed autonomously")
	ErrCapabilityProhibited       = errors.New("tool gateway: capability is explicitly prohibited by policy")
	ErrUnsatisfiedObligation      = errors.New("tool gateway: mandatory policy obligation cannot be satisfied")
	ErrUnknownObligation          = errors.New("tool gateway: unknown policy obligation type")
	ErrIdempotencyConflict        = errors.New("tool gateway: idempotency key conflict (different payload for same key)")
	ErrPreconditionFailed         = errors.New("tool gateway: resource precondition failed (TOCTOU violation)")
	ErrInputValidationFailed      = errors.New("tool gateway: input arguments failed schema validation")
	ErrInputTooLarge              = errors.New("tool gateway: input payload exceeds maximum allowed size")
	ErrOutputTooLarge             = errors.New("tool gateway: tool output exceeded maximum allowed size")
	ErrOutputValidationFailed     = errors.New("tool gateway: tool output failed schema/security validation")
	ErrShadowModeProhibited       = errors.New("tool gateway: side-effectful tool execution blocked in SHADOW mode")
	ErrToolExecutionTimeout       = errors.New("tool gateway: tool execution timed out")
	ErrToolPanicRecovered         = errors.New("tool gateway: tool execution panicked and was recovered")
	ErrRateLimitExceeded          = errors.New("tool gateway: tenant/caller tool rate limit exceeded")
	ErrIrreversibleFinancialAgent = errors.New("tool gateway: IRREVERSIBLE_FINANCIAL tools cannot be registered for agent execution")
)

// SideEffectClass categorizes the blast radius and external impact of a tool.
type SideEffectClass string

const (
	SideEffectReadOnly              SideEffectClass = "READ_ONLY"
	SideEffectInternalStateWrite    SideEffectClass = "INTERNAL_STATE_WRITE"
	SideEffectCandidateSandboxWrite SideEffectClass = "CANDIDATE_SANDBOX_WRITE"
	SideEffectReversibleExternal    SideEffectClass = "REVERSIBLE_EXTERNAL"
	SideEffectIrreversibleExternal  SideEffectClass = "IRREVERSIBLE_EXTERNAL"
	SideEffectIrreversibleFinancial SideEffectClass = "IRREVERSIBLE_FINANCIAL"
)

// ManifestStatus represents the lifecycle of a ToolManifest.
type ManifestStatus string

const (
	ManifestStatusActive     ManifestStatus = "ACTIVE"
	ManifestStatusDeprecated ManifestStatus = "DEPRECATED"
	ManifestStatusRetired    ManifestStatus = "RETIRED"
)

// InvocationStatus tracks the lifecycle of a tool invocation.
type InvocationStatus string

const (
	StatusReceived   InvocationStatus = "RECEIVED"
	StatusAuthorized InvocationStatus = "AUTHORIZED"
	StatusExecuting  InvocationStatus = "EXECUTING"
	StatusSucceeded  InvocationStatus = "SUCCEEDED"
	StatusDenied     InvocationStatus = "DENIED"
	StatusFailed     InvocationStatus = "FAILED"
	StatusTimedOut   InvocationStatus = "TIMED_OUT"
	StatusUncertain  InvocationStatus = "UNCERTAIN"
)

// ToolCapability represents a strongly typed capability required by a tool.
type ToolCapability string

const (
	// Implemented P04 Capabilities
	CapIncidentRead         ToolCapability = "INCIDENT_READ"
	CapArtifactMetadataRead ToolCapability = "ARTIFACT_METADATA_READ"
	CapFindingsReadRedacted ToolCapability = "FINDINGS_READ_REDACTED"
	CapRunbookRead          ToolCapability = "RUNBOOK_READ"
	CapWorkflowRead         ToolCapability = "WORKFLOW_READ"
	CapTelemetryRead        ToolCapability = "TELEMETRY_READ"

	// Reserved future capabilities
	CapCandidateCreate         ToolCapability = "CANDIDATE_CREATE"
	CapAnalyticsQuery          ToolCapability = "ANALYTICS_QUERY"
	CapEnterpriseActionPrepare ToolCapability = "ENTERPRISE_ACTION_PREPARE"
	CapEnterpriseActionExecute ToolCapability = "ENTERPRISE_ACTION_EXECUTE"
)

// DataClassification defines authoritative confidentiality tiers for tool schemas and outputs.
type DataClassification string

const (
	ClassificationPublic             DataClassification = "PUBLIC"
	ClassificationInternal           DataClassification = "INTERNAL"
	ClassificationConfidential       DataClassification = "CONFIDENTIAL"
	ClassificationFinancialSensitive DataClassification = "FINANCIAL_SENSITIVE"
	ClassificationPII                DataClassification = "PII"
	ClassificationSecret             DataClassification = "SECRET"
	ClassificationMetadataOnly       DataClassification = "METADATA_ONLY"
	ClassificationRedactedFindings   DataClassification = "REDACTED_FINDINGS"
)

// Standard Caller Types
const (
	CallerTypeAgent                = "AGENT"
	CallerTypeHuman                = "HUMAN"
	CallerTypeService              = "SERVICE"
	CallerTypeDeterministicControl = "DETERMINISTIC_CONTROL"
	CallerTypeAPI                  = "API"
)

// Default constants for Tool Gateway
const (
	DefaultToolTimeout    = 10 * time.Second
	MaxToolTimeout        = 60 * time.Second
	DefaultMaxOutputBytes = 1024 * 1024 // 1 MB
	DefaultMaxInputBytes  = 256 * 1024  // 256 KB
	ToolGatewayVersion    = "1.0.0"
)
