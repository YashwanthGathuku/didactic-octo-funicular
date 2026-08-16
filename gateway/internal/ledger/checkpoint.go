package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Periodic verification, and the honest position on external anchoring.
//
// What is implemented: a job that walks every tenant's chain, records the
// result, and keeps that record in the same append-only table it is checking so
// a verification cannot be quietly deleted.
//
// What is NOT implemented, and is not claimed anywhere in this system: an
// external signed checkpoint. The design is stated below because the gap is
// worth being specific about rather than leaving as an absence.
//
// # Why a self-verified chain is limited
//
// This chain proves internal consistency. Every record carries its
// predecessor's digest, so altering one requires recomputing every digest after
// it. That defeats casual tampering and accidental corruption, and it is the
// property most audit trails actually need.
//
// It does not prove authenticity. The party that can rewrite the rows is the
// party that can recompute the digests, and nothing here is signed -- a SHA-256
// digest is not a digital signature. An operator with database write access can
// produce a chain that verifies perfectly and describes events that never
// happened. `Verify` returning `Intact: true` means "consistent", not
// "trustworthy", and any report built on it must say so.
//
// # The design that would close it
//
// Periodically publish a checkpoint -- tenant, sequence number, head hash,
// timestamp -- to somewhere the operator of this system cannot rewrite:
// signed by a key held outside the deployment, or written to an append-only
// external log, or anchored to a public ledger. Verification then becomes:
// the chain is internally consistent AND its head at sequence N matches the
// checkpoint that a third party holds for sequence N. Rewriting history then
// requires compromising both.
//
// That needs a key or a service this repository does not have. Implementing the
// local half and calling the result "anchored" would be exactly the kind of
// claim this programme exists to remove, so `Checkpoint.Anchored` is a field
// that is always false, and it is checked by a test.

// Checkpoint records a chain's head at a point in time.
type Checkpoint struct {
	TenantID   string    `json:"tenantId"`
	SequenceNo int64     `json:"sequenceNo"`
	HeadHash   string    `json:"headHash"`
	TakenAt    time.Time `json:"takenAt"`

	// Anchored reports whether this checkpoint has been published somewhere
	// outside this system's control.
	//
	// It is always false. No external anchoring exists, and a checkpoint stored
	// only in the database it attests to is worth exactly as much as the
	// database -- which is to say, nothing against an operator who can rewrite
	// both. The field exists so that a future implementation has somewhere to
	// put the truth, and so that any report rendering this cannot omit it.
	Anchored bool `json:"anchored"`

	// AnchorGap states, in words, what is missing. It is carried alongside the
	// checkpoint so an export cannot present a checkpoint without its
	// qualification.
	AnchorGap string `json:"anchorGap"`
}

// AnchorGapStatement is the single wording used wherever a checkpoint appears.
const AnchorGapStatement = "Not externally anchored: this checkpoint is stored in the same database it attests to, " +
	"so it proves internal consistency and not authenticity. Nothing here is signed."

// TakeCheckpoint records a tenant's current head.
func (l *Ledger) TakeCheckpoint(ctx context.Context, tenantID string) (*Checkpoint, error) {
	seq, hash, err := l.Head(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &Checkpoint{
		TenantID:   tenantID,
		SequenceNo: seq,
		HeadHash:   hash,
		TakenAt:    l.now().UTC(),
		Anchored:   false,
		AnchorGap:  AnchorGapStatement,
	}, nil
}

// VerifyAll walks every tenant's chain and records each result.
//
// The result is written into the ledger itself, as a record, so a verification
// is subject to the same append-only guarantees as everything else: a failed
// check cannot be deleted, and the record of it becomes part of the chain that
// the next check will verify.
func (l *Ledger) VerifyAll(ctx context.Context) ([]*VerificationResult, error) {
	tenants, err := l.Tenants(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]*VerificationResult, 0, len(tenants))
	for _, tenant := range tenants {
		result, err := l.Verify(ctx, tenant)
		if err != nil {
			return results, fmt.Errorf("verify %s: %w", tenant, err)
		}
		results = append(results, result)

		checkpoint, err := l.TakeCheckpoint(ctx, tenant)
		if err != nil {
			return results, err
		}

		// The verification record is appended whether the chain passed or
		// failed. Recording only successes would make the trail's silence
		// ambiguous: no record could mean "not checked" or "checked and
		// broken", and those call for very different responses.
		payload := map[string]any{
			"recordsChecked": result.RecordsChecked,
			"intact":         result.Intact,
			"headHash":       checkpoint.HeadHash,
			"headSequence":   checkpoint.SequenceNo,
			"anchored":       checkpoint.Anchored,
			"anchorGap":      checkpoint.AnchorGap,
		}
		if !result.Intact {
			payload["firstBreakAt"] = result.FirstBreakAt
			payload["reason"] = result.Reason
		}

		if _, err := l.Append(ctx, AppendRequest{
			TenantID:      tenant,
			Action:        "LEDGER_VERIFIED",
			Actor:         "system:ledger-verifier",
			ObjectType:    "ledger",
			ObjectID:      checkpoint.SequenceNo,
			CorrelationID: fmt.Sprintf("verify-%d", l.now().UTC().Unix()),
			Payload:       payload,
		}); err != nil {
			return results, fmt.Errorf("record verification for %s: %w", tenant, err)
		}

		if !result.Intact {
			// A broken chain is not a routine log line. It is stated plainly,
			// once, with the location.
			log.Printf("EVIDENCE CHAIN BROKEN for tenant %s at sequence %d: %s",
				tenant, result.FirstBreakAt, result.Reason)
		}
	}
	return results, nil
}

// RunVerifier runs VerifyAll on an interval until the context is cancelled.
func (l *Ledger) RunVerifier(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			results, err := l.VerifyAll(ctx)
			if err != nil {
				log.Printf("ledger verification pass: %v", err)
				continue
			}
			broken := 0
			for _, r := range results {
				if !r.Intact {
					broken++
				}
			}
			if broken > 0 {
				log.Printf("ledger verification: %d of %d tenant chains are broken", broken, len(results))
			}
		}
	}
}

// LastVerification returns the most recent recorded verification for a tenant,
// so a reader can tell when the chain was last checked rather than assuming.
func (l *Ledger) LastVerification(ctx context.Context, tenantID string) (*VerificationResult, error) {
	var payload string
	var createdAt any
	err := l.db.QueryRowContext(ctx, l.rebind(`
		SELECT payload, created_at FROM audit_events
		WHERE tenant_id = ? AND event_type = 'LEDGER_VERIFIED'
		ORDER BY sequence_no DESC LIMIT 1`), tenantID).Scan(&payload, &createdAt)
	if err != nil {
		return nil, err
	}

	var fields struct {
		RecordsChecked int64  `json:"recordsChecked"`
		Intact         bool   `json:"intact"`
		FirstBreakAt   int64  `json:"firstBreakAt"`
		Reason         string `json:"reason"`
		HeadHash       string `json:"headHash"`
	}
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return nil, err
	}
	checkedAt, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}

	return &VerificationResult{
		TenantID:       tenantID,
		RecordsChecked: fields.RecordsChecked,
		Intact:         fields.Intact,
		FirstBreakAt:   fields.FirstBreakAt,
		Reason:         fields.Reason,
		HeadHash:       fields.HeadHash,
		CheckedAt:      checkedAt,
	}, nil
}
