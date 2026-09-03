package billing

import (
	"errors"
)

var (
	ErrSettlementConflict  = errors.New("task billing settlement conflict")
	ErrIntegrity           = errors.New("task billing integrity error")
	ErrIdempotencyConflict = errors.New("task billing idempotency conflict")
)

type SettlementState string

const (
	SettlementStateFree     SettlementState = "free"
	SettlementStateHeld     SettlementState = "held"
	SettlementStateCaptured SettlementState = "captured"
	SettlementStateReleased SettlementState = "released"
)
