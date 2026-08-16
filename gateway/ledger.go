package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"sentinel-gateway/internal/ledger"
)

// The evidence ledger, as the API surface sees it.
//
// The implementation moved to internal/ledger in Prompt 09. What was here read
// the chain head on one connection, computed a hash outside any transaction,
// and inserted on another -- so two concurrent appends could read the same
// predecessor. The per-tenant unique constraints stopped that forking the
// chain, but the loser received a constraint error and its audit record was
// simply dropped. Under the worker pool added in Prompt 08, that is not a
// theoretical race.
//
// This file is now an adapter: it keeps the shapes the HTTP handlers already
// return and delegates every append and every verification to the package that
// serialises them.

// AuditEvent is one record, as returned by the API.
type AuditEvent struct {
	ID           int64                  `json:"id"`
	SequenceNo   int64                  `json:"sequenceNo"`
	EventType    string                 `json:"eventType"`
	Actor        string                 `json:"actor"`
	ObjectType   string                 `json:"objectType,omitempty"`
	ObjectID     int64                  `json:"objectId,omitempty"`
	Correlation  string                 `json:"correlationId,omitempty"`
	Payload      map[string]interface{} `json:"payload"`
	PreviousHash string                 `json:"previousHash"`
	CurrentHash  string                 `json:"currentHash"`
	CreatedAt    string                 `json:"createdAt"`

	// IntegrityStatus is computed at read time.
	IntegrityStatus string `json:"integrityStatus,omitempty"`
}

// LedgerSummary is one tenant's chain plus the result of verifying it.
type LedgerSummary struct {
	TotalEvents   int    `json:"totalEvents"`
	IsChainValid  bool   `json:"isChainValid"`
	LastEventHash string `json:"lastEventHash"`

	// FirstBreachSequence is the earliest sequence number failing verification
	// (0 = none).
	FirstBreachSequence int64  `json:"firstBreachSequence"`
	BreachReason        string `json:"breachReason,omitempty"`

	// AnchorGap states what a passing verification does and does not prove.
	// It is carried on every summary so a report cannot render "valid" without
	// the qualification that makes it honest.
	AnchorGap string `json:"anchorGap"`

	Events []AuditEvent `json:"events"`
}

// ledgerFor builds a ledger bound to the handle it is given.
//
// It is constructed per call rather than cached in a package variable. A cached
// one holds whichever *sql.DB it saw first, which in a test binary is a
// database that has since been closed -- every ledger test after the first
// failed with "sql: database is closed" until this was per-call. The
// construction is a struct literal and a driver check, so there is nothing to
// amortise.
func ledgerFor(db *sql.DB) (*ledger.Ledger, error) {
	return ledger.New(db, "sqlite")
}

// AppendAuditEvent appends one record to a tenant's chain.
//
// The signature is unchanged from what it replaces so call sites did not need
// rewriting, but the behaviour differs in two ways that matter: the whole
// read-compute-write is one serialised transaction, and a payload carrying
// credential material or a financial record is refused rather than recorded.
func AppendAuditEvent(db *sql.DB, tenantID string, eventType string, actor string, payload map[string]interface{}) (*AuditEvent, error) {
	l, err := ledgerFor(db)
	if err != nil {
		return nil, err
	}

	objectType, objectID, correlation := auditSubject(eventType, payload)

	rec, err := l.Append(context.Background(), ledger.AppendRequest{
		TenantID:      tenantID,
		Action:        eventType,
		Actor:         actor,
		ObjectType:    objectType,
		ObjectID:      objectID,
		CorrelationID: correlation,
		Payload:       payload,
	})
	if err != nil {
		return nil, err
	}

	return &AuditEvent{
		ID:           rec.ID,
		SequenceNo:   rec.SequenceNo,
		EventType:    rec.Action,
		Actor:        rec.Actor,
		ObjectType:   rec.ObjectType,
		ObjectID:     rec.ObjectID,
		Correlation:  rec.CorrelationID,
		Payload:      payload,
		PreviousHash: rec.PreviousHash,
		CurrentHash:  rec.CurrentHash,
		CreatedAt:    rec.OccurredAt.Format("2006-01-02T15:04:05.000000Z07:00"),
	}, nil
}

// auditSubject extracts the object a record concerns from its payload.
//
// Callers pass a loose map, and the ledger requires an object type. Rather than
// making every call site change at once, the common identifiers are recognised
// here and anything unrecognised is recorded as a system event -- which is
// accurate rather than a guess.
func auditSubject(eventType string, payload map[string]interface{}) (objectType string, objectID int64, correlation string) {
	objectType = "system"

	if v, ok := payload["correlationId"].(string); ok {
		correlation = v
	} else if v, ok := payload["dedupeKey"].(string); ok {
		correlation = v
	}

	for _, key := range []string{"artifactId", "fileId", "file_instance_id"} {
		if id, ok := numericField(payload[key]); ok {
			return "artifact", id, correlation
		}
	}
	for _, key := range []string{"incidentId", "incident_id"} {
		if id, ok := numericField(payload[key]); ok {
			return "incident", id, correlation
		}
	}
	return objectType, 0, correlation
}

func numericField(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

// GetLedger reads one tenant's chain and verifies it.
//
// A caller cannot read another tenant's chain: the tenant is a parameter and
// every query filters on it.
func GetLedger(db *sql.DB, tenantID string) (*LedgerSummary, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("ledger read requires a tenant scope")
	}
	l, err := ledgerFor(db)
	if err != nil {
		return nil, err
	}

	result, err := l.Verify(context.Background(), tenantID)
	if err != nil {
		return nil, err
	}

	summary := &LedgerSummary{
		IsChainValid:        result.Intact,
		LastEventHash:       result.HeadHash,
		FirstBreachSequence: result.FirstBreakAt,
		BreachReason:        result.Reason,
		AnchorGap:           ledger.AnchorGapStatement,
		Events:              make([]AuditEvent, 0),
	}
	if summary.LastEventHash == "" {
		summary.LastEventHash = ledger.GenesisHash
	}

	rows, err := db.Query(`
		SELECT id, sequence_no, event_type, actor, object_type, object_id,
		       correlation_id, payload, previous_hash, current_hash, created_at
		FROM audit_events WHERE tenant_id = ? ORDER BY sequence_no ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ev AuditEvent
		var rawPayload string
		var objectType, correlation sql.NullString
		var objectID sql.NullInt64
		if err := rows.Scan(&ev.ID, &ev.SequenceNo, &ev.EventType, &ev.Actor,
			&objectType, &objectID, &correlation, &rawPayload,
			&ev.PreviousHash, &ev.CurrentHash, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.ObjectType = objectType.String
		ev.ObjectID = objectID.Int64
		ev.Correlation = correlation.String
		_ = json.Unmarshal([]byte(rawPayload), &ev.Payload)

		// Per-record status comes from where the chain first broke. Records
		// before the break verified; the break and everything after it cannot
		// be relied on, because a chain is only as good as its weakest link and
		// every subsequent hash was computed from the broken one.
		switch {
		case result.Intact:
			ev.IntegrityStatus = "VERIFIED"
		case ev.SequenceNo < result.FirstBreakAt:
			ev.IntegrityStatus = "VERIFIED"
		case ev.SequenceNo == result.FirstBreakAt:
			ev.IntegrityStatus = "BROKEN"
		default:
			ev.IntegrityStatus = "UNVERIFIABLE_AFTER_BREAK"
		}

		summary.Events = append(summary.Events, ev)
	}
	summary.TotalEvents = len(summary.Events)
	return summary, rows.Err()
}
