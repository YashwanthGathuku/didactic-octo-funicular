package lens

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrUnknownDataset   = errors.New("lens: unknown dataset")
	ErrUnknownField     = errors.New("lens: unknown field")
	ErrUnsafeQuery      = errors.New("lens: unsafe query intent")
	ErrInvalidTimeRange = errors.New("lens: invalid time range")
)

type compiledField struct {
	ID        string
	Expr      string
	Kind      string
	ValueType string
}

type datasetSpec struct {
	Definition DatasetDefinition
	FromSQL    string
	TimeExpr   string
	Fields     map[string]compiledField
}

type Service struct {
	db       *sql.DB
	datasets map[string]datasetSpec
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, datasets: defaultDatasets()}
}

func (s *Service) ListDatasets() []DatasetDefinition {
	out := make([]DatasetDefinition, 0, len(s.datasets))
	for _, ds := range s.datasets {
		out = append(out, ds.Definition)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func defaultDatasets() map[string]datasetSpec {
	makeFields := func(items ...compiledField) map[string]compiledField {
		m := make(map[string]compiledField, len(items))
		for _, item := range items {
			m[item.ID] = item
		}
		return m
	}
	toPublic := func(fields map[string]compiledField) []FieldDefinition {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]FieldDefinition, 0, len(keys))
		for _, k := range keys {
			f := fields[k]
			out = append(out, FieldDefinition{ID: f.ID, Label: fieldLabel(f.ID), Kind: f.Kind, ValueType: f.ValueType})
		}
		return out
	}

	achFields := makeFields(
		compiledField{ID: "day", Expr: "strftime('%Y-%m-%d', occurred_at)", Kind: "DIMENSION", ValueType: "DATE"},
		compiledField{ID: "partner_id", Expr: "partner_id", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "return_code", Expr: "return_code", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "source_type", Expr: "source_type", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "return_count", Expr: "COUNT(*)", Kind: "METRIC", ValueType: "INTEGER"},
		compiledField{ID: "associated_amount_cents", Expr: "COALESCE(SUM(amount_cents),0)", Kind: "METRIC", ValueType: "INTEGER"},
		compiledField{ID: "avg_amount_cents", Expr: "COALESCE(AVG(amount_cents),0)", Kind: "METRIC", ValueType: "NUMBER"},
	)
	incidentFields := makeFields(
		compiledField{ID: "day", Expr: "strftime('%Y-%m-%d', created_at)", Kind: "DIMENSION", ValueType: "DATE"},
		compiledField{ID: "type", Expr: "type", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "severity", Expr: "severity", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "status", Expr: "status", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "incident_count", Expr: "COUNT(*)", Kind: "METRIC", ValueType: "INTEGER"},
	)
	findingFields := makeFields(
		compiledField{ID: "day", Expr: "strftime('%Y-%m-%d', created_at)", Kind: "DIMENSION", ValueType: "DATE"},
		compiledField{ID: "code", Expr: "code", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "severity", Expr: "severity", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "provenance", Expr: "provenance", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "finding_count", Expr: "COUNT(*)", Kind: "METRIC", ValueType: "INTEGER"},
		compiledField{ID: "blocking_count", Expr: "SUM(CASE WHEN severity = 'BLOCKING' THEN 1 ELSE 0 END)", Kind: "METRIC", ValueType: "INTEGER"},
	)
	fileFields := makeFields(
		compiledField{ID: "day", Expr: "strftime('%Y-%m-%d', created_at)", Kind: "DIMENSION", ValueType: "DATE"},
		compiledField{ID: "status", Expr: "status", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "file_count", Expr: "COUNT(*)", Kind: "METRIC", ValueType: "INTEGER"},
		compiledField{ID: "total_bytes", Expr: "COALESCE(SUM(size_bytes),0)", Kind: "METRIC", ValueType: "INTEGER"},
	)
	agentFields := makeFields(
		compiledField{ID: "day", Expr: "strftime('%Y-%m-%d', started_at)", Kind: "DIMENSION", ValueType: "DATE"},
		compiledField{ID: "agent_name", Expr: "agent_name", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "status", Expr: "status", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "model_name", Expr: "COALESCE(model_name,'')", Kind: "DIMENSION", ValueType: "STRING"},
		compiledField{ID: "run_count", Expr: "COUNT(*)", Kind: "METRIC", ValueType: "INTEGER"},
		compiledField{ID: "avg_latency_ms", Expr: "COALESCE(AVG(latency_ms),0)", Kind: "METRIC", ValueType: "NUMBER"},
		compiledField{ID: "total_tokens", Expr: "COALESCE(SUM(input_tokens + output_tokens),0)", Kind: "METRIC", ValueType: "INTEGER"},
	)

	return map[string]datasetSpec{
		"ach_return_intelligence": {Definition: DatasetDefinition{ID: "ach_return_intelligence", Label: "ACH return intelligence", Description: "Return trends, counterparties and reason-code concentration. Synthetic demo rows are clearly separated from curated imports.", TimeField: "occurred_at", Fields: toPublic(achFields), SourceClass: "MIXED_PROVENANCE"}, FromSQL: "lens_return_events", TimeExpr: "occurred_at", Fields: achFields},
		"incident_trends":         {Definition: DatasetDefinition{ID: "incident_trends", Label: "Incident trends", Description: "Tenant-scoped operational incident patterns from SentinelFlow system records.", TimeField: "created_at", Fields: toPublic(incidentFields), SourceClass: "SYSTEM_RECORDS"}, FromSQL: "incidents", TimeExpr: "created_at", Fields: incidentFields},
		"validation_findings":     {Definition: DatasetDefinition{ID: "validation_findings", Label: "Validation findings", Description: "Deterministic validation findings and blocking-control concentration.", TimeField: "created_at", Fields: toPublic(findingFields), SourceClass: "DETERMINISTIC_FINDINGS"}, FromSQL: "validation_findings", TimeExpr: "created_at", Fields: findingFields},
		"file_operations":         {Definition: DatasetDefinition{ID: "file_operations", Label: "File operations", Description: "Artifact state and ingestion volume metadata without raw payment payloads.", TimeField: "created_at", Fields: toPublic(fileFields), SourceClass: "SYSTEM_RECORDS"}, FromSQL: "file_instances", TimeExpr: "created_at", Fields: fileFields},
		"agent_operations":        {Definition: DatasetDefinition{ID: "agent_operations", Label: "Agent operations", Description: "Bounded agent runtime status, latency and token telemetry; no private chain-of-thought.", TimeField: "started_at", Fields: toPublic(agentFields), SourceClass: "AGENT_TELEMETRY"}, FromSQL: "agent_runs", TimeExpr: "started_at", Fields: agentFields},
	}
}

func (s *Service) Execute(ctx context.Context, tenantID string, in QueryIntent) (*QueryResult, error) {
	if s.db == nil {
		return nil, errors.New("lens: database unavailable")
	}
	ds, ok := s.datasets[in.DatasetID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownDataset, in.DatasetID)
	}
	if in.SchemaVersion == "" {
		in.SchemaVersion = "1.0"
	}
	if in.SchemaVersion != "1.0" {
		return nil, fmt.Errorf("%w: unsupported schema version", ErrUnsafeQuery)
	}
	if tenantID == "" {
		return nil, fmt.Errorf("%w: tenant required", ErrUnsafeQuery)
	}
	if in.TimeRange.Start.IsZero() || in.TimeRange.End.IsZero() || !in.TimeRange.Start.Before(in.TimeRange.End) {
		return nil, ErrInvalidTimeRange
	}
	if in.TimeRange.End.Sub(in.TimeRange.Start) > MaxQueryDays*24*time.Hour {
		return nil, fmt.Errorf("%w: maximum range is %d days", ErrInvalidTimeRange, MaxQueryDays)
	}
	if len(in.Metrics) == 0 {
		return nil, fmt.Errorf("%w: at least one metric required", ErrUnsafeQuery)
	}
	if in.Limit <= 0 {
		in.Limit = 200
	}
	if in.Limit > MaxRows {
		return nil, fmt.Errorf("%w: limit exceeds %d", ErrUnsafeQuery, MaxRows)
	}

	selects := make([]string, 0, len(in.Dimensions)+len(in.Metrics))
	groups := make([]string, 0, len(in.Dimensions))
	columns := make([]string, 0, len(in.Dimensions)+len(in.Metrics))
	seenFields := make(map[string]struct{}, len(in.Dimensions)+len(in.Metrics))
	for _, id := range in.Dimensions {
		if _, exists := seenFields[id]; exists {
			return nil, fmt.Errorf("%w: duplicate field %s", ErrUnsafeQuery, id)
		}
		seenFields[id] = struct{}{}
		f, ok := ds.Fields[id]
		if !ok || f.Kind != "DIMENSION" {
			return nil, fmt.Errorf("%w: dimension %s", ErrUnknownField, id)
		}
		selects = append(selects, f.Expr+" AS \""+id+"\"")
		groups = append(groups, f.Expr)
		columns = append(columns, id)
	}
	for _, id := range in.Metrics {
		if _, exists := seenFields[id]; exists {
			return nil, fmt.Errorf("%w: duplicate field %s", ErrUnsafeQuery, id)
		}
		seenFields[id] = struct{}{}
		f, ok := ds.Fields[id]
		if !ok || f.Kind != "METRIC" {
			return nil, fmt.Errorf("%w: metric %s", ErrUnknownField, id)
		}
		selects = append(selects, f.Expr+" AS \""+id+"\"")
		columns = append(columns, id)
	}

	where := []string{"tenant_id = ?", ds.TimeExpr + " >= ?", ds.TimeExpr + " < ?"}
	args := []any{tenantID, in.TimeRange.Start.UTC(), in.TimeRange.End.UTC()}
	for _, flt := range in.Filters {
		f, ok := ds.Fields[flt.Field]
		if !ok || f.Kind != "DIMENSION" {
			return nil, fmt.Errorf("%w: filter %s", ErrUnknownField, flt.Field)
		}
		if len(flt.Values) == 0 || len(flt.Values) > 20 {
			return nil, fmt.Errorf("%w: invalid filter values", ErrUnsafeQuery)
		}
		switch strings.ToUpper(strings.TrimSpace(flt.Op)) {
		case "EQ":
			if len(flt.Values) != 1 {
				return nil, fmt.Errorf("%w: EQ requires one value", ErrUnsafeQuery)
			}
			where = append(where, f.Expr+" = ?")
			args = append(args, flt.Values[0])
		case "IN":
			marks := make([]string, len(flt.Values))
			for i, v := range flt.Values {
				marks[i] = "?"
				args = append(args, v)
			}
			where = append(where, f.Expr+" IN ("+strings.Join(marks, ",")+")")
		default:
			return nil, fmt.Errorf("%w: filter operator %s", ErrUnsafeQuery, flt.Op)
		}
	}

	query := "SELECT " + strings.Join(selects, ", ") + " FROM " + ds.FromSQL + " WHERE " + strings.Join(where, " AND ")
	if len(groups) > 0 {
		query += " GROUP BY " + strings.Join(groups, ", ")
	}
	if len(in.OrderBy) > 0 {
		orders := make([]string, 0, len(in.OrderBy))
		for _, ob := range in.OrderBy {
			if _, ok := ds.Fields[ob.Field]; !ok {
				return nil, fmt.Errorf("%w: order field %s", ErrUnknownField, ob.Field)
			}
			dir := strings.ToUpper(strings.TrimSpace(ob.Direction))
			if dir != "ASC" && dir != "DESC" {
				return nil, fmt.Errorf("%w: order direction", ErrUnsafeQuery)
			}
			orders = append(orders, "\""+ob.Field+"\" "+dir)
		}
		query += " ORDER BY " + strings.Join(orders, ", ")
	} else if contains(in.Dimensions, "day") {
		query += " ORDER BY \"day\" ASC"
	}
	query += " LIMIT ?"
	args = append(args, in.Limit)

	queryHash, err := hashJSON(in)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("lens execute: %w", err)
	}
	defer rows.Close()
	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		vals := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for i, c := range columns {
			row[c] = normalizeSQLValue(vals[i])
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	resultHash, err := hashJSON(resultRows)
	if err != nil {
		return nil, err
	}
	evidence, sourceClass, err := s.provenance(ctx, tenantID, ds, in)
	if err != nil {
		return nil, err
	}
	chart := recommendChart(ds.Definition.Label, in)
	return &QueryResult{DatasetID: in.DatasetID, Columns: columns, Rows: resultRows, Chart: chart, Provenance: Provenance{SourceClass: sourceClass, QueryHash: queryHash, ResultHash: resultHash, EvidenceRefs: evidence, ExecutedAt: time.Now().UTC().Format(time.RFC3339Nano), RowCount: len(resultRows), AdvisoryOnly: true}, Meta: map[string]interface{}{"max_rows": MaxRows, "max_days": MaxQueryDays}}, nil
}

func (s *Service) provenance(ctx context.Context, tenantID string, ds datasetSpec, in QueryIntent) ([]string, string, error) {
	if in.DatasetID == "ach_return_intelligence" {
		where := []string{"tenant_id = ?", "occurred_at >= ?", "occurred_at < ?"}
		args := []any{tenantID, in.TimeRange.Start.UTC(), in.TimeRange.End.UTC()}
		for _, flt := range in.Filters {
			f, ok := ds.Fields[flt.Field]
			if !ok || f.Kind != "DIMENSION" {
				continue
			}
			switch strings.ToUpper(flt.Op) {
			case "EQ":
				if len(flt.Values) == 1 {
					where = append(where, f.Expr+" = ?")
					args = append(args, flt.Values[0])
				}
			case "IN":
				if len(flt.Values) > 0 {
					marks := make([]string, len(flt.Values))
					for i, v := range flt.Values {
						marks[i] = "?"
						args = append(args, v)
					}
					where = append(where, f.Expr+" IN ("+strings.Join(marks, ",")+")")
				}
			}
		}
		var synth, curated int
		if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(CASE WHEN source_type='SYNTHETIC_DEMO' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN source_type='CURATED_IMPORT' THEN 1 ELSE 0 END),0) FROM lens_return_events WHERE "+strings.Join(where, " AND "), args...).Scan(&synth, &curated); err != nil && err != sql.ErrNoRows {
			return nil, "", err
		}
		sourceClass := "EMPTY"
		switch {
		case synth > 0 && curated > 0:
			sourceClass = "MIXED_PROVENANCE"
		case synth > 0:
			sourceClass = "SYNTHETIC_DEMO"
		case curated > 0:
			sourceClass = "CURATED_IMPORT"
		}
		rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT incident_id FROM lens_return_events WHERE "+strings.Join(where, " AND ")+" AND source_type='CURATED_IMPORT' AND verified=1 AND incident_id IS NOT NULL LIMIT 25", args...)
		if err != nil {
			return nil, "", err
		}
		defer rows.Close()
		refs := make([]string, 0)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, "", err
			}
			refs = append(refs, fmt.Sprintf("INCIDENT/%d", id))
		}
		return refs, sourceClass, rows.Err()
	}

	return []string{}, ds.Definition.SourceClass, nil
}

func recommendChart(title string, in QueryIntent) ChartSpec {
	chart := ChartSpec{Kind: "table", Title: title}
	if len(in.Metrics) == 0 {
		return chart
	}
	chart.Y = in.Metrics[0]
	chart.ValueLabel = in.Metrics[0]
	if contains(in.Dimensions, "day") {
		chart.Kind = "line"
		chart.X = "day"
		for _, d := range in.Dimensions {
			if d != "day" {
				chart.Series = d
				break
			}
		}
		return chart
	}
	if len(in.Dimensions) > 0 {
		chart.Kind = "bar"
		chart.X = in.Dimensions[0]
		if len(in.Dimensions) > 1 {
			chart.Series = in.Dimensions[1]
		}
	}
	return chart
}

func contains(items []string, want string) bool {
	for _, v := range items {
		if v == want {
			return true
		}
	}
	return false
}
func normalizeSQLValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}
func hashJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func fieldLabel(id string) string {
	parts := strings.Split(id, "_")
	for i, part := range parts {
		if strings.EqualFold(part, "usd") {
			parts[i] = "USD"
			continue
		}
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}
