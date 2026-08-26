package lens

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (s *Service) CreateInvestigation(ctx context.Context, tenantID, title, actor string) (*Investigation, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 160 {
		return nil, fmt.Errorf("lens: title is required and must be <=160 chars")
	}
	id, err := randomID("lens")
	if err != nil {
		return nil, err
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT INTO lens_investigations (id, tenant_id, title, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, id, tenantID, title, actor, createdAt, createdAt)
	if err != nil {
		return nil, err
	}
	return s.GetInvestigation(ctx, tenantID, id)
}

func (s *Service) AddNode(ctx context.Context, tenantID, investigationID, parentID, question, actor string, intent QueryIntent) (*InvestigationNode, error) {
	question = strings.TrimSpace(question)
	if question == "" || len(question) > 500 {
		return nil, fmt.Errorf("lens: question is required and must be <=500 chars")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lens_investigations WHERE id=? AND tenant_id=?`, investigationID, tenantID).Scan(&exists); err != nil || exists != 1 {
		return nil, sql.ErrNoRows
	}
	if parentID != "" {
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lens_investigation_nodes WHERE id=? AND investigation_id=? AND tenant_id=?`, parentID, investigationID, tenantID).Scan(&exists); err != nil || exists != 1 {
			return nil, fmt.Errorf("lens: parent node not found in investigation")
		}
	}
	result, err := s.Execute(ctx, tenantID, intent)
	if err != nil {
		return nil, err
	}
	intentJSON, _ := json.Marshal(intent)
	chartJSON, _ := json.Marshal(result.Chart)
	refsJSON, _ := json.Marshal(result.Provenance.EvidenceRefs)
	id, err := randomID("node")
	if err != nil {
		return nil, err
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `INSERT INTO lens_investigation_nodes (id, investigation_id, tenant_id, parent_node_id, question, query_intent_json, query_hash, result_hash, chart_spec_json, evidence_refs_json, created_by, created_at) VALUES (?, ?, ?, NULLIF(?,''), ?, ?, ?, ?, ?, ?, ?, ?)`, id, investigationID, tenantID, parentID, question, string(intentJSON), result.Provenance.QueryHash, result.Provenance.ResultHash, string(chartJSON), string(refsJSON), actor, createdAt)
	if err != nil {
		return nil, err
	}
	return &InvestigationNode{ID: id, InvestigationID: investigationID, ParentNodeID: parentID, Question: question, Intent: intent, QueryHash: result.Provenance.QueryHash, ResultHash: result.Provenance.ResultHash, Chart: result.Chart, EvidenceRefs: result.Provenance.EvidenceRefs, CreatedBy: actor, CreatedAt: createdAt}, nil
}

func randomID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("lens: generate id: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(b[:]), nil
}

func (s *Service) GetInvestigation(ctx context.Context, tenantID, id string) (*Investigation, error) {
	var inv Investigation
	if err := s.db.QueryRowContext(ctx, `SELECT id,title,created_by,created_at FROM lens_investigations WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&inv.ID, &inv.Title, &inv.CreatedBy, &inv.CreatedAt); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, parent_node_id, question, query_intent_json, query_hash, result_hash, chart_spec_json, evidence_refs_json, created_by, created_at FROM lens_investigation_nodes WHERE investigation_id=? AND tenant_id=? ORDER BY created_at,id`, id, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	inv.Nodes = make([]InvestigationNode, 0)
	for rows.Next() {
		var n InvestigationNode
		var parent sql.NullString
		var intentJSON, chartJSON, refsJSON string
		n.InvestigationID = id
		if err := rows.Scan(&n.ID, &parent, &n.Question, &intentJSON, &n.QueryHash, &n.ResultHash, &chartJSON, &refsJSON, &n.CreatedBy, &n.CreatedAt); err != nil {
			return nil, err
		}
		if parent.Valid {
			n.ParentNodeID = parent.String
		}
		if err := json.Unmarshal([]byte(intentJSON), &n.Intent); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(chartJSON), &n.Chart); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(refsJSON), &n.EvidenceRefs); err != nil {
			return nil, err
		}
		inv.Nodes = append(inv.Nodes, n)
	}
	return &inv, rows.Err()
}
