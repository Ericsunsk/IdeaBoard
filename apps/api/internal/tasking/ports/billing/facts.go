package billing

import "errors"

var (
	// ErrSettlementConflict and ErrIntegrity stay local: they describe
	// task-billing settlement-state facts with no canonical counterpart
	// in billing or credits.
	ErrSettlementConflict = errors.New("task billing settlement conflict")
	ErrIntegrity          = errors.New("task billing integrity error")
)

type SettlementState string

const (
	SettlementStateFree     SettlementState = "free"
	SettlementStateHeld     SettlementState = "held"
	SettlementStateCaptured SettlementState = "captured"
	SettlementStateReleased SettlementState = "released"
)
