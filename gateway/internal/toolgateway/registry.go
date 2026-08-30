package toolgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolHandler is the execution function for a registered tool.
// It receives the verified TrustedExecutionContext and untrusted input args, returning output or error.
type ToolHandler func(ctx context.Context, execCtx *TrustedExecutionContext, args json.RawMessage) (json.RawMessage, error)

// RegisteredTool wraps a validated ToolManifest and its executable ToolHandler.
type RegisteredTool struct {
	Manifest *ToolManifest
	Handler  ToolHandler
}

// Registry provides thread-safe, immutable/versioned tool registration and lookup.
type Registry struct {
	mu             sync.RWMutex
	tools          map[string]map[string]*RegisteredTool // tool_id -> version -> tool
	activeVersions map[string]string                     // tool_id -> active version
}

// NewRegistry creates a new, empty Tool Registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:          make(map[string]map[string]*RegisteredTool),
		activeVersions: make(map[string]string),
	}
}

// Register registers a new tool manifest and handler.
// It validates all manifest invariants and computes the RFC 8785 manifest hash before activation.
// Re-registering an existing (tool_id, version) returns ErrDuplicateToolRegistration.
func (r *Registry) Register(m *ToolManifest, handler ToolHandler) error {
	if m == nil {
		return fmt.Errorf("%w: nil manifest", ErrInvalidManifest)
	}
	if handler == nil {
		return fmt.Errorf("%w: nil handler", ErrInvalidManifest)
	}

	if err := m.Validate(); err != nil {
		return err
	}

	manifestHash, err := m.ComputeManifestHash()
	if err != nil {
		return fmt.Errorf("compute manifest hash: %w", err)
	}
	if m.ManifestHash != "" && m.ManifestHash != manifestHash {
		return fmt.Errorf("%w: manifest hash mismatch (provided: %s, computed: %s)", ErrInvalidManifest, m.ManifestHash, manifestHash)
	}
	m.ManifestHash = manifestHash

	r.mu.Lock()
	defer r.mu.Unlock()

	versions, exists := r.tools[m.ToolID]
	if !exists {
		versions = make(map[string]*RegisteredTool)
		r.tools[m.ToolID] = versions
	}

	if _, alreadyRegistered := versions[m.Version]; alreadyRegistered {
		return fmt.Errorf("%w: tool %s v%s", ErrDuplicateToolRegistration, m.ToolID, m.Version)
	}

	registered := &RegisteredTool{
		Manifest: m,
		Handler:  handler,
	}

	versions[m.Version] = registered

	// If active or first version, set as default active version
	if m.Status == ManifestStatusActive || r.activeVersions[m.ToolID] == "" {
		r.activeVersions[m.ToolID] = m.Version
	}

	return nil
}

// RegisterOrReplace registers or updates a tool manifest and handler.
// Unlike Register, it does not fail if the tool and version are already registered.
func (r *Registry) RegisterOrReplace(m *ToolManifest, handler ToolHandler) error {
	if m == nil {
		return fmt.Errorf("%w: nil manifest", ErrInvalidManifest)
	}
	if handler == nil {
		return fmt.Errorf("%w: nil handler", ErrInvalidManifest)
	}

	if err := m.Validate(); err != nil {
		return err
	}

	manifestHash, err := m.ComputeManifestHash()
	if err != nil {
		return fmt.Errorf("compute manifest hash: %w", err)
	}
	if m.ManifestHash != "" && m.ManifestHash != manifestHash {
		return fmt.Errorf("%w: manifest hash mismatch (provided: %s, computed: %s)", ErrInvalidManifest, m.ManifestHash, manifestHash)
	}
	m.ManifestHash = manifestHash

	r.mu.Lock()
	defer r.mu.Unlock()

	versions, exists := r.tools[m.ToolID]
	if !exists {
		versions = make(map[string]*RegisteredTool)
		r.tools[m.ToolID] = versions
	}

	registered := &RegisteredTool{
		Manifest: m,
		Handler:  handler,
	}

	versions[m.Version] = registered

	if m.Status == ManifestStatusActive || r.activeVersions[m.ToolID] == "" {
		r.activeVersions[m.ToolID] = m.Version
	}

	return nil
}

// Lookup finds a registered tool by ID and version.
// If version is empty, it resolves to the active version of the tool.
func (r *Registry) Lookup(toolID, version string) (*RegisteredTool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, exists := r.tools[toolID]
	if !exists || len(versions) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnregisteredTool, toolID)
	}

	targetVersion := version
	if targetVersion == "" {
		targetVersion = r.activeVersions[toolID]
	}

	tool, found := versions[targetVersion]
	if !found {
		return nil, fmt.Errorf("%w: tool %s v%s", ErrUnknownToolVersion, toolID, targetVersion)
	}

	if tool.Manifest.Status == ManifestStatusRetired {
		return nil, fmt.Errorf("%w: tool %s v%s is retired", ErrUnregisteredTool, toolID, targetVersion)
	}

	return tool, nil
}

// ListManifests returns all registered tool manifests.
func (r *Registry) ListManifests() []*ToolManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ToolManifest
	for _, versions := range r.tools {
		for _, tool := range versions {
			result = append(result, tool.Manifest)
		}
	}
	return result
}
