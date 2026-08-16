package connectors

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Carrying conformance evidence into a running process.
//
// The suite runs in CI against a real server, and a process that has just
// booted has verified nothing itself. Promoting a connector to AVAILABLE from a
// constant in the binary would be a claim of verification made by the component
// that did not do the verifying -- the same defect as returning
// `mTLSVerified: true`.
//
// So the evidence travels as a file the CI run produces and the deployment
// ships. The binary validates it before trusting it, and every failure to
// validate leaves the connector unselectable rather than defaulting to
// available.

// EvidenceFileEnv names the environment variable pointing at the evidence file.
const EvidenceFileEnv = "SENTINEL_CONNECTOR_EVIDENCE"

// maxEvidenceAge bounds how long a conformance run stays good.
//
// Ninety days is a judgement, not a measurement, and it is stated as one. The
// reason there is a bound at all: a driver verified against a server version
// nobody runs any more, by a test suite that has since been rewritten, is not
// evidence about the code now shipping. Expiry makes a stale record fail loudly
// instead of silently vouching for something else.
const maxEvidenceAge = 90 * 24 * time.Hour

// EvidenceFile is the artefact a conformance run produces.
type EvidenceFile struct {
	// GeneratedAt is when the run happened, not when the file was written.
	GeneratedAt time.Time            `json:"generatedAt"`
	Records     []*ConformanceRecord `json:"records"`
}

// LoadEvidence reads and validates the evidence file named by the environment.
//
// A missing variable is not an error: it is the ordinary state of a development
// machine, and the result is simply that no connector is selectable. An
// unreadable or invalid file *is* an error, because a deployment that meant to
// carry evidence and shipped something broken must find out.
func LoadEvidence() (map[string]*ConformanceRecord, error) {
	path := os.Getenv(EvidenceFileEnv)
	if path == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read connector evidence: %w", err)
	}

	var file EvidenceFile
	if err := json.Unmarshal(body, &file); err != nil {
		return nil, fmt.Errorf("connector evidence at %s is not readable JSON: %w", path, err)
	}

	out := map[string]*ConformanceRecord{}
	for _, rec := range file.Records {
		if rec == nil || rec.ConnectorType == "" {
			return nil, fmt.Errorf("connector evidence at %s contains a record with no connector type", path)
		}
		// Every rejection below leaves the connector unselectable, which is the
		// safe direction: a connector nobody can use is a missing feature, and
		// a connector wrongly marked verified is a false assurance.
		if !rec.Complete() {
			return nil, fmt.Errorf(
				"connector evidence for %s is incomplete (%d passed, %d failed, %d skipped); "+
					"it will not make the connector available",
				rec.ConnectorType, rec.Passed, rec.Failed, len(rec.Skipped))
		}
		if age := time.Since(rec.RunAt); age > maxEvidenceAge {
			return nil, fmt.Errorf(
				"connector evidence for %s is %d days old, past the %d-day limit; re-run the "+
					"conformance suite",
				rec.ConnectorType, int(age.Hours()/24), int(maxEvidenceAge.Hours()/24))
		}
		out[rec.ConnectorType] = rec
	}
	return out, nil
}

// ApplyEvidence registers drivers with whatever evidence exists for them.
//
// A driver with no matching record is registered anyway, as IMPLEMENTING: the
// code exists and is visible, and it is not selectable. A record whose driver
// version disagrees with the running driver is rejected -- it verified
// different code.
func ApplyEvidence(r *Registry, drivers []Connector, evidence map[string]*ConformanceRecord) []string {
	var notes []string
	for _, d := range drivers {
		rec := evidence[d.Type()]
		if rec != nil && rec.DriverVersion != "" {
			if current := driverVersion(d); current != "" && current != rec.DriverVersion {
				notes = append(notes, fmt.Sprintf(
					"%s: the evidence was produced against driver %q and this build carries %q, "+
						"so it verified different code; the connector stays unselectable",
					d.Type(), rec.DriverVersion, current))
				rec = nil
			}
		}
		if err := r.Register(d, rec); err != nil {
			notes = append(notes, fmt.Sprintf("%s: %v", d.Type(), err))
			continue
		}
		if rec == nil {
			notes = append(notes, fmt.Sprintf(
				"%s: no conformance evidence in this deployment, so it is visible and not selectable",
				d.Type()))
		} else {
			notes = append(notes, fmt.Sprintf(
				"%s: AVAILABLE on evidence from %s against %s (commit %s)",
				d.Type(), rec.RunAt.Format(time.RFC3339), rec.ServerVersion, rec.TestCommit))
		}
	}
	return notes
}

// versioned is implemented by a driver that can name the library it uses.
type versioned interface{ Version() string }

func driverVersion(c Connector) string {
	if v, ok := c.(versioned); ok {
		return v.Version()
	}
	return ""
}

// WriteEvidence serialises a set of reports into the artefact format.
//
// Used by the conformance run so the file CI publishes is produced by the same
// code that consumes it -- a format defined in two places drifts, and the drift
// presents as a deployment that silently has no evidence.
func WriteEvidence(path string, reports ...ConformanceReport) error {
	file := EvidenceFile{GeneratedAt: time.Now().UTC()}
	for _, rep := range reports {
		if !rep.Passed() {
			return fmt.Errorf("refusing to write evidence for %s: the run did not pass",
				rep.ConnectorType)
		}
		file.Records = append(file.Records, rep.Record)
	}
	body, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}
