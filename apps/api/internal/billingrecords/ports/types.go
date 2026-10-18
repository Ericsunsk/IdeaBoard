package ports

import (
	"errors"
	"time"
)

const (
	DefaultPageLimit = 20
	MaxPageLimit     = 200
	// MaxSafeInteger is the canonical JavaScript safe-integer bound
	// (2^53-1) shared by billing-records and usage guards. usage keeps a
	// re-exported name for its existing callers; new code references
	// this constant directly.
	MaxSafeInteger = 9_007_199_254_740_991
)

var ErrUnsafeNumber = errors.New("billing record value exceeds JavaScript safe integer range")

// Service error sentinels. Application/domain errors originate here as
// typed sentinels; fmt.Errorf is only a %w wrapper, never an origin.
// The v1 billing-records edge (v1/billing.go
// renderV1BillingRecordsError) maps invalid input to 400 bad_request
// (ErrInvalidListInput, ErrInvalidPageLimit); every other List failure
// stays 503 service_unavailable.
var (
	ErrRepositoryNotConfigured = errors.New("billing records service: repository not configured")
	ErrInvalidListInput        = errors.New("billing records service: invalid list input")
	ErrInvalidPageLimit        = errors.New("billing records service: invalid page limit")
	ErrMissingSnapshotName     = errors.New("AI usage is missing its immutable Task Billing Snapshot name")
	ErrUndisplayableCorrection = errors.New("correction does not reference a displayable adjustment")
	ErrUnsupportedCommandType  = errors.New("billing records service: unsupported command type")
	ErrPoolNotConfigured       = errors.New("billing records repository: database pool not configured")
)

type Kind string

const (
	KindAIUsage          Kind = "ai_usage"
	KindUsage            Kind = "usage"
	KindReward           Kind = "reward"
	KindSystemGrant      Kind = "system_grant"
	KindSystemAdjustment Kind = "system_adjustment"
	KindTopUp            Kind = "top_up"
)

type Record struct {
	RecordID              string    `json:"record_id"`
	OccurredAt            time.Time `json:"occurred_at"`
	Kind                  Kind      `json:"kind"`
	Title                 string    `json:"title"`
	CreditDelta           int64     `json:"credit_delta"`
	AvailableCreditsAfter int64     `json:"available_credits_after"`
}

type ListInput struct {
	UserID         int64
	BeforeSequence int64
	Limit          int
}

type Page struct {
	Records            []Record `json:"records"`
	NextBeforeSequence int64    `json:"next_before_sequence,omitempty"`
}
