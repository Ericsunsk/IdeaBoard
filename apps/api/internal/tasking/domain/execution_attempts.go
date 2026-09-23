package domain

import "time"

// ExecutionAttemptStart opens one gateway execution attempt. The claim and
// the attempt row commit atomically, so a fenced token always has its row:
// the attempt sequence is server-assigned, never caller-supplied. The
// execution token must be unique per attempt; finalization requires the same
// token, which is how stale workers lose the race.
type ExecutionAttemptStart struct {
	TaskID         string
	RiverJobID     int64
	ExecutionToken string
	FeatureSlug    string
	RequestedModel string
	Provider       string
	StartedAt      time.Time
}

// ExecutionAttemptFinish closes one running attempt. A zero-token match
// reports zero affected rows instead of an error: the caller already lost
// the race to another worker and must treat the task as converged.
type ExecutionAttemptFinish struct {
	ExecutionToken  string
	TaskID          string
	Status          string
	ResolvedModel   string
	GatewayStep     string
	ReceiptLogID    string
	ReceiptEventID  string
	CacheHit        bool
	FailureStage    string
	FailureCategory string
	ErrorMsg        string
	FinishedAt      time.Time
	CompletedAt     time.Time
}
