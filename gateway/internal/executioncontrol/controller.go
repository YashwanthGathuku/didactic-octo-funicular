// Package executioncontrol provides deterministic, fail-closed operational
// controls for bounded agent execution.
//
// It does not decide business policy and does not replace Tool Gateway,
// PolicyEngine, workflow state or human authorization.  It exists to stop or
// bound agent execution when operators declare a kill switch, a workflow
// deadline expires, or a trusted execution budget is exhausted.
package executioncontrol

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrKilled              = errors.New("agent execution disabled by kill switch")
	ErrToolBudgetExhausted = errors.New("agent tool-call budget exhausted")
	ErrConcurrentExceeded  = errors.New("agent concurrent-execution budget exhausted")
	ErrDeadlineExceeded    = errors.New("agent workflow execution deadline exceeded")
	ErrStaleGeneration     = errors.New("kill-switch generation must increase monotonically")
	ErrInvalidLimits       = errors.New("invalid execution limits")
)

// Scope identifies an operational kill-switch boundary.
type Scope string

const (
	ScopeGlobal   Scope = "GLOBAL"
	ScopeTenant   Scope = "TENANT"
	ScopeWorkflow Scope = "WORKFLOW"
	ScopeAgent    Scope = "AGENT"
)

// Limits are trusted control-plane limits. Zero means "not additionally
// constrained" for that dimension; callers should still apply their existing
// model/token/time budgets independently.
type Limits struct {
	MaxToolCalls  uint64        `json:"max_tool_calls"`
	MaxConcurrent int           `json:"max_concurrent"`
	MaxDuration   time.Duration `json:"max_duration"`
}

func (l Limits) Validate() error {
	if l.MaxConcurrent < 0 || l.MaxDuration < 0 {
		return ErrInvalidLimits
	}
	return nil
}

// KillSwitch is versioned so stale operator/config updates cannot accidentally
// re-enable execution after a newer stop instruction.
type KillSwitch struct {
	Scope      Scope     `json:"scope"`
	ScopeID    string    `json:"scope_id"`
	Enabled    bool      `json:"enabled"`
	Reason     string    `json:"reason"`
	Generation uint64    `json:"generation"`
	UpdatedAt  time.Time `json:"updated_at"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

// CheckRequest contains only server-generated identity/workflow metadata.
type CheckRequest struct {
	TenantID   string
	WorkflowID string
	CallerID   string
	CallerType string
	Now        time.Time
}

type workflowKey struct {
	tenant   string
	workflow string
}

type workflowState struct {
	limits    Limits
	startedAt time.Time
	toolCalls uint64
	inFlight  int
}

// Controller is concurrency-safe. It intentionally keeps no financial state.
// A process restart resets counters; durable business idempotency remains owned
// by ToolGateway/workflow persistence. The capability matrix must therefore
// describe this as process-level operational control unless a durable adapter is
// configured later.
type Controller struct {
	mu sync.Mutex

	defaultLimits Limits
	workflows     map[workflowKey]*workflowState
	killSwitches  map[string]KillSwitch
}

func NewController(defaultLimits Limits) (*Controller, error) {
	if err := defaultLimits.Validate(); err != nil {
		return nil, err
	}
	return &Controller{
		defaultLimits: defaultLimits,
		workflows:     make(map[workflowKey]*workflowState),
		killSwitches:  make(map[string]KillSwitch),
	}, nil
}

func switchKey(scope Scope, scopeID string) string {
	return string(scope) + ":" + strings.TrimSpace(scopeID)
}

// SetKillSwitch applies a monotonic operator/config update.
func (c *Controller) SetKillSwitch(sw KillSwitch) error {
	if c == nil {
		return errors.New("executioncontrol: nil controller")
	}
	if sw.Scope == "" {
		return errors.New("executioncontrol: kill-switch scope is required")
	}
	if sw.Scope != ScopeGlobal && strings.TrimSpace(sw.ScopeID) == "" {
		return errors.New("executioncontrol: non-global kill switch requires scope_id")
	}
	if sw.Generation == 0 {
		return errors.New("executioncontrol: kill-switch generation must be > 0")
	}
	if sw.UpdatedAt.IsZero() {
		sw.UpdatedAt = time.Now().UTC()
	}

	key := switchKey(sw.Scope, sw.ScopeID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if prior, ok := c.killSwitches[key]; ok && sw.Generation <= prior.Generation {
		return fmt.Errorf("%w: %s generation %d <= %d", ErrStaleGeneration, key, sw.Generation, prior.Generation)
	}
	c.killSwitches[key] = sw
	return nil
}

// ConfigureWorkflow installs trusted workflow-specific limits before execution.
func (c *Controller) ConfigureWorkflow(tenantID, workflowID string, limits Limits, startedAt time.Time) error {
	if err := limits.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(workflowID) == "" {
		return errors.New("executioncontrol: tenant_id and workflow_id are required")
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := workflowKey{tenant: tenantID, workflow: workflowID}
	current := c.workflows[key]
	if current == nil {
		c.workflows[key] = &workflowState{limits: limits, startedAt: startedAt}
		return nil
	}
	// Reconfiguration never resets consumed budget or start time.
	current.limits = limits
	return nil
}

func switchActive(sw KillSwitch, now time.Time) bool {
	if !sw.Enabled {
		return false
	}
	return sw.ExpiresAt.IsZero() || now.Before(sw.ExpiresAt)
}

func (c *Controller) activeKillLocked(req CheckRequest) (KillSwitch, bool) {
	keys := []string{
		switchKey(ScopeGlobal, ""),
		switchKey(ScopeTenant, req.TenantID),
		switchKey(ScopeWorkflow, req.WorkflowID),
		switchKey(ScopeAgent, req.CallerID),
	}
	for _, key := range keys {
		if sw, ok := c.killSwitches[key]; ok && switchActive(sw, req.Now) {
			return sw, true
		}
	}
	return KillSwitch{}, false
}

// Permit holds one in-flight execution slot. Release must be called exactly
// once; it is idempotent to make deferred cleanup safe.
type Permit struct {
	once sync.Once
	c    *Controller
	key  workflowKey
}

func (p *Permit) Release() {
	if p == nil || p.c == nil {
		return
	}
	p.once.Do(func() {
		p.c.mu.Lock()
		defer p.c.mu.Unlock()
		if state := p.c.workflows[p.key]; state != nil && state.inFlight > 0 {
			state.inFlight--
		}
	})
}

// Acquire enforces kill switches and process-level workflow budgets for AGENT
// callers. Humans/services are not blocked by the agent kill switch because
// operators must retain the ability to investigate and recover the system.
func (c *Controller) Acquire(req CheckRequest) (*Permit, error) {
	if c == nil {
		return nil, errors.New("executioncontrol: nil controller")
	}
	if !strings.EqualFold(strings.TrimSpace(req.CallerType), "AGENT") {
		return &Permit{}, nil
	}
	if strings.TrimSpace(req.TenantID) == "" || strings.TrimSpace(req.CallerID) == "" {
		return nil, errors.New("executioncontrol: tenant_id and caller_id are required")
	}
	if req.Now.IsZero() {
		req.Now = time.Now().UTC()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if sw, ok := c.activeKillLocked(req); ok {
		return nil, fmt.Errorf("%w: scope=%s reason=%s generation=%d", ErrKilled, sw.Scope, sw.Reason, sw.Generation)
	}

	key := workflowKey{tenant: req.TenantID, workflow: req.WorkflowID}
	state := c.workflows[key]
	if state == nil {
		state = &workflowState{limits: c.defaultLimits, startedAt: req.Now}
		c.workflows[key] = state
	}

	if state.limits.MaxDuration > 0 && !state.startedAt.IsZero() && req.Now.Sub(state.startedAt) >= state.limits.MaxDuration {
		return nil, fmt.Errorf("%w: max_duration=%s", ErrDeadlineExceeded, state.limits.MaxDuration)
	}
	if state.limits.MaxToolCalls > 0 && state.toolCalls >= state.limits.MaxToolCalls {
		return nil, fmt.Errorf("%w: used=%d max=%d", ErrToolBudgetExhausted, state.toolCalls, state.limits.MaxToolCalls)
	}
	if state.limits.MaxConcurrent > 0 && state.inFlight >= state.limits.MaxConcurrent {
		return nil, fmt.Errorf("%w: inflight=%d max=%d", ErrConcurrentExceeded, state.inFlight, state.limits.MaxConcurrent)
	}

	state.toolCalls++
	state.inFlight++
	return &Permit{c: c, key: key}, nil
}

// Snapshot is safe, non-sensitive operational evidence for UI/telemetry.
type Snapshot struct {
	TenantID      string `json:"tenant_id"`
	WorkflowID    string `json:"workflow_id"`
	ToolCallsUsed uint64 `json:"tool_calls_used"`
	InFlight      int    `json:"in_flight"`
	MaxToolCalls  uint64 `json:"max_tool_calls"`
	MaxConcurrent int    `json:"max_concurrent"`
	MaxDurationMS int64  `json:"max_duration_ms"`
}

func (c *Controller) Snapshot(tenantID, workflowID string) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.workflows[workflowKey{tenant: tenantID, workflow: workflowID}]
	if state == nil {
		return Snapshot{TenantID: tenantID, WorkflowID: workflowID}
	}
	return Snapshot{
		TenantID:      tenantID,
		WorkflowID:    workflowID,
		ToolCallsUsed: state.toolCalls,
		InFlight:      state.inFlight,
		MaxToolCalls:  state.limits.MaxToolCalls,
		MaxConcurrent: state.limits.MaxConcurrent,
		MaxDurationMS: state.limits.MaxDuration.Milliseconds(),
	}
}
