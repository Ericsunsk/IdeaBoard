package reconciliation

import (
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"time"
)

// Claim fences a short-lived worker lease. External provider calls happen
// after this claim transaction commits.
type Claim struct {
	Order      paymentsdomain.PaymentOrder
	Token      string
	Generation int64
}

type Outcome struct {
	CheckedAt     time.Time
	NextAttemptAt time.Time
	ErrorCode     string
	ErrorSummary  string
	Anomalous     bool
	ClearAnomaly  bool
}

type Anomaly struct {
	PaymentOrderID    string
	PublicOrderNumber string
	UserID            int64
	Status            paymentsdomain.PaymentOrderStatus
	LastErrorCode     string
	LastErrorSummary  string
	LastCheckedAt     *time.Time
	AnomalousAt       time.Time
	CreatedAt         time.Time
}

// merged from internal/payments/application/reconciliation/policy.go (structural consolidation; symbols unchanged).
const (
	Interval      = 5 * time.Minute
	OperableDelay = time.Hour
	AnomalyAge    = 24 * time.Hour
	Lease         = 5 * time.Minute
)
