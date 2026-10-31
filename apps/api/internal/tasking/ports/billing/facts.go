package billingfacts

import "errors"

var (
	// ErrSettlementConflict and ErrIntegrity stay local: they describe
	// task-billing settlement-state facts with no canonical counterpart
	// in billing or credits.
	ErrSettlementConflict = errors.New("task billing settlement conflict")
	ErrIntegrity          = errors.New("task billing integrity error")
)

// SettlementState is the durable task-billing settlement vocabulary. The
// validation home is the deep settlement kernel
// (internal/tasking/settlement: IsInitialState/IsTerminalState and the
// Validate* errors); this type only names the words so handler and store
// code never re-spells the literals.
type SettlementState string

const (
	SettlementStateFree     SettlementState = "free"
	SettlementStateHeld     SettlementState = "held"
	SettlementStateCaptured SettlementState = "captured"
	SettlementStateReleased SettlementState = "released"
)
