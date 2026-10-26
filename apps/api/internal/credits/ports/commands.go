package ports

import (
	"context"
	"time"

	"ideaboard/api/internal/credits/domain"
)

type CommandResult struct {
	TransactionID string               `json:"transaction_id"`
	CommandType   domain.CommandType   `json:"command_type"`
	Amount        int64                `json:"amount_credits"`
	Wallet        domain.WalletBalance `json:"wallet"`
	Replayed      bool                 `json:"replayed"`
	CreatedAt     time.Time            `json:"created_at"`
}

type GrantCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

// PurchaseCommand credits a paid external order. Only the Payments service
// may issue it, and callers should compose it into the Payment Order transaction.
type PurchaseCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

type PurchaseRefundHoldCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

type PurchaseRefundCommand struct {
	BillingAccountID  int64
	HoldTransactionID string
	Metadata          domain.CommandMetadata
}

// PaymentDisputeDebitCommand is a provider-driven, bounded recovery debit.
// The database routine caps the actual posting at the current available
// balance and returns the unrecovered remainder without creating a negative
// projection.
type PaymentDisputeDebitCommand struct {
	BillingAccountID int64
	RequestedAmount  domain.CreditAmount
	Metadata         domain.CommandMetadata
}

type PaymentDisputeDebitResult struct {
	CommandResult
	RequestedAmount   int64 `json:"requested_amount_credits"`
	UnrecoveredAmount int64 `json:"unrecovered_credits"`
}

type PaymentDisputeRestoreCommand struct {
	BillingAccountID      int64
	OriginalTransactionID string
	Amount                domain.CreditAmount
	Metadata              domain.CommandMetadata
}

// DeductionCommand records a discretionary administrative debit. It can only
// consume available credits and always creates a new immutable transaction.
type DeductionCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

// CorrectionCommand partially or fully reverses one prior administrative
// Grant, Deduction, or Refund. Capture corrections remain linked Refunds.
type CorrectionCommand struct {
	BillingAccountID      int64
	OriginalTransactionID string
	Amount                domain.CreditAmount
	Metadata              domain.CommandMetadata
}

type HoldCommand struct {
	BillingAccountID int64
	Amount           domain.CreditAmount
	Metadata         domain.CommandMetadata
}

type CaptureCommand struct {
	BillingAccountID  int64
	HoldTransactionID string
	Amount            domain.CreditAmount
	Metadata          domain.CommandMetadata
}

type ReleaseCommand struct {
	BillingAccountID  int64
	HoldTransactionID string
	Amount            domain.CreditAmount
	Metadata          domain.CommandMetadata
}

type RefundCommand struct {
	BillingAccountID    int64
	ChargeTransactionID string
	Amount              domain.CreditAmount
	Metadata            domain.CommandMetadata
}

// Narrow settlement-role faces. Under the ADR-0001 centralized Command
// decision, caller attribution is a compile-time property: each owning
// context depends only on its role face, so calling the wrong Command is a
// build error instead of a review finding.
//
//   - AdministrativeCommands: Control Plane fund repair (Grant, Deduct,
//     Correct, Refund).
//   - SettlementCommands: workspace-scoped spending (Tasking, Vision
//     Search) Hold/Capture/Release convergence.
//   - PaymentCommands: Payments-owned external-money convergence
//     (Purchase, PurchaseRefundHold, PurchaseRefund, plus the generic
//     Release used by the refund-failed path, and the dispute pair).
//   - Granter: Rewards Grant on the caller's initial personal account.
//
// The DEFINER function seam and privilege policy behind these faces are
// unchanged.
type Granter interface {
	Grant(context.Context, GrantCommand) (CommandResult, error)
}

type Deductor interface {
	Deduct(context.Context, DeductionCommand) (CommandResult, error)
}

type Corrector interface {
	Correct(context.Context, CorrectionCommand) (CommandResult, error)
}

type Refunder interface {
	Refund(context.Context, RefundCommand) (CommandResult, error)
}

type Holder interface {
	Hold(context.Context, HoldCommand) (CommandResult, error)
}

type Capturer interface {
	Capture(context.Context, CaptureCommand) (CommandResult, error)
}

type Releaser interface {
	Release(context.Context, ReleaseCommand) (CommandResult, error)
}

type Purchaser interface {
	Purchase(context.Context, PurchaseCommand) (CommandResult, error)
}

type PurchaseRefundHolder interface {
	PurchaseRefundHold(context.Context, PurchaseRefundHoldCommand) (CommandResult, error)
}

type PurchaseRefunder interface {
	PurchaseRefund(context.Context, PurchaseRefundCommand) (CommandResult, error)
}

type DisputeDebiter interface {
	PaymentDisputeDebit(context.Context, PaymentDisputeDebitCommand) (PaymentDisputeDebitResult, error)
}

type DisputeRestorer interface {
	PaymentDisputeRestore(context.Context, PaymentDisputeRestoreCommand) (CommandResult, error)
}

// AdministrativeCommands is the Control Plane fund-repair face.
type AdministrativeCommands interface {
	Granter
	Deductor
	Corrector
	Refunder
}

// SettlementCommands is the workspace-scoped spending face (Tasking,
// Vision Search, and the generic Release leg of terminal convergence).
type SettlementCommands interface {
	Holder
	Capturer
	Releaser
}

// PurchaseCommands is the Payments purchase/refund face. Refund failure
// releases the pending hold through the generic Release, so the face
// includes Releaser alongside the Payments-typed settlement pair.
type PurchaseCommands interface {
	Purchaser
	PurchaseRefundHolder
	PurchaseRefunder
	Releaser
}

// DisputeCommands is the Payments dispute face.
type DisputeCommands interface {
	DisputeDebiter
	DisputeRestorer
}

// PaymentCommands is the full Payments-owned external-money face.
type PaymentCommands interface {
	PurchaseCommands
	DisputeCommands
}

// Commands is the union face for generic carriers (managed-transaction
// runner, multi-role tests). Production callers depend on the role face
// above, never on this union.
type Commands interface {
	AdministrativeCommands
	SettlementCommands
	PaymentCommands
}
