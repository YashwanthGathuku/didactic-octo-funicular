package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type AuditEvent struct {
	ID           int64                  `json:"id"`
	EventType    string                 `json:"eventType"`
	Actor        string                 `json:"actor"`
	Payload      map[string]interface{} `json:"payload"`
	PreviousHash string                 `json:"previousHash"`
	CurrentHash  string                 `json:"currentHash"`
	CreatedAt    string                 `json:"createdAt"`
	// IntegrityStatus is computed at read time: VERIFIED | BROKEN_LINK | CONTENT_TAMPERED
	IntegrityStatus string `json:"integrityStatus,omitempty"`
}

type LedgerSummary struct {
	TotalEvents   int    `json:"totalEvents"`
	IsChainValid  bool   `json:"isChainValid"`
	LastEventHash string `json:"lastEventHash"`
	// FirstBreachEvent is the id of the earliest row failing verification (0 = none)
	FirstBreachEvent int64        `json:"firstBreachEvent"`
	Events           []AuditEvent `json:"events"`
}

// AppendAuditEvent inserts a new event into audit_events, computing the SHA-256 hash chained from the last event.
func AppendAuditEvent(db *sql.DB, eventType string, actor string, payload map[string]interface{}) (*AuditEvent, error) {
	// Find last event's hash
	var lastHash string
	row := db.QueryRow("SELECT current_hash FROM audit_events ORDER BY id DESC LIMIT 1")
	err := row.Scan(&lastHash)
	if err == sql.ErrNoRows {
		// Genesis block
		lastHash = "0000000000000000000000000000000000000000000000000000000000000000"
	} else if err != nil {
		return nil, fmt.Errorf("failed to query last audit hash: %w", err)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	hashInput := fmt.Sprintf("%s|%s|%s|%s|%s", lastHash, eventType, actor, string(payloadBytes), now)
	hasher := sha256.New()
	hasher.Write([]byte(hashInput))
	currentHash := hex.EncodeToString(hasher.Sum(nil))

	res, err := db.Exec(`
		INSERT INTO audit_events (event_type, actor, payload, previous_hash, current_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, eventType, actor, string(payloadBytes), lastHash, currentHash, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert audit event: %w", err)
	}

	id, _ := res.LastInsertId()
	return &AuditEvent{
		ID:           id,
		EventType:    eventType,
		Actor:        actor,
		Payload:      payload,
		PreviousHash: lastHash,
		CurrentHash:  currentHash,
		CreatedAt:    now,
	}, nil
}

// recomputeHash reproduces the hash for a stored row exactly as AppendAuditEvent
// computed it. Verification MUST recompute -- checking only that
// row[n].previous_hash == row[n-1].current_hash detects deletion and hash edits
// but is blind to edits of payload/actor/event_type/created_at, which is the
// tamper vector that actually matters for SEC 17a-4 evidentiary value.
func recomputeHash(prevHash, eventType, actor, rawPayload, createdAt string) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s|%s|%s|%s|%s", prevHash, eventType, actor, rawPayload, createdAt)))
	return hex.EncodeToString(h.Sum(nil))
}

// GetLedger retrieves the audit events and verifies hash chain integrity.
func GetLedger(db *sql.DB) (*LedgerSummary, error) {
	rows, err := db.Query("SELECT id, event_type, actor, payload, previous_hash, current_hash, created_at FROM audit_events ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]AuditEvent, 0)
	isValid := true
	var firstBreachID int64
	expectedPrev := "0000000000000000000000000000000000000000000000000000000000000000"
	lastHash := expectedPrev

	for rows.Next() {
		var ev AuditEvent
		var rawPayload string
		if err := rows.Scan(&ev.ID, &ev.EventType, &ev.Actor, &rawPayload, &ev.PreviousHash, &ev.CurrentHash, &ev.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rawPayload), &ev.Payload)

		// (a) linkage: this row must chain to the previous row's hash
		if ev.PreviousHash != expectedPrev {
			isValid = false
			ev.IntegrityStatus = "BROKEN_LINK"
		}
		// (b) content integrity: recompute the hash from the stored fields
		want := recomputeHash(ev.PreviousHash, ev.EventType, ev.Actor, rawPayload, ev.CreatedAt)
		if want != ev.CurrentHash {
			isValid = false
			ev.IntegrityStatus = "CONTENT_TAMPERED"
			if firstBreachID == 0 {
				firstBreachID = ev.ID
			}
		} else if ev.IntegrityStatus == "" {
			ev.IntegrityStatus = "VERIFIED"
		}
		if ev.IntegrityStatus == "BROKEN_LINK" && firstBreachID == 0 {
			firstBreachID = ev.ID
		}

		expectedPrev = ev.CurrentHash
		lastHash = ev.CurrentHash
		events = append(events, ev)
	}

	return &LedgerSummary{
		TotalEvents:      len(events),
		IsChainValid:     isValid,
		LastEventHash:    lastHash,
		FirstBreachEvent: firstBreachID,
		Events:           events,
	}, nil
}
