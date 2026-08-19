package connectors

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Tests for Prompt 16 Stage 16.1: Connector contracts and catalog

func TestCatalog_ContainsAllNineEntries(t *testing.T) {
	reg := NewRegistry()
	descriptors := reg.Catalog()

	if len(descriptors) != 9 {
		t.Fatalf("expected exactly 9 catalog entries, got %d", len(descriptors))
	}

	expectedTypes := map[string]bool{
		"postgresql": false,
		"mysql":      false,
		"mariadb":    false,
		"sqlserver":  false,
		"oracle":     false,
		"snowflake":  false,
		"redshift":   false,
		"bigquery":   false,
		"databricks": false,
	}

	for _, d := range descriptors {
		if _, ok := expectedTypes[d.Type]; !ok {
			t.Errorf("unexpected catalog entry type: %q", d.Type)
		}
		expectedTypes[d.Type] = true

		// By default, every entry must start as PLANNED and not selectable
		if d.Status != StatusPlanned {
			t.Errorf("catalog entry %q has status %q, want PLANNED", d.Type, d.Status)
		}
		if d.Status.Selectable() {
			t.Errorf("planned catalog entry %q reports selectable=true; must be false", d.Type)
		}
		if d.StatusReason == "" {
			t.Errorf("catalog entry %q is missing a StatusReason", d.Type)
		}
		if len(d.Fields) == 0 {
			t.Errorf("catalog entry %q has no fields defined in its descriptor", d.Type)
		}
	}

	for typ, found := range expectedTypes {
		if !found {
			t.Errorf("missing expected catalog entry: %q", typ)
		}
	}
}

func TestCatalog_PlannedDriversReturnTypedErrors(t *testing.T) {
	reg := NewRegistry()
	types := []string{
		"postgresql", "mysql", "mariadb", "sqlserver",
		"oracle", "snowflake", "redshift", "bigquery", "databricks",
	}

	ctx := context.Background()
	cfg := Config{Type: "oracle", Fields: map[string]string{"host": "oracle.internal"}}
	sec := NewSecrets(nil)

	for _, typ := range types {
		// Calling reg.Driver on unselectable connector must return ErrNotSelectable
		_, _, err := reg.Driver(typ)
		if !errors.Is(err, ErrNotSelectable) {
			t.Fatalf("reg.Driver(%q) returned err=%v, want ErrNotSelectable", typ, err)
		}

		// Directly testing notImplemented driver behavior
		driver := notImplemented{connectorType: typ, reason: "testing"}

		if driver.Type() != typ {
			t.Errorf("driver.Type() = %q, want %q", driver.Type(), typ)
		}

		// ValidateConfig must fail with ErrNotImplemented
		if err := driver.ValidateConfig(cfg); !errors.Is(err, ErrNotImplemented) {
			t.Errorf("ValidateConfig on %q returned err=%v, want ErrNotImplemented", typ, err)
		}

		// TestConnection must return HealthNeverChecked and ErrNotImplemented
		h, err := driver.TestConnection(ctx, cfg, sec)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("TestConnection on %q returned err=%v, want ErrNotImplemented", typ, err)
		}
		if h.State != HealthNeverChecked {
			t.Errorf("TestConnection on %q returned state=%q, want NEVER_CHECKED", typ, h.State)
		}

		// DiscoverResources must fail with ErrNotImplemented
		if _, err := driver.DiscoverResources(ctx, cfg, sec); !errors.Is(err, ErrNotImplemented) {
			t.Errorf("DiscoverResources on %q returned err=%v, want ErrNotImplemented", typ, err)
		}

		// ExecuteTemplate must fail with ErrNotImplemented
		if _, err := driver.ExecuteTemplate(ctx, cfg, sec, Template{}, nil, DefaultLimits()); !errors.Is(err, ErrNotImplemented) {
			t.Errorf("ExecuteTemplate on %q returned err=%v, want ErrNotImplemented", typ, err)
		}

		// Health check must return HealthNeverChecked
		h = driver.Health(ctx, cfg, sec)
		if h.State != HealthNeverChecked {
			t.Errorf("Health on %q returned state=%q, want NEVER_CHECKED", typ, h.State)
		}

		// Close must succeed cleanly
		if err := driver.Close(); err != nil {
			t.Errorf("Close on %q returned err=%v", typ, err)
		}
	}
}

func TestCatalog_RegistrationLifecycleAndStatusTransitions(t *testing.T) {
	reg := NewRegistry()

	// 1. Initial status is PLANNED
	d, err := reg.Descriptor("postgresql")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != StatusPlanned {
		t.Fatalf("expected status PLANNED, got %s", d.Status)
	}

	// 2. Registering a driver with nil conformance makes it IMPLEMENTING
	mockDriver := &mockTestConnector{connectorType: "postgresql"}
	if err := reg.Register(mockDriver, nil); err != nil {
		t.Fatalf("failed to register driver: %v", err)
	}

	d, _ = reg.Descriptor("postgresql")
	if d.Status != StatusImplementing {
		t.Errorf("status after register with nil conformance = %q, want IMPLEMENTING", d.Status)
	}
	if d.Status.Selectable() {
		t.Errorf("IMPLEMENTING status must not be selectable")
	}

	// 3. Registering with an incomplete conformance record remains IMPLEMENTING
	incompleteRecord := &ConformanceRecord{
		ConnectorType: "postgresql",
		ServerVersion: "16.1",
		TestCommit:    "abc1234",
		RunAt:         time.Now(),
		Passed:        5,
		Failed:        1, // Failed test disqualifies
	}
	if err := reg.Register(mockDriver, incompleteRecord); err != nil {
		t.Fatal(err)
	}
	d, _ = reg.Descriptor("postgresql")
	if d.Status != StatusImplementing {
		t.Errorf("status with failing conformance = %q, want IMPLEMENTING", d.Status)
	}

	// 4. Registering with a complete conformance record transitions to AVAILABLE
	completeRecord := &ConformanceRecord{
		ConnectorType: "postgresql",
		ServerVersion: "PostgreSQL 16.1",
		DriverVersion: "pgx/v5.5.0",
		TestCommit:    "git-commit-hash-test",
		RunAt:         time.Now(),
		Passed:        12,
		Failed:        0,
		Skipped:       nil,
	}
	if err := reg.Register(mockDriver, completeRecord); err != nil {
		t.Fatal(err)
	}
	d, _ = reg.Descriptor("postgresql")
	if d.Status != StatusAvailable {
		t.Errorf("status with complete conformance = %q, want AVAILABLE", d.Status)
	}
	if !d.Status.Selectable() {
		t.Errorf("AVAILABLE status must be selectable")
	}

	// 5. Setting status to DEGRADED stops selectable
	if err := reg.MarkDegraded("postgresql", "connection pool timeout"); err != nil {
		t.Fatal(err)
	}
	d, _ = reg.Descriptor("postgresql")
	if d.Status != StatusDegraded {
		t.Errorf("status = %q, want DEGRADED", d.Status)
	}
	if d.Status.Selectable() {
		t.Errorf("DEGRADED status must not be selectable")
	}
}

func TestCatalog_LimitsClamping(t *testing.T) {
	platform := DefaultLimits()

	// Normal clamp inside bounds
	custom := Limits{
		MaxRows:    500,
		MaxBytes:   1 << 20,
		Timeout:    10 * time.Second,
		CursorSize: 100,
	}
	clamped := custom.Clamp(platform)
	if clamped.MaxRows != 500 || clamped.MaxBytes != 1<<20 || clamped.Timeout != 10*time.Second {
		t.Errorf("unexpected clamped limits: %+v", clamped)
	}

	// Attempting to exceed platform bounds gets clamped down to platform default
	excessive := Limits{
		MaxRows:    999_999,
		MaxBytes:   100 << 20,
		Timeout:    10 * time.Minute,
		CursorSize: 50_000,
	}
	clamped = excessive.Clamp(platform)
	if clamped.MaxRows != platform.MaxRows {
		t.Errorf("MaxRows was not clamped to platform limit: got %d, want %d", clamped.MaxRows, platform.MaxRows)
	}
	if clamped.MaxBytes != platform.MaxBytes {
		t.Errorf("MaxBytes was not clamped: got %d, want %d", clamped.MaxBytes, platform.MaxBytes)
	}
	if clamped.Timeout != platform.Timeout {
		t.Errorf("Timeout was not clamped: got %v, want %v", clamped.Timeout, platform.Timeout)
	}
}

type mockTestConnector struct {
	connectorType string
}

func (m *mockTestConnector) Type() string                { return m.connectorType }
func (m *mockTestConnector) Capabilities() Capabilities  { return Capabilities{SupportsSchemas: true} }
func (m *mockTestConnector) ValidateConfig(Config) error { return nil }
func (m *mockTestConnector) TestConnection(context.Context, Config, Secrets) (Health, error) {
	return Health{State: HealthHealthy, CheckedAt: time.Now()}, nil
}
func (m *mockTestConnector) DiscoverResources(context.Context, Config, Secrets) ([]Resource, error) {
	return []Resource{{Schema: "public", Name: "batches", Kind: "TABLE"}}, nil
}
func (m *mockTestConnector) ExecuteTemplate(context.Context, Config, Secrets, Template, map[string]any, Limits) (*QueryResult, error) {
	return &QueryResult{Columns: []Column{{Name: "id"}}, Rows: [][]any{{1}}}, nil
}
func (m *mockTestConnector) Health(context.Context, Config, Secrets) Health {
	return Health{State: HealthHealthy, CheckedAt: time.Now()}
}
func (m *mockTestConnector) Close() error { return nil }
