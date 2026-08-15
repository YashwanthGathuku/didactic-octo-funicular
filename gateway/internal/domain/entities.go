package domain

import "time"

// The remaining entities of the minimum domain model. Each carries TenantID,
// because a business record without a tenant cannot be safely queried.

// Tenant is the isolation boundary. Every other record in this package belongs
// to exactly one.
type Tenant struct {
	ID        TenantID
	Name      string
	CreatedAt time.Time
}

// Partner is a counterparty that sends or receives files.
type Partner struct {
	ID            int64
	TenantID      TenantID
	Name          string
	RoutingNumber string
	CreatedAt     time.Time
}

// FeedContract is the stable identity of an agreement with a partner. Its
// mutable terms live in versions, never here.
type FeedContract struct {
	ID        int64
	TenantID  TenantID
	PartnerID int64
	FeedID    string
	Direction string // INBOUND | OUTBOUND
	CreatedAt time.Time
}

// FeedContractVersion is an immutable set of terms effective over an interval.
//
// Editing a contract creates a new version. A historical occurrence resolves to
// the version that was active on its business date, so changing today's terms
// cannot retroactively alter whether last month's file was late.
type FeedContractVersion struct {
	ID              int64
	TenantID        TenantID
	ContractID      int64
	Version         int
	FilenamePattern string
	Format          string // NACHA for production; others are experimental
	ExpectedLocal   string // HH:MM:SS in the contract's own timezone
	Timezone        string // IANA name, e.g. America/New_York
	GraceMinutes    int
	CalendarID      string
	BalancedMode    string // BALANCED | UNBALANCED_AUTHORIZED
	EffectiveFrom   time.Time
	EffectiveTo     *time.Time // nil = open interval
	CreatedAt       time.Time
}

// ExpectationOccurrence is one concrete file expected on one business date.
//
// It is materialized ahead of time. That is the whole mechanism by which a
// missing file is detectable: a row exists and ages into OVERDUE even though no
// arrival event ever occurs.
type ExpectationOccurrence struct {
	ID                int64
	TenantID          TenantID
	ContractID        int64
	ContractVersionID int64
	BusinessDate      time.Time
	DueAt             time.Time
	GraceExpiresAt    time.Time
	State             ExpectationState
	MatchedArtifactID *int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Version           int
}

// TransitionTo moves an occurrence, refusing undefined edges.
func (e *ExpectationOccurrence) TransitionTo(next ExpectationState, at time.Time) error {
	if !e.TenantID.Valid() {
		return ErrNoTenant
	}
	if !CanTransitionExpectation(e.State, next) {
		return &TransitionError{Machine: "expectation", From: string(e.State), To: string(next)}
	}
	e.State = next
	e.UpdatedAt = at
	e.Version++
	return nil
}

// IngestionJob is a durable unit of work. Leases, heartbeats and backoff are
// Prompt 08; this is the record they operate on.
type IngestionJob struct {
	ID             int64
	TenantID       TenantID
	ArtifactID     int64
	IdempotencyKey string
	State          JobState
	AttemptCount   int
	MaxAttempts    int
	LeaseOwner     string
	LeaseExpiresAt *time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
}

// TransitionTo moves a job, refusing undefined edges and refusing to retry past
// the attempt budget.
func (j *IngestionJob) TransitionTo(next JobState, at time.Time) error {
	if !j.TenantID.Valid() {
		return ErrNoTenant
	}
	if !CanTransitionJob(j.State, next) {
		return &TransitionError{Machine: "job", From: string(j.State), To: string(next)}
	}
	if next == JobQueued && j.State == JobRetryable && j.AttemptCount >= j.MaxAttempts {
		return &TransitionError{
			Machine: "job", From: string(j.State), To: string(next),
			Reason: "retry budget exhausted; the job must move to DEAD",
		}
	}
	j.State = next
	j.UpdatedAt = at
	j.Version++
	return nil
}

// JobAttempt is one execution of a job. Attempts are append-only, so a retry
// never erases the record of the failure that caused it.
type JobAttempt struct {
	ID         int64
	TenantID   TenantID
	JobID      int64
	Number     int
	Outcome    JobState
	Error      string
	StartedAt  time.Time
	FinishedAt *time.Time
}

// NotificationIntent records that a notification should be delivered. It is
// written in the same transaction as the business change and dispatched
// separately, so a crash between the two cannot lose the intent.
type NotificationIntent struct {
	ID          int64
	TenantID    TenantID
	Kind        string
	SubjectType string
	SubjectID   int64
	Payload     string
	DeliveredAt *time.Time
	CreatedAt   time.Time
}

// AuditEvent is one entry in the append-only application hash chain.
//
// Named a hash chain, not a Merkle tree: it has no history tree, membership
// proof, consistency proof, or external anchor.
type AuditEvent struct {
	ID            int64
	TenantID      TenantID
	Sequence      int64
	ActorID       string
	Action        string
	ObjectType    string
	ObjectID      int64
	CorrelationID string
	PreviousHash  string
	PayloadHash   string
	Metadata      string
	CreatedAt     time.Time
}

// StatusChange is one row of append-only status history. Nothing updates these;
// a state machine writes one per transition.
type StatusChange struct {
	ID         int64
	TenantID   TenantID
	ObjectType string // artifact | expectation | job | decision
	ObjectID   int64
	FromState  string
	ToState    string
	ActorID    string
	Reason     string
	OccurredAt time.Time
}
