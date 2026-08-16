package connectors

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// The catalog.
//
// Nine entries, one driver. That ratio is the honest state of this platform and
// the registry enforces it: `Register` refuses a driver that claims AVAILABLE
// without a conformance record, so the only way an entry becomes selectable is
// for a real driver to pass the shared suite against a real server.
//
// The eight without drivers are listed anyway. A catalog that hid them would
// make the roadmap invisible and invite two people to implement Oracle twice;
// listing them as PLANNED, with the reason, makes the gap reviewable.

// ConformanceRecord is the evidence behind an AVAILABLE status.
//
// It is required, and its absence is what keeps a driver out of the selectable
// set. The fields are the ones needed to decide whether the evidence is still
// worth anything: a pass recorded against a server version nobody runs, or a
// commit from before the driver was rewritten, is not evidence.
type ConformanceRecord struct {
	ConnectorType string    `json:"connectorType"`
	ServerVersion string    `json:"serverVersion"`
	DriverVersion string    `json:"driverVersion"`
	TestCommit    string    `json:"testCommit"`
	RunAt         time.Time `json:"runAt"`
	Passed        int       `json:"passed"`
	Failed        int       `json:"failed"`
	// Skipped names checks that did not run, so a partial pass cannot be
	// mistaken for a full one.
	Skipped []string `json:"skipped,omitempty"`
}

// Complete reports whether the record supports an AVAILABLE status.
//
// A skipped check disqualifies the record. RunConformance already folds skips
// into the failure count, but a record can also arrive hand-written or from an
// evidence file, and those paths would otherwise let a run with its TLS
// verification untested vouch for a driver.
func (r *ConformanceRecord) Complete() bool {
	return r != nil && r.Failed == 0 && len(r.Skipped) == 0 && r.Passed > 0 &&
		r.ServerVersion != "" && r.TestCommit != "" && !r.RunAt.IsZero()
}

// Registry holds the catalog.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

type entry struct {
	descriptor Descriptor
	driver     Connector
}

// NewRegistry builds the catalog with every entry in its honest state.
func NewRegistry() *Registry {
	r := &Registry{entries: map[string]*entry{}}
	for _, d := range catalogDescriptors() {
		r.entries[d.Type] = &entry{
			descriptor: d,
			driver:     notImplemented{connectorType: d.Type, reason: d.StatusReason},
		}
	}
	return r
}

// Register attaches a real driver to a catalog entry.
//
// A driver may be registered with a nil conformance record, in which case the
// entry becomes IMPLEMENTING: the code exists, it is visible, and it is not
// selectable. It becomes AVAILABLE only with a complete record.
//
// This is the structural rule that prevents the Integration Hub's return. There
// is no argument by which a caller can assert availability; it is derived from
// evidence or it does not happen.
func (r *Registry) Register(driver Connector, record *ConformanceRecord) error {
	if driver == nil {
		return fmt.Errorf("a nil driver cannot be registered")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[driver.Type()]
	if !ok {
		return fmt.Errorf("%q is not a catalog entry; add a descriptor before registering a driver",
			driver.Type())
	}

	e.driver = driver
	e.descriptor.Capabilities = driver.Capabilities()

	switch {
	case record == nil:
		e.descriptor.Status = StatusImplementing
		e.descriptor.StatusReason = "a driver exists and has not yet passed the shared conformance suite"
		e.descriptor.Conformance = nil
	case !record.Complete():
		e.descriptor.Status = StatusImplementing
		e.descriptor.StatusReason = fmt.Sprintf(
			"the last conformance run recorded %d passed and %d failed; every check must pass",
			record.Passed, record.Failed)
		e.descriptor.Conformance = record
	default:
		e.descriptor.Status = StatusAvailable
		e.descriptor.StatusReason = ""
		e.descriptor.Conformance = record
	}
	return nil
}

// Disable takes an entry out of service with a stated reason.
func (r *Registry) Disable(connectorType, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[connectorType]
	if !ok {
		return fmt.Errorf("%q is not a catalog entry", connectorType)
	}
	e.descriptor.Status = StatusDisabled
	e.descriptor.StatusReason = reason
	return nil
}

// MarkDegraded records that an available connector's own health checks are
// failing. Existing connections continue where they can; new ones are refused,
// because a connection configured against a failing connector has never worked
// and would look as though it had.
func (r *Registry) MarkDegraded(connectorType, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[connectorType]
	if !ok {
		return fmt.Errorf("%q is not a catalog entry", connectorType)
	}
	if e.descriptor.Status != StatusAvailable && e.descriptor.Status != StatusDegraded {
		return fmt.Errorf("%q is %s and cannot be marked degraded", connectorType, e.descriptor.Status)
	}
	e.descriptor.Status = StatusDegraded
	e.descriptor.StatusReason = reason
	return nil
}

// Catalog returns every descriptor, sorted by display name.
//
// Everything is returned, including entries with no driver. Hiding them would
// make the platform look complete.
func (r *Registry) Catalog() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Descriptor, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DisplayName < out[j].DisplayName })
	return out
}

// Descriptor returns one entry.
func (r *Registry) Descriptor(connectorType string) (Descriptor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[connectorType]
	if !ok {
		return Descriptor{}, fmt.Errorf("%q is not a connector in this catalog", connectorType)
	}
	return e.descriptor, nil
}

// Driver returns the implementation for a selectable connector.
//
// It refuses anything that is not AVAILABLE. This is the single choke point:
// every path that could contact a customer database goes through here, so a
// connector without conformance evidence cannot be reached even by a caller
// that has its descriptor.
func (r *Registry) Driver(connectorType string) (Connector, Descriptor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[connectorType]
	if !ok {
		return nil, Descriptor{}, fmt.Errorf("%q is not a connector in this catalog", connectorType)
	}
	if !e.descriptor.Status.Selectable() {
		return nil, e.descriptor, fmt.Errorf("%s is %s: %w (%s)",
			connectorType, e.descriptor.Status, ErrNotSelectable, e.descriptor.StatusReason)
	}
	return e.driver, e.descriptor, nil
}

// Available returns the connector types a tenant may currently select.
func (r *Registry) Available() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for t, e := range r.entries {
		if e.descriptor.Status.Selectable() {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}
