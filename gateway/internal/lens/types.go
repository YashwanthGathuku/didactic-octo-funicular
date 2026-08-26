package lens

import "time"

const (
	MaxQueryDays = 90
	MaxRows      = 500
)

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type Filter struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Values []string `json:"values"`
}

type OrderBy struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type QueryIntent struct {
	SchemaVersion string    `json:"schema_version"`
	DatasetID     string    `json:"dataset_id"`
	TimeRange     TimeRange `json:"time_range"`
	Metrics       []string  `json:"metrics"`
	Dimensions    []string  `json:"dimensions"`
	Filters       []Filter  `json:"filters,omitempty"`
	OrderBy       []OrderBy `json:"order_by,omitempty"`
	Limit         int       `json:"limit,omitempty"`
}

type FieldDefinition struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"` // DIMENSION | METRIC
	ValueType   string `json:"value_type"`
	Description string `json:"description,omitempty"`
}

type DatasetDefinition struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	Description string            `json:"description"`
	TimeField   string            `json:"time_field"`
	Fields      []FieldDefinition `json:"fields"`
	SourceClass string            `json:"source_class"`
}

type Provenance struct {
	SourceClass  string   `json:"source_class"`
	QueryHash    string   `json:"query_hash"`
	ResultHash   string   `json:"result_hash"`
	EvidenceRefs []string `json:"evidence_refs"`
	ExecutedAt   string   `json:"executed_at"`
	RowCount     int      `json:"row_count"`
	AdvisoryOnly bool     `json:"advisory_only"`
}

type QueryResult struct {
	DatasetID  string                 `json:"dataset_id"`
	Columns    []string               `json:"columns"`
	Rows       []map[string]any       `json:"rows"`
	Chart      ChartSpec              `json:"chart"`
	Provenance Provenance             `json:"provenance"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
}

type ChartSpec struct {
	Kind       string `json:"kind"` // line | bar | table
	X          string `json:"x,omitempty"`
	Y          string `json:"y,omitempty"`
	Series     string `json:"series,omitempty"`
	Title      string `json:"title"`
	ValueLabel string `json:"value_label,omitempty"`
}

type Investigation struct {
	ID        string              `json:"id"`
	Title     string              `json:"title"`
	CreatedBy string              `json:"created_by"`
	CreatedAt string              `json:"created_at"`
	Nodes     []InvestigationNode `json:"nodes"`
}

type InvestigationNode struct {
	ID              string      `json:"id"`
	InvestigationID string      `json:"investigation_id"`
	ParentNodeID    string      `json:"parent_node_id,omitempty"`
	Question        string      `json:"question"`
	Intent          QueryIntent `json:"query_intent"`
	QueryHash       string      `json:"query_hash"`
	ResultHash      string      `json:"result_hash"`
	Chart           ChartSpec   `json:"chart"`
	EvidenceRefs    []string    `json:"evidence_refs"`
	CreatedBy       string      `json:"created_by"`
	CreatedAt       string      `json:"created_at"`
}
