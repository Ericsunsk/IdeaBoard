package billingrecords

import (
	"errors"
	"time"
)

const (
	DefaultPageLimit = 20
	MaxPageLimit     = 200
	MaxSafeInteger   = 9_007_199_254_740_991
)

var ErrUnsafeNumber = errors.New("billing record value exceeds JavaScript safe integer range")

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
